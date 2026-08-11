package agent

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/RenAhsAcme/SCWebVPN/internal/config"
)

type Config struct {
	ControllerURL  string                 `json:"controller_url"`
	AgentID        string                 `json:"agent_id"`
	PrivateKeyFile string                 `json:"private_key_file"`
	STUNURLs       []string               `json:"stun_urls"`
	Services       []ServiceConfig        `json:"services"`
	Temporary      *TemporaryPolicyConfig `json:"temporary,omitempty"`
}

type TemporaryPolicyConfig struct {
	AllowedCIDRs []string `json:"allowed_cidrs"`
	AllowedPorts []uint16 `json:"allowed_ports"`
	CAFiles      []string `json:"ca_files,omitempty"`
}

type ServiceConfig struct {
	ID           string     `json:"id"`
	PolicyRef    string     `json:"policy_ref"`
	Kind         string     `json:"kind"`
	TargetHost   string     `json:"target_host"`
	TargetPort   uint16     `json:"target_port"`
	AllowedCIDRs []string   `json:"allowed_cidrs"`
	TLS          *TLSConfig `json:"tls,omitempty"`
}

type TLSConfig struct {
	ServerName string   `json:"server_name"`
	CAFiles    []string `json:"ca_files,omitempty"`
	SPKIPins   []string `json:"spki_sha256,omitempty"`
}

var permittedNetworks = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	var value Config
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return Config{}, fmt.Errorf("decode Agent config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("Agent config must contain one valid JSON value")
	}
	if err := value.Validate(); err != nil {
		return Config{}, err
	}
	return value, nil
}

func (value Config) Validate() error {
	controller, err := url.Parse(value.ControllerURL)
	if err != nil || controller.Scheme != "https" || controller.Host == "" || controller.User != nil || controller.RawQuery != "" || controller.Fragment != "" || controller.Path != "" && controller.Path != "/" {
		return errors.New("controller_url must be an HTTPS origin")
	}
	if !validOpaqueID(value.AgentID) {
		return errors.New("agent_id must be a 128-bit opaque value")
	}
	if !absoluteTargetPath(value.PrivateKeyFile) {
		return errors.New("private_key_file must be an absolute path")
	}
	if err := config.ValidateSTUNURLs(value.STUNURLs); err != nil {
		return err
	}
	if len(value.Services) == 0 {
		return errors.New("at least one Agent service is required")
	}
	ids, policyRefs := make(map[string]struct{}), make(map[string]struct{})
	for _, service := range value.Services {
		if err := service.Validate(); err != nil {
			return fmt.Errorf("service %q: %w", service.PolicyRef, err)
		}
		if _, exists := ids[service.ID]; exists {
			return fmt.Errorf("duplicate service ID %q", service.ID)
		}
		if _, exists := policyRefs[service.PolicyRef]; exists {
			return fmt.Errorf("duplicate policy reference %q", service.PolicyRef)
		}
		ids[service.ID], policyRefs[service.PolicyRef] = struct{}{}, struct{}{}
	}
	if value.Temporary != nil {
		if err := value.Temporary.Validate(); err != nil {
			return fmt.Errorf("temporary policy: %w", err)
		}
	}
	return nil
}

func (value TemporaryPolicyConfig) Validate() error {
	if len(value.AllowedCIDRs) == 0 || len(value.AllowedPorts) == 0 || len(value.AllowedPorts) > 32 {
		return errors.New("allowed_cidrs and 1 to 32 allowed_ports are required")
	}
	for _, rawPrefix := range value.AllowedCIDRs {
		prefix, err := netip.ParsePrefix(rawPrefix)
		if err != nil || prefix.Addr().Is4In6() || !permittedPrefix(prefix.Masked()) {
			return fmt.Errorf("CIDR %q is not an explicit private, loopback, ULA, or CGNAT prefix", rawPrefix)
		}
	}
	seen := make(map[uint16]struct{}, len(value.AllowedPorts))
	for _, port := range value.AllowedPorts {
		if port == 0 {
			return errors.New("temporary port 0 is invalid")
		}
		if _, exists := seen[port]; exists {
			return fmt.Errorf("duplicate temporary port %d", port)
		}
		seen[port] = struct{}{}
	}
	for _, path := range value.CAFiles {
		if !absoluteTargetPath(path) {
			return errors.New("temporary private CA files must use absolute OpenWrt paths")
		}
	}
	return nil
}

func (service ServiceConfig) Validate() error {
	if !validOpaqueID(service.ID) {
		return errors.New("id must be a 128-bit opaque value")
	}
	if service.PolicyRef == "" || service.PolicyRef != strings.TrimSpace(service.PolicyRef) || len(service.PolicyRef) > 120 || strings.ContainsAny(service.PolicyRef, "\r\n\x00") {
		return errors.New("policy_ref must contain 1 to 120 safe bytes")
	}
	if service.Kind != "http" && service.Kind != "https" && service.Kind != "guacamole" {
		return errors.New("kind must be http, https, or guacamole")
	}
	if !validHostname(service.TargetHost) || service.TargetPort == 0 {
		return errors.New("target_host and target_port are invalid")
	}
	if len(service.AllowedCIDRs) == 0 {
		return errors.New("allowed_cidrs must not be empty")
	}
	for _, rawPrefix := range service.AllowedCIDRs {
		prefix, err := netip.ParsePrefix(rawPrefix)
		if err != nil || prefix.Addr().Is4In6() || !permittedPrefix(prefix.Masked()) {
			return fmt.Errorf("CIDR %q is not an explicit private, loopback, ULA, or CGNAT prefix", rawPrefix)
		}
	}
	if service.Kind == "https" && service.TLS == nil {
		return errors.New("https service requires TLS policy")
	}
	if service.Kind == "http" && service.TLS != nil {
		return errors.New("http service must not contain TLS policy")
	}
	if service.TLS != nil {
		if err := service.TLS.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (value TLSConfig) Validate() error {
	if !validHostname(value.ServerName) {
		return errors.New("TLS server_name is invalid")
	}
	for _, path := range value.CAFiles {
		if !absoluteTargetPath(path) {
			return errors.New("private CA files must use absolute OpenWrt paths")
		}
	}
	for _, encoded := range value.SPKIPins {
		if _, err := decodeDigest(encoded); err != nil {
			return fmt.Errorf("invalid SPKI SHA-256 pin: %w", err)
		}
	}
	return nil
}

func decodeDigest(value string) ([32]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil || len(decoded) != 32 {
		return [32]byte{}, errors.New("pin must be a base64-encoded 32-byte digest")
	}
	var digest [32]byte
	copy(digest[:], decoded)
	return digest, nil
}

func validOpaqueID(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 16
}

func validHostname(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.HasSuffix(value, ".") || strings.ContainsAny(value, "\x00/@[]") {
		return false
	}
	if address := net.ParseIP(value); address != nil {
		return !address.IsUnspecified() && !address.IsMulticast()
	}
	if ambiguousIPAddress(value) {
		return false
	}
	if len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func ambiguousIPAddress(value string) bool {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "0x") {
		return true
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return true
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func absoluteTargetPath(value string) bool {
	return strings.HasPrefix(value, "/") || filepath.IsAbs(value)
}

func permittedPrefix(prefix netip.Prefix) bool {
	for _, permitted := range permittedNetworks {
		if permitted.Bits() <= prefix.Bits() && permitted.Contains(prefix.Addr()) {
			return true
		}
	}
	return false
}
