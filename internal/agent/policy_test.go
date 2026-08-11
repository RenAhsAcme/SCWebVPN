package agent

import (
	"bufio"
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type fakeResolver map[string][]netip.Addr

func (resolver fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	return resolver[host], nil
}

func TestVirtualAuthorityDoesNotExposeTarget(t *testing.T) {
	policy, err := CompilePolicy(validConfig())
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := policy.VirtualAuthority("AQIDBAUGBwgJCgsMDQ4PEA")
	if err != nil {
		t.Fatal(err)
	}
	if host == "192.168.1.1" || port != 443 {
		t.Fatalf("virtual authority leaked or changed target semantics: %s:%d", host, port)
	}
	if _, _, err := policy.VirtualAuthority("ERITFBUWFxgZGhscHR4fIA"); !errors.Is(err, ErrBlocked) {
		t.Fatal("unknown service was not blocked")
	}
}

func TestHTTPSServiceTerminatesAndValidatesTLSOnlyAtAgent(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	server.StartTLS()
	defer server.Close()
	certificate := server.Certificate()
	caPath := filepath.Join(t.TempDir(), "private-ca.pem")
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(caPath, certificatePEM, 0o600); err != nil {
		t.Fatal(err)
	}
	port, err := strconv.ParseUint(strings.TrimPrefix(server.URL, "https://127.0.0.1:"), 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	config := validConfig()
	config.Services[0].TargetHost = "127.0.0.1"
	config.Services[0].TargetPort = uint16(port)
	config.Services[0].AllowedCIDRs = []string{"127.0.0.0/8"}
	config.Services[0].TLS = &TLSConfig{ServerName: "127.0.0.1", CAFiles: []string{caPath}}
	policy, err := CompilePolicy(config)
	if err != nil {
		t.Fatal(err)
	}
	host, portNumber, err := policy.VirtualAuthority(config.Services[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := policy.DialVirtual(context.Background(), host, portNumber)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := fmt.Fprintf(connection, "GET / HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected HTTPS response: %s", response.Status)
	}
}

func TestEveryDNSResultMustRemainInsideAllowlist(t *testing.T) {
	config := validConfig()
	config.Services[0].TargetHost = "openwrt.lan"
	policy, err := CompilePolicy(config)
	if err != nil {
		t.Fatal(err)
	}
	policy.resolver = fakeResolver{"openwrt.lan": {
		netip.MustParseAddr("192.168.1.1"),
		netip.MustParseAddr("203.0.113.10"),
	}}
	if _, err := policy.ResolveTarget(t.Context(), config.Services[0].ID); !errors.Is(err, ErrBlocked) {
		t.Fatalf("mixed public DNS result was not blocked: %v", err)
	}
	policy.resolver = fakeResolver{"openwrt.lan": {netip.MustParseAddr("192.168.1.1")}}
	resolved, err := policy.ResolveTarget(t.Context(), config.Services[0].ID)
	if err != nil || len(resolved.Addresses) != 1 {
		t.Fatalf("allowed target did not resolve: %#v, %v", resolved, err)
	}
}

func TestDiagnosticDNSUsesTheSameServiceAllowlist(t *testing.T) {
	config := validConfig()
	policy, err := CompilePolicy(config)
	if err != nil {
		t.Fatal(err)
	}
	policy.resolver = fakeResolver{
		"safe.lan":    {netip.MustParseAddr("192.168.1.1")},
		"rebound.lan": {netip.MustParseAddr("192.168.1.1"), netip.MustParseAddr("203.0.113.10")},
	}
	if _, err := policy.ResolveName(context.Background(), config.Services[0].ID, "safe.lan"); err != nil {
		t.Fatal(err)
	}
	if _, err := policy.ResolveName(context.Background(), config.Services[0].ID, "rebound.lan"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("diagnostic DNS rebinding escaped the service allowlist: %v", err)
	}
}

func TestTemporaryTargetIsPinnedWithoutExposingItsAddress(t *testing.T) {
	config := validConfig()
	config.Temporary = &TemporaryPolicyConfig{
		AllowedCIDRs: []string{"192.168.1.0/24"},
		AllowedPorts: []uint16{80, 443},
	}
	policy, err := CompilePolicy(config)
	if err != nil {
		t.Fatal(err)
	}
	policy.localAddresses = func() (map[netip.Addr]struct{}, error) {
		return map[netip.Addr]struct{}{netip.MustParseAddr("192.168.1.1"): {}}, nil
	}
	target, err := policy.PrepareTemporary(t.Context(), TemporaryTarget{
		ServiceID: "AQIDBAUGBwgJCgsMDQ4PEA", Kind: "http", Host: "192.168.1.20", Port: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(target.addresses) != 1 || target.addresses[0].String() != "192.168.1.20" || strings.Contains(target.virtual, "192.168.1.20") {
		t.Fatalf("temporary target was not pinned behind an opaque virtual authority: %#v", target)
	}
}

func TestTemporaryTargetRejectsAgentAddressDNSRebindingAndUnlistedPort(t *testing.T) {
	config := validConfig()
	config.Temporary = &TemporaryPolicyConfig{
		AllowedCIDRs: []string{"192.168.1.0/24"},
		AllowedPorts: []uint16{443},
	}
	policy, err := CompilePolicy(config)
	if err != nil {
		t.Fatal(err)
	}
	localAddress := netip.MustParseAddr("192.168.1.1")
	policy.localAddresses = func() (map[netip.Addr]struct{}, error) {
		return map[netip.Addr]struct{}{localAddress: {}}, nil
	}
	policy.resolver = fakeResolver{
		"agent.lan":   {localAddress},
		"rebound.lan": {netip.MustParseAddr("192.168.1.20"), netip.MustParseAddr("203.0.113.10")},
	}

	tests := []TemporaryTarget{
		{ServiceID: "AQIDBAUGBwgJCgsMDQ4PEA", Kind: "https", Host: "agent.lan", Port: 443},
		{ServiceID: "AQIDBAUGBwgJCgsMDQ4PEA", Kind: "https", Host: "rebound.lan", Port: 443},
		{ServiceID: "AQIDBAUGBwgJCgsMDQ4PEA", Kind: "http", Host: "192.168.1.20", Port: 80},
	}
	for _, target := range tests {
		if _, err := policy.PrepareTemporary(t.Context(), target); !errors.Is(err, ErrBlocked) {
			t.Fatalf("temporary target escaped policy: %#v, %v", target, err)
		}
	}
}
