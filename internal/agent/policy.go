package agent

import (
	"context"
	"crypto/tls"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

var ErrBlocked = errors.New("destination blocked by Agent policy")

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type Policy struct {
	servicesByID      map[string]compiledService
	servicesByVirtual map[string]compiledService
	resolver          Resolver
	localAddresses    func() (map[netip.Addr]struct{}, error)
	temporary         *compiledTemporaryPolicy
}

type compiledTemporaryPolicy struct {
	prefixes []netip.Prefix
	ports    map[uint16]struct{}
	caFiles  []string
}

type TemporaryTarget struct {
	ServiceID string
	Kind      string
	Host      string
	Port      uint16
}

type compiledTemporaryTarget struct {
	config    TemporaryTarget
	addresses []netip.Addr
	virtual   string
	tls       *tls.Config
}

type compiledService struct {
	config   ServiceConfig
	prefixes []netip.Prefix
	virtual  string
	tls      *tls.Config
}

type ResolvedTarget struct {
	Service   ServiceConfig
	Addresses []netip.Addr
}

func CompilePolicy(config Config) (*Policy, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	policy := &Policy{
		servicesByID:      make(map[string]compiledService, len(config.Services)),
		servicesByVirtual: make(map[string]compiledService, len(config.Services)),
		resolver:          net.DefaultResolver,
		localAddresses:    interfaceAddresses,
	}
	for _, service := range config.Services {
		compiled := compiledService{config: service, virtual: virtualHostname(service.ID)}
		if service.TLS != nil {
			tlsConfig, err := BuildTLSConfig(*service.TLS)
			if err != nil {
				return nil, fmt.Errorf("compile service %q TLS policy: %w", service.PolicyRef, err)
			}
			compiled.tls = tlsConfig
		}
		for _, rawPrefix := range service.AllowedCIDRs {
			prefix, _ := netip.ParsePrefix(rawPrefix)
			compiled.prefixes = append(compiled.prefixes, prefix.Masked())
		}
		policy.servicesByID[service.ID] = compiled
		policy.servicesByVirtual[compiled.virtual] = compiled
	}
	if config.Temporary != nil {
		compiled := &compiledTemporaryPolicy{
			ports:   make(map[uint16]struct{}, len(config.Temporary.AllowedPorts)),
			caFiles: append([]string(nil), config.Temporary.CAFiles...),
		}
		for _, rawPrefix := range config.Temporary.AllowedCIDRs {
			prefix, _ := netip.ParsePrefix(rawPrefix)
			compiled.prefixes = append(compiled.prefixes, prefix.Masked())
		}
		for _, port := range config.Temporary.AllowedPorts {
			compiled.ports[port] = struct{}{}
		}
		policy.temporary = compiled
	}
	return policy, nil
}

func (policy *Policy) AcceptsTemporary(serviceID string) bool {
	return policy.temporary != nil && validOpaqueID(serviceID)
}

func (policy *Policy) PrepareTemporary(ctx context.Context, target TemporaryTarget) (*compiledTemporaryTarget, error) {
	if policy.temporary == nil || !validOpaqueID(target.ServiceID) || target.Kind != "http" && target.Kind != "https" || !validHostname(target.Host) {
		return nil, ErrBlocked
	}
	if _, ok := policy.temporary.ports[target.Port]; !ok {
		return nil, ErrBlocked
	}
	addresses, err := policy.resolveHost(ctx, target.Host, policy.temporary.prefixes)
	if err != nil {
		return nil, err
	}
	local, err := policy.localAddresses()
	if err != nil {
		return nil, fmt.Errorf("enumerate Agent addresses: %w", err)
	}
	for _, address := range addresses {
		if address.IsLoopback() {
			return nil, fmt.Errorf("%w: temporary targets cannot address Agent loopback", ErrBlocked)
		}
		if _, exists := local[address]; exists {
			return nil, fmt.Errorf("%w: temporary targets cannot address the Agent itself", ErrBlocked)
		}
	}
	compiled := &compiledTemporaryTarget{
		config: target, addresses: addresses, virtual: virtualHostname(target.ServiceID),
	}
	if target.Kind == "https" {
		compiled.tls, err = BuildTLSConfig(TLSConfig{ServerName: target.Host, CAFiles: policy.temporary.caFiles})
		if err != nil {
			return nil, err
		}
	}
	return compiled, nil
}

func (policy *Policy) DialTemporary(ctx context.Context, target *compiledTemporaryTarget, hostname string, port uint16) (net.Conn, error) {
	if target == nil || !strings.EqualFold(hostname, target.virtual) || port != temporaryVirtualPort(target.config.Kind) {
		return nil, ErrBlocked
	}
	dialer := net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(target.addresses[0].String(), fmt.Sprint(target.config.Port)))
	if err != nil || target.tls == nil {
		return connection, err
	}
	secure := tls.Client(connection, target.tls.Clone())
	if err := secure.HandshakeContext(ctx); err != nil {
		connection.Close()
		return nil, err
	}
	return secure, nil
}

