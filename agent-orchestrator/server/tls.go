package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// resolveTLSCert returns the certificate and key file paths for the server's TLS
// listener. If both certFile and keyFile are provided, they are used as-is.
// Otherwise a self-signed certificate is generated once under {storageRoot}/tls
// and reused on subsequent starts. sans lists the DNS names / IP addresses the
// generated certificate should cover.
func resolveTLSCert(certFile, keyFile, storageRoot string, sans []string) (string, string, error) {
	if certFile != "" && keyFile != "" {
		return certFile, keyFile, nil
	}
	dir := filepath.Join(storageRoot, "tls")
	crt := filepath.Join(dir, "server.crt")
	key := filepath.Join(dir, "server.key")
	if fileExists(crt) && fileExists(key) {
		return crt, key, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("tls: mkdir %q: %w", dir, err)
	}
	if err := generateSelfSignedCert(crt, key, sans); err != nil {
		return "", "", err
	}
	return crt, key, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// generateSelfSignedCert writes a PEM certificate + EC private key pair covering
// the given SANs (DNS names and IP addresses) to certPath and keyPath.
func generateSelfSignedCert(certPath, keyPath string, sans []string) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("tls: generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("tls: serial: %w", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "agent-orchestrator"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, s := range sans {
		if s == "" {
			continue
		}
		if ip := net.ParseIP(s); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, s)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("tls: create cert: %w", err)
	}

	certOut, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("tls: open cert file: %w", err)
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return fmt.Errorf("tls: encode cert: %w", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("tls: marshal key: %w", err)
	}
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("tls: open key file: %w", err)
	}
	defer keyOut.Close()
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return fmt.Errorf("tls: encode key: %w", err)
	}
	return nil
}
