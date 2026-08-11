package controller

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/audit"
	"github.com/RenAhsAcme/SCWebVPN/internal/binding"
	"github.com/RenAhsAcme/SCWebVPN/internal/catalog"
	"github.com/RenAhsAcme/SCWebVPN/internal/config"
	"github.com/RenAhsAcme/SCWebVPN/internal/httpapi"
	"github.com/RenAhsAcme/SCWebVPN/internal/identity"
	"github.com/RenAhsAcme/SCWebVPN/internal/presence"
	"github.com/RenAhsAcme/SCWebVPN/internal/session"
	"github.com/RenAhsAcme/SCWebVPN/internal/signal"
	"github.com/RenAhsAcme/SCWebVPN/internal/signaling"
)

const (
	maxJSONBody = 1 << 20
	longPoll    = 25 * time.Second
)

type Dependencies struct {
	BrowserAuth   *httpapi.BrowserAuth
	Bindings      *binding.Service
	Catalog       catalog.Store
	CatalogAdmin  *catalog.Manager
	Sessions      *session.Service
	Challenges    *identity.ChallengeIssuer
	AgentAuth     *identity.AgentAuthenticator
	Signals       *signaling.Broker
	STUNURLs      []string
	Audit         *audit.Recorder
	Presence      *presence.Service
	PublicBaseURL string
}

type Server struct {
	dependencies Dependencies
	handler      http.Handler
	portalHost   string
}

func New(dependencies Dependencies) (*Server, error) {
	if dependencies.BrowserAuth == nil || dependencies.Bindings == nil || dependencies.Catalog == nil || dependencies.CatalogAdmin == nil ||
		dependencies.Sessions == nil || dependencies.Challenges == nil || dependencies.AgentAuth == nil || dependencies.Signals == nil ||
		dependencies.Audit == nil || dependencies.Presence == nil {
		return nil, errors.New("all Controller dependencies are required")
	}
	if err := config.ValidateSTUNURLs(dependencies.STUNURLs); err != nil {
		return nil, err
	}
	base, err := url.Parse(dependencies.PublicBaseURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.Path != "" && base.Path != "/" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("public base URL must be an HTTPS origin")
	}
	dependencies.STUNURLs = append([]string(nil), dependencies.STUNURLs...)
	server := &Server{dependencies: dependencies, portalHost: strings.ToLower(base.Hostname())}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.Handle("GET /api/v1/browser/config", dependencies.BrowserAuth.Wrap(http.HandlerFunc(server.browserConfig)))
	mux.Handle("GET /api/v1/browser/services", dependencies.BrowserAuth.Wrap(http.HandlerFunc(server.listServices)))
	mux.Handle("POST /api/v1/browser/services", dependencies.BrowserAuth.Wrap(http.HandlerFunc(server.createService)))
	mux.Handle("POST /api/v1/browser/temporary-services", dependencies.BrowserAuth.Wrap(http.HandlerFunc(server.createTemporaryService)))
	mux.Handle("GET /api/v1/browser/temporary-services/current", dependencies.BrowserAuth.Wrap(http.HandlerFunc(server.currentTemporaryService)))
	mux.Handle("POST /api/v1/browser/binding-codes", dependencies.BrowserAuth.Wrap(http.HandlerFunc(server.issueBindingCode)))
	mux.Handle("GET /api/v1/browser/agents/{id}/status", dependencies.BrowserAuth.Wrap(http.HandlerFunc(server.agentStatus)))
	mux.Handle("POST /api/v1/browser/connections", dependencies.BrowserAuth.Wrap(http.HandlerFunc(server.createConnection)))
	mux.Handle("GET /api/v1/browser/connections/{id}", dependencies.BrowserAuth.Wrap(http.HandlerFunc(server.connectionStatus)))
	mux.Handle("POST /api/v1/browser/logout", dependencies.BrowserAuth.Wrap(http.HandlerFunc(server.logout)))
	mux.HandleFunc("POST /api/v1/agents/bind", server.bindAgent)
	mux.HandleFunc("POST /api/v1/agent/challenges", server.issueChallenge)
	mux.HandleFunc("POST /api/v1/agent/offers/next", server.nextOffer)
	mux.HandleFunc("PUT /api/v1/agent/connections/{id}", server.finishConnection)
	server.handler = httpapi.SecurityHeaders(mux)
	return server, nil
}

func (server *Server) browserConfig(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"stun_urls": server.dependencies.STUNURLs})
}

func (server *Server) createService(writer http.ResponseWriter, request *http.Request) {
	var input catalog.CreateRequest
	if !decodeJSON(writer, request.Body, &input) {
		return
	}
	service, err := server.dependencies.CatalogAdmin.Create(request.Context(), input)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "could not create service")
		return
	}
	identityContext, _ := httpapi.BrowserFromContext(request.Context())
	server.audit(request.Context(), audit.Event{ActorDigest: &identityContext.UserDigest, AgentID: service.AgentID, ServiceID: service.ID, Type: "service_created", ResultCode: "ok"})
	writeJSON(writer, http.StatusCreated, service)
}

