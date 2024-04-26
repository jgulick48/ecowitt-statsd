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
	"time"
)

type Client interface {
	StartScan()
	StopScan()
}

var metricsClient metrics.Client

type client struct {
	address    string
	shouldStop chan bool
	tickRate   time.Duration
	httpClient *http.Client
	metrics    metrics.Client
}

func NewClient(address string, tickRate time.Duration, httpClient *http.Client, metrics metrics.Client) Client {
	metricsClient = metrics
	return &client{
		address:    address,
		shouldStop: make(chan bool),
		tickRate:   tickRate,
		httpClient: httpClient,
	}
}

func (c *client) StartScan() {
	t := time.NewTicker(c.tickRate)
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

func (c *client) scanMetrics() {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/get_livedata_info?", c.address), nil)
	if err != nil {
		fmt.Println("Error getting data from gateway " + fmt.Sprintf("%s", err))
		return
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Println("Error getting data from gateway " + fmt.Sprintf("%s", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Println("Error getting data from gateway, unexpected status code " + fmt.Sprintf("%v", resp.StatusCode))
		return
	}
	var scanResponse ScanResponse
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error getting data from gateway " + fmt.Sprintf("%s", err))
		return
	}
	err = json.Unmarshal(body, &scanResponse)
	if err != nil {
		log.Println("Error unmarshalling json " + fmt.Sprintf("%s", err))
		return
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
