package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"testing"
	"time"
)

func TestPrivateCAAndPinnedExceptionValidation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	root, rootKey := issueCertificate(t, nil, nil, x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test Root"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	})
	leaf, _ := issueCertificate(t, root, rootKey, x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "openwrt.lan"}, DNSNames: []string{"openwrt.lan"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	roots := x509.NewCertPool()
	roots.AddCert(root)
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}
	if err := verifyConnection(state, roots, "openwrt.lan", nil, now); err != nil {
		t.Fatalf("private CA chain was rejected: %v", err)
	}

	emptyRoots := x509.NewCertPool()
	pin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	if err := verifyConnection(state, emptyRoots, "openwrt.lan", map[[32]byte]struct{}{pin: {}}, now); err != nil {
		t.Fatalf("explicit pinned exception was rejected: %v", err)
	}
	wrongPin := sha256.Sum256([]byte("wrong"))
	if err := verifyConnection(state, emptyRoots, "openwrt.lan", map[[32]byte]struct{}{wrongPin: {}}, now); err == nil {
		t.Fatal("wrong pinned exception was accepted")
	}
	if err := verifyConnection(state, emptyRoots, "other.lan", map[[32]byte]struct{}{pin: {}}, now); err == nil {
		t.Fatal("pinned certificate bypassed hostname validation")
	}
	encoded := base64.StdEncoding.EncodeToString(pin[:])
	if _, err := decodeDigest(encoded); err != nil {
		t.Fatal(err)
	}
}

func issueCertificate(t *testing.T, parent *x509.Certificate, parentKey ed25519.PrivateKey, template x509.Certificate) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if parent == nil {
		parent, parentKey = &template, privateKey
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, parent, publicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, privateKey
}
