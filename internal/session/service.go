package session

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/catalog"
)

const (
	IdleTTL     = 30 * time.Minute
	AbsoluteTTL = 2 * time.Hour
)

var ErrNotFound = errors.New("browser session not found or expired")

type Record struct {
	ID                string
	TokenDigest       [32]byte
	UserDigest        [32]byte
	AgentID           string
	ServiceID         string
	Temporary         bool
	TemporaryHost     string
	CreatedAt         time.Time
	LastSeenAt        time.Time
	ExpiresAt         time.Time
	AbsoluteExpiresAt time.Time
}

type Store interface {
	catalog.Store
	CreateBrowserSession(context.Context, Record) error
	BrowserSession(context.Context, string, [32]byte, time.Time) (Record, error)
	TouchBrowserSession(context.Context, string, [32]byte, time.Time, time.Time) error
	RevokeBrowserSession(context.Context, string, [32]byte, time.Time) error
	RevokeBrowserSessions(context.Context, [32]byte, time.Time) error
}

type Created struct {
	Record Record
	Token  string
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (service *Service) Create(ctx context.Context, userDigest [32]byte, serviceID string) (Created, error) {
	target, err := service.store.AuthorizedService(ctx, serviceID, userDigest, service.now().UTC())
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			return Created{}, ErrNotFound
		}
		return Created{}, err
	}
	if !target.Enabled {
		return Created{}, ErrNotFound
	}
	id, err := randomID()
	if err != nil {
		return Created{}, err
	}
	token, tokenDigest, err := NewToken()
	if err != nil {
		return Created{}, err
	}
	now := service.now().UTC()
	record := Record{
		ID:                id,
		TokenDigest:       tokenDigest,
		UserDigest:        userDigest,
		AgentID:           target.AgentID,
		ServiceID:         target.ID,
		Temporary:         target.Temporary,
		CreatedAt:         now,
		LastSeenAt:        now,
		ExpiresAt:         now.Add(IdleTTL),
		AbsoluteExpiresAt: now.Add(AbsoluteTTL),
	}
	if err := service.store.CreateBrowserSession(ctx, record); err != nil {
		return Created{}, err
	}
	return Created{Record: record, Token: token}, nil
}

func (service *Service) Lookup(ctx context.Context, id string, userDigest [32]byte, token string) (Record, error) {
	if ValidateToken(token) != nil {
		return Record{}, ErrNotFound
	}
	now := service.now().UTC()
	record, err := service.store.BrowserSession(ctx, id, userDigest, now)
	if err != nil {
		return Record{}, ErrNotFound
	}
	providedDigest := HashToken(token)
	if subtle.ConstantTimeCompare(record.TokenDigest[:], providedDigest[:]) != 1 {
		return Record{}, ErrNotFound
	}
	target, err := service.store.AuthorizedService(ctx, record.ServiceID, userDigest, now)
	if err != nil || target.AgentID != record.AgentID || target.Temporary != record.Temporary {
		return Record{}, ErrNotFound
	}
	nextExpiry := now.Add(IdleTTL)
	if nextExpiry.After(record.AbsoluteExpiresAt) {
		nextExpiry = record.AbsoluteExpiresAt
	}
	if err := service.store.TouchBrowserSession(ctx, id, userDigest, now, nextExpiry); err != nil {
		return Record{}, err
	}
	record.LastSeenAt, record.ExpiresAt = now, nextExpiry
	return record, nil
}

func (service *Service) RevokeAll(ctx context.Context, userDigest [32]byte) error {
	return service.store.RevokeBrowserSessions(ctx, userDigest, service.now().UTC())
}

func (service *Service) Revoke(ctx context.Context, id string, userDigest [32]byte) error {
	return service.store.RevokeBrowserSession(ctx, id, userDigest, service.now().UTC())
}

func randomID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
