package identity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

type fakeAgentStore struct {
	key      ed25519.PublicKey
	keyErr   error
	consumed map[[32]byte]bool
}

func (store *fakeAgentStore) AgentPublicKey(context.Context, string) (ed25519.PublicKey, error) {
	if store.keyErr != nil {
		return nil, store.keyErr
	}
	return store.key, nil
}

func TestAgentAuthenticatorPreservesStorageFailure(t *testing.T) {
	storageFailure := errors.New("database unavailable")
	auth := NewAgentAuthenticator(&fakeAgentStore{keyErr: storageFailure})
	now := time.Unix(1_800_000_000, 0).UTC()
	auth.now = func() time.Time { return now }
	request := httptest.NewRequest(http.MethodGet, "https://vpn.example/api/v1/agent/poll", nil)
	request.Header.Set("X-WebVPN-Agent-ID", opaqueID(16))
	request.Header.Set("X-WebVPN-Nonce", opaqueID(32))
	request.Header.Set("X-WebVPN-Issued-At", strconv.FormatInt(now.Unix(), 10))
	request.Header.Set("X-WebVPN-Signature", base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)))
	if _, _, err := auth.Verify(request); !errors.Is(err, storageFailure) {
		t.Fatalf("storage failure was hidden: %v", err)
	}
}

func (store *fakeAgentStore) ConsumeNonce(_ context.Context, _ string, digest [32]byte, _ time.Time) error {
	if store.consumed[digest] {
		return ErrNonceUsed
	}
	store.consumed[digest] = true
	return nil
}

func TestAgentAuthenticatorConsumesNonceOnce(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	store := &fakeAgentStore{key: publicKey, consumed: make(map[[32]byte]bool)}
	auth := NewAgentAuthenticator(store)
	auth.now = func() time.Time { return now }
	body := []byte(`{"state":"online"}`)
	agentID, nonce := opaqueID(16), opaqueID(32)
	signature := ed25519.Sign(privateKey, CanonicalMessage(agentID, nonce, now, http.MethodPost, "/api/v1/agent/poll", body))

	makeRequest := func() *http.Request {
		request := httptest.NewRequest(http.MethodPost, "https://vpn.example/api/v1/agent/poll", bytes.NewReader(body))
		request.Header.Set("X-WebVPN-Agent-ID", agentID)
		request.Header.Set("X-WebVPN-Nonce", nonce)
		request.Header.Set("X-WebVPN-Issued-At", strconv.FormatInt(now.Unix(), 10))
		request.Header.Set("X-WebVPN-Signature", base64.RawURLEncoding.EncodeToString(signature))
		return request
	}
	if _, _, err := auth.Verify(makeRequest()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Verify(makeRequest()); !errors.Is(err, ErrNonceUsed) {
		t.Fatalf("replayed nonce was not rejected: %v", err)
	}
	digest := sha256.Sum256([]byte("scwebvpn-nonce-v1\x00" + nonce))
	if !store.consumed[digest] {
		t.Fatal("nonce digest was not consumed")
	}
}

func TestAgentAuthenticatorBindsQueryString(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	store := &fakeAgentStore{key: publicKey, consumed: make(map[[32]byte]bool)}
	auth := NewAgentAuthenticator(store)
	auth.now = func() time.Time { return now }
	agentID, nonce := opaqueID(16), opaqueID(32)
	signature := ed25519.Sign(privateKey, CanonicalMessage(agentID, nonce, now, http.MethodGet, "/api/v1/agent/poll?cursor=1", nil))
	request := httptest.NewRequest(http.MethodGet, "https://vpn.example/api/v1/agent/poll?cursor=2", nil)
	request.Header.Set("X-WebVPN-Agent-ID", agentID)
	request.Header.Set("X-WebVPN-Nonce", nonce)
	request.Header.Set("X-WebVPN-Issued-At", strconv.FormatInt(now.Unix(), 10))
	request.Header.Set("X-WebVPN-Signature", base64.RawURLEncoding.EncodeToString(signature))
	if _, _, err := auth.Verify(request); err == nil {
		t.Fatal("request with a modified query string was accepted")
	}
}
