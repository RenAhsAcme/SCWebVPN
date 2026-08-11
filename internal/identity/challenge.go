package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"
)

const (
	challengeSize = 32
	ChallengeTTL  = 2 * time.Minute
)

type ChallengeStore interface {
	SaveNonce(context.Context, string, [32]byte, time.Time, time.Time) error
}

type ChallengeIssuer struct {
	store ChallengeStore
	now   func() time.Time
}

func NewChallengeIssuer(store ChallengeStore) *ChallengeIssuer {
	return &ChallengeIssuer{store: store, now: time.Now}
}

func (issuer *ChallengeIssuer) Issue(ctx context.Context, agentID string) (string, time.Time, error) {
	if !validOpaqueID(agentID, 16) {
		return "", time.Time{}, ErrAgentNotFound
	}
	raw := make([]byte, challengeSize)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	plain := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte("scwebvpn-nonce-v1\x00" + plain))
	now := issuer.now().UTC()
	expiresAt := now.Add(ChallengeTTL)
	if err := issuer.store.SaveNonce(ctx, agentID, digest, expiresAt, now); err != nil {
		if errors.Is(err, ErrAgentNotFound) {
			return "", time.Time{}, ErrAgentNotFound
		}
		return "", time.Time{}, err
	}
	return plain, expiresAt, nil
}
