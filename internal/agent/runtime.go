package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/RenAhsAcme/SCWebVPN/internal/session"
	"github.com/RenAhsAcme/SCWebVPN/internal/signal"
	"github.com/RenAhsAcme/SCWebVPN/internal/signaling"
	"github.com/RenAhsAcme/SCWebVPN/internal/wisp"
)

const (
	authPrefix          = "webvpn-auth-v1:"
	authenticatedReply  = "webvpn-auth-ok-v1"
	authenticatedReady  = "webvpn-auth-ready-v1"
	disconnectedTimeout = 45 * time.Second
	maxICERestarts      = 2
)

var dataLabels = map[string]struct{}{
	"wisp-interactive-v2": {},
	"wisp-bulk-v2":        {},
	"guacamole-v1":        {},
}

var controlIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

type Runtime struct {
	config     Config
	policy     *Policy
	controller *ControllerClient
	api        *webrtc.API

	mu    sync.Mutex
	peers map[string]*peer
}

type peer struct {
	runtime          *Runtime
	connection       *webrtc.PeerConnection
	serviceID        string
	temporary        bool
	temporaryTarget  *compiledTemporaryTarget
	capabilityDigest [32]byte

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once

	mu              sync.Mutex
	aliases         map[string]struct{}
	labels          map[string]struct{}
	restartCount    int
	disconnectTimer *time.Timer
	controlMu       sync.Mutex
	diagnosticSlots chan struct{}
}

func NewRuntime(config Config, privateKey ed25519.PrivateKey) (*Runtime, error) {
	policy, err := CompilePolicy(config)
	if err != nil {
		return nil, err
	}
	controller, err := NewControllerClient(config.ControllerURL, config.AgentID, privateKey, nil)
	if err != nil {
		return nil, err
	}
	var settings webrtc.SettingEngine
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6})
	return &Runtime{
		config: config, policy: policy, controller: controller,
		api: webrtc.NewAPI(webrtc.WithSettingEngine(settings)), peers: make(map[string]*peer),
	}, nil
}

func (runtime *Runtime) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			runtime.Close()
			return err
		}
		offer, available, err := runtime.controller.NextOffer(ctx)
		if err != nil {
			slog.Warn("Controller poll failed", "error", publicError(err))
			if !waitContext(ctx, backoff) {
				runtime.Close()
				return ctx.Err()
			}
			if backoff < 15*time.Second {
				backoff *= 2
				if backoff > 15*time.Second {
					backoff = 15 * time.Second
				}
			}
			continue
		}
		backoff = time.Second
		if !available {
			continue
		}
		answer, failureCode := runtime.acceptOffer(ctx, offer)
		result := signaling.Result{FailureCode: failureCode}
		if answer != nil {
			result.Answer, result.FailureCode = answer, ""
		}
		if err := runtime.controller.Finish(ctx, offer.ID, result); err != nil {
			slog.Warn("Controller result delivery failed", "error", publicError(err))
			runtime.mu.Lock()
			current := runtime.peers[offer.ID]
			runtime.mu.Unlock()
			if current != nil {
				current.close()
			}
		}
	}
}

func (runtime *Runtime) Close() {
	runtime.mu.Lock()
	unique := make(map[*peer]struct{})
	for _, current := range runtime.peers {
		unique[current] = struct{}{}
	}
	runtime.mu.Unlock()
	for current := range unique {
		current.close()
	}
}

func (runtime *Runtime) acceptOffer(ctx context.Context, offer signaling.Offer) (*signal.Description, string) {
	if err := signal.ValidateDescription(offer.Offer, "offer"); err != nil {
		return nil, "invalid_offer"
	}
	capabilityDigest, err := decodeCapabilityDigest(offer.CapabilityDigest)
	if err != nil {
		return nil, "invalid_capability"
	}
	if offer.Temporary {
		if !runtime.policy.AcceptsTemporary(offer.ServiceID) {
			return nil, "service_blocked"
		}
	} else if _, _, err := runtime.policy.VirtualAuthority(offer.ServiceID); err != nil {
		return nil, "service_blocked"
	}
	var current *peer
	if offer.RestartOf == "" {
		current, err = runtime.newPeer(ctx, offer.ServiceID, capabilityDigest, offer.Temporary)
		if err != nil {
			return nil, "peer_setup_failed"
		}
	} else {
		runtime.mu.Lock()
		current = runtime.peers[offer.RestartOf]
		runtime.mu.Unlock()
		if current == nil || current.serviceID != offer.ServiceID || current.capabilityDigest != capabilityDigest || current.temporary != offer.Temporary || !current.beginRestart() {
			return nil, "restart_rejected"
		}
	}
	answer, err := current.applyOffer(ctx, offer.Offer)
	if err != nil {
		if offer.RestartOf == "" {
			current.close()
		}
		return nil, "negotiation_failed"
	}
	current.addAlias(offer.ID)
	return &answer, ""
}

