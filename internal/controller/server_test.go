package controller

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/audit"
	"github.com/RenAhsAcme/SCWebVPN/internal/binding"
	"github.com/RenAhsAcme/SCWebVPN/internal/catalog"
	"github.com/RenAhsAcme/SCWebVPN/internal/httpapi"
	"github.com/RenAhsAcme/SCWebVPN/internal/identity"
	"github.com/RenAhsAcme/SCWebVPN/internal/presence"
	"github.com/RenAhsAcme/SCWebVPN/internal/session"
	"github.com/RenAhsAcme/SCWebVPN/internal/signal"
	"github.com/RenAhsAcme/SCWebVPN/internal/signaling"
)

type fakeControlStore struct {
	service        catalog.Service
	publicKey      ed25519.PublicKey
	record         session.Record
	nonceDigest    [32]byte
	nonceExpiry    time.Time
	nonceUsed      bool
	lastSeen       time.Time
	audits         []audit.Event
	temporaryOwner [32]byte
}

func (store *fakeControlStore) RecordAudit(_ context.Context, event audit.Event) error {
	store.audits = append(store.audits, event)
	return nil
}

func (store *fakeControlStore) TouchAgent(_ context.Context, agentID string, seen time.Time) error {
	if agentID != store.service.AgentID {
		return presence.ErrNotFound
	}
	store.lastSeen = seen
	return nil
}

func (store *fakeControlStore) AgentLastSeen(_ context.Context, agentID string) (time.Time, error) {
	if agentID != store.service.AgentID {
		return time.Time{}, presence.ErrNotFound
	}
	return store.lastSeen, nil
}

func (store *fakeControlStore) SaveBindingCode(context.Context, [32]byte, time.Time, time.Time) error {
	return nil
}

func (store *fakeControlStore) ConsumeBindingCode(context.Context, [32]byte, binding.Agent, time.Time) error {
	return nil
}

func (store *fakeControlStore) ListServices(context.Context) ([]catalog.Service, error) {
	return []catalog.Service{store.service}, nil
}

func (store *fakeControlStore) ServiceByID(_ context.Context, id string) (catalog.Service, error) {
	if id != store.service.ID {
		return catalog.Service{}, catalog.ErrNotFound
	}
	return store.service, nil
}

func (store *fakeControlStore) AuthorizedService(_ context.Context, id string, owner [32]byte, _ time.Time) (catalog.Service, error) {
	if id != store.service.ID || store.service.Temporary && owner != store.temporaryOwner {
		return catalog.Service{}, catalog.ErrNotFound
	}
	return store.service, nil
}

func (store *fakeControlStore) TemporaryServiceBySlug(_ context.Context, slug string, owner [32]byte, _ time.Time) (catalog.Service, error) {
	if !store.service.Temporary || slug != store.service.Slug || owner != store.temporaryOwner {
		return catalog.Service{}, catalog.ErrNotFound
	}
	return store.service, nil
}

func (store *fakeControlStore) RevokeTemporaryServices(_ context.Context, owner [32]byte, _ time.Time) error {
	if owner == store.temporaryOwner {
		store.service.Enabled = false
	}
	return nil
}

func (store *fakeControlStore) CreateService(_ context.Context, service catalog.Service, _ time.Time) error {
	store.service = service
	return nil
}

func (store *fakeControlStore) CreateTemporaryService(_ context.Context, service catalog.Service, owner [32]byte, _, _, _ time.Time) error {
	store.service, store.temporaryOwner = service, owner
	return nil
}

func (store *fakeControlStore) CreateBrowserSession(_ context.Context, record session.Record) error {
	store.record = record
	return nil
}

func (store *fakeControlStore) BrowserSession(_ context.Context, id string, userDigest [32]byte, now time.Time) (session.Record, error) {
	if id != store.record.ID || userDigest != store.record.UserDigest || !now.Before(store.record.ExpiresAt) || !now.Before(store.record.AbsoluteExpiresAt) {
		return session.Record{}, session.ErrNotFound
	}
	return store.record, nil
}

func (store *fakeControlStore) TouchBrowserSession(_ context.Context, _ string, _ [32]byte, seenAt, expiresAt time.Time) error {
	store.record.LastSeenAt, store.record.ExpiresAt = seenAt, expiresAt
	return nil
}

func (store *fakeControlStore) RevokeBrowserSession(context.Context, string, [32]byte, time.Time) error {
	return nil
}

func (store *fakeControlStore) RevokeBrowserSessions(context.Context, [32]byte, time.Time) error {
	return nil
}

