package models

import (
	"fmt"
	"time"
)

// DefaultPollInterval is used when pollInterval is not configured.
const DefaultPollInterval = 10 * time.Second

// MinPollInterval is the smallest accepted pollInterval.
const MinPollInterval = 2 * time.Second

type PrometheusConfig struct {
	Enabled       bool   `json:"enabled"`
	ListenAddress string `json:"listenAddress"`
}

type EcowittConfiguration struct {
	StatsServer string           `json:"statsServer"`
	Host        string           `json:"host"`
	Port        int              `json:"port"`
	DefaultTags []string         `json:"defaultTags"`
	Prometheus  PrometheusConfig `json:"prometheus"`
	MQTT        MQTTConfig       `json:"mqtt"`
	// FailureThreshold is the number of consecutive failed scans before the
	// gateway is considered down. Defaults to 3 when <= 0.
	FailureThreshold int `json:"failureThreshold"`
	// PollInterval is a Go duration string such as "10s" or "1m". Defaults to
	// 10s; the minimum is 2s.
	PollInterval string `json:"pollInterval"`
	// ListenAddress is the address of the HTTP server (healthz, readyz and,
	// when enabled, metrics). Falls back to prometheus.listenAddress, then :9090.
	ListenAddress string `json:"listenAddress"`
}

// PollIntervalDuration parses PollInterval, applying the default and minimum.
func (c EcowittConfiguration) PollIntervalDuration() (time.Duration, error) {
	if c.PollInterval == "" {
		return DefaultPollInterval, nil
	}
	d, err := time.ParseDuration(c.PollInterval)
	if err != nil {
		return 0, fmt.Errorf("invalid pollInterval %q: %w", c.PollInterval, err)
	}
	if d < MinPollInterval {
		return 0, fmt.Errorf("pollInterval %q is too small, the minimum is %s", c.PollInterval, MinPollInterval)
	}
	return d, nil
}

// MQTTConfig configures the MQTT / Home Assistant discovery output.
type MQTTConfig struct {
	Enabled         bool          `json:"enabled"`
	Broker          string        `json:"broker"`
	Username        string        `json:"username"`
	Password        string        `json:"password"`
	ClientID        string        `json:"clientId"`
	TopicPrefix     string        `json:"topicPrefix"`
	DiscoveryPrefix string        `json:"discoveryPrefix"`
	DeviceName      string        `json:"deviceName"`
	Retain          bool          `json:"retain"`
	TLS             MQTTTLSConfig `json:"tls"`
}

// MQTTTLSConfig configures TLS and mutual TLS for the MQTT connection. It only
// applies when the broker URL uses a TLS scheme such as ssl://.
type MQTTTLSConfig struct {
	// CAFile is a PEM bundle of CA certificates appended to the system pool.
	CAFile string `json:"caFile"`
	// CertFile and KeyFile are the client certificate and key (mutual TLS).
	// Both must be set, or neither.
	CertFile           string `json:"certFile"`
	KeyFile            string `json:"keyFile"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	// ServerName overrides SNI and certificate verification.
	ServerName string `json:"serverName"`
}

// WithDefaults returns a copy of the config with empty fields defaulted.
func (c MQTTConfig) WithDefaults() MQTTConfig {
	if c.ClientID == "" {
		c.ClientID = "ecowitt-statsd"
	}
	if c.TopicPrefix == "" {
		c.TopicPrefix = "ecowitt"
	}
	if c.DiscoveryPrefix == "" {
		c.DiscoveryPrefix = "homeassistant"
	}
	if c.DeviceName == "" {
		c.DeviceName = "Ecowitt Gateway"
	}
	return c
}
