package wisp

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

type fakeDataChannel struct {
	mu       sync.Mutex
	buffered uint64
	sent     [][]byte
	low      func()
	message  func(webrtc.DataChannelMessage)
	close    func()
	notify   chan struct{}
}

func newFakeDataChannel() *fakeDataChannel {
	return &fakeDataChannel{notify: make(chan struct{}, 32)}
}

func (channel *fakeDataChannel) SetBufferedAmountLowThreshold(uint64) {}
func (channel *fakeDataChannel) OnBufferedAmountLow(callback func())  { channel.low = callback }
func (channel *fakeDataChannel) OnMessage(callback func(webrtc.DataChannelMessage)) {
	channel.message = callback
}
func (channel *fakeDataChannel) OnClose(callback func()) { channel.close = callback }
func (channel *fakeDataChannel) BufferedAmount() uint64 {
	channel.mu.Lock()
	defer channel.mu.Unlock()
	return channel.buffered
}
func (channel *fakeDataChannel) Send(value []byte) error {
	channel.mu.Lock()
	channel.sent = append(channel.sent, append([]byte(nil), value...))
	channel.mu.Unlock()
	select {
	case channel.notify <- struct{}{}:
	default:
	}
	return nil
}
func (channel *fakeDataChannel) receive(value []byte) {
	channel.message(webrtc.DataChannelMessage{Data: value})
}
func (channel *fakeDataChannel) setBuffered(value uint64) {
	channel.mu.Lock()
	channel.buffered = value
	channel.mu.Unlock()
	if channel.low != nil {
		channel.low()
	}
}
func (channel *fakeDataChannel) frames() []frame {
	channel.mu.Lock()
	defer channel.mu.Unlock()
	result := make([]frame, 0, len(channel.sent))
	for _, value := range channel.sent {
		parsed, err := parsePacket(value)
		if err == nil {
			result = append(result, parsed)
		}
	}
	return result
}

func encodeFrame(kind byte, streamID uint32, payload []byte) []byte {
	value := make([]byte, 5+len(payload))
	value[0] = kind
	binary.LittleEndian.PutUint32(value[1:5], streamID)
	copy(value[5:], payload)
	return value
}

func waitFrame(t *testing.T, channel *fakeDataChannel, match func(frame) bool) frame {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		for _, candidate := range channel.frames() {
			if match(candidate) {
				return candidate
			}
		}
		select {
		case <-channel.notify:
		case <-deadline.C:
			t.Fatal("timed out waiting for Wisp frame")
		}
	}
}

func completeHandshake(t *testing.T, server *Server, channel *fakeDataChannel) {
	t.Helper()
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	channel.receive(encodeFrame(packetInfo, 0, []byte{2, 1}))
	response := waitFrame(t, channel, func(value frame) bool {
		return value.kind == packetContinue && value.streamID == 0
	})
	if len(response.payload) != 4 || binary.LittleEndian.Uint32(response.payload) != initialPacketAllowance {
		t.Fatalf("unexpected Wisp handshake allowance: %v", response.payload)
	}
}

func TestPacketRoundTripShape(t *testing.T) {
	data := make([]byte, 8)
	data[0] = packetConnect
	binary.LittleEndian.PutUint32(data[1:5], 42)
	copy(data[5:], []byte{streamTCP, 80, 0})
	packet, err := parsePacket(data)
	if err != nil {
		t.Fatal(err)
	}
	if packet.kind != packetConnect || packet.streamID != 42 {
		t.Fatalf("unexpected packet: %#v", packet)
	}
}

func TestBlockedDialErrorCarriesWispReason(t *testing.T) {
	cause := errors.New("outside allowlist")
	err := Blocked(cause)
	var mapped *dialError
	if !errors.As(err, &mapped) || mapped.reason != closeBlocked || !errors.Is(err, cause) {
		t.Fatalf("unexpected blocked error mapping: %v", err)
	}
}

