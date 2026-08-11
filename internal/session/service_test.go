package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/catalog"
)

type fakeSessionStore struct {
	target  catalog.Service
	record  Record
	revoked bool
}

func (store *fakeSessionStore) ListServices(context.Context) ([]catalog.Service, error) {
	return []catalog.Service{store.target}, nil
}

func (store *fakeSessionStore) ServiceByID(_ context.Context, id string) (catalog.Service, error) {
	if id != store.target.ID {
		return catalog.Service{}, catalog.ErrNotFound
	}
	return store.target, nil
}

func (store *fakeSessionStore) AuthorizedService(_ context.Context, id string, _ [32]byte, _ time.Time) (catalog.Service, error) {
	return store.ServiceByID(context.Background(), id)
}

func (store *fakeSessionStore) TemporaryServiceBySlug(context.Context, string, [32]byte, time.Time) (catalog.Service, error) {
	return catalog.Service{}, catalog.ErrNotFound
}

func (store *fakeSessionStore) RevokeTemporaryServices(context.Context, [32]byte, time.Time) error {
	return nil
}

func (store *fakeSessionStore) CreateBrowserSession(_ context.Context, record Record) error {
	store.record = record
	return nil
}

func (store *fakeSessionStore) BrowserSession(_ context.Context, id string, user [32]byte, now time.Time) (Record, error) {
	if store.revoked || id != store.record.ID || user != store.record.UserDigest || !now.Before(store.record.ExpiresAt) || !now.Before(store.record.AbsoluteExpiresAt) {
		return Record{}, ErrNotFound
	}
	return store.record, nil
}

func (store *fakeSessionStore) TouchBrowserSession(_ context.Context, _ string, _ [32]byte, seenAt, expiresAt time.Time) error {
	store.record.LastSeenAt, store.record.ExpiresAt = seenAt, expiresAt
	return nil
}

func (store *fakeSessionStore) RevokeBrowserSessions(_ context.Context, _ [32]byte, _ time.Time) error {
	store.revoked = true
	return nil
}

func (store *fakeSessionStore) RevokeBrowserSession(_ context.Context, id string, user [32]byte, _ time.Time) error {
	if id != store.record.ID || user != store.record.UserDigest {
		return ErrNotFound
	}
	store.revoked = true
	return nil
}

func TestSessionLifetimesAndUserBinding(t *testing.T) {
	store := &fakeSessionStore{target: catalog.Service{
		ID: "AQIDBAUGBwgJCgsMDQ4PEA", AgentID: "ERITFBUWFxgZGhscHR4fIA", Enabled: true,
	}}
	service := NewService(store)
	now := time.Unix(1_800_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	user := [32]byte{1}
	created, err := service.Create(t.Context(), user, store.target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.Record.ExpiresAt != now.Add(IdleTTL) || created.Record.AbsoluteExpiresAt != now.Add(AbsoluteTTL) {
		t.Fatal("session did not receive the required capability and lifetimes")
	}
	if _, err := service.Lookup(t.Context(), created.Record.ID, [32]byte{2}, created.Token); !errors.Is(err, ErrNotFound) {
		t.Fatal("another user accessed the session")
	}
	now = now.Add(20 * time.Minute)
	record, err := service.Lookup(t.Context(), created.Record.ID, user, created.Token)
	if err != nil {
		t.Fatal(err)
	}
	if record.ExpiresAt != now.Add(IdleTTL) {
		t.Fatal("idle lease was not renewed")
	}
	for _, elapsed := range []time.Duration{40 * time.Minute, 60 * time.Minute, 80 * time.Minute, 100 * time.Minute} {
		now = created.Record.CreatedAt.Add(elapsed)
		record, err = service.Lookup(t.Context(), created.Record.ID, user, created.Token)
		if err != nil {
			t.Fatal(err)
		}
	}
	if record.ExpiresAt != created.Record.AbsoluteExpiresAt {
		t.Fatal("idle renewal exceeded the absolute lifetime")
	}
}

func TestWrongCapabilityDoesNotRenewLease(t *testing.T) {
	store := &fakeSessionStore{target: catalog.Service{
		ID: "AQIDBAUGBwgJCgsMDQ4PEA", AgentID: "ERITFBUWFxgZGhscHR4fIA", Enabled: true,
	}}
	service := NewService(store)
	now := time.Unix(1_800_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	created, err := service.Create(t.Context(), [32]byte{1}, store.target.ID)
	if err != nil {
		t.Fatal(err)
	}
	originalExpiry := store.record.ExpiresAt
	now = now.Add(10 * time.Minute)
	wrongToken, _, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Lookup(t.Context(), created.Record.ID, created.Record.UserDigest, wrongToken); !errors.Is(err, ErrNotFound) {
		t.Fatal("wrong capability was accepted")
	}
	if store.record.ExpiresAt != originalExpiry {
		t.Fatal("wrong capability renewed the lease")
	}
}
