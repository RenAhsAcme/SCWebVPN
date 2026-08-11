package presence

import (
	"context"
	"errors"
	"time"
)

const OnlineWindow = 45 * time.Second

var ErrNotFound = errors.New("Agent not found")

type Store interface {
	TouchAgent(context.Context, string, time.Time) error
	AgentLastSeen(context.Context, string) (time.Time, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (service *Service) Touch(ctx context.Context, agentID string) error {
	return service.store.TouchAgent(ctx, agentID, service.now().UTC())
}

func (service *Service) Status(ctx context.Context, agentID string) (bool, time.Time, error) {
	lastSeen, err := service.store.AgentLastSeen(ctx, agentID)
	if err != nil {
		return false, time.Time{}, err
	}
	return service.now().UTC().Sub(lastSeen) <= OnlineWindow, lastSeen, nil
}
