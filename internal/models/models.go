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
}

// MQTTConfig configures the MQTT / Home Assistant discovery output.
type MQTTConfig struct {
	Enabled         bool   `json:"enabled"`
	Broker          string `json:"broker"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	ClientID        string `json:"clientId"`
	TopicPrefix     string `json:"topicPrefix"`
	DiscoveryPrefix string `json:"discoveryPrefix"`
	DeviceName      string `json:"deviceName"`
	Retain          bool   `json:"retain"`
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
