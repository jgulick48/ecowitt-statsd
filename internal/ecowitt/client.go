package ecowitt

import (
	"encoding/json"
	"fmt"
	"github.com/jgulick48/ecowitt-statsd/internal/metrics"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)

// DefaultFailureThreshold is the number of consecutive failed scans after
// which the gateway is considered down.
const DefaultFailureThreshold = 3

// Health describes the reachability of the gateway.
type Health struct {
	Up                  bool
	LastSuccess         time.Time
	ConsecutiveFailures int
	LastError           string
}

// Options configures a client created with NewClientWithOptions.
type Options struct {
	TickRate         time.Duration
	FailureThreshold int // defaults to DefaultFailureThreshold when <= 0
}

type Client interface {
	StartScan()
	StopScan()
	Health() Health
}

var metricsClient metrics.Client

type client struct {
	address          string
	shouldStop       chan bool
	tickRate         time.Duration
	httpClient       *http.Client
	metrics          metrics.Client
	failureThreshold int

	mu     sync.Mutex
	health Health
}

func NewClient(address string, tickRate time.Duration, httpClient *http.Client, metrics metrics.Client) Client {
	return NewClientWithOptions(address, httpClient, metrics, Options{TickRate: tickRate})
}

func NewClientWithOptions(address string, httpClient *http.Client, metrics metrics.Client, opts Options) Client {
	metricsClient = metrics
	if opts.FailureThreshold <= 0 {
		opts.FailureThreshold = DefaultFailureThreshold
	}
	return &client{
		address:          address,
		shouldStop:       make(chan bool),
		tickRate:         opts.TickRate,
		httpClient:       httpClient,
		metrics:          metrics,
		failureThreshold: opts.FailureThreshold,
	}
}

// Health returns a snapshot of the gateway health.
func (c *client) Health() Health {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.health
}

func (c *client) StartScan() {
	t := time.NewTicker(c.tickRate)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			c.scanMetrics()
			continue
		case <-c.shouldStop:
		}
		break
	}
}
func (c *client) StopScan() {
	c.shouldStop <- true
}

// recordResult updates health after a scan and notifies the metrics client of
// up/down transitions.
func (c *client) recordResult(err error) {
	c.mu.Lock()
	wasUp := c.health.Up
	if err == nil {
		c.health.Up = true
		c.health.LastSuccess = time.Now()
		c.health.ConsecutiveFailures = 0
		c.health.LastError = ""
	} else {
		c.health.ConsecutiveFailures++
		c.health.LastError = err.Error()
		if c.health.ConsecutiveFailures >= c.failureThreshold {
			c.health.Up = false
		}
	}
	h := c.health
	c.mu.Unlock()

	if c.metrics == nil {
		return
	}
	if h.Up != wasUp {
		if sr, ok := c.metrics.(metrics.StatusReceiver); ok {
			sr.SetGatewayStatus(h.Up)
		}
	}
	upValue := 0.0
	if h.Up {
		upValue = 1
	}
	c.metrics.SendGaugeMetric("ecowitt.up", nil, upValue)
	if err == nil {
		c.metrics.SendGaugeMetric("ecowitt.lastSuccessTimestamp", nil, float64(h.LastSuccess.Unix()))
	}
}

func (c *client) scanMetrics() error {
	err := c.fetchAndEmit()
	c.recordResult(err)
	return err
}

func (c *client) fetchAndEmit() error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/get_livedata_info?", c.address), nil)
	if err != nil {
		fmt.Println("Error getting data from gateway " + fmt.Sprintf("%s", err))
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Println("Error getting data from gateway " + fmt.Sprintf("%s", err))
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Println("Error getting data from gateway, unexpected status code " + fmt.Sprintf("%v", resp.StatusCode))
		return fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	var scanResponse ScanResponse
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error getting data from gateway " + fmt.Sprintf("%s", err))
		return err
	}
	err = json.Unmarshal(body, &scanResponse)
	if err != nil {
		log.Println("Error unmarshalling json " + fmt.Sprintf("%s", err))
		return err
	}
	scanResponse.emitAll()
	log.Println("Scanned status")
	return nil
}
