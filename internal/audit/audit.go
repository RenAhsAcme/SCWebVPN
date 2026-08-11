package audit

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var codePattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

type Event struct {
	OccurredAt  time.Time
	ActorDigest *[32]byte
	AgentID     string
	ServiceID   string
	Type        string
	ResultCode  string
	Candidate   string
	Latency     string
	Bytes       string
}

type Store interface {
	RecordAudit(context.Context, Event) error
}

type Recorder struct {
	store Store
	now   func() time.Time
}

func NewRecorder(store Store) *Recorder {
	return &Recorder{store: store, now: time.Now}
}

func (recorder *Recorder) Record(ctx context.Context, event Event) error {
	if !codePattern.MatchString(event.Type) || !codePattern.MatchString(event.ResultCode) {
		return errors.New("audit event type and result must be bounded codes")
	}
	if event.Candidate != "" && event.Candidate != "host" && event.Candidate != "srflx" && event.Candidate != "prflx" {
		return errors.New("audit candidate type is invalid")
	}
	event.OccurredAt = recorder.now().UTC()
	return recorder.store.RecordAudit(ctx, event)
}
