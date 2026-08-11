package signaling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/signal"
)

func TestBrokerRoutesOnlyToBoundAgent(t *testing.T) {
	broker := NewBroker()
	now := time.Unix(1_800_000_000, 0).UTC()
	broker.now = func() time.Time { return now }
	agentID := mustRandomID(t)
	request := createRequest(t, agentID)
	sessionID, err := broker.Create(request)
	if err != nil {
		t.Fatal(err)
	}
	otherCtx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := broker.Next(otherCtx, mustRandomID(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("another Agent claimed the offer: %v", err)
	}
	offer, err := broker.Next(t.Context(), agentID)
	if err != nil || offer.ID != sessionID {
		t.Fatalf("bound Agent did not receive the offer: %#v, %v", offer, err)
	}
	answer := validDescription("answer")
	if err := broker.Finish(agentID, sessionID, Result{Answer: &answer}); err != nil {
		t.Fatal(err)
	}
	status, err := broker.Status(sessionID, request.UserDigest)
	if err != nil || status.State != "answered" {
		t.Fatalf("unexpected status: %#v, %v", status, err)
	}
}

func TestBrokerRejectsCrossAgentRestart(t *testing.T) {
	broker := NewBroker()
	firstAgent, otherAgent := mustRandomID(t), mustRandomID(t)
	request := createRequest(t, firstAgent)
	first, err := broker.Create(request)
	if err != nil {
		t.Fatal(err)
	}
	crossAgent := request
	crossAgent.AgentID, crossAgent.RestartOf = otherAgent, first
	if _, err := broker.Create(crossAgent); err == nil {
		t.Fatal("cross-Agent ICE restart chain was accepted")
	}
	restart := request
	restart.RestartOf = first
	if _, err := broker.Create(restart); err != nil {
		t.Fatalf("same-Agent ICE restart chain was rejected: %v", err)
	}
}

func TestBrokerExpiresSDPFromMemory(t *testing.T) {
	broker := NewBroker()
	now := time.Unix(1_800_000_000, 0).UTC()
	broker.now = func() time.Time { return now }
	request := createRequest(t, mustRandomID(t))
	id, err := broker.Create(request)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(SessionTTL)
	if _, err := broker.Status(id, request.UserDigest); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired signaling description remained available: %v", err)
	}
}

func TestBrokerRejectsRelayAndFreeFormFailure(t *testing.T) {
	broker := NewBroker()
	agentID := mustRandomID(t)
	relay := signal.Description{Type: "offer", SDP: "v=0\r\na=candidate:1 1 udp 1 203.0.113.1 5000 typ relay\r\n"}
	relayRequest := createRequest(t, agentID)
	relayRequest.Offer = relay
	if _, err := broker.Create(relayRequest); err == nil {
		t.Fatal("relay offer was accepted")
	}
	request := createRequest(t, agentID)
	id, err := broker.Create(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Next(t.Context(), agentID); err != nil {
		t.Fatal(err)
	}
	if err := broker.Finish(agentID, id, Result{FailureCode: "raw error: 192.168.1.1"}); err == nil {
		t.Fatal("free-form failure text was accepted")
	}
}

func TestBrokerHidesStatusFromAnotherUser(t *testing.T) {
	broker := NewBroker()
	request := createRequest(t, mustRandomID(t))
	id, err := broker.Create(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Status(id, [32]byte{9}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user observed signaling status: %v", err)
	}
}

func mustRandomID(t *testing.T) string {
	t.Helper()
	value, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func validDescription(kind string) signal.Description {
	return signal.Description{Type: kind, SDP: "v=0\r\na=candidate:1 1 udp 1 192.0.2.1 5000 typ host\r\n"}
}

func createRequest(t *testing.T, agentID string) CreateRequest {
	t.Helper()
	return CreateRequest{
		AgentID: agentID, BrowserSessionID: mustRandomID(t), ServiceID: mustRandomID(t), UserDigest: [32]byte{1},
		CapabilityDigest: [32]byte{2}, Offer: validDescription("offer"),
	}
}