func TestInvalidPacketSizesAreRejected(t *testing.T) {
	if _, err := parsePacket(make([]byte, 4)); err == nil {
		t.Fatal("short Wisp packet was accepted")
	}
	if _, err := parsePacket(make([]byte, maxPacketSize+1)); err == nil {
		t.Fatal("oversized Wisp packet was accepted")
	}
}

func TestWispHandshakeAndBidirectionalTCP(t *testing.T) {
	channel := newFakeDataChannel()
	client, target := net.Pipe()
	defer target.Close()
	server := newServer(t.Context(), channel, func(_ context.Context, hostname string, port uint16) (net.Conn, error) {
		if hostname != "service.webvpn.invalid" || port != 443 {
			t.Fatalf("unexpected dial target: %s:%d", hostname, port)
		}
		return client, nil
	})
	defer server.Close()
	completeHandshake(t, server, channel)

	connect := append([]byte{streamTCP, 443 & 0xff, 443 >> 8}, []byte("service.webvpn.invalid")...)
	channel.receive(encodeFrame(packetConnect, 7, connect))
	channel.receive(encodeFrame(packetData, 7, []byte("browser-to-target")))
	buffer := make([]byte, len("browser-to-target"))
	if _, err := io.ReadFull(target, buffer); err != nil || string(buffer) != "browser-to-target" {
		t.Fatalf("target did not receive browser payload: %q, %v", buffer, err)
	}
	if _, err := target.Write([]byte("target-to-browser")); err != nil {
		t.Fatal(err)
	}
	response := waitFrame(t, channel, func(value frame) bool {
		return value.kind == packetData && value.streamID == 7
	})
	if string(response.payload) != "target-to-browser" {
		t.Fatalf("browser did not receive target payload: %q", response.payload)
	}
}

func TestBlockedDialAndStreamLimitUseExplicitCloseCodes(t *testing.T) {
	channel := newFakeDataChannel()
	server := newServer(t.Context(), channel, func(context.Context, string, uint16) (net.Conn, error) {
		return nil, Blocked(errors.New("outside allowlist"))
	})
	defer server.Close()
	completeHandshake(t, server, channel)
	connect := append([]byte{streamTCP, 80, 0}, []byte("blocked.webvpn.invalid")...)
	channel.receive(encodeFrame(packetConnect, 1, connect))
	response := waitFrame(t, channel, func(value frame) bool {
		return value.kind == packetClose && value.streamID == 1
	})
	if len(response.payload) != 1 || response.payload[0] != closeBlocked {
		t.Fatalf("blocked dial used the wrong close reason: %v", response.payload)
	}

	blocked := make(chan struct{})
	limitChannel := newFakeDataChannel()
	limitServer := newServer(t.Context(), limitChannel, func(ctx context.Context, _ string, _ uint16) (net.Conn, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-blocked:
			return nil, errors.New("released")
		}
	})
	defer limitServer.Close()
	completeHandshake(t, limitServer, limitChannel)
	for id := uint32(1); id <= maxConcurrentStreams+1; id++ {
		limitChannel.receive(encodeFrame(packetConnect, id, connect))
	}
	throttled := waitFrame(t, limitChannel, func(value frame) bool {
		return value.kind == packetClose && value.streamID == maxConcurrentStreams+1
	})
	if len(throttled.payload) != 1 || throttled.payload[0] != closeThrottled {
		t.Fatalf("stream limit used the wrong close reason: %v", throttled.payload)
	}
}

func TestSendWaitsForBufferedAmountToFall(t *testing.T) {
	channel := newFakeDataChannel()
	channel.setBuffered(maxBufferedAmount + 1)
	server := newServer(t.Context(), channel, func(context.Context, string, uint16) (net.Conn, error) {
		return nil, errors.New("unused")
	})
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- server.Start() }()
	select {
	case err := <-done:
		t.Fatalf("send bypassed DataChannel backpressure: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	channel.setBuffered(0)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("send did not resume after buffered amount fell")
	}
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }

func TestWriteAllRejectsZeroProgress(t *testing.T) {
	if err := writeAll(zeroWriter{}, []byte("payload")); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("zero-progress writer was not rejected: %v", err)
	}
}
