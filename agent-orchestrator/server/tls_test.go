package server

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTLSCert_ProvidedPathsTakePrecedence(t *testing.T) {
	crt, key, err := resolveTLSCert("/my/cert.pem", "/my/key.pem", t.TempDir(), []string{"localhost"})
	if err != nil {
		t.Fatalf("resolveTLSCert: %v", err)
	}
	if crt != "/my/cert.pem" || key != "/my/key.pem" {
		t.Errorf("provided paths not returned as-is: crt=%q key=%q", crt, key)
	}
}

func TestResolveTLSCert_GeneratesAndReuses(t *testing.T) {
	root := t.TempDir()
	sans := []string{"localhost", "127.0.0.1"}

	crt, key, err := resolveTLSCert("", "", root, sans)
	if err != nil {
		t.Fatalf("resolveTLSCert (generate): %v", err)
	}
	if !fileExists(crt) || !fileExists(key) {
		t.Fatalf("expected generated cert/key to exist: %q %q", crt, key)
	}
	if crt != filepath.Join(root, "tls", "server.crt") {
		t.Errorf("unexpected cert path: %q", crt)
	}

	// Capture content, then call again — it must reuse, not regenerate.
	before, err := os.ReadFile(crt)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	crt2, key2, err := resolveTLSCert("", "", root, sans)
	if err != nil {
		t.Fatalf("resolveTLSCert (reuse): %v", err)
	}
	if crt2 != crt || key2 != key {
		t.Errorf("reuse returned different paths: %q %q", crt2, key2)
	}
	after, err := os.ReadFile(crt)
	if err != nil {
		t.Fatalf("re-read cert: %v", err)
	}
	if string(before) != string(after) {
		t.Error("certificate was regenerated on second call; expected reuse")
	}
}

func TestGenerateSelfSignedCert_ValidWithSANs(t *testing.T) {
	dir := t.TempDir()
	crt := filepath.Join(dir, "server.crt")
	key := filepath.Join(dir, "server.key")

	if err := generateSelfSignedCert(crt, key, []string{"localhost", "127.0.0.1", "::1"}); err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}

	// Loadable as a TLS keypair.
	pair, err := tls.LoadX509KeyPair(crt, key)
	if err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	var hasLocalhost bool
	for _, n := range leaf.DNSNames {
		if n == "localhost" {
			hasLocalhost = true
		}
	}
	if !hasLocalhost {
		t.Errorf("DNSNames missing localhost: %v", leaf.DNSNames)
	}
	if len(leaf.IPAddresses) != 2 {
		t.Errorf("expected 2 IP SANs (127.0.0.1, ::1), got %v", leaf.IPAddresses)
	}
}