func (store *fakeControlStore) AgentPublicKey(_ context.Context, agentID string) (ed25519.PublicKey, error) {
	if agentID != store.service.AgentID {
		return nil, identity.ErrAgentNotFound
	}
	return store.publicKey, nil
}

func (store *fakeControlStore) SaveNonce(_ context.Context, agentID string, digest [32]byte, expiresAt, _ time.Time) error {
	if agentID != store.service.AgentID {
		return identity.ErrAgentNotFound
	}
	store.nonceDigest, store.nonceExpiry, store.nonceUsed = digest, expiresAt, false
	return nil
}

func (store *fakeControlStore) ConsumeNonce(_ context.Context, agentID string, digest [32]byte, now time.Time) error {
	if agentID != store.service.AgentID || digest != store.nonceDigest || store.nonceUsed || !now.Before(store.nonceExpiry) {
		return identity.ErrNonceUsed
	}
	store.nonceUsed = true
	return nil
}

func TestBrowserToAgentSignalingFlowAndRestart(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeControlStore{
		publicKey: publicKey,
		service: catalog.Service{
			ID: "AQIDBAUGBwgJCgsMDQ4PEA", AgentID: "ERITFBUWFxgZGhscHR4fIA",
			Slug: "openwrt", DisplayName: "OpenWrt", Kind: "https", PolicyRef: "openwrt-luci", Enabled: true,
		},
	}
	secret := []byte("0123456789abcdef0123456789abcdef")
	browserAuth, err := httpapi.NewBrowserAuth(secret)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Dependencies{
		BrowserAuth:   browserAuth,
		Bindings:      binding.NewService(store),
		Catalog:       store,
		CatalogAdmin:  catalog.NewManager(store),
		Sessions:      session.NewService(store),
		Challenges:    identity.NewChallengeIssuer(store),
		AgentAuth:     identity.NewAgentAuthenticator(store),
		Signals:       signaling.NewBroker(),
		STUNURLs:      []string{"stun:stun.example.net:3478"},
		Audit:         audit.NewRecorder(store),
		Presence:      presence.NewService(store),
		PublicBaseURL: "https://vpn.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	initial := connectionRequest{ServiceID: store.service.ID, Offer: testDescription("offer")}
	initialResponse := doBrowserJSON(t, server, secret, http.MethodPost, "/api/v1/browser/connections", initial)
	if initialResponse.Code != http.StatusCreated {
		t.Fatalf("create connection: %d %s", initialResponse.Code, initialResponse.Body.String())
	}
	var created struct {
		ID               string `json:"id"`
		BrowserSessionID string `json:"browser_session_id"`
		Capability       string `json:"capability"`
	}
	decodeResponse(t, initialResponse, &created)
	if session.ValidateToken(created.Capability) != nil || created.BrowserSessionID == "" {
		t.Fatal("browser did not receive an opaque session capability")
	}

	offer := pollAgentOffer(t, server, store, privateKey)
	if offer.ID != created.ID || offer.BrowserSessionID != created.BrowserSessionID || offer.ServiceID != store.service.ID {
		t.Fatalf("Agent received the wrong authorized offer: %#v", offer)
	}
	expectedDigest := session.HashToken(created.Capability)
	if offer.CapabilityDigest != base64.RawURLEncoding.EncodeToString(expectedDigest[:]) {
		t.Fatal("Agent did not receive the browser capability digest")
	}
	if store.lastSeen.IsZero() {
		t.Fatal("signed Agent poll did not update presence")
	}
	presenceResponse := doBrowserJSON(t, server, secret, http.MethodGet, "/api/v1/browser/agents/"+store.service.AgentID+"/status", nil)
	if presenceResponse.Code != http.StatusOK {
		t.Fatalf("read Agent presence: %d %s", presenceResponse.Code, presenceResponse.Body.String())
	}
	var agentStatus struct {
		Online bool `json:"online"`
	}
	decodeResponse(t, presenceResponse, &agentStatus)
	if !agentStatus.Online {
		t.Fatal("recently polling Agent was reported offline")
	}

	answer := testDescription("answer")
	finishAgentConnection(t, server, store, privateKey, created.ID, signaling.Result{Answer: &answer})
	statusResponse := doBrowserJSON(t, server, secret, http.MethodGet, "/api/v1/browser/connections/"+created.ID, nil)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("read connection status: %d %s", statusResponse.Code, statusResponse.Body.String())
	}
	var status signaling.Status
	decodeResponse(t, statusResponse, &status)
	if status.State != "answered" {
		t.Fatalf("unexpected connection status: %#v", status)
	}

	restart := connectionRequest{
		ServiceID: store.service.ID, BrowserSessionID: created.BrowserSessionID,
		Capability: created.Capability, Offer: testDescription("offer"), RestartOf: created.ID,
	}
	restartResponse := doBrowserJSON(t, server, secret, http.MethodPost, "/api/v1/browser/connections", restart)
	if restartResponse.Code != http.StatusCreated {
		t.Fatalf("create ICE restart: %d %s", restartResponse.Code, restartResponse.Body.String())
	}
}

