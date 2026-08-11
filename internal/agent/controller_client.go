package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/identity"
	"github.com/RenAhsAcme/SCWebVPN/internal/signaling"
)

const maxControllerResponse = 1 << 20

type ControllerClient struct {
	baseURL    *url.URL
	agentID    string
	privateKey ed25519.PrivateKey
	http       *http.Client
	now        func() time.Time
}

type controllerHTTPError struct {
	operation string
	status    int
}

type controllerStageError struct {
	operation string
	stage     string
	cause     error
}

func (err *controllerHTTPError) Error() string {
	return fmt.Sprintf("Controller %s returned HTTP %d", err.operation, err.status)
}

func (err *controllerHTTPError) PublicCode() string {
	return fmt.Sprintf("controller_%s_http_%d", err.operation, err.status)
}

func (err *controllerStageError) Error() string {
	return fmt.Sprintf("Controller %s %s failed", err.operation, err.stage)
}

func (err *controllerStageError) Unwrap() error {
	return err.cause
}

func (err *controllerStageError) PublicCode() string {
	return fmt.Sprintf("controller_%s_%s", err.operation, err.stage)
}

func BindAgent(ctx context.Context, rawBaseURL, code, displayName string, publicKey ed25519.PublicKey, client *http.Client) (string, error) {
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || baseURL.Path != "" && baseURL.Path != "/" {
		return "", errors.New("Controller URL must be an HTTPS origin")
	}
	decodedCode, err := base64.RawURLEncoding.DecodeString(code)
	if err != nil || len(decodedCode) != 32 || len(publicKey) != ed25519.PublicKeySize || displayName == "" || len(displayName) > 80 || strings.ContainsAny(displayName, "\r\n\x00") {
		return "", errors.New("invalid Agent binding input")
	}
	if client == nil {
		client = defaultControllerHTTPClient()
	}
	body, _ := json.Marshal(map[string]string{
		"code": code, "display_name": displayName,
		"public_key": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/api/v1/agents/bind"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "scwebvpn-agent/1")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("Controller binding returned HTTP %d", response.StatusCode)
	}
	var result struct {
		AgentID string `json:"agent_id"`
	}
	if err := decodeControllerJSON(response.Body, &result); err != nil || !validOpaqueID(result.AgentID) {
		return "", errors.New("Controller returned an invalid Agent identity")
	}
	return result.AgentID, nil
}

func NewControllerClient(rawBaseURL, agentID string, privateKey ed25519.PrivateKey, client *http.Client) (*ControllerClient, error) {
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || baseURL.Path != "" && baseURL.Path != "/" {
		return nil, errors.New("Controller URL must be an HTTPS origin")
	}
	if !validOpaqueID(agentID) || len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("valid Agent ID and Ed25519 private key are required")
	}
	if client == nil {
		client = defaultControllerHTTPClient()
	}
	return &ControllerClient{
		baseURL: baseURL, agentID: agentID, privateKey: append(ed25519.PrivateKey(nil), privateKey...),
		http: client, now: time.Now,
	}, nil
}

func defaultControllerHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2: true, IdleConnTimeout: 60 * time.Second,
		},
		Timeout: 40 * time.Second,
	}
}

func (client *ControllerClient) NextOffer(ctx context.Context) (signaling.Offer, bool, error) {
	response, err := client.signedJSON(ctx, "poll", http.MethodPost, "/api/v1/agent/offers/next", struct{}{})
	if err != nil {
		return signaling.Offer{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return signaling.Offer{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return signaling.Offer{}, false, &controllerHTTPError{operation: "poll", status: response.StatusCode}
	}
	var offer signaling.Offer
	if err := decodeControllerJSON(response.Body, &offer); err != nil {
		return signaling.Offer{}, false, &controllerStageError{operation: "poll", stage: "decode", cause: err}
	}
	return offer, true, nil
}

func (client *ControllerClient) Finish(ctx context.Context, signalID string, result signaling.Result) error {
	if !validOpaqueID(signalID) {
		return errors.New("invalid signaling ID")
	}
	response, err := client.signedJSON(ctx, "result", http.MethodPut, "/api/v1/agent/connections/"+signalID, result)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return &controllerHTTPError{operation: "result", status: response.StatusCode}
	}
	return nil
}

func (client *ControllerClient) signedJSON(ctx context.Context, operation, method, path string, value any) (*http.Response, error) {
	nonce, err := client.challenge(ctx)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, &controllerStageError{operation: operation, stage: "encode", cause: err}
	}
	request, err := http.NewRequestWithContext(ctx, method, client.resolve(path), bytes.NewReader(body))
	if err != nil {
		return nil, &controllerStageError{operation: operation, stage: "request", cause: err}
	}
	issuedAt := client.now().UTC()
	signature := ed25519.Sign(client.privateKey, identity.CanonicalMessage(client.agentID, nonce, issuedAt, method, request.URL.RequestURI(), body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "scwebvpn-agent/1")
	request.Header.Set("X-WebVPN-Agent-ID", client.agentID)
	request.Header.Set("X-WebVPN-Nonce", nonce)
	request.Header.Set("X-WebVPN-Issued-At", strconv.FormatInt(issuedAt.Unix(), 10))
	request.Header.Set("X-WebVPN-Signature", base64.RawURLEncoding.EncodeToString(signature))
	response, err := client.http.Do(request)
	if err != nil {
		return nil, &controllerStageError{operation: operation, stage: "transport", cause: err}
	}
	return response, nil
}

func (client *ControllerClient) challenge(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]string{"agent_id": client.agentID})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.resolve("/api/v1/agent/challenges"), bytes.NewReader(body))
	if err != nil {
		return "", &controllerStageError{operation: "challenge", stage: "request", cause: err}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "scwebvpn-agent/1")
	response, err := client.http.Do(request)
	if err != nil {
		return "", &controllerStageError{operation: "challenge", stage: "transport", cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return "", &controllerHTTPError{operation: "challenge", status: response.StatusCode}
	}
	var result struct {
		Nonce     string    `json:"nonce"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := decodeControllerJSON(response.Body, &result); err != nil {
		return "", &controllerStageError{operation: "challenge", stage: "decode", cause: err}
	}
	decoded, err := base64.RawURLEncoding.DecodeString(result.Nonce)
	if err != nil || len(decoded) != 32 {
		return "", &controllerStageError{operation: "challenge", stage: "value", cause: errors.New("Controller returned an invalid challenge")}
	}
	return result.Nonce, nil
}

func (client *ControllerClient) resolve(path string) string {
	return strings.TrimSuffix(client.baseURL.String(), "/") + path
}

func decodeControllerJSON(reader io.Reader, target any) error {
	limited := io.LimitReader(reader, maxControllerResponse+1)
	body, err := io.ReadAll(limited)
	if err != nil || len(body) > maxControllerResponse {
		return errors.New("Controller response exceeded its limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("Controller returned invalid JSON")
	}
	if !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return errors.New("Controller returned multiple JSON values")
	}
	return nil
}
