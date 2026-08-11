package agent

import "testing"

func TestAgentConfigRejectsTURNAndPublicTargets(t *testing.T) {
	config := validConfig()
	config.STUNURLs = []string{"turn:relay.example:3478"}
	if err := config.Validate(); err == nil {
		t.Fatal("TURN configuration was accepted")
	}
	config = validConfig()
	config.Services[0].AllowedCIDRs = []string{"0.0.0.0/0"}
	if err := config.Validate(); err == nil {
		t.Fatal("public target allowlist was accepted")
	}
}

func TestPrivateCAAndPinStayInAgentPolicy(t *testing.T) {
	config := validConfig()
	config.Services[0].TLS = &TLSConfig{
		ServerName: "openwrt.lan",
		CAFiles:    []string{"/etc/webvpn-agent/ca/openwrt.pem"},
		SPKIPins:   []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="},
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	config.Services[0].TLS.CAFiles[0] = "relative-ca.pem"
	if err := config.Validate(); err == nil {
		t.Fatal("relative private CA path was accepted")
	}
}

func TestServiceIdentifiersMustBeUnique(t *testing.T) {
	config := validConfig()
	config.Services = append(config.Services, config.Services[0])
	if err := config.Validate(); err == nil {
		t.Fatal("duplicate service policy was accepted")
	}
}

func TestTemporaryPolicyRequiresExplicitPrivatePrefixesAndPorts(t *testing.T) {
	config := validConfig()
	config.Temporary = &TemporaryPolicyConfig{
		AllowedCIDRs: []string{"192.168.1.0/24", "192.168.3.0/24"},
		AllowedPorts: []uint16{80, 443},
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	config.Temporary.AllowedCIDRs = []string{"0.0.0.0/0"}
	if err := config.Validate(); err == nil {
		t.Fatal("public temporary target prefix was accepted")
	}
	config.Temporary.AllowedCIDRs = []string{"192.168.1.0/24"}
	config.Temporary.AllowedPorts = []uint16{443, 443}
	if err := config.Validate(); err == nil {
		t.Fatal("duplicate temporary target port was accepted")
	}
}

func validConfig() Config {
	return Config{
		ControllerURL:  "https://vpn.example.com",
		AgentID:        "ERITFBUWFxgZGhscHR4fIA",
		PrivateKeyFile: "/etc/webvpn-agent/agent.key",
		STUNURLs:       []string{"stun:stun.cloudflare.com:3478"},
		Services: []ServiceConfig{{
			ID: "AQIDBAUGBwgJCgsMDQ4PEA", PolicyRef: "openwrt-luci", Kind: "https",
			TargetHost: "192.168.1.1", TargetPort: 443, AllowedCIDRs: []string{"192.168.1.1/32"},
			TLS: &TLSConfig{ServerName: "openwrt.lan"},
		}},
	}
}