func (server *Server) createTemporaryService(writer http.ResponseWriter, request *http.Request) {
	identityContext, _ := httpapi.BrowserFromContext(request.Context())
	var input catalog.CreateTemporaryRequest
	if !decodeJSON(writer, request.Body, &input) {
		return
	}
	service, err := server.dependencies.CatalogAdmin.CreateTemporary(request.Context(), identityContext.SessionDigest, input)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "could not create temporary service")
		return
	}
	server.audit(request.Context(), audit.Event{ActorDigest: &identityContext.UserDigest, AgentID: service.AgentID, ServiceID: service.ID, Type: "temporary_service_created", ResultCode: "ok"})
	writeJSON(writer, http.StatusCreated, service)
}

func (server *Server) currentTemporaryService(writer http.ResponseWriter, request *http.Request) {
	identityContext, _ := httpapi.BrowserFromContext(request.Context())
	slug, ok := temporarySlug(request.Host, server.portalHost)
	if !ok {
		writeError(writer, http.StatusNotFound, "temporary service not found or expired")
		return
	}
	service, err := server.dependencies.Catalog.TemporaryServiceBySlug(request.Context(), slug, identityContext.SessionDigest, time.Now().UTC())
	if err != nil {
		writeError(writer, http.StatusNotFound, "temporary service not found or expired")
		return
	}
	writeJSON(writer, http.StatusOK, service)
}

func temporarySlug(requestHost, portalHost string) (string, bool) {
	host := strings.ToLower(requestHost)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	suffix := "." + portalHost
	if !strings.HasSuffix(host, suffix) {
		return "", false
	}
	slug := strings.TrimSuffix(host, suffix)
	return slug, strings.HasPrefix(slug, "tmp-") && !strings.Contains(slug, ".")
}

func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	server.handler.ServeHTTP(writer, request)
}

func (server *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, "ok\n")
}

func (server *Server) listServices(writer http.ResponseWriter, request *http.Request) {
	services, err := server.dependencies.Catalog.ListServices(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "service directory unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"services": services})
}

func (server *Server) issueBindingCode(writer http.ResponseWriter, request *http.Request) {
	code, expiresAt, err := server.dependencies.Bindings.Issue(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "could not create binding code")
		return
	}
	identityContext, _ := httpapi.BrowserFromContext(request.Context())
	server.audit(request.Context(), audit.Event{ActorDigest: &identityContext.UserDigest, Type: "binding_code_issued", ResultCode: "ok"})
	writeJSON(writer, http.StatusCreated, map[string]any{"code": code, "expires_at": expiresAt})
}

func (server *Server) agentStatus(writer http.ResponseWriter, request *http.Request) {
	online, lastSeen, err := server.dependencies.Presence.Status(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusNotFound, "Agent not found")
		return
	}
	var seen any
	if !lastSeen.IsZero() {
		seen = lastSeen
	}
	writeJSON(writer, http.StatusOK, map[string]any{"online": online, "last_seen_at": seen})
}

type bindAgentRequest struct {
	Code        string `json:"code"`
	DisplayName string `json:"display_name"`
	PublicKey   string `json:"public_key"`
}

func (server *Server) bindAgent(writer http.ResponseWriter, request *http.Request) {
	var input bindAgentRequest
	if !decodeJSON(writer, request.Body, &input) {
		return
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(input.PublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		writeError(writer, http.StatusBadRequest, "invalid binding request")
		return
	}
	agent, err := server.dependencies.Bindings.Bind(request.Context(), input.Code, input.DisplayName, ed25519.PublicKey(publicKey))
	if err != nil {
		if errors.Is(err, binding.ErrInvalidCode) {
			writeError(writer, http.StatusUnauthorized, "invalid binding request")
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "could not bind Agent")
		return
	}
	server.audit(request.Context(), audit.Event{AgentID: agent.ID, Type: "agent_bound", ResultCode: "ok"})
	writeJSON(writer, http.StatusCreated, map[string]string{"agent_id": agent.ID})
}

type challengeRequest struct {
	AgentID string `json:"agent_id"`
}

func (server *Server) issueChallenge(writer http.ResponseWriter, request *http.Request) {
	var input challengeRequest
	if !decodeJSON(writer, request.Body, &input) {
		return
	}
	nonce, expiresAt, err := server.dependencies.Challenges.Issue(request.Context(), input.AgentID)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "Agent authentication failed")
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"nonce": nonce, "expires_at": expiresAt})
}

type connectionRequest struct {
	ServiceID        string             `json:"service_id"`
	BrowserSessionID string             `json:"browser_session_id,omitempty"`
	Capability       string             `json:"capability,omitempty"`
	Offer            signal.Description `json:"offer"`
	RestartOf        string             `json:"restart_of,omitempty"`
}

