package binding

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	digest     [32]byte
	expiresAt  time.Time
	consumed   bool
	agent      Agent
	consumeErr error
}

func (store *fakeStore) SaveBindingCode(_ context.Context, digest [32]byte, expiresAt, _ time.Time) error {
	store.digest, store.expiresAt = digest, expiresAt
	return nil
}

func (store *fakeStore) ConsumeBindingCode(_ context.Context, digest [32]byte, agent Agent, now time.Time) error {
	if store.consumeErr != nil {
		return store.consumeErr
	}
	if store.consumed || digest != store.digest || !now.Before(store.expiresAt) {
		return ErrInvalidCode
	}
	store.consumed, store.agent = true, agent
	return nil
}

func TestBindingPreservesStorageFailure(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	code, _, err := service.Issue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	storageFailure := errors.New("database unavailable")
	store.consumeErr = storageFailure
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bind(t.Context(), code, "OpenWrt", publicKey); !errors.Is(err, storageFailure) {
		t.Fatalf("storage failure was hidden: %v", err)
	}
}

func TestBindingCodeIsSingleUseAndStoredAsDigest(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	now := time.Unix(1_800_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	code, expiresAt, err := service.Issue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if code == "" || store.digest == [32]byte{} || expiresAt != now.Add(DefaultTTL) {
		t.Fatal("binding code was not issued as an opaque, digested capability")
	}
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := service.Bind(t.Context(), code, "OpenWrt", publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if agent.ID == "" || !store.consumed {
		t.Fatal("binding did not create an agent and consume the code")
	}
	if _, err := service.Bind(t.Context(), code, "OpenWrt", publicKey); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("binding code replay was not rejected: %v", err)
	}
}

func TestBindingRejectsUnsafeDisplayName(t *testing.T) {
	service := NewService(&fakeStore{})
	code, _, err := service.Issue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bind(t.Context(), code, "OpenWrt\nspoof", publicKey); err == nil {
		t.Fatal("unsafe display name was accepted")
	}
}
