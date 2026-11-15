// Package mqtt publishes gauges over MQTT with Home Assistant discovery.
package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/jgulick48/ecowitt-statsd/internal/models"
)

// Publisher publishes a single MQTT message.
type Publisher interface {
	Publish(topic string, qos byte, retained bool, payload []byte) error
}

type pahoPublisher struct{ c paho.Client }

func (p pahoPublisher) Publish(topic string, qos byte, retained bool, payload []byte) error {
	if !p.c.IsConnectionOpen() {
		return fmt.Errorf("not connected, dropping message for %s", topic)
	}
	tok := p.c.Publish(topic, qos, retained, payload)
	if !tok.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("publish to %s timed out", topic)
	}
	return tok.Error()
}

// Client implements metrics.Client, publishing state and HA discovery.
type Client struct {
	cfg      models.MQTTConfig
	deviceID string

	mu        sync.Mutex
	publisher Publisher
	sent      map[string]bool
	closer    func()
}

// NewClient creates a Client. A nil publisher drops all messages.
func NewClient(cfg models.MQTTConfig, publisher Publisher, deviceID string) *Client {
	return &Client{cfg: cfg.WithDefaults(), publisher: publisher, deviceID: deviceID, sent: map[string]bool{}}
}

func (c *Client) statusTopic() string { return c.cfg.TopicPrefix + "/status" }

// ResetDiscovery clears the discovery cache so configs are re-published.
func (c *Client) ResetDiscovery() {
	c.mu.Lock()
	c.sent = map[string]bool{}
	c.mu.Unlock()
}

var pahoLogOnce sync.Once

// enablePahoErrorLogging routes paho's ERROR/CRITICAL output (no-ops by
// default) to the standard logger. Without it, connection failures such as a
// TLS handshake or client-certificate rejection are retried silently and the
// cause never appears in the logs.
func enablePahoErrorLogging() {
	pahoLogOnce.Do(func() {
		paho.ERROR = log.New(log.Writer(), "MQTT error: ", log.LstdFlags)
		paho.CRITICAL = log.New(log.Writer(), "MQTT critical: ", log.LstdFlags)
	})
}

// Connect connects to the broker described by cfg and returns a ready Client.
func Connect(cfg models.MQTTConfig, deviceID string) (*Client, error) {
	enablePahoErrorLogging()
	cfg = cfg.WithDefaults()
	c := NewClient(cfg, nil, deviceID)
	opts := paho.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(30*time.Second).
		SetConnectTimeout(10*time.Second).
		SetWill(c.statusTopic(), "offline", 0, true)
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username).SetPassword(cfg.Password)
	}
	tlsCfg, err := BuildTLSConfig(cfg.TLS)
	if err != nil {
		return nil, err
	}
	if tlsCfg != nil {
		opts.SetTLSConfig(tlsCfg)
		if u, perr := url.Parse(cfg.Broker); perr != nil || !tlsScheme(u.Scheme) {
			log.Printf("MQTT TLS settings will be ignored unless the broker URL uses a TLS scheme such as ssl:// (broker: %s)", cfg.Broker)
		}
	}
	opts.SetOnConnectHandler(func(pc paho.Client) {
		c.ResetDiscovery()
		if err := (pahoPublisher{pc}).Publish(c.statusTopic(), 0, true, []byte("online")); err != nil {
			log.Printf("MQTT online publish failed: %s", err)
		}
	})
	opts.SetConnectionLostHandler(func(_ paho.Client, err error) {
		log.Printf("MQTT connection lost: %s", err)
	})
	pc := paho.NewClient(opts)
	// With ConnectRetry the token only completes once connected, so a broker
	// that is down at startup is retried in the background instead of
	// disabling MQTT. Publishes are dropped until the connection is open.
	tok := pc.Connect()
	if tok.WaitTimeout(15*time.Second) && tok.Error() != nil {
		return nil, tok.Error()
	}
	if !pc.IsConnected() {
		log.Printf("MQTT broker %s not reachable yet, retrying in the background", cfg.Broker)
	}
	c.mu.Lock()
	c.publisher = pahoPublisher{pc}
	c.closer = func() { pc.Disconnect(250) }
	c.mu.Unlock()
	return c, nil
}

// Close publishes offline and disconnects.
func (c *Client) Close() {
	c.mu.Lock()
	p, closer := c.publisher, c.closer
	c.mu.Unlock()
	if p != nil {
		if err := p.Publish(c.statusTopic(), 0, true, []byte("offline")); err != nil {
			log.Printf("MQTT offline publish failed: %s", err)
		}
	}
	if closer != nil {
		closer()
	}
}

func tagValue(tags []string, key string) string {
	for _, t := range tags {
		if k, v, ok := strings.Cut(t, ":"); ok && k == key {
			return v
		}
	}
	return ""
}

func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// SendGaugeMetric publishes discovery (once per object) and the state value.
func (c *Client) SendGaugeMetric(name string, tags []string, value float64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("MQTT panic recovered: %v", r)
		}
	}()
	if baseName(name) == "" {
		return
	}
	c.mu.Lock()
	p := c.publisher
	c.mu.Unlock()
	if p == nil {
		return
	}
	objectID := ObjectName(name)
	channel := tagValue(tags, "channel")
	if channel != "" {
		objectID += "_ch" + sanitizeID(channel)
	}

	c.mu.Lock()
	need := !c.sent[objectID]
	c.mu.Unlock()
	if need {
		unit := tagValue(tags, "unit")
		cfg := map[string]interface{}{
			"name":               humanName(name, channel, tagValue(tags, "name")),
			"unique_id":          c.deviceID + "_" + objectID,
			"default_entity_id":  "sensor." + c.deviceID + "_" + objectID,
			"state_topic":        c.stateTopic(objectID),
			"availability_topic": c.statusTopic(),
			"state_class":        StateClass(name),
			"device": map[string]interface{}{
				"identifiers":  []string{c.deviceID},
				"name":         c.cfg.DeviceName,
				"manufacturer": "Ecowitt",
				"model":        "Gateway",
			},
		}
		if u := HAUnit(name, unit); u != "" {
			cfg["unit_of_measurement"] = u
		}
		if dc := DeviceClass(name, unit); dc != "" {
			cfg["device_class"] = dc
		}
		payload, err := json.Marshal(cfg)
		if err != nil {
			log.Printf("MQTT discovery marshal failed for %s: %s", objectID, err)
			return
		}
		topic := fmt.Sprintf("%s/sensor/%s/%s/config", c.cfg.DiscoveryPrefix, c.deviceID, objectID)
		if err := p.Publish(topic, 0, true, payload); err != nil {
			log.Printf("MQTT discovery publish failed for %s: %s", objectID, err)
		} else {
			c.mu.Lock()
			c.sent[objectID] = true
			c.mu.Unlock()
		}
	}
	if err := p.Publish(c.stateTopic(objectID), 0, c.cfg.Retain, []byte(strconv.FormatFloat(value, 'f', -1, 64))); err != nil {
		log.Printf("MQTT state publish failed for %s: %s", objectID, err)
	}
}

func (c *Client) stateTopic(objectID string) string {
	return c.cfg.TopicPrefix + "/" + objectID + "/state"
}

func tlsScheme(s string) bool {
	switch strings.ToLower(s) {
	case "ssl", "tls", "mqtts", "mqtt+ssl", "tcps", "wss":
		return true
	}
	return false
}
