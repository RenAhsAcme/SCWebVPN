package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const privateKeyPEMType = "WEBVPN AGENT PRIVATE KEY"

func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	if !absoluteTargetPath(path) {
		return nil, errors.New("Agent private key path must be absolute")
	}
	return loadPrivateKey(path)
}

func LoadOrCreatePrivateKey(path string) (ed25519.PrivateKey, error) {
	if !absoluteTargetPath(path) {
		return nil, errors.New("Agent private key path must be absolute")
	}
	key, err := LoadPrivateKey(path)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	_, generated, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	encoded, err := marshalPrivateKey(generated)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return LoadPrivateKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("create Agent private key: %w", err)
	}
	removeOnFailure := true
	defer func() {
		_ = file.Close()
		if removeOnFailure {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(encoded); err != nil {
		return nil, fmt.Errorf("write Agent private key: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("sync Agent private key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close Agent private key: %w", err)
	}
	removeOnFailure = false
	return generated, nil
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("Agent private key must be a regular file, not a symlink")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("Agent private key permissions must not grant group or other access")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, (16<<10)+1))
	if err != nil {
		return nil, err
	}
	if len(encoded) > 16<<10 {
		return nil, errors.New("Agent private key file is too large")
	}
	return parsePrivateKey(encoded)
}

func marshalPrivateKey(key ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: privateKeyPEMType, Bytes: der}), nil
}

func parsePrivateKey(encoded []byte) (ed25519.PrivateKey, error) {
	block, remainder := pem.Decode(encoded)
	if block == nil || block.Type != privateKeyPEMType || len(remainder) != 0 {
		return nil, errors.New("invalid Agent private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid Agent private key")
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("Agent private key is not Ed25519")
	}
	return key, nil
}

func PrivateKeyDirectory(path string) string {
	return filepath.Dir(path)
}
