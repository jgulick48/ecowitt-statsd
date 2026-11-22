package ecowitt

import (
	"encoding/json"
	"fmt"
	"github.com/jgulick48/ecowitt-statsd/internal/metrics"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
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
	LastAttempt         time.Time // set on every scan, successful or not
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
	// Poll right away so data and health appear before the first tick.
	c.scanMetrics()
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
	c.health.LastAttempt = time.Now()
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
	for _, sensorValue := range scanResponse.CommonList {
		sensorValue.EmitMetric()
	}
	for _, wh25 := range scanResponse.WH25 {
		wh25.EmitMetrics()
	}
	for _, channelSensor := range scanResponse.ChAisle {
		channelSensor.EmitMetrics()
	}
	for _, sensorValue := range scanResponse.Rain {
		sensorValue.EmitMetric()
	}
	log.Println("Scanned status")
	return nil
}

type ScanResponse struct {
	CommonList []SensorValue        `json:"common_list"`
	WH25       []WH25               `json:"wh25"`
	ChAisle    []ChannelSensorValue `json:"ch_aisle"`
	Rain       []SensorValue        `json:"rain"`
}

type SensorValue struct {
	ID      string `json:"id"`
	Value   string `json:"val"`
	Battery string `json:"battery,omitempty"`
	Unit    string `json:"unit,omitempty"`
}

func (s *SensorValue) EmitMetric() {
	metricName := s.getSensorTypeFromID()
	valueString := strings.Replace(s.Value, "%", "", -1)
	valueParts := strings.Split(valueString, " ")
	var unit string
	if len(valueParts) == 2 {
		unit = valueParts[1]
	} else if s.Unit != "" {
		unit = s.Unit
	}
	value, err := strconv.ParseFloat(valueParts[0], 64)
	if err != nil {
		fmt.Println("Error parsing value for string " + fmt.Sprintf("%s:%s", valueParts[0], err))
		return
	}
	metricsClient.SendGaugeMetric(fmt.Sprintf("ecowitt.%s", metricName), []string{metrics.FormatTag("unit", unit)}, value)
}

func (s *SensorValue) getSensorTypeFromID() string {
	switch s.ID {
	case "3":
		return "feelsLike"
	case "0x02":
		return "outdoorTemperature"
	case "0x03":
		return "dewPoint"
	case "0x07":
		return "outdoorHumidity"
	case "0x0A":
		return "windDirection"
	case "0x0B":
		return "windSpeed"
	case "0x0C":
		return "windGust"
	case "0x0D":
		return "rainEvent"
	case "0x0E":
		return "rainRate"
	case "0x10":
		return "rainDay"
	case "0x11":
		return "rainWeek"
	case "0x12":
		return "rainMonth"
	case "0x13":
		return "rainYear"
	case "0x15":
		return "solarIrradiance"
	case "0x17":
		return "uvIndex"
	case "0x19":
		return "maxWindGust"
	default:
		return ""
	}
}

type ChannelSensorValue struct {
	Channel  string `json:"channel"`
	Name     string `json:"name"`
	Battery  string `json:"battery"`
	Temp     string `json:"temp"`
	Unit     string `json:"unit"`
	Humidity string `json:"humidity"`
}

func (cs *ChannelSensorValue) EmitMetrics() {
	tags := []string{
		metrics.FormatTag("channel", cs.Channel),
		metrics.FormatTag("name", cs.Name),
	}
	temperature, err := strconv.ParseFloat(cs.Temp, 64)
	if err != nil {
		log.Println("Error parsing value for string " + fmt.Sprintf("%s:%s", cs.Temp, err))
		return
	}
	humidity, err := strconv.ParseFloat(strings.Replace(cs.Humidity, "%", "", -1), 64)
	if err != nil {
		log.Println("Error parsing value for string " + fmt.Sprintf("%s:%s", cs.Temp, err))
		return
	}
	metricsClient.SendGaugeMetric("ecowitt.channelTemperature", append(tags, metrics.FormatTag("unit", cs.Unit)), temperature)
	metricsClient.SendGaugeMetric("ecowitt.channelHumidity", tags, humidity)
}

type WH25 struct {
	InTemp string `json:"intemp"`
	Unit   string `json:"unit"`
	InHumi string `json:"inhumi"`
	Abs    string `json:"abs"`
	Rel    string `json:"rel"`
}

func (w *WH25) EmitMetrics() {
	value, err := strconv.ParseFloat(w.InTemp, 64)
	if err != nil {
		log.Println("Error parsing value for string " + fmt.Sprintf("%s:%s", w.InTemp, err))
		return
	}
	metricsClient.SendGaugeMetric("ecowitt.indoorTemperature", []string{metrics.FormatTag("unit", w.Unit)}, value)
	humidityString := strings.Replace(w.InHumi, "%", "", -1)
	value, err = strconv.ParseFloat(humidityString, 64)
	if err != nil {
		log.Println("Error parsing value for string " + fmt.Sprintf("%s:%s", w.InHumi, err))
		return
	}
	metricsClient.SendGaugeMetric("ecowitt.indoorHumidity", []string{}, value)
	if absPressure, unit, ok := getPressureValue(w.Abs); ok {
		metricsClient.SendGaugeMetric("ecowitt.absolutePressure", []string{metrics.FormatTag("unit", unit)}, absPressure)
	}
	if relPressure, unit, ok := getPressureValue(w.Rel); ok {
		metricsClient.SendGaugeMetric("ecowitt.relativePressure", []string{metrics.FormatTag("unit", unit)}, relPressure)
	}
}

func getPressureValue(pressure string) (float64, string, bool) {
	valueParts := strings.Split(pressure, " ")
	var unit string
	if len(valueParts) == 2 {
		unit = valueParts[1]
	}
	value, err := strconv.ParseFloat(valueParts[0], 64)
	if err != nil {
		log.Println("Error parsing value for string " + fmt.Sprintf("%s:%s", valueParts[0], err))
		return 0, "", false
	}
	return value, unit, true
}
