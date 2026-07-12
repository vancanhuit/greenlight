package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// minimal PEM CA fixture generated once; any valid CA cert works.
const testCAPEM = `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`

func TestTLSConfigDisabledWhenNoCert(t *testing.T) {
	app := &application{}
	cfg, err := app.tlsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil tls.Config when no cert configured, got %v", cfg)
	}
}

func TestTLSConfigSetsMinVersion(t *testing.T) {
	app := &application{}
	app.config.tls.certFile = "cert.pem"
	app.config.tls.keyFile = "key.pem"

	cfg, err := app.tlsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d", cfg.MinVersion, tls.VersionTLS12)
	}
	if cfg.ClientAuth != tls.NoClientCert {
		t.Errorf("ClientAuth = %d, want NoClientCert", cfg.ClientAuth)
	}
}

func TestTLSConfigEnablesMTLS(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, []byte(testCAPEM), 0o600); err != nil {
		t.Fatal(err)
	}

	app := &application{}
	app.config.tls.certFile = "cert.pem"
	app.config.tls.keyFile = "key.pem"
	app.config.tls.clientCAFile = caPath

	cfg, err := app.tlsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %d, want RequireAndVerifyClientCert", cfg.ClientAuth)
	}
	if cfg.ClientCAs == nil {
		t.Error("expected ClientCAs pool to be set")
	}
}

// TestMTLSHandshake exercises tlsConfig() end-to-end over a real TLS handshake:
// a request without a client cert is rejected, one presenting a client-auth
// cert signed by the configured CA is accepted, and a cert carrying only the
// server-auth EKU is rejected. The last case regresses the mTLS bug where the
// server leaf was mistakenly reused as the client cert.
func TestMTLSHandshake(t *testing.T) {
	now := time.Now()

	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Greenlight Test CA"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caCertPEM, _, caLeaf, caKey := makeCert(t, caTmpl, caTmpl, nil)

	serverTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverCertPEM, serverKeyPEM, _, _ := makeCert(t, serverTmpl, caLeaf, caKey)
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	serverAsClientCert := serverCert // ServerAuth EKU only

	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "greenlight-client"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientCertPEM, clientKeyPEM, _, _ := makeCert(t, clientTmpl, caLeaf, caKey)
	clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		t.Fatalf("client keypair: %v", err)
	}

	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, caCertPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	app := &application{}
	app.config.tls.certFile = "present" // sentinel: tlsConfig only reads clientCAFile from disk
	app.config.tls.clientCAFile = caPath

	cfg, err := app.tlsConfig()
	if err != nil {
		t.Fatalf("tlsConfig: %v", err)
	}
	cfg.Certificates = []tls.Certificate{serverCert}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts.TLS = cfg
	ts.StartTLS()
	defer ts.Close()

	rootPool := x509.NewCertPool()
	if !rootPool.AppendCertsFromPEM(caCertPEM) {
		t.Fatal("failed to add CA to root pool")
	}
	newClient := func(certs ...tls.Certificate) *http.Client {
		return &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: rootPool, Certificates: certs},
		}}
	}

	t.Run("no client cert rejected", func(t *testing.T) {
		res, err := newClient().Get(ts.URL)
		if err == nil {
			_ = res.Body.Close()
			t.Fatal("expected handshake failure without a client cert")
		}
	})

	t.Run("valid client cert accepted", func(t *testing.T) {
		res, err := newClient(clientCert).Get(ts.URL)
		if err != nil {
			t.Fatalf("expected success with valid client cert, got %v", err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", res.StatusCode, http.StatusOK)
		}
	})

	t.Run("server-auth cert rejected as client cert", func(t *testing.T) {
		res, err := newClient(serverAsClientCert).Get(ts.URL)
		if err == nil {
			_ = res.Body.Close()
			t.Fatal("expected rejection of a server-auth-only cert used for client auth")
		}
	})
}

// makeCert issues an X.509 certificate from tmpl signed by parent/parentKey.
// When parentKey is nil the certificate is self-signed (used for the CA).
func makeCert(t *testing.T, tmpl, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (certPEM, keyPEM []byte, leaf *x509.Certificate, key *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signerKey := parentKey
	if signerKey == nil {
		signerKey = key
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	leaf, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, leaf, key
}
