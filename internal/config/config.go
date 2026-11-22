// Package config loads the daemon configuration from an optional JSON file,
// overridden by ECOWITT_* environment variables.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/jgulick48/ecowitt-statsd/internal/models"
)

type override struct {
	env   string
	apply func(*models.EcowittConfiguration, string) error
}

func setString(f func(*models.EcowittConfiguration) *string) func(*models.EcowittConfiguration, string) error {
	return func(c *models.EcowittConfiguration, v string) error {
		*f(c) = v
		return nil
	}
}

func setBool(env string, f func(*models.EcowittConfiguration) *bool) func(*models.EcowittConfiguration, string) error {
	return func(c *models.EcowittConfiguration, v string) error {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid boolean for %s", env)
		}
		*f(c) = b
		return nil
	}
}

// overrides is the table of environment variables applied over the file.
// Add new variables here.
var overrides = []override{
	{"ECOWITT_HOST", setString(func(c *models.EcowittConfiguration) *string { return &c.Host })},
	{"ECOWITT_STATS_SERVER", setString(func(c *models.EcowittConfiguration) *string { return &c.StatsServer })},
	{"ECOWITT_DEFAULT_TAGS", func(c *models.EcowittConfiguration, v string) error {
		var tags []string
		for _, t := range strings.Split(v, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		c.DefaultTags = tags
		return nil
	}},
	{"ECOWITT_PROMETHEUS_ENABLED", setBool("ECOWITT_PROMETHEUS_ENABLED", func(c *models.EcowittConfiguration) *bool { return &c.Prometheus.Enabled })},
	{"ECOWITT_PROMETHEUS_LISTEN_ADDRESS", setString(func(c *models.EcowittConfiguration) *string { return &c.Prometheus.ListenAddress })},
	{"ECOWITT_MQTT_ENABLED", setBool("ECOWITT_MQTT_ENABLED", func(c *models.EcowittConfiguration) *bool { return &c.MQTT.Enabled })},
	{"ECOWITT_MQTT_BROKER", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.Broker })},
	{"ECOWITT_MQTT_USERNAME", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.Username })},
	{"ECOWITT_MQTT_PASSWORD", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.Password })},
	{"ECOWITT_MQTT_CLIENT_ID", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.ClientID })},
	{"ECOWITT_MQTT_TOPIC_PREFIX", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.TopicPrefix })},
	{"ECOWITT_MQTT_DISCOVERY_PREFIX", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.DiscoveryPrefix })},
	{"ECOWITT_MQTT_DEVICE_NAME", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.DeviceName })},
	{"ECOWITT_MQTT_RETAIN", setBool("ECOWITT_MQTT_RETAIN", func(c *models.EcowittConfiguration) *bool { return &c.MQTT.Retain })},
	{"ECOWITT_MQTT_TLS_CA_FILE", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.TLS.CAFile })},
	{"ECOWITT_MQTT_TLS_CERT_FILE", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.TLS.CertFile })},
	{"ECOWITT_MQTT_TLS_KEY_FILE", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.TLS.KeyFile })},
	{"ECOWITT_MQTT_TLS_SERVER_NAME", setString(func(c *models.EcowittConfiguration) *string { return &c.MQTT.TLS.ServerName })},
	{"ECOWITT_MQTT_TLS_INSECURE_SKIP_VERIFY", setBool("ECOWITT_MQTT_TLS_INSECURE_SKIP_VERIFY", func(c *models.EcowittConfiguration) *bool { return &c.MQTT.TLS.InsecureSkipVerify })},
}

// fileVars lists variables that also accept a NAME_FILE variant holding a path
// to a file whose contents are the value (Docker secrets style).
var fileVars = []string{"ECOWITT_MQTT_USERNAME", "ECOWITT_MQTT_PASSWORD"}

// Load reads the optional JSON file at path, then applies environment
// overrides via getenv, and validates the result.
func Load(path string, getenv func(string) string) (models.EcowittConfiguration, error) {
	var cfg models.EcowittConfiguration
	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := json.Unmarshal(data, &cfg); err != nil {
				return cfg, fmt.Errorf("invalid config file %s: %w", path, err)
			}
			log.Printf("Loaded configuration from %s", path)
		case errors.Is(err, fs.ErrNotExist):
			log.Printf("Config file %s not found, using environment variables only", path)
		default:
			return cfg, fmt.Errorf("reading config file %s: %w", path, err)
		}
	}

	for _, o := range overrides {
		v := getenv(o.env)
		if v == "" {
			continue
		}
		if err := o.apply(&cfg, v); err != nil {
			return cfg, err
		}
	}

	for _, name := range fileVars {
		fileEnv := name + "_FILE"
		fp := getenv(fileEnv)
		if fp == "" {
			continue
		}
		if getenv(name) != "" {
			return cfg, fmt.Errorf("%s and %s are both set; use only one", name, fileEnv)
		}
		data, err := os.ReadFile(fp)
		if err != nil {
			return cfg, fmt.Errorf("reading %s: %w", fileEnv, err)
		}
		val := strings.TrimRight(string(data), " \t\r\n")
		for _, o := range overrides {
			if o.env == name {
				if err := o.apply(&cfg, val); err != nil {
					return cfg, err
				}
			}
		}
	}

	if cfg.Host == "" {
		return cfg, errors.New(`gateway host is required (set "host" in the config file or ECOWITT_HOST)`)
	}
	return cfg, nil
}
