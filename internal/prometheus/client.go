// Package prometheus exposes gauges through a Prometheus registry.
package prometheus

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type gaugeEntry struct {
	vec  *prometheus.GaugeVec
	keys []string
}

// Client implements metrics.Client and serves the gauges over HTTP.
type Client struct {
	registry    *prometheus.Registry
	defaultTags []string
	mu          sync.Mutex
	gauges      map[string]*gaugeEntry
}

// NewClient creates a Client with its own registry.
func NewClient(defaultTags []string) *Client {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return &Client{
		registry:    reg,
		defaultTags: append([]string(nil), defaultTags...),
		gauges:      map[string]*gaugeEntry{},
	}
}

// Handler serves the client's registry.
func (c *Client) Handler() http.Handler {
	return promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{})
}

func isAlnum(r rune) bool {
	return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// ConvertName converts a name such as ecowitt.outdoorTemperature into a valid
// Prometheus metric name such as ecowitt_outdoor_temperature.
func ConvertName(name string) string {
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		switch {
		case unicode.IsUpper(r) && r < unicode.MaxASCII:
			if i > 0 {
				prev := runes[i-1]
				prevLowerOrDigit := (unicode.IsLower(prev) || unicode.IsDigit(prev)) && prev < unicode.MaxASCII
				nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1]) && runes[i+1] < unicode.MaxASCII
				prevUpper := unicode.IsUpper(prev) && prev < unicode.MaxASCII
				if prevLowerOrDigit || (prevUpper && nextLower) {
					b.WriteByte('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		case isAlnum(r) || r == '_' || r == ':':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "_"
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "_" + out
	}
	return out
}

// SanitizeLabelName makes name a valid Prometheus label name.
func SanitizeLabelName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if isAlnum(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "_"
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "_" + out
	}
	return out
}

func parseLabels(tagSets ...[]string) map[string]string {
	labels := map[string]string{}
	for _, tags := range tagSets {
		for _, tag := range tags {
			k, v, ok := strings.Cut(tag, ":")
			if !ok {
				v = ""
			}
			labels[SanitizeLabelName(k)] = v
		}
	}
	return labels
}

// SendGaugeMetric records a gauge value.
func (c *Client) SendGaugeMetric(name string, tags []string, value float64) {
	metricName := ConvertName(name)
	labels := parseLabels(c.defaultTags, tags)
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.gauges[metricName]
	if !ok {
		vec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: metricName,
			Help: "Ecowitt metric " + name,
		}, keys)
		if err := c.registry.Register(vec); err != nil {
			log.Printf("Unable to register prometheus metric %s: %s", metricName, err)
			return
		}
		entry = &gaugeEntry{vec: vec, keys: keys}
		c.gauges[metricName] = entry
	} else if strings.Join(entry.keys, "\x00") != strings.Join(keys, "\x00") {
		log.Printf("Dropping sample for %s: label keys %v do not match registered keys %v", metricName, keys, entry.keys)
		return
	}
	g, err := entry.vec.GetMetricWith(prometheus.Labels(labels))
	if err != nil {
		log.Printf("Dropping sample for %s: %s", metricName, err)
		return
	}
	g.Set(value)
}

// resetExempt lists gauges that survive a gateway-down reset because they
// describe the gateway's health rather than sensor readings.
var resetExempt = map[string]bool{
	"ecowitt_up":                     true,
	"ecowitt_last_success_timestamp": true,
}

// SetGatewayStatus clears stale sensor series when the gateway goes down.
// When it comes back up the series repopulate on the next scan.
func (c *Client) SetGatewayStatus(up bool) {
	if up {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, entry := range c.gauges {
		if !resetExempt[name] {
			entry.vec.Reset()
		}
	}
}
