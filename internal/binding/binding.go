package binding

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

const (
	bindingCodeSize = 32
	agentIDSize     = 16
	DefaultTTL      = 10 * time.Minute
)

var ErrInvalidCode = errors.New("binding code is invalid, expired, or already used")

type Agent struct {
	ID          string
	DisplayName string
	PublicKey   ed25519.PublicKey
	CreatedAt   time.Time
}

type Store interface {
	SaveBindingCode(context.Context, [32]byte, time.Time, time.Time) error
	ConsumeBindingCode(context.Context, [32]byte, Agent, time.Time) error
}

type Service struct {
	store Store
	now   func() time.Time
	ttl   time.Duration
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now, ttl: DefaultTTL}
}

func (service *Service) Issue(ctx context.Context) (string, time.Time, error) {
	plain, digest, err := newBindingCode()
	if err != nil {
		return "", time.Time{}, err
	}
	now := service.now().UTC()
	expiresAt := now.Add(service.ttl)
	if err := service.store.SaveBindingCode(ctx, digest, expiresAt, now); err != nil {
		return "", time.Time{}, err
	}
	return plain, expiresAt, nil
}

func (service *Service) Bind(ctx context.Context, code, displayName string, publicKey ed25519.PublicKey) (Agent, error) {
	digest, err := bindingDigest(code)
	if err != nil {
		return Agent{}, ErrInvalidCode
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 80 || strings.ContainsAny(displayName, "\r\n\x00") {
		return Agent{}, errors.New("display name must contain 1 to 80 safe bytes")
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return Agent{}, errors.New("invalid Ed25519 public key")
	}
	id, err := randomOpaque(agentIDSize)
	if err != nil {
		return Agent{}, err
	}
	now := service.now().UTC()
	agent := Agent{ID: id, DisplayName: displayName, PublicKey: append(ed25519.PublicKey(nil), publicKey...), CreatedAt: now}
	if err := service.store.ConsumeBindingCode(ctx, digest, agent, now); err != nil {
		if errors.Is(err, ErrInvalidCode) {
			return Agent{}, ErrInvalidCode
		}
		return Agent{}, err
	}
	return agent, nil
}

func newBindingCode() (string, [32]byte, error) {
	plain, err := randomOpaque(bindingCodeSize)
	if err != nil {
		return "", [32]byte{}, err
	}
	digest, err := bindingDigest(plain)
	return plain, digest, err
}

func bindingDigest(value string) ([32]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) != bindingCodeSize {
		return [32]byte{}, ErrInvalidCode
	}
	return sha256.Sum256([]byte("scwebvpn-binding-v1\x00" + value)), nil
}

func randomOpaque(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
