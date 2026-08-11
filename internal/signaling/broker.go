package signaling

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"regexp"
	"sync"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/signal"
)

const SessionTTL = 2 * time.Minute

var (
	ErrNotFound        = errors.New("signaling session not found or expired")
	ErrAlreadyClaimed  = errors.New("signaling session was already claimed")
	failureCodePattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
)

type Offer struct {
	ID               string             `json:"id"`
	BrowserSessionID string             `json:"browser_session_id"`
	ServiceID        string             `json:"service_id"`
	CapabilityDigest string             `json:"capability_digest"`
	Offer            signal.Description `json:"offer"`
	RestartOf        string             `json:"restart_of,omitempty"`
	Temporary        bool               `json:"temporary,omitempty"`
}

type CreateRequest struct {
	AgentID          string
	BrowserSessionID string
	ServiceID        string
	UserDigest       [32]byte
	CapabilityDigest [32]byte
	Offer            signal.Description
	RestartOf        string
	Temporary        bool
}

type Result struct {
	Answer      *signal.Description `json:"answer,omitempty"`
	FailureCode string              `json:"failure_code,omitempty"`
}

type Status struct {
	State       string              `json:"state"`
	Answer      *signal.Description `json:"answer,omitempty"`
	FailureCode string              `json:"failure_code,omitempty"`
}

type entry struct {
	id               string
	agentID          string
	browserSessionID string
	serviceID        string
	userDigest       [32]byte
	capabilityDigest [32]byte
	offer            signal.Description
	restartOf        string
	temporary        bool
	answer           *signal.Description
	failureCode      string
	createdAt        time.Time
	claimed          bool
}

type Broker struct {
	mu            sync.Mutex
	sessions      map[string]*entry
	notifications map[string]chan struct{}
	now           func() time.Time
}

func NewBroker() *Broker {
	return &Broker{
		sessions:      make(map[string]*entry),
		notifications: make(map[string]chan struct{}),
		now:           time.Now,
	}
}

func (broker *Broker) Create(request CreateRequest) (string, error) {
	if !validOpaqueID(request.AgentID) || !validOpaqueID(request.BrowserSessionID) || !validOpaqueID(request.ServiceID) {
		return "", errors.New("invalid Agent, service, or browser session ID")
	}
	if request.UserDigest == [32]byte{} || request.CapabilityDigest == [32]byte{} {
		return "", errors.New("user and capability digests are required")
	}
	if err := signal.ValidateDescription(request.Offer, "offer"); err != nil {
		return "", err
	}
	id, err := randomID()
	if err != nil {
		return "", err
	}
	now := broker.now().UTC()
	broker.mu.Lock()
	defer broker.mu.Unlock()
	broker.pruneLocked(now)
	if request.RestartOf != "" {
		previous := broker.sessions[request.RestartOf]
		if previous == nil || previous.agentID != request.AgentID || previous.browserSessionID != request.BrowserSessionID || previous.serviceID != request.ServiceID ||
			previous.userDigest != request.UserDigest || previous.capabilityDigest != request.CapabilityDigest || previous.temporary != request.Temporary {
			return "", errors.New("restart_of must reference the same live authorized session")
		}
	}
	broker.sessions[id] = &entry{
		id: id, agentID: request.AgentID, browserSessionID: request.BrowserSessionID, serviceID: request.ServiceID,
		userDigest: request.UserDigest, capabilityDigest: request.CapabilityDigest,
		offer: request.Offer, restartOf: request.RestartOf, temporary: request.Temporary, createdAt: now,
	}
	notify := broker.notificationLocked(request.AgentID)
	select {
	case notify <- struct{}{}:
	default:
	}
	return id, nil
}

func (broker *Broker) Next(ctx context.Context, agentID string) (Offer, error) {
	for {
		broker.mu.Lock()
		broker.pruneLocked(broker.now().UTC())
		var selected *entry
		for _, current := range broker.sessions {
			if current.agentID == agentID && !current.claimed && current.answer == nil && current.failureCode == "" &&
				(selected == nil || current.createdAt.Before(selected.createdAt)) {
				selected = current
			}
		}
		if selected != nil {
			selected.claimed = true
			result := Offer{
				ID: selected.id, BrowserSessionID: selected.browserSessionID, ServiceID: selected.serviceID,
				CapabilityDigest: base64.RawURLEncoding.EncodeToString(selected.capabilityDigest[:]),
				Offer:            selected.offer, RestartOf: selected.restartOf,
				Temporary: selected.temporary,
			}
			broker.mu.Unlock()
			return result, nil
		}
		notify := broker.notificationLocked(agentID)
		broker.mu.Unlock()
		select {
		case <-ctx.Done():
			return Offer{}, ctx.Err()
		case <-notify:
		}
	}
}

func (broker *Broker) Finish(agentID, sessionID string, result Result) error {
	if (result.Answer == nil) == (result.FailureCode == "") {
		return errors.New("exactly one of answer or failure_code is required")
	}
	if result.Answer != nil {
		if err := signal.ValidateDescription(*result.Answer, "answer"); err != nil {
			return err
		}
	}
	if result.FailureCode != "" && !failureCodePattern.MatchString(result.FailureCode) {
		return errors.New("invalid failure code")
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	broker.pruneLocked(broker.now().UTC())
	current := broker.sessions[sessionID]
	if current == nil || current.agentID != agentID {
		return ErrNotFound
	}
	if !current.claimed || current.answer != nil || current.failureCode != "" {
		return ErrAlreadyClaimed
	}
	if result.Answer != nil {
		answer := *result.Answer
		current.answer = &answer
	} else {
		current.failureCode = result.FailureCode
	}
	return nil
}

func (broker *Broker) Status(sessionID string, userDigest [32]byte) (Status, error) {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	broker.pruneLocked(broker.now().UTC())
	current := broker.sessions[sessionID]
	if current == nil || subtle.ConstantTimeCompare(current.userDigest[:], userDigest[:]) != 1 {
		return Status{}, ErrNotFound
	}
	status := Status{State: "pending"}
	if current.answer != nil {
		answer := *current.answer
		status.State, status.Answer = "answered", &answer
	} else if current.failureCode != "" {
		status.State, status.FailureCode = "failed", current.failureCode
	}
	return status, nil
}

func (broker *Broker) notificationLocked(agentID string) chan struct{} {
	if broker.notifications[agentID] == nil {
		broker.notifications[agentID] = make(chan struct{}, 1)
	}
	return broker.notifications[agentID]
}

func (broker *Broker) pruneLocked(now time.Time) {
	for id, current := range broker.sessions {
		if now.Sub(current.createdAt) >= SessionTTL {
			delete(broker.sessions, id)
		}
	}
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validOpaqueID(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 16
}
