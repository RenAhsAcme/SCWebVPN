package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const MaxSignedBody = 1 << 20

var (
	ErrAgentNotFound = errors.New("agent not found")
	ErrNonceUsed     = errors.New("nonce is invalid, expired, or already used")
)

type AgentStore interface {
	AgentPublicKey(context.Context, string) (ed25519.PublicKey, error)
	ConsumeNonce(context.Context, string, [32]byte, time.Time) error
}

type AgentAuthenticator struct {
	store AgentStore
	now   func() time.Time
}

func NewAgentAuthenticator(store AgentStore) *AgentAuthenticator {
	return &AgentAuthenticator{store: store, now: time.Now}
}

func (auth *AgentAuthenticator) Verify(request *http.Request) ([]byte, SignedRequest, error) {
	signed, err := ParseSignedRequest(request)
	if err != nil {
		return nil, SignedRequest{}, err
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, MaxSignedBody+1))
	if err != nil {
		return nil, SignedRequest{}, fmt.Errorf("read signed body: %w", err)
	}
	if len(body) > MaxSignedBody {
		return nil, SignedRequest{}, errors.New("signed request body is too large")
	}
	publicKey, err := auth.store.AgentPublicKey(request.Context(), signed.AgentID)
	if err != nil {
		if errors.Is(err, ErrAgentNotFound) {
			return nil, SignedRequest{}, ErrAgentNotFound
		}
		return nil, SignedRequest{}, fmt.Errorf("load Agent identity: %w", err)
	}
	if err := Verify(publicKey, signed, request.Method, request.URL.RequestURI(), body, auth.now().UTC()); err != nil {
		return nil, SignedRequest{}, err
	}
	nonceDigest := sha256.Sum256([]byte("scwebvpn-nonce-v1\x00" + signed.Nonce))
	if err := auth.store.ConsumeNonce(request.Context(), signed.AgentID, nonceDigest, auth.now().UTC()); err != nil {
		return nil, SignedRequest{}, ErrNonceUsed
	}
	return body, signed, nil
}