func TestBrowserConfigIsAuthenticatedAndSTUNOnly(t *testing.T) {
	server, secret := newTestServer(t)
	response := doBrowserJSON(t, server, secret, http.MethodGet, "/api/v1/browser/config", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("read browser config: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		STUNURLs []string `json:"stun_urls"`
	}
	decodeResponse(t, response, &payload)
	if len(payload.STUNURLs) != 1 || payload.STUNURLs[0] != "stun:stun.example.net:3478" {
		t.Fatalf("unexpected browser config: %#v", payload)
	}

	unauthenticated := doJSON(t, server, http.MethodGet, "/api/v1/browser/config", nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated config status: %d", unauthenticated.Code)
	}
}

func TestControllerRejectsTURNDependency(t *testing.T) {
	store := &fakeControlStore{}
	secret := []byte("0123456789abcdef0123456789abcdef")
	browserAuth, err := httpapi.NewBrowserAuth(secret)
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(Dependencies{
		BrowserAuth: browserAuth, Bindings: binding.NewService(store), Catalog: store,
		CatalogAdmin: catalog.NewManager(store), Sessions: session.NewService(store),
		Challenges: identity.NewChallengeIssuer(store), AgentAuth: identity.NewAgentAuthenticator(store),
		Signals: signaling.NewBroker(), STUNURLs: []string{"turn:relay.example:3478"},
		Audit: audit.NewRecorder(store), Presence: presence.NewService(store),
		PublicBaseURL: "https://vpn.example.com",
	})
	if err == nil {
		t.Fatal("Controller accepted a TURN URL")
	}
}

func TestTemporaryServiceIsRandomAndBoundToAuthenticatedHostAndUser(t *testing.T) {
	server, secret := newTestServer(t)
	createdResponse := doBrowserJSON(t, server, secret, http.MethodPost, "/api/v1/browser/temporary-services", catalog.CreateTemporaryRequest{
		AgentID: "ERITFBUWFxgZGhscHR4fIA", DisplayName: "Temporary HTTPS", Kind: "https",
	})
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create temporary service: %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created catalog.Service
	decodeResponse(t, createdResponse, &created)
	if !created.Temporary || len(created.Slug) != 30 || created.PolicyRef != "temporary" {
		t.Fatalf("unexpected temporary service: %#v", created)
	}

	current := doBrowserJSONAt(t, server, secret, "owner", created.Slug+".vpn.example.com", http.MethodGet, "/api/v1/browser/temporary-services/current", nil)
	if current.Code != http.StatusOK {
		t.Fatalf("resolve temporary service: %d %s", current.Code, current.Body.String())
	}
	otherUser := doBrowserJSONAt(t, server, secret, "other", created.Slug+".vpn.example.com", http.MethodGet, "/api/v1/browser/temporary-services/current", nil)
	if otherUser.Code != http.StatusNotFound {
		t.Fatalf("another user resolved temporary service: %d", otherUser.Code)
	}
	portal := doBrowserJSONAt(t, server, secret, "owner", "vpn.example.com", http.MethodGet, "/api/v1/browser/temporary-services/current", nil)
	if portal.Code != http.StatusNotFound {
		t.Fatalf("portal host resolved a temporary service: %d", portal.Code)
	}
}

func TestTemporarySlugRejectsSiblingAndNestedHosts(t *testing.T) {
	for _, host := range []string{"tmp-token.evil.example", "nested.tmp-token.vpn.example.com", "vpn.example.com"} {
		if _, ok := temporarySlug(host, "vpn.example.com"); ok {
			t.Fatalf("accepted invalid temporary host %q", host)
		}
	}
	if slug, ok := temporarySlug("tmp-token.vpn.example.com", "vpn.example.com"); !ok || slug != "tmp-token" {
		t.Fatal("valid temporary host was rejected")
	}
}

func newTestServer(t *testing.T) (*Server, []byte) {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeControlStore{
		publicKey: publicKey,
		service:   catalog.Service{ID: "AQIDBAUGBwgJCgsMDQ4PEA", AgentID: "ERITFBUWFxgZGhscHR4fIA", Slug: "openwrt", DisplayName: "OpenWrt", Kind: "https", PolicyRef: "openwrt-luci", Enabled: true},
	}
	secret := []byte("0123456789abcdef0123456789abcdef")
	browserAuth, err := httpapi.NewBrowserAuth(secret)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Dependencies{
		BrowserAuth: browserAuth, Bindings: binding.NewService(store), Catalog: store,
		CatalogAdmin: catalog.NewManager(store), Sessions: session.NewService(store),
		Challenges: identity.NewChallengeIssuer(store), AgentAuth: identity.NewAgentAuthenticator(store),
		Signals: signaling.NewBroker(), STUNURLs: []string{"stun:stun.example.net:3478"},
		Audit: audit.NewRecorder(store), Presence: presence.NewService(store),
		PublicBaseURL: "https://vpn.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	return server, secret
}

func pollAgentOffer(t *testing.T, server http.Handler, store *fakeControlStore, privateKey ed25519.PrivateKey) signaling.Offer {
	t.Helper()
	nonce := issueAgentChallenge(t, server, store.service.AgentID)
	body := []byte("{}")
	request := signedAgentRequest(t, privateKey, store.service.AgentID, nonce, http.MethodPost, "/api/v1/agent/offers/next", body)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("poll Agent offer: %d %s", response.Code, response.Body.String())
	}
	var offer signaling.Offer
	decodeResponse(t, response, &offer)
	return offer
}

func finishAgentConnection(t *testing.T, server http.Handler, store *fakeControlStore, privateKey ed25519.PrivateKey, id string, result signaling.Result) {
	t.Helper()
	nonce := issueAgentChallenge(t, server, store.service.AgentID)
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/agent/connections/" + id
	request := signedAgentRequest(t, privateKey, store.service.AgentID, nonce, http.MethodPut, path, body)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("finish Agent connection: %d %s", response.Code, response.Body.String())
	}
}