func (runtime *Runtime) newPeer(parent context.Context, serviceID string, capabilityDigest [32]byte, temporary bool) (*peer, error) {
	iceServers := []webrtc.ICEServer{{URLs: append([]string(nil), runtime.config.STUNURLs...)}}
	connection, err := runtime.api.NewPeerConnection(webrtc.Configuration{
		ICEServers: iceServers, ICETransportPolicy: webrtc.ICETransportPolicyAll,
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	current := &peer{
		runtime: runtime, connection: connection, serviceID: serviceID, capabilityDigest: capabilityDigest, temporary: temporary,
		ctx: ctx, cancel: cancel, aliases: make(map[string]struct{}), labels: make(map[string]struct{}),
		diagnosticSlots: make(chan struct{}, 2),
	}
	connection.OnDataChannel(current.onDataChannel)
	connection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil && !allowedCandidateType(candidate.Typ) {
			current.close()
		}
	})
	connection.OnConnectionStateChange(current.onConnectionState)
	return current, nil
}

func (current *peer) applyOffer(ctx context.Context, description signal.Description) (signal.Description, error) {
	if err := current.connection.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: description.SDP}); err != nil {
		return signal.Description{}, err
	}
	answer, err := current.connection.CreateAnswer(nil)
	if err != nil {
		return signal.Description{}, err
	}
	gathered := webrtc.GatheringCompletePromise(current.connection)
	if err := current.connection.SetLocalDescription(answer); err != nil {
		return signal.Description{}, err
	}
	select {
	case <-ctx.Done():
		return signal.Description{}, ctx.Err()
	case <-current.ctx.Done():
		return signal.Description{}, current.ctx.Err()
	case <-gathered:
	}
	local := current.connection.LocalDescription()
	if local == nil {
		return signal.Description{}, errors.New("Pion returned no local description")
	}
	result := signal.Description{Type: "answer", SDP: local.SDP}
	if err := signal.ValidateDescription(result, "answer"); err != nil {
		return signal.Description{}, err
	}
	return result, nil
}

func (current *peer) beginRestart() bool {
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.restartCount >= maxICERestarts {
		return false
	}
	current.restartCount++
	return true
}

func (current *peer) addAlias(id string) {
	current.mu.Lock()
	current.aliases[id] = struct{}{}
	current.mu.Unlock()
	current.runtime.mu.Lock()
	current.runtime.peers[id] = current
	current.runtime.mu.Unlock()
}

func (current *peer) onDataChannel(channel *webrtc.DataChannel) {
	label := channel.Label()
	if !current.validLabel(label) || !channel.Ordered() || channel.MaxRetransmits() != nil || channel.MaxPacketLifeTime() != nil {
		_ = channel.Close()
		return
	}
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		if !message.IsString || !verifyCapability(message.Data, current.capabilityDigest) {
			_ = channel.Close()
			return
		}
		if !current.reserveLabel(label) {
			_ = channel.Close()
			return
		}
		if err := channel.SendText(authenticatedReply); err != nil {
			_ = channel.Close()
			return
		}
		if label == "webvpn-control-v1" {
			channel.OnMessage(func(control webrtc.DataChannelMessage) { current.handleControl(channel, control) })
			return
		}
		channel.OnMessage(func(ready webrtc.DataChannelMessage) {
			if !ready.IsString || string(ready.Data) != authenticatedReady {
				_ = channel.Close()
				return
			}
			dial := current.serviceDial()
			server := wisp.NewServer(current.ctx, channel, dial)
			if err := server.Start(); err != nil {
				_ = channel.Close()
			}
		})
	})
}

func (current *peer) validLabel(label string) bool {
	if label != "webvpn-control-v1" {
		if _, ok := dataLabels[label]; !ok {
			return false
		}
		if label == "guacamole-v1" {
			if current.temporary {
				return false
			}
			target, ok := current.runtime.policy.Service(current.serviceID)
			if !ok || target.Kind != "guacamole" {
				return false
			}
		}
	}
	return true
}

func (current *peer) reserveLabel(label string) bool {
	current.mu.Lock()
	defer current.mu.Unlock()
	if _, exists := current.labels[label]; exists || len(current.labels) >= 4 {
		return false
	}
	current.labels[label] = struct{}{}
	return true
}

