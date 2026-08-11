package wisp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/pion/webrtc/v4"
)

const (
	packetConnect  = 0x01
	packetData     = 0x02
	packetContinue = 0x03
	packetClose    = 0x04
	packetInfo     = 0x05

	streamTCP = 0x01

	closeVoluntary   = 0x02
	closeNetwork     = 0x03
	closeInvalidInfo = 0x41
	closeUnreachable = 0x42
	closeBlocked     = 0x48
	closeThrottled   = 0x49

	initialPacketAllowance = 64
	continueThreshold      = initialPacketAllowance / 2
	maxBufferedAmount      = 4 << 20
	readBufferSize         = 16 << 10
	maxPacketSize          = 64 << 10
	maxConcurrentStreams   = 16
)

type DialFunc func(context.Context, string, uint16) (net.Conn, error)

type dialError struct {
	reason byte
	err    error
}

func (failure *dialError) Error() string { return failure.err.Error() }
func (failure *dialError) Unwrap() error { return failure.err }

func Blocked(err error) error {
	return &dialError{reason: closeBlocked, err: err}
}

type Server struct {
	ctx    context.Context
	cancel context.CancelFunc
	dc     dataChannel
	dial   DialFunc

	mu         sync.Mutex
	streams    map[uint32]*stream
	handshaken bool

	sendMu      sync.Mutex
	bufferedLow chan struct{}
}

type dataChannel interface {
	SetBufferedAmountLowThreshold(uint64)
	OnBufferedAmountLow(func())
	OnMessage(func(webrtc.DataChannelMessage))
	OnClose(func())
	BufferedAmount() uint64
	Send([]byte) error
}

type stream struct {
	id     uint32
	server *Server
	ctx    context.Context
	cancel context.CancelFunc

	connMu    sync.Mutex
	conn      net.Conn
	connReady chan struct{}
	write     chan []byte
	closeOnce sync.Once
}

func NewServer(parent context.Context, dataChannel *webrtc.DataChannel, dial DialFunc) *Server {
	return newServer(parent, dataChannel, dial)
}

func newServer(parent context.Context, channel dataChannel, dial DialFunc) *Server {
	ctx, cancel := context.WithCancel(parent)
	server := &Server{
		ctx: ctx, cancel: cancel, dc: channel, dial: dial,
		streams: make(map[uint32]*stream), bufferedLow: make(chan struct{}, 1),
	}
	channel.SetBufferedAmountLowThreshold(maxBufferedAmount / 2)
	channel.OnBufferedAmountLow(func() {
		select {
		case server.bufferedLow <- struct{}{}:
		default:
		}
	})
	channel.OnMessage(server.onMessage)
	channel.OnClose(server.Close)
	return server
}

func (server *Server) Start() error {
	return server.sendFrame(packetInfo, 0, []byte{2, 1})
}

func (server *Server) Close() {
	server.cancel()
	server.mu.Lock()
	streams := make([]*stream, 0, len(server.streams))
	for _, current := range server.streams {
		streams = append(streams, current)
	}
	server.mu.Unlock()
	for _, current := range streams {
		current.close(false, closeVoluntary)
	}
}

func (server *Server) onMessage(message webrtc.DataChannelMessage) {
	packet, err := parsePacket(message.Data)
	if err != nil {
		_ = server.sendFrame(packetClose, 0, []byte{closeInvalidInfo})
		server.Close()
		return
	}
	if packet.streamID == 0 {
		server.handleHandshake(packet)
		return
	}
	server.mu.Lock()
	handshaken := server.handshaken
	current := server.streams[packet.streamID]
	server.mu.Unlock()
	if !handshaken {
		_ = server.sendFrame(packetClose, 0, []byte{closeInvalidInfo})
		server.Close()
		return
	}
	switch packet.kind {
	case packetConnect:
		if current != nil {
			_ = server.sendFrame(packetClose, packet.streamID, []byte{closeInvalidInfo})
			return
		}
		server.openStream(packet)
	case packetData:
		if current == nil {
			_ = server.sendFrame(packetClose, packet.streamID, []byte{closeInvalidInfo})
			return
		}
		payload := append([]byte(nil), packet.payload...)
		select {
		case current.write <- payload:
		default:
			current.close(true, closeThrottled)
		}
	case packetClose:
		if current != nil {
			current.close(false, closeVoluntary)
		}
	default:
		_ = server.sendFrame(packetClose, packet.streamID, []byte{closeInvalidInfo})
	}
}

func (server *Server) handleHandshake(packet frame) {
	if packet.kind != packetInfo || len(packet.payload) < 2 || packet.payload[0] != 2 {
		_ = server.sendFrame(packetClose, 0, []byte{closeInvalidInfo})
		server.Close()
		return
	}
	server.mu.Lock()
	if server.handshaken {
		server.mu.Unlock()
		return
	}
	server.handshaken = true
	server.mu.Unlock()
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, initialPacketAllowance)
	if err := server.sendFrame(packetContinue, 0, payload); err != nil {
		server.Close()
	}
}

