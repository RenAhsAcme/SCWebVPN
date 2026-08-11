package agent

import (
	"encoding/base64"
	"testing"

	"github.com/RenAhsAcme/SCWebVPN/internal/session"
)

func TestCapabilityAuthentication(t *testing.T) {
	token, digest, err := session.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if !verifyCapability([]byte(authPrefix+token), digest) {
		t.Fatal("valid browser capability was rejected")
	}
	other, _, err := session.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if verifyCapability([]byte(authPrefix+other), digest) || verifyCapability([]byte(token), digest) {
		t.Fatal("invalid browser capability was accepted")
	}
}

func TestCapabilityDigestShape(t *testing.T) {
	value := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if _, err := decodeCapabilityDigest(value); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCapabilityDigest("short"); err == nil {
		t.Fatal("short capability digest was accepted")
	}
}