func (current *peer) serviceDial() wisp.DialFunc {
	if current.temporary {
		return func(ctx context.Context, hostname string, port uint16) (net.Conn, error) {
			current.mu.Lock()
			target := current.temporaryTarget
			current.mu.Unlock()
			connection, err := current.runtime.policy.DialTemporary(ctx, target, hostname, port)
			if errors.Is(err, ErrBlocked) {
				return nil, wisp.Blocked(err)
			}
			return connection, err
		}
	}
	expectedHost, expectedPort, _ := current.runtime.policy.VirtualAuthority(current.serviceID)
	return func(ctx context.Context, hostname string, port uint16) (net.Conn, error) {
		if !strings.EqualFold(hostname, expectedHost) || port != expectedPort {
			return nil, wisp.Blocked(ErrBlocked)
		}
		connection, err := current.runtime.policy.DialVirtual(ctx, hostname, port)
		if errors.Is(err, ErrBlocked) {
			return nil, wisp.Blocked(err)
		}
		return connection, err
	}
}

func (current *peer) onConnectionState(state webrtc.PeerConnectionState) {
	switch state {
	case webrtc.PeerConnectionStateConnected:
		current.mu.Lock()
		if current.disconnectTimer != nil {
			current.disconnectTimer.Stop()
			current.disconnectTimer = nil
		}
		current.mu.Unlock()
		go current.verifySelectedPair()
	case webrtc.PeerConnectionStateDisconnected:
		current.mu.Lock()
		if current.disconnectTimer == nil {
			current.disconnectTimer = time.AfterFunc(disconnectedTimeout, current.close)
		}
		current.mu.Unlock()
	case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
		current.close()
	}
}

func (current *peer) verifySelectedPair() {
	for attempt := 0; attempt < 20; attempt++ {
		transport := current.connection.SCTP()
		if transport != nil && transport.Transport() != nil && transport.Transport().ICETransport() != nil {
			pair, err := transport.Transport().ICETransport().GetSelectedCandidatePair()
			if err == nil && pair != nil {
				if pair.Local == nil || pair.Remote == nil || !allowedCandidateType(pair.Local.Typ) || !allowedCandidateType(pair.Remote.Typ) {
					current.close()
				}
				return
			}
		}
		if !waitContext(current.ctx, 100*time.Millisecond) {
			return
		}
	}
	current.close()
}

func (current *peer) close() {
	current.closeOnce.Do(func() {
		current.cancel()
		current.mu.Lock()
		if current.disconnectTimer != nil {
			current.disconnectTimer.Stop()
		}
		aliases := make([]string, 0, len(current.aliases))
		for id := range current.aliases {
			aliases = append(aliases, id)
		}
		current.mu.Unlock()
		current.runtime.mu.Lock()
		for _, id := range aliases {
			if current.runtime.peers[id] == current {
				delete(current.runtime.peers, id)
			}
		}
		current.runtime.mu.Unlock()
		_ = current.connection.Close()
	})
}

func verifyCapability(data []byte, expected [32]byte) bool {
	value, ok := strings.CutPrefix(string(data), authPrefix)
	if !ok || session.ValidateToken(value) != nil {
		return false
	}
	digest := session.HashToken(value)
	return subtle.ConstantTimeCompare(digest[:], expected[:]) == 1
}

func decodeCapabilityDigest(value string) ([32]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return [32]byte{}, errors.New("invalid capability digest")
	}
	var digest [32]byte
	copy(digest[:], decoded)
	return digest, nil
}

