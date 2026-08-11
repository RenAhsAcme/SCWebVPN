package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/identity"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestControllerClientSignsPollAfterChallenge(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	agentID := "ERITFBUWFxgZGhscHR4fIA"
	nonce := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	now := time.Unix(1_800_000_000, 0).UTC()
	step := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		step++
		switch step {
		case 1:
			if request.URL.Path != "/api/v1/agent/challenges" {
				t.Fatalf("unexpected challenge path %s", request.URL.Path)
			}
			return response(http.StatusCreated, `{"nonce":"`+nonce+`","expires_at":"`+now.Add(2*time.Minute).Format(time.RFC3339Nano)+`"}`), nil
		case 2:
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			signed, err := identity.ParseSignedRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			if err := identity.Verify(publicKey, signed, request.Method, request.URL.RequestURI(), body, now); err != nil {
				t.Fatal(err)
			}
			return response(http.StatusNoContent, ""), nil
		default:
			t.Fatal("unexpected additional Controller request")
			return nil, nil
		}
	})}
	client, err := NewControllerClient("https://vpn.example.com", agentID, privateKey, httpClient)
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return now }
	if _, available, err := client.NextOffer(t.Context()); err != nil || available {
		t.Fatalf("unexpected poll result: available=%v err=%v", available, err)
	}
	if step != 2 {
		t.Fatalf("Controller received %d requests", step)
	}
}

func TestBindAgentSendsOnlyPublicIdentityAndBindingCode(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	code := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/v1/agents/bind" {
			t.Fatalf("unexpected binding path %s", request.URL.Path)
		}
		var input map[string]string
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input["code"] != code || input["public_key"] != base64.RawURLEncoding.EncodeToString(publicKey) || input["display_name"] != "OpenWrt" {
			t.Fatalf("unexpected binding body: %#v", input)
		}
		return response(http.StatusCreated, `{"agent_id":"ERITFBUWFxgZGhscHR4fIA"}`), nil
	})}
	agentID, err := BindAgent(t.Context(), "https://vpn.example.com", code, "OpenWrt", publicKey, httpClient)
	if err != nil {
		t.Fatal(err)
	}
	if agentID != "ERITFBUWFxgZGhscHR4fIA" {
		t.Fatalf("unexpected Agent ID %q", agentID)
	}
}

func TestControllerHTTPErrorHasSecretFreePublicCode(t *testing.T) {
	err := &controllerHTTPError{operation: "poll", status: http.StatusUnauthorized}
	if got := publicError(err); got != "controller_poll_http_401" {
		t.Fatalf("unexpected public error code %q", got)
	}
}

func TestControllerTransportErrorHasSecretFreePublicCode(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("sensitive transport detail")
	})}
	client, err := NewControllerClient("https://vpn.example.com", "ERITFBUWFxgZGhscHR4fIA", privateKey, httpClient)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.NextOffer(t.Context())
	if got := publicError(err); got != "controller_challenge_transport" {
		t.Fatalf("unexpected public error code %q", got)
	}
}

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