func issueAgentChallenge(t *testing.T, server http.Handler, agentID string) string {
	t.Helper()
	response := doJSON(t, server, http.MethodPost, "/api/v1/agent/challenges", challengeRequest{AgentID: agentID})
	if response.Code != http.StatusCreated {
		t.Fatalf("issue challenge: %d %s", response.Code, response.Body.String())
	}
	var challenge struct {
		Nonce string `json:"nonce"`
	}
	decodeResponse(t, response, &challenge)
	return challenge.Nonce
}

func signedAgentRequest(t *testing.T, privateKey ed25519.PrivateKey, agentID, nonce, method, path string, body []byte) *http.Request {
	t.Helper()
	now := time.Now().UTC()
	signature := ed25519.Sign(privateKey, identity.CanonicalMessage(agentID, nonce, now, method, path, body))
	request := httptest.NewRequest(method, "https://vpn.example.com"+path, bytes.NewReader(body))
	request.Header.Set("X-WebVPN-Agent-ID", agentID)
	request.Header.Set("X-WebVPN-Nonce", nonce)
	request.Header.Set("X-WebVPN-Issued-At", strconv.FormatInt(now.Unix(), 10))
	request.Header.Set("X-WebVPN-Signature", base64.RawURLEncoding.EncodeToString(signature))
	return request
}

func doBrowserJSON(t *testing.T, handler http.Handler, secret []byte, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return doBrowserJSONAt(t, handler, secret, "owner", "vpn.example.com", method, path, body)
}

func doBrowserJSONAt(t *testing.T, handler http.Handler, secret []byte, username, host, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	request := newJSONRequestAt(t, host, method, path, body)
	request.RemoteAddr = "127.0.0.1:32100"
	request.Header.Set(httpapi.InternalAuthHeader, string(secret))
	request.Header.Set(httpapi.UserHeader, username)
	request.Header.Set(httpapi.SessionHeader, "authelia-session-"+username)
	request.Header.Set("Origin", "https://"+host)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func doJSON(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	request := newJSONRequest(t, method, path, body)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func newJSONRequest(t *testing.T, method, path string, body any) *http.Request {
	return newJSONRequestAt(t, "vpn.example.com", method, path, body)
}

func newJSONRequestAt(t *testing.T, host, method, path string, body any) *http.Request {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	return httptest.NewRequest(method, "https://"+host+path, bytes.NewReader(encoded))
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func testDescription(kind string) signal.Description {
	return signal.Description{Type: kind, SDP: "v=0\r\na=candidate:1 1 udp 1 192.0.2.1 5000 typ host\r\n"}
}
