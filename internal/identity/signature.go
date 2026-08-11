package identity

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	SignatureVersion = "webvpn-agent-v1"
	MaxClockSkew     = 90 * time.Second
)

type SignedRequest struct {
	AgentID   string
	Nonce     string
	IssuedAt  time.Time
	Signature []byte
}

func ParseSignedRequest(request *http.Request) (SignedRequest, error) {
	issuedAt, err := strconv.ParseInt(request.Header.Get("X-WebVPN-Issued-At"), 10, 64)
	if err != nil {
		return SignedRequest{}, errors.New("invalid agent request timestamp")
	}
	signature, err := base64.RawURLEncoding.DecodeString(request.Header.Get("X-WebVPN-Signature"))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return SignedRequest{}, errors.New("invalid agent request signature")
	}
	result := SignedRequest{
		AgentID:   request.Header.Get("X-WebVPN-Agent-ID"),
		Nonce:     request.Header.Get("X-WebVPN-Nonce"),
		IssuedAt:  time.Unix(issuedAt, 0).UTC(),
		Signature: signature,
	}
	if !validOpaqueID(result.AgentID, 16) || !validOpaqueID(result.Nonce, 32) {
		return SignedRequest{}, errors.New("invalid agent ID or nonce")
	}
	return result, nil
}

func Verify(publicKey ed25519.PublicKey, signed SignedRequest, method, path string, body []byte, now time.Time) error {
	if len(publicKey) != ed25519.PublicKeySize || len(signed.Signature) != ed25519.SignatureSize {
		return errors.New("invalid Ed25519 key or signature size")
	}
	if delta := now.Sub(signed.IssuedAt); delta < -MaxClockSkew || delta > MaxClockSkew {
		return errors.New("agent request timestamp is outside the permitted window")
	}
	message := CanonicalMessage(signed.AgentID, signed.Nonce, signed.IssuedAt, method, path, body)
	if !ed25519.Verify(publicKey, message, signed.Signature) {
		return errors.New("agent request signature verification failed")
	}
	return nil
}

func CanonicalMessage(agentID, nonce string, issuedAt time.Time, method, path string, body []byte) []byte {
	digest := sha256.Sum256(body)
	return []byte(strings.Join([]string{
		SignatureVersion,
		agentID,
		nonce,
		strconv.FormatInt(issuedAt.Unix(), 10),
		strings.ToUpper(method),
		path,
		base64.RawURLEncoding.EncodeToString(digest[:]),
	}, "\n"))
}

func EqualSecret(expected, provided []byte) bool {
	return len(expected) == len(provided) && subtle.ConstantTimeCompare(expected, provided) == 1
}

func validOpaqueID(value string, bytes int) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == bytes
}

func EncodePublicKey(key ed25519.PublicKey) (string, error) {
	if len(key) != ed25519.PublicKeySize {
		return "", fmt.Errorf("public key must contain %d bytes", ed25519.PublicKeySize)
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}
