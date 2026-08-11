package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"
)

func TestSignedRequestBindsMethodPathAndBody(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	signed := SignedRequest{
		AgentID:  opaqueID(16),
		Nonce:    opaqueID(32),
		IssuedAt: now,
	}
	signed.Signature = ed25519.Sign(privateKey, CanonicalMessage(signed.AgentID, signed.Nonce, now, "POST", "/api/v1/agent/poll", []byte("{}")))
	if err := Verify(publicKey, signed, "POST", "/api/v1/agent/poll", []byte("{}"), now); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []struct {
		method string
		path   string
		body   string
	}{
		{"PUT", "/api/v1/agent/poll", "{}"},
		{"POST", "/api/v1/agent/other", "{}"},
		{"POST", "/api/v1/agent/poll", `{"changed":true}`},
	} {
		if err := Verify(publicKey, signed, changed.method, changed.path, []byte(changed.body), now); err == nil {
			t.Fatal("modified signed request was accepted")
		}
	}
}

func TestSignedRequestRejectsClockSkew(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	signed := SignedRequest{AgentID: opaqueID(16), Nonce: opaqueID(32), IssuedAt: issuedAt}
	signed.Signature = ed25519.Sign(privateKey, CanonicalMessage(signed.AgentID, signed.Nonce, issuedAt, "GET", "/", nil))
	if err := Verify(publicKey, signed, "GET", "/", nil, issuedAt.Add(MaxClockSkew+time.Second)); err == nil {
		t.Fatal("stale signed request was accepted")
	}
}

func opaqueID(size int) string {
	value := make([]byte, size)
	for index := range value {
		value[index] = byte(index + 1)
	}
	return base64.RawURLEncoding.EncodeToString(value)
}
