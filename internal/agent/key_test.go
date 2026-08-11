package agent

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrivateKeyRoundTrip(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := marshalPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parsePrivateKey(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key, parsed) {
		t.Fatal("private key changed during PKCS#8 round trip")
	}
}

func TestLoadOrCreatePrivateKeyIsStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.key")
	first, err := LoadOrCreatePrivateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreatePrivateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("existing Agent identity was replaced")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("private key permissions are %o", info.Mode().Perm())
		}
	}
}
