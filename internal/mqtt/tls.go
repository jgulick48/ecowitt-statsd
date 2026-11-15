package mqtt

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/jgulick48/ecowitt-statsd/internal/models"
)

// BuildTLSConfig converts the TLS settings into a *tls.Config. It returns
// nil, nil when no setting is used, leaving the paho defaults unchanged.
func BuildTLSConfig(c models.MQTTTLSConfig) (*tls.Config, error) {
	if c == (models.MQTTTLSConfig{}) {
		return nil, nil
	}
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
	}
	if c.CAFile != "" {
		pem, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("reading MQTT TLS CA file %q: %w", c.CAFile, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("MQTT TLS CA file %q contains no valid PEM certificates", c.CAFile)
		}
		cfg.RootCAs = pool
	}
	if (c.CertFile == "") != (c.KeyFile == "") {
		return nil, fmt.Errorf("MQTT TLS certFile (%q) and keyFile (%q) must both be set or both empty", c.CertFile, c.KeyFile)
	}
	if c.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading MQTT TLS client certificate %q and key %q: %w", c.CertFile, c.KeyFile, err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