func (policy *Policy) ResolveTemporaryName(ctx context.Context, name string) ([]netip.Addr, error) {
	if policy.temporary == nil || !validHostname(name) {
		return nil, ErrBlocked
	}
	return policy.resolveHost(ctx, name, policy.temporary.prefixes)
}

func (policy *Policy) VirtualAuthority(serviceID string) (string, uint16, error) {
	service, ok := policy.servicesByID[serviceID]
	if !ok {
		return "", 0, ErrBlocked
	}
	return service.virtual, virtualPort(service.config), nil
}

func (policy *Policy) Service(serviceID string) (ServiceConfig, bool) {
	service, ok := policy.servicesByID[serviceID]
	return service.config, ok
}

func (policy *Policy) ResolveTarget(ctx context.Context, serviceID string) (ResolvedTarget, error) {
	service, ok := policy.servicesByID[serviceID]
	if !ok {
		return ResolvedTarget{}, ErrBlocked
	}
	addresses, err := policy.resolve(ctx, service)
	if err != nil {
		return ResolvedTarget{}, err
	}
	return ResolvedTarget{Service: service.config, Addresses: addresses}, nil
}

func (policy *Policy) ResolveName(ctx context.Context, serviceID, name string) ([]netip.Addr, error) {
	service, ok := policy.servicesByID[serviceID]
	if !ok || !validHostname(name) {
		return nil, ErrBlocked
	}
	return policy.resolveHost(ctx, name, service.prefixes)
}

func (policy *Policy) DialVirtual(ctx context.Context, hostname string, port uint16) (net.Conn, error) {
	service, ok := policy.servicesByVirtual[strings.ToLower(hostname)]
	if !ok || port != virtualPort(service.config) {
		return nil, fmt.Errorf("%w: unknown virtual service", ErrBlocked)
	}
	addresses, err := policy.resolve(ctx, service)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(addresses[0].String(), fmt.Sprint(service.config.TargetPort)))
	if err != nil || service.tls == nil {
		return connection, err
	}
	secure := tls.Client(connection, service.tls.Clone())
	if err := secure.HandshakeContext(ctx); err != nil {
		connection.Close()
		return nil, err
	}
	return secure, nil
}

func (policy *Policy) resolve(ctx context.Context, service compiledService) ([]netip.Addr, error) {
	return policy.resolveHost(ctx, service.config.TargetHost, service.prefixes)
}

func (policy *Policy) resolveHost(ctx context.Context, host string, prefixes []netip.Prefix) ([]netip.Addr, error) {
	var addresses []netip.Addr
	if address, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{address.Unmap()}
	} else {
		resolved, err := policy.resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve configured target: %w", err)
		}
		for _, address := range resolved {
			addresses = append(addresses, address.Unmap())
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("configured target resolved to no addresses")
	}
	for _, address := range addresses {
		allowed := false
		for _, prefix := range prefixes {
			if prefix.Contains(address) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("%w: configured target resolved outside its allowlist", ErrBlocked)
		}
	}
	return addresses, nil
}

func virtualHostname(serviceID string) string {
	raw, _ := base64.RawURLEncoding.DecodeString(serviceID)
	label := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))
	return "s-" + label + ".webvpn.invalid"
}

func virtualPort(service ServiceConfig) uint16 {
	if service.Kind == "https" || service.Kind == "guacamole" && service.TLS != nil {
		return 443
	}
	return 80
}

func temporaryVirtualPort(kind string) uint16 {
	if kind == "https" {
		return 443
	}
	return 80
}

func interfaceAddresses() (map[netip.Addr]struct{}, error) {
	values, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	result := make(map[netip.Addr]struct{}, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value.String())
		if err == nil {
			result[prefix.Addr().Unmap()] = struct{}{}
		}
	}
	return result, nil
}
