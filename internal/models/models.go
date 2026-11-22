package models

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
