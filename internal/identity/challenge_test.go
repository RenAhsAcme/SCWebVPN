package identity

import (
	"context"
	"testing"
	"time"
)

type fakeChallengeStore struct {
	agentID   string
	digest    [32]byte
	expiresAt time.Time
}

func (store *fakeChallengeStore) SaveNonce(_ context.Context, agentID string, digest [32]byte, expiresAt, _ time.Time) error {
	store.agentID, store.digest, store.expiresAt = agentID, digest, expiresAt
	return nil
}

func TestChallengeIsOpaqueAndStoredAsDigest(t *testing.T) {
	store := &fakeChallengeStore{}
	issuer := NewChallengeIssuer(store)
	now := time.Unix(1_800_000_000, 0).UTC()
	issuer.now = func() time.Time { return now }
	agentID := opaqueID(16)
	plain, expiresAt, err := issuer.Issue(t.Context(), agentID)
	if err != nil {
		t.Fatal(err)
	}
	if plain == "" || store.digest == [32]byte{} || store.agentID != agentID || expiresAt != now.Add(ChallengeTTL) {
		t.Fatal("challenge was not stored as an expiring digest")
	}
}
