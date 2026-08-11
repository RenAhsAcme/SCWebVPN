package config

import "testing"

func TestControllerRequiresLoopback(t *testing.T) {
	cfg := validController()
	cfg.Listen = "0.0.0.0:8789"
	if err := cfg.Validate(); err == nil {
		t.Fatal("public controller listen address was accepted")
	}
}

func TestControllerAcceptsIPv4AndIPv6Loopback(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:8789", "[::1]:8789"} {
		cfg := validController()
		cfg.Listen = listen
		if err := cfg.Validate(); err != nil {
			t.Fatalf("%s: %v", listen, err)
		}
	}
}

func TestSTUNOnly(t *testing.T) {
	for _, value := range []string{
		"turn:relay.example:3478", "turns:relay.example:5349", "stun:user@example.net", " stun:example.net",
		"stun:turn:relay.example", "stun:example.net/path", "stun:example.net?transport=udp",
		"stun:127.0.0.1:3478", "stun:192.168.1.1:3478", "stun:example.net:0", "stun:example.net:65536",
		"stun:localhost:3478", "stun:stun.local:3478", "stun:router.lan:3478",
	} {
		if err := ValidateSTUNURLs([]string{value}); err == nil {
			t.Fatalf("unsafe ICE URL %q was accepted", value)
		}
	}
	if err := ValidateSTUNURLs([]string{"stun:stun.example.net:3478"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSTUNURLs([]string{"stun:[2001:4860:4860::8888]:3478"}); err != nil {
		t.Fatal(err)
	}
}

func TestControllerRequiresAbsoluteDatabasePath(t *testing.T) {
	cfg := validController()
	cfg.DatabasePath = "controller.sqlite3"
	if err := cfg.Validate(); err == nil {
		t.Fatal("relative database path was accepted")
	}
}

func validController() Controller {
	return Controller{
		Listen:                 "127.0.0.1:8789",
		DatabasePath:           "/var/lib/scwebvpn/controller.sqlite3",
		InternalAuthSecretFile: "/etc/scwebvpn/internal-auth",
		PublicBaseURL:          "https://vpn.example.com",
		STUNURLs:               []string{"stun:stun.example.net:3478"},
	}
}