func (server *Server) createConnection(writer http.ResponseWriter, request *http.Request) {
	identityContext, ok := httpapi.BrowserFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	var input connectionRequest
	if !decodeJSON(writer, request.Body, &input) {
		return
	}
	var (
		record     session.Record
		capability string
		created    bool
	)
	if input.RestartOf == "" {
		if input.BrowserSessionID != "" || input.Capability != "" {
			writeError(writer, http.StatusBadRequest, "new connection must not include restart authorization")
			return
		}
		result, err := server.dependencies.Sessions.Create(request.Context(), identityContext.SessionDigest, input.ServiceID)
		if err != nil {
			writeError(writer, http.StatusNotFound, "service unavailable")
			return
		}
		record, capability, created = result.Record, result.Token, true
	} else {
		if input.BrowserSessionID == "" || session.ValidateToken(input.Capability) != nil {
			writeError(writer, http.StatusBadRequest, "invalid ICE restart authorization")
			return
		}
		var err error
		record, err = server.dependencies.Sessions.Lookup(request.Context(), input.BrowserSessionID, identityContext.SessionDigest, input.Capability)
		if err != nil || record.ServiceID != input.ServiceID {
			writeError(writer, http.StatusNotFound, "connection session unavailable")
			return
		}
		capability = input.Capability
	}
	signalID, err := server.dependencies.Signals.Create(signaling.CreateRequest{
		AgentID: record.AgentID, BrowserSessionID: record.ID, ServiceID: record.ServiceID, UserDigest: identityContext.SessionDigest,
		CapabilityDigest: record.TokenDigest, Offer: input.Offer, RestartOf: input.RestartOf, Temporary: record.Temporary,
	})
	if err != nil {
		if created {
			_ = server.dependencies.Sessions.Revoke(request.Context(), record.ID, identityContext.SessionDigest)
		}
		writeError(writer, http.StatusBadRequest, "invalid connection offer")
		return
	}
	eventType := "connection_created"
	if input.RestartOf != "" {
		eventType = "ice_restart_created"
	}
	server.audit(request.Context(), audit.Event{ActorDigest: &identityContext.UserDigest, AgentID: record.AgentID, ServiceID: record.ServiceID, Type: eventType, ResultCode: "ok"})
	writeJSON(writer, http.StatusCreated, map[string]any{
		"id": signalID, "browser_session_id": record.ID, "capability": capability,
		"expires_at": record.ExpiresAt, "absolute_expires_at": record.AbsoluteExpiresAt,
	})
}

func (server *Server) connectionStatus(writer http.ResponseWriter, request *http.Request) {
	identityContext, ok := httpapi.BrowserFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	status, err := server.dependencies.Signals.Status(request.PathValue("id"), identityContext.SessionDigest)
	if err != nil {
		writeError(writer, http.StatusNotFound, "connection not found or expired")
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (server *Server) logout(writer http.ResponseWriter, request *http.Request) {
	identityContext, ok := httpapi.BrowserFromContext(request.Context())
	if !ok {
		writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	if err := server.dependencies.Sessions.RevokeAll(request.Context(), identityContext.SessionDigest); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "could not revoke sessions")
		return
	}
	if err := server.dependencies.Catalog.RevokeTemporaryServices(request.Context(), identityContext.SessionDigest, time.Now().UTC()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "could not revoke temporary services")
		return
	}
	server.audit(request.Context(), audit.Event{ActorDigest: &identityContext.UserDigest, Type: "browser_logout", ResultCode: "ok"})
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) nextOffer(writer http.ResponseWriter, request *http.Request) {
	body, signed, err := server.dependencies.AgentAuth.Verify(request)
	if err != nil || !decodeJSONBytes(body, &struct{}{}) {
		writeError(writer, http.StatusUnauthorized, "Agent authentication failed")
		return
	}
	if err := server.dependencies.Presence.Touch(request.Context(), signed.AgentID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "Agent state unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), longPoll)
	defer cancel()
	offer, err := server.dependencies.Signals.Next(ctx, signed.AgentID)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "signaling unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, offer)
}

func (server *Server) finishConnection(writer http.ResponseWriter, request *http.Request) {
	body, signed, err := server.dependencies.AgentAuth.Verify(request)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "Agent authentication failed")
		return
	}
	var result signaling.Result
	if !decodeJSONBytes(body, &result) {
		writeError(writer, http.StatusBadRequest, "invalid signed result")
		return
	}
	if err := server.dependencies.Presence.Touch(request.Context(), signed.AgentID); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "Agent state unavailable")
		return
	}
	if err := server.dependencies.Signals.Finish(signed.AgentID, request.PathValue("id"), result); err != nil {
		writeError(writer, http.StatusBadRequest, "could not accept signaling result")
		return
	}
	resultCode := "answered"
	if result.FailureCode != "" {
		resultCode = result.FailureCode
	}
	server.audit(request.Context(), audit.Event{AgentID: signed.AgentID, Type: "connection_finished", ResultCode: resultCode})
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) audit(ctx context.Context, event audit.Event) {
	if err := server.dependencies.Audit.Record(ctx, event); err != nil {
		slog.Warn("audit metadata write failed", "error", publicError(err))
	}
}

func publicError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}

func decodeJSON(writer http.ResponseWriter, reader io.Reader, target any) bool {
	limited := io.LimitReader(reader, maxJSONBody+1)
	body, err := io.ReadAll(limited)
	if err != nil || len(body) > maxJSONBody || !decodeJSONBytes(body, target) {
		writeError(writer, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func decodeJSONBytes(body []byte, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	return errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}
