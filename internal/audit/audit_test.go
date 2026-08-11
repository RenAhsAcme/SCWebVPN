package audit

import (
	"context"
	"testing"
)

type fakeStore struct{ event Event }

func (store *fakeStore) RecordAudit(_ context.Context, event Event) error {
	store.event = event
	return nil
}

func TestRecorderAcceptsOnlyMetadataCodes(t *testing.T) {
	store := &fakeStore{}
	recorder := NewRecorder(store)
	if err := recorder.Record(context.Background(), Event{Type: "connection_created", ResultCode: "ok", Candidate: "srflx"}); err != nil {
		t.Fatal(err)
	}
	if store.event.OccurredAt.IsZero() {
		t.Fatal("audit timestamp was not assigned")
	}
	for _, event := range []Event{
		{Type: "contains target 192.0.2.1", ResultCode: "ok"},
		{Type: "connection", ResultCode: "arbitrary failure text"},
		{Type: "connection", ResultCode: "ok", Candidate: "relay"},
	} {
		if err := recorder.Record(context.Background(), event); err == nil {
			t.Fatalf("unsafe audit event was accepted: %#v", event)
		}
	}
}
