package presence

import (
	"context"
	"testing"
	"time"
)

type fakeStore struct{ seen time.Time }

func (store *fakeStore) TouchAgent(_ context.Context, _ string, seen time.Time) error {
	store.seen = seen
	return nil
}

func (store *fakeStore) AgentLastSeen(context.Context, string) (time.Time, error) {
	return store.seen, nil
}

func TestOnlineWindow(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{seen: now.Add(-OnlineWindow)}
	service := NewService(store)
	service.now = func() time.Time { return now }
	online, _, err := service.Status(context.Background(), "agent")
	if err != nil || !online {
		t.Fatalf("Agent at the online boundary was offline: %v", err)
	}
	store.seen = now.Add(-OnlineWindow - time.Nanosecond)
	online, _, err = service.Status(context.Background(), "agent")
	if err != nil || online {
		t.Fatalf("stale Agent was online: %v", err)
	}
}
