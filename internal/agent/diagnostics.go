package agent

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const diagnosticTimeout = 2 * time.Second

func pingTarget(ctx context.Context, address netip.Addr) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, diagnosticTimeout)
	defer cancel()
	network, local, protocol := "udp4", "0.0.0.0", 1
	requestType, replyType := icmp.Type(ipv4.ICMPTypeEcho), icmp.Type(ipv4.ICMPTypeEchoReply)
	if address.Is6() {
		network, local, protocol = "udp6", "::", 58
		requestType, replyType = ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply
	}
	connection, err := icmp.ListenPacket(network, local)
	if err != nil {
		return 0, err
	}
	defer connection.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(diagnosticTimeout)
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return 0, err
	}
	var randomID [2]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return 0, err
	}
	id := int(binary.BigEndian.Uint16(randomID[:]))
	message := icmp.Message{Type: requestType, Code: 0, Body: &icmp.Echo{ID: id, Seq: 1, Data: []byte("scwebvpn-ping-v1")}}
	payload, err := message.Marshal(nil)
	if err != nil {
		return 0, err
	}
	started := time.Now()
	if _, err := connection.WriteTo(payload, &net.UDPAddr{IP: net.IP(address.AsSlice())}); err != nil {
		return 0, err
	}
	buffer := make([]byte, 1_500)
	for {
		count, _, err := connection.ReadFrom(buffer)
		if err != nil {
			return 0, err
		}
		response, err := icmp.ParseMessage(protocol, buffer[:count])
		if err != nil || response.Type != replyType {
			continue
		}
		echo, ok := response.Body.(*icmp.Echo)
		if !ok || echo.ID != id || echo.Seq != 1 {
			continue
		}
		return time.Since(started), nil
	}
}

func diagnosticCode(err error) string {
	if errors.Is(err, ErrBlocked) {
		return "blocked"
	}
	return "unavailable"
}
