package agent

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"
)

func BuildTLSConfig(policy TLSConfig) (*tls.Config, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	for _, path := range policy.CAFiles {
		pem, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read private CA: %w", err)
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("private CA file %q contained no certificates", path)
		}
	}
	pins := make(map[[32]byte]struct{}, len(policy.SPKIPins))
	for _, encoded := range policy.SPKIPins {
		digest, _ := decodeDigest(encoded)
		pins[digest] = struct{}{}
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: policy.ServerName,
		// 标准库需要跳过内建步骤，才能在同一回调中实现“严格 CA 或显式 SPKI 例外”；下方回调仍完整校验链、主机名、有效期和用途。
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			return verifyConnection(state, roots, policy.ServerName, pins, time.Now().UTC())
		},
	}, nil
}

func verifyConnection(state tls.ConnectionState, roots *x509.CertPool, serverName string, pins map[[32]byte]struct{}, now time.Time) error {
	if len(state.PeerCertificates) == 0 {
		return errors.New("TLS peer returned no certificate")
	}
	leaf := state.PeerCertificates[0]
	intermediates := x509.NewCertPool()
	for _, certificate := range state.PeerCertificates[1:] {
		intermediates.AddCert(certificate)
	}
	_, chainError := leaf.Verify(x509.VerifyOptions{
		DNSName: serverName, Roots: roots, Intermediates: intermediates,
		CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if len(pins) == 0 {
		return chainError
	}
	digest := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	if _, ok := pins[digest]; !ok {
		return errors.New("TLS peer SPKI did not match the configured exception")
	}
	if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
		return errors.New("pinned TLS certificate is not currently valid")
	}
	if err := leaf.VerifyHostname(serverName); err != nil {
		return fmt.Errorf("pinned TLS certificate hostname: %w", err)
	}
	if !permitsServerAuth(leaf.ExtKeyUsage) {
		return errors.New("pinned TLS certificate does not permit server authentication")
	}
	return nil
}

func permitsServerAuth(usages []x509.ExtKeyUsage) bool {
	if len(usages) == 0 {
		return true
	}
	for _, usage := range usages {
		if usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny {
			return true
		}
	}
	return false
}