func (server *Server) openStream(packet frame) {
	if len(packet.payload) < 4 || packet.payload[0] != streamTCP {
		_ = server.sendFrame(packetClose, packet.streamID, []byte{closeInvalidInfo})
		return
	}
	port := binary.LittleEndian.Uint16(packet.payload[1:3])
	hostname := string(packet.payload[3:])
	if hostname == "" || port == 0 {
		_ = server.sendFrame(packetClose, packet.streamID, []byte{closeInvalidInfo})
		return
	}
	current := &stream{
		id: packet.streamID, server: server,
		connReady: make(chan struct{}), write: make(chan []byte, initialPacketAllowance),
	}
	current.ctx, current.cancel = context.WithCancel(server.ctx)
	server.mu.Lock()
	if _, exists := server.streams[current.id]; exists {
		server.mu.Unlock()
		_ = server.sendFrame(packetClose, packet.streamID, []byte{closeInvalidInfo})
		return
	}
	if len(server.streams) >= maxConcurrentStreams {
		server.mu.Unlock()
		_ = server.sendFrame(packetClose, packet.streamID, []byte{closeThrottled})
		return
	}
	server.streams[current.id] = current
	server.mu.Unlock()
	go current.connect(hostname, port)
	go current.writeLoop()
}

func (current *stream) connect(hostname string, port uint16) {
	connection, err := current.server.dial(current.ctx, hostname, port)
	current.connMu.Lock()
	if current.ctx.Err() != nil && connection != nil {
		_ = connection.Close()
		connection = nil
	}
	current.conn = connection
	close(current.connReady)
	current.connMu.Unlock()
	if err != nil {
		reason := byte(closeUnreachable)
		var mapped *dialError
		if errors.As(err, &mapped) {
			reason = mapped.reason
		}
		current.close(true, reason)
		return
	}
	go current.readLoop(connection)
}

func (current *stream) writeLoop() {
	select {
	case <-current.server.ctx.Done():
		return
	case <-current.connReady:
	}
	current.connMu.Lock()
	connection := current.conn
	current.connMu.Unlock()
	if connection == nil {
		return
	}
	consumed := 0
	for {
		select {
		case <-current.ctx.Done():
			return
		case payload := <-current.write:
			if err := writeAll(connection, payload); err != nil {
				current.close(true, closeNetwork)
				return
			}
			consumed++
			if consumed >= continueThreshold {
				remaining := initialPacketAllowance - len(current.write)
				response := make([]byte, 4)
				binary.LittleEndian.PutUint32(response, uint32(remaining))
				if err := current.server.sendFrame(packetContinue, current.id, response); err != nil {
					current.close(false, closeNetwork)
					return
				}
				consumed = 0
			}
		}
	}
}

func (current *stream) readLoop(connection net.Conn) {
	buffer := make([]byte, readBufferSize)
	for {
		size, err := connection.Read(buffer)
		if size > 0 {
			if sendError := current.server.sendFrame(packetData, current.id, buffer[:size]); sendError != nil {
				current.close(false, closeNetwork)
				return
			}
		}
		if err != nil {
			reason := byte(closeNetwork)
			if errors.Is(err, io.EOF) {
				reason = closeVoluntary
			}
			current.close(true, reason)
			return
		}
	}
}

func (current *stream) close(notify bool, reason byte) {
	current.closeOnce.Do(func() {
		current.cancel()
		current.server.mu.Lock()
		delete(current.server.streams, current.id)
		current.server.mu.Unlock()
		current.connMu.Lock()
		if current.conn != nil {
			_ = current.conn.Close()
		}
		current.connMu.Unlock()
		if notify {
			_ = current.server.sendFrame(packetClose, current.id, []byte{reason})
		}
	})
}

func (server *Server) sendFrame(kind byte, streamID uint32, payload []byte) error {
	packet := make([]byte, 5+len(payload))
	packet[0] = kind
	binary.LittleEndian.PutUint32(packet[1:5], streamID)
	copy(packet[5:], payload)
	server.sendMu.Lock()
	defer server.sendMu.Unlock()
	for server.dc.BufferedAmount() > maxBufferedAmount {
		select {
		case <-server.ctx.Done():
			return server.ctx.Err()
		case <-server.bufferedLow:
		}
	}
	return server.dc.Send(packet)
}

type frame struct {
	kind     byte
	streamID uint32
	payload  []byte
}

func parsePacket(data []byte) (frame, error) {
	if len(data) < 5 {
		return frame{}, errors.New("Wisp packet is too short")
	}
	if len(data) > maxPacketSize {
		return frame{}, errors.New("Wisp packet exceeds the message limit")
	}
	kind := data[0]
	if kind < packetConnect || kind > packetInfo {
		return frame{}, fmt.Errorf("unknown Wisp packet type: %d", kind)
	}
	return frame{kind: kind, streamID: binary.LittleEndian.Uint32(data[1:5]), payload: data[5:]}, nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrNoProgress
		}
		data = data[written:]
	}
	return nil
}
