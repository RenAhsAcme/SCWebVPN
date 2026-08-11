package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

const tokenSize = 32

func NewToken() (plain string, digest [32]byte, err error) {
	raw := make([]byte, tokenSize)
	if _, err = rand.Read(raw); err != nil {
		return "", digest, err
	}
	plain = base64.RawURLEncoding.EncodeToString(raw)
	return plain, HashToken(plain), nil
}

func HashToken(value string) [32]byte {
	return sha256.Sum256([]byte("scwebvpn-session-v1\x00" + value))
}

func ValidateToken(value string) error {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) != tokenSize {
		return errors.New("invalid session capability")
	}
	return nil
}