func (current *peer) handleControl(channel *webrtc.DataChannel, message webrtc.DataChannelMessage) {
	if !message.IsString || len(message.Data) > 512 {
		_ = channel.Close()
		return
	}
	var input struct {
		Type string `json:"type"`
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
		Host string `json:"host,omitempty"`
		Port uint16 `json:"port,omitempty"`
		Kind string `json:"kind,omitempty"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(message.Data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		_ = channel.Close()
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		_ = channel.Close()
		return
	}
	switch input.Type {
	case "ping":
		if input.ID != "" || input.Name != "" || input.Host != "" || input.Port != 0 || input.Kind != "" {
			_ = channel.Close()
			return
		}
		current.sendControl(channel, map[string]any{"type": "pong"})
	case "dns", "diagnostic_ping":
		if !controlIDPattern.MatchString(input.ID) || input.Host != "" || input.Port != 0 || input.Kind != "" || input.Type == "dns" && !validHostname(input.Name) || input.Type == "diagnostic_ping" && input.Name != "" {
			current.sendControl(channel, map[string]any{"type": "diagnostic_result", "id": input.ID, "code": "invalid_request"})
			return
		}
		select {
		case current.diagnosticSlots <- struct{}{}:
			go current.runDiagnostic(channel, input.Type, input.ID, input.Name)
		default:
			current.sendControl(channel, map[string]any{"type": "diagnostic_result", "id": input.ID, "code": "busy"})
		}
	case "temporary_target":
		if !current.temporary || !controlIDPattern.MatchString(input.ID) || input.Name != "" || input.Host == "" || input.Port == 0 || input.Kind != "http" && input.Kind != "https" {
			current.sendControl(channel, map[string]any{"type": "temporary_result", "id": input.ID, "code": "invalid_request"})
			return
		}
		select {
		case current.diagnosticSlots <- struct{}{}:
			go current.prepareTemporary(channel, input.ID, input.Host, input.Port, input.Kind)
		default:
			current.sendControl(channel, map[string]any{"type": "temporary_result", "id": input.ID, "code": "busy"})
		}
	default:
		_ = channel.Close()
	}
}

func (current *peer) prepareTemporary(channel *webrtc.DataChannel, id, host string, port uint16, kind string) {
	defer func() { <-current.diagnosticSlots }()
	current.mu.Lock()
	alreadyConfigured := current.temporaryTarget != nil
	current.mu.Unlock()
	if alreadyConfigured {
		current.sendControl(channel, map[string]any{"type": "temporary_result", "id": id, "code": "invalid_request"})
		return
	}
	target, err := current.runtime.policy.PrepareTemporary(current.ctx, TemporaryTarget{
		ServiceID: current.serviceID, Kind: kind, Host: host, Port: port,
	})
	if err != nil {
		current.sendControl(channel, map[string]any{"type": "temporary_result", "id": id, "code": diagnosticCode(err)})
		return
	}
	current.mu.Lock()
	if current.temporaryTarget != nil {
		current.mu.Unlock()
		current.sendControl(channel, map[string]any{"type": "temporary_result", "id": id, "code": "invalid_request"})
		return
	}
	current.temporaryTarget = target
	current.mu.Unlock()
	current.sendControl(channel, map[string]any{"type": "temporary_result", "id": id, "code": "ok"})
}

func (current *peer) runDiagnostic(channel *webrtc.DataChannel, kind, id, name string) {
	defer func() { <-current.diagnosticSlots }()
	if kind == "dns" {
		var addresses []netip.Addr
		var err error
		if current.temporary {
			addresses, err = current.runtime.policy.ResolveTemporaryName(current.ctx, name)
		} else {
			addresses, err = current.runtime.policy.ResolveName(current.ctx, current.serviceID, name)
		}
		if err != nil {
			current.sendControl(channel, map[string]any{"type": "diagnostic_result", "id": id, "code": diagnosticCode(err)})
			return
		}
		if len(addresses) > 8 {
			addresses = addresses[:8]
		}
		values := make([]string, len(addresses))
		for index, address := range addresses {
			values[index] = address.String()
		}
		current.sendControl(channel, map[string]any{"type": "diagnostic_result", "id": id, "code": "ok", "addresses": values})
		return
	}
	var addresses []netip.Addr
	var err error
	if current.temporary {
		current.mu.Lock()
		if current.temporaryTarget != nil {
			addresses = append(addresses, current.temporaryTarget.addresses...)
		}
		current.mu.Unlock()
	} else {
		target, resolveErr := current.runtime.policy.ResolveTarget(current.ctx, current.serviceID)
		err, addresses = resolveErr, target.Addresses
	}
	if err != nil || len(addresses) == 0 {
		current.sendControl(channel, map[string]any{"type": "diagnostic_result", "id": id, "code": diagnosticCode(err)})
		return
	}
	duration, err := pingTarget(current.ctx, addresses[0])
	if err != nil {
		current.sendControl(channel, map[string]any{"type": "diagnostic_result", "id": id, "code": "unavailable"})
		return
	}
	current.sendControl(channel, map[string]any{"type": "diagnostic_result", "id": id, "code": "ok", "rtt_ms": duration.Milliseconds()})
}

func (current *peer) sendControl(channel *webrtc.DataChannel, value map[string]any) {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > 512 {
		_ = channel.Close()
		return
	}
	current.controlMu.Lock()
	defer current.controlMu.Unlock()
	if err := channel.SendText(string(encoded)); err != nil {
		_ = channel.Close()
	}
}

func allowedCandidateType(candidateType webrtc.ICECandidateType) bool {
	return candidateType == webrtc.ICECandidateTypeHost || candidateType == webrtc.ICECandidateTypeSrflx || candidateType == webrtc.ICECandidateTypePrflx
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func publicError(err error) string {
	if err == nil {
		return ""
	}
	var coded interface{ PublicCode() string }
	if errors.As(err, &coded) {
		return coded.PublicCode()
	}
	return fmt.Sprintf("%T", err)
}
