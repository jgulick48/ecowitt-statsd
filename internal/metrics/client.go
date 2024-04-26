package metrics

import (
	"fmt"
	"log"

	"github.com/DataDog/datadog-go/statsd"
)

func FormatTag(key, value string) string {
	return fmt.Sprintf("%s:%s", key, value)
}
func (c *client) SendGaugeMetric(name string, tags []string, value float64) {
	if c.statsEnabled {
		err := c.statsdClient.Gauge(name, value, append(c.defaultTags, tags...), 1)
		if err != nil {
			log.Printf("Got error trying to send metric %s", err.Error())
		}
	}
}

type Client interface {
	SendGaugeMetric(name string, tags []string, value float64)
}

type client struct {
	statsdClient *statsd.Client
	statsEnabled bool
	defaultTags  []string
}

func NewClient(metricsClient *statsd.Client, tags []string) Client {
	return &client{
		statsdClient: metricsClient,
		defaultTags:  tags,
		statsEnabled: true,
	}
}
