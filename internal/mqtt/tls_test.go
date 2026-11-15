package mqtt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jgulick48/ecowitt-statsd/internal/models"
)

type testPKI struct {
	dir                       string
	caFile, certFile, keyFile string
	otherKeyFile, keyPEM      string
	leaf                      *x509.Certificate
}

func writePEM(t *testing.T, path, typ string, der []byte) string {
	t.Helper()
	b := pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newPKI(t *testing.T) testPKI {
	t.Helper()
	dir := t.TempDir()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(leafDER)
	keyDER, _ := x509.MarshalECPrivateKey(leafKey)
	otherKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	otherDER, _ := x509.MarshalECPrivateKey(otherKey)

	p := testPKI{dir: dir, leaf: leaf}
	p.caFile = filepath.Join(dir, "ca.pem")
	writePEM(t, p.caFile, "CERTIFICATE", caDER)
	p.certFile = filepath.Join(dir, "client.crt")
	writePEM(t, p.certFile, "CERTIFICATE", leafDER)
	p.keyFile = filepath.Join(dir, "client.key")
	p.keyPEM = writePEM(t, p.keyFile, "EC PRIVATE KEY", keyDER)
	p.otherKeyFile = filepath.Join(dir, "other.key")
	writePEM(t, p.otherKeyFile, "EC PRIVATE KEY", otherDER)
	return p
}

func TestBuildTLSConfigEmpty(t *testing.T) {
	cfg, err := BuildTLSConfig(models.MQTTTLSConfig{})
	if err != nil || cfg != nil {
		t.Fatalf("got %v, %v; want nil, nil", cfg, err)
	}
}

func TestBuildTLSConfigCAOnly(t *testing.T) {
	p := newPKI(t)
	cfg, err := BuildTLSConfig(models.MQTTTLSConfig{CAFile: p.caFile})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %x", cfg.MinVersion)
	}
	if len(cfg.Certificates) != 0 {
		t.Errorf("unexpected client certs")
	}
	_, err = p.leaf.Verify(x509.VerifyOptions{
		Roots:     cfg.RootCAs,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		t.Errorf("leaf should verify against pool: %v", err)
	}
}

func TestBuildTLSConfigClientCert(t *testing.T) {
	p := newPKI(t)
	cfg, err := BuildTLSConfig(models.MQTTTLSConfig{CertFile: p.certFile, KeyFile: p.keyFile})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("Certificates = %d, want 1", len(cfg.Certificates))
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %x", cfg.MinVersion)
	}
}

func TestBuildTLSConfigPassThrough(t *testing.T) {
	cfg, err := BuildTLSConfig(models.MQTTTLSConfig{InsecureSkipVerify: true, ServerName: "broker.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.InsecureSkipVerify || cfg.ServerName != "broker.example.com" {
		t.Errorf("not passed through: %+v", cfg)
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %x", cfg.MinVersion)
	}
}

func TestBuildTLSConfigErrors(t *testing.T) {
	p := newPKI(t)
	garbage := filepath.Join(p.dir, "garbage.pem")
	if err := os.WriteFile(garbage, []byte("not a pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(p.dir, "missing.pem")
	tests := []struct {
		name string
		cfg  models.MQTTTLSConfig
		path string
	}{
		{"missing CA", models.MQTTTLSConfig{CAFile: missing}, missing},
		{"garbage CA", models.MQTTTLSConfig{CAFile: garbage}, garbage},
		{"cert without key", models.MQTTTLSConfig{CertFile: p.certFile}, p.certFile},
		{"key without cert", models.MQTTTLSConfig{KeyFile: p.keyFile}, p.keyFile},
		{"mismatched key", models.MQTTTLSConfig{CertFile: p.certFile, KeyFile: p.otherKeyFile}, p.otherKeyFile},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := BuildTLSConfig(tc.cfg)
			if err == nil || cfg != nil {
				t.Fatalf("got %v, %v; want error", cfg, err)
			}
			if !strings.Contains(err.Error(), tc.path) {
				t.Errorf("error %q does not mention %q", err, tc.path)
			}
			if strings.Contains(err.Error(), "PRIVATE KEY") {
				t.Errorf("error leaks key PEM: %q", err)
			}
		})
	}
}

func TestBuildTLSConfigErrorOmitsKeyContents(t *testing.T) {
	p := newPKI(t)
	_, err := BuildTLSConfig(models.MQTTTLSConfig{CertFile: p.certFile, KeyFile: p.otherKeyFile})
	if err == nil {
		t.Fatal("expected error")
	}
	body := strings.TrimSpace(strings.Split(p.keyPEM, "\n")[1])
	if strings.Contains(err.Error(), body) {
		t.Errorf("error contains key material")
	}
}
