package ecowitt

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jgulick48/ecowitt-statsd/internal/metrics"
)

// flex is a string that also accepts JSON numbers, booleans and null, so a
// firmware that sends 5 instead of "5" does not break the whole scan.
type flex string

func (f *flex) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	switch {
	case s == "" || s == "null" || s[0] == '{' || s[0] == '[':
		*f = ""
	case s[0] == '"':
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = flex(v)
	default:
		*f = flex(s)
	}
	return nil
}

// ScanResponse is the payload of GET /get_livedata_info. Parsing is tolerant:
// sections and fields that a firmware does not provide are simply empty.
type ScanResponse struct {
	CommonList []SensorValue        `json:"common_list"`
	WH25       []WH25               `json:"wh25"`
	ChAisle    []ChannelSensorValue `json:"ch_aisle"`
	Rain       []SensorValue        `json:"rain"`
	PiezoRain  []SensorValue        `json:"piezoRain"`
	Lightning  []Lightning          `json:"lightning"`
	CO2        []CO2                `json:"co2"`
	ChPM25     []ChannelPM25        `json:"ch_pm25"`
	ChLeak     []ChannelLeak        `json:"ch_leak"`
	ChSoil     []ChannelSoil        `json:"ch_soil"`
	ChTemp     []ChannelTemp        `json:"ch_temp"`
	ChLeaf     []ChannelLeaf        `json:"ch_leaf"`
}

func (r *ScanResponse) emitAll() {
	for i := range r.CommonList {
		r.CommonList[i].emit("")
	}
	for i := range r.WH25 {
		r.WH25[i].EmitMetrics()
	}
	for i := range r.ChAisle {
		r.ChAisle[i].EmitMetrics()
	}
	emitSensorSection(r.Rain, "", "wh40")
	emitSensorSection(r.PiezoRain, "piezo", "ws90")
	for i := range r.Lightning {
		r.Lightning[i].EmitMetrics()
	}
	for i := range r.CO2 {
		r.CO2[i].EmitMetrics()
	}
	for i := range r.ChPM25 {
		r.ChPM25[i].EmitMetrics()
	}
	for i := range r.ChLeak {
		r.ChLeak[i].EmitMetrics()
	}
	for i := range r.ChSoil {
		r.ChSoil[i].EmitMetrics()
	}
	for i := range r.ChTemp {
		r.ChTemp[i].EmitMetrics()
	}
	for i := range r.ChLeaf {
		r.ChLeaf[i].EmitMetrics()
	}
}

// emitSensorSection emits a rain-like section. The gateway repeats the yearly
// entry carrying the battery, so each metric is emitted once per section.
func emitSensorSection(values []SensorValue, prefix, batterySensor string) {
	seen := map[string]bool{}
	for i := range values {
		s := &values[i]
		if name := s.metricName(prefix); name == "" || !seen[name] {
			seen[name] = true
			s.emit(prefix)
		}
		emitBattery(batteryLevel, batterySensor, "", string(s.Battery), string(s.Voltage))
	}
}

// ---- gauges and helpers ----

func send(name string, tags []string, value float64) {
	if metricsClient == nil || name == "" {
		return
	}
	metricsClient.SendGaugeMetric("ecowitt."+name, tags, value)
}

func unitTags(unit string) []string { return []string{metrics.FormatTag("unit", unit)} }

func channelTags(channel, name, unit string) []string {
	return []string{
		metrics.FormatTag("channel", channel),
		metrics.FormatTag("name", name),
		metrics.FormatTag("unit", unit),
	}
}

// parseNumber parses values such as "72.5", "55%", "12 mi" or "0.5 kPa" into a
// number and an optional unit. Empty and "--" placeholders are skipped silently.
func parseNumber(raw string) (float64, string, bool) {
	s := strings.TrimSpace(strings.Replace(raw, "%", "", -1))
	if s == "" || s == "--" || s == "-" {
		return 0, "", false
	}
	parts := strings.Fields(s)
	v, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		log.Printf("Error parsing value %q: %v", raw, err)
		return 0, "", false
	}
	unit := ""
	if len(parts) > 1 {
		unit = parts[1]
	}
	return v, unit, true
}

// noUnit forces an empty unit tag in emitValue.
const noUnit = "-"

// emitValue parses raw and emits it as name with the given tags. When unit is
// non-empty it overrides any unit found in the raw value.
func emitValue(name, raw string, tags func(unit string) []string, unit string) {
	v, u, ok := parseNumber(raw)
	if !ok {
		return
	}
	if unit == noUnit {
		u = ""
	} else if unit != "" {
		u = unit
	}
	send(name, tags(u), v)
}

// ---- batteries ----

type batteryKind int

const (
	// batteryBinary: 0 = OK, 1 = low.
	batteryBinary batteryKind = iota
	// batteryLevel: 0-5 (5 = full, <=1 = low), 6 = DC/USB powered.
	batteryLevel
)

func batteryTags(sensor, channel, unit string) []string {
	return []string{
		metrics.FormatTag("sensor", sensor),
		metrics.FormatTag("channel", channel),
		metrics.FormatTag("unit", unit),
	}
}

// emitBattery emits batteryLow, batteryLevel (level kinds) and batteryVoltage
// for one sensor. Empty or unparseable fields are skipped.
func emitBattery(kind batteryKind, sensor, channel, battery, voltage string) {
	battery = strings.TrimSpace(battery)
	if battery != "" {
		if b, err := strconv.ParseFloat(battery, 64); err != nil || math.IsNaN(b) || math.IsInf(b, 0) {
			log.Printf("Error parsing battery %q for %s: %v", battery, sensor, err)
		} else {
			low := 0.0
			switch kind {
			case batteryBinary:
				if b != 0 {
					low = 1
				}
			case batteryLevel:
				if b == 6 {
					break // DC powered: never low, no level
				}
				if b <= 1 {
					low = 1
				}
				send("batteryLevel", batteryTags(sensor, channel, "%"), math.Max(0, math.Min(100, b*20)))
			}
			send("batteryLow", batteryTags(sensor, channel, ""), low)
		}
	}
	if v, _, ok := parseNumber(strings.TrimSuffix(strings.TrimSpace(voltage), "V")); ok {
		send("batteryVoltage", batteryTags(sensor, channel, "V"), v)
	}
}

// ---- common_list / rain / piezoRain ----

type SensorValue struct {
	ID      flex `json:"id"`
	Value   flex `json:"val"`
	Battery flex `json:"battery,omitempty"`
	Voltage flex `json:"voltage,omitempty"`
	Unit    flex `json:"unit,omitempty"`
}

var (
	unknownMu  sync.Mutex
	unknownIDs = map[string]bool{}
)

func logUnknownID(id, val string) {
	unknownMu.Lock()
	seen := unknownIDs[id]
	unknownIDs[id] = true
	unknownMu.Unlock()
	if !seen {
		log.Printf("Unknown sensor id %q (value %q) ignored; please report it at https://github.com/jgulick48/ecowitt-statsd/issues so it can be added", id, val)
	}
}

// EmitMetric emits a common_list entry.
func (s *SensorValue) EmitMetric() { s.emit("") }

func (s *SensorValue) emit(prefix string) {
	if string(s.ID) == "srain_piezo" {
		emitValue("piezoRaining", string(s.Value), unitTags, "")
		return
	}
	name := s.metricName(prefix)
	if name == "" {
		if strings.TrimSpace(string(s.ID)) == "" {
			return
		}
		logUnknownID(string(s.ID), string(s.Value))
		return
	}
	v, unit, ok := parseNumber(string(s.Value))
	if !ok {
		return
	}
	if unit == "" {
		unit = string(s.Unit)
	}
	if unit == "" && name == "uvRadiation" {
		unit = "µW/m²"
	}
	send(name, unitTags(unit), v)
}

// metricName returns the metric name (without ecowitt.) for the sensor ID, or
// "" when the ID is unknown or is handled elsewhere. A prefix of "piezo"
// renames the rain metrics (rainDay becomes piezoRainDay).
func (s *SensorValue) metricName(prefix string) string {
	if string(s.ID) == "srain_piezo" {
		return ""
	}
	name := s.getSensorTypeFromID()
	if prefix != "" && strings.HasPrefix(name, "rain") {
		name = prefix + strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}

func (s *SensorValue) getSensorTypeFromID() string {
	switch string(s.ID) {
	case "0x01":
		return "indoorTemperature"
	case "0x02":
		return "outdoorTemperature"
	case "3":
		return "feelsLike"
	case "0x03":
		return "dewPoint"
	case "0x04":
		return "windChill"
	case "0x05":
		return "heatIndex"
	case "5":
		return "vaporPressureDeficit"
	case "0x06":
		return "indoorHumidity"
	case "0x07":
		return "outdoorHumidity"
	case "0x08":
		return "absolutePressure"
	case "0x09":
		return "relativePressure"
	case "0x0A":
		return "windDirection"
	case "0x6D":
		return "windDirectionAvg10m"
	case "0x0B":
		return "windSpeed"
	case "0x0C":
		return "windGust"
	case "0x0D":
		return "rainEvent"
	case "0x0E":
		return "rainRate"
	case "0x0F":
		return "rainHour"
	case "0x10":
		return "rainDay"
	case "0x11":
		return "rainWeek"
	case "0x12":
		return "rainMonth"
	case "0x13":
		return "rainYear"
	case "0x14":
		return "rainTotal"
	case "0x15":
		return "solarIrradiance"
	case "0x16":
		return "uvRadiation"
	case "0x17":
		return "uvIndex"
	case "0x19":
		return "maxWindGust"
	default:
		return ""
	}
}

// ---- WH31 (ch_aisle) ----

type ChannelSensorValue struct {
	Channel  flex `json:"channel"`
	Name     flex `json:"name"`
	Battery  flex `json:"battery"`
	Voltage  flex `json:"voltage"`
	Temp     flex `json:"temp"`
	Unit     flex `json:"unit"`
	Humidity flex `json:"humidity"`
}

func (cs *ChannelSensorValue) EmitMetrics() {
	emitBattery(batteryBinary, "wh31", string(cs.Channel), string(cs.Battery), string(cs.Voltage))
	tags := []string{
		metrics.FormatTag("channel", string(cs.Channel)),
		metrics.FormatTag("name", string(cs.Name)),
	}
	temperature, err := strconv.ParseFloat(string(cs.Temp), 64)
	if err != nil {
		log.Println("Error parsing value for string " + fmt.Sprintf("%s:%s", cs.Temp, err))
		return
	}
	humidity, err := strconv.ParseFloat(strings.Replace(string(cs.Humidity), "%", "", -1), 64)
	if err != nil {
		log.Println("Error parsing value for string " + fmt.Sprintf("%s:%s", cs.Humidity, err))
		return
	}
	send("channelTemperature", append(tags, metrics.FormatTag("unit", string(cs.Unit))), temperature)
	send("channelHumidity", tags, humidity)
}

// ---- WH25 ----

type WH25 struct {
	InTemp  flex `json:"intemp"`
	Unit    flex `json:"unit"`
	InHumi  flex `json:"inhumi"`
	Abs     flex `json:"abs"`
	Rel     flex `json:"rel"`
	Battery flex `json:"battery"`
	Voltage flex `json:"voltage"`
}

func (w *WH25) EmitMetrics() {
	emitBattery(batteryBinary, "wh25", "", string(w.Battery), string(w.Voltage))
	if v, _, ok := parseNumber(string(w.InTemp)); ok {
		send("indoorTemperature", unitTags(string(w.Unit)), v)
	}
	// Same tag keys as the common_list indoorHumidity (0x06) emit.
	emitValue("indoorHumidity", string(w.InHumi), unitTags, "")
	emitValue("absolutePressure", string(w.Abs), unitTags, "")
	emitValue("relativePressure", string(w.Rel), unitTags, "")
}

// ---- lightning (WH57) ----

type Lightning struct {
	Distance  flex `json:"distance"`
	Date      flex `json:"date"`
	Timestamp flex `json:"timestamp"`
	Count     flex `json:"count"`
	Battery   flex `json:"battery"`
	Voltage   flex `json:"voltage"`
}

var lightningLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"01/02/2006 15:04:05",
	"1/2/2006 15:04:05",
}

func parseLightningTime(vals ...string) (time.Time, bool) {
	for _, raw := range vals {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		for _, layout := range lightningLayouts {
			if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func (l *Lightning) EmitMetrics() {
	emitBattery(batteryLevel, "wh57", "", string(l.Battery), string(l.Voltage))
	emitValue("lightningDistance", string(l.Distance), unitTags, "")
	emitValue("lightningCount", string(l.Count), unitTags, noUnit)
	if t, ok := parseLightningTime(string(l.Date), string(l.Timestamp)); ok {
		send("lightningLastStrikeTimestamp", unitTags(""), float64(t.Unix()))
	}
}

// ---- WH45 (co2) ----

type CO2 struct {
	TempF    flex `json:"temperf"`
	TempC    flex `json:"tempc"`
	Humidity flex `json:"humidity"`
	PM25     flex `json:"PM25"`
	PM25AQI  flex `json:"PM25_RealAQI"`
	PM10     flex `json:"PM10"`
	PM10AQI  flex `json:"PM10_RealAQI"`
	CO2      flex `json:"CO2"`
	CO224H   flex `json:"CO2_24H"`
	Battery  flex `json:"battery"`
	Voltage  flex `json:"voltage"`
}

func (c *CO2) EmitMetrics() {
	emitBattery(batteryLevel, "wh45", "", string(c.Battery), string(c.Voltage))
	if string(c.TempF) != "" {
		emitValue("co2Temperature", string(c.TempF), unitTags, "F")
	} else {
		emitValue("co2Temperature", string(c.TempC), unitTags, "C")
	}
	emitValue("co2Humidity", string(c.Humidity), unitTags, "%")
	emitValue("co2Pm25", string(c.PM25), unitTags, "µg/m³")
	emitValue("co2Pm25Aqi", string(c.PM25AQI), unitTags, noUnit)
	emitValue("co2Pm10", string(c.PM10), unitTags, "µg/m³")
	emitValue("co2Pm10Aqi", string(c.PM10AQI), unitTags, noUnit)
	emitValue("co2", string(c.CO2), unitTags, "ppm")
	emitValue("co2Avg24h", string(c.CO224H), unitTags, "ppm")
}

// ---- channel sensors ----

func channelEmit(name, channel, label, raw, unit string) {
	emitValue(name, raw, func(u string) []string { return channelTags(channel, label, u) }, unit)
}

// ChannelPM25 is a WH41/WH43 PM2.5 sensor.
type ChannelPM25 struct {
	Channel flex `json:"channel"`
	Name    flex `json:"name"`
	PM25    flex `json:"PM25"`
	AQI     flex `json:"PM25_RealAQI"`
	AQI24H  flex `json:"PM25_24HAQI"`
	Battery flex `json:"battery"`
	Voltage flex `json:"voltage"`
}

func (c *ChannelPM25) EmitMetrics() {
	ch, name := string(c.Channel), string(c.Name)
	emitBattery(batteryLevel, "wh41", ch, string(c.Battery), string(c.Voltage))
	channelEmit("pm25", ch, name, string(c.PM25), "µg/m³")
	channelEmit("pm25Aqi", ch, name, string(c.AQI), noUnit)
	channelEmit("pm25Avg24hAqi", ch, name, string(c.AQI24H), noUnit)
}

// ChannelLeak is a WH55 leak sensor.
type ChannelLeak struct {
	Channel flex `json:"channel"`
	Name    flex `json:"name"`
	Battery flex `json:"battery"`
	Voltage flex `json:"voltage"`
	Status  flex `json:"status"`
}

func (c *ChannelLeak) EmitMetrics() {
	ch, name := string(c.Channel), string(c.Name)
	emitBattery(batteryLevel, "wh55", ch, string(c.Battery), string(c.Voltage))
	status := strings.TrimSpace(string(c.Status))
	if status == "" {
		return
	}
	leak := 1.0
	if strings.EqualFold(status, "normal") {
		leak = 0
	}
	send("leak", channelTags(ch, name, ""), leak)
}

// ChannelSoil is a WH51 soil moisture sensor.
type ChannelSoil struct {
	Channel  flex `json:"channel"`
	Name     flex `json:"name"`
	Battery  flex `json:"battery"`
	Voltage  flex `json:"voltage"`
	Humidity flex `json:"humidity"`
}

func (c *ChannelSoil) EmitMetrics() {
	ch, name := string(c.Channel), string(c.Name)
	emitBattery(batteryLevel, "wh51", ch, string(c.Battery), string(c.Voltage))
	channelEmit("soilMoisture", ch, name, string(c.Humidity), "%")
}

// ChannelTemp is a WN34 temperature probe.
type ChannelTemp struct {
	Channel flex `json:"channel"`
	Name    flex `json:"name"`
	Battery flex `json:"battery"`
	Voltage flex `json:"voltage"`
	Temp    flex `json:"temp"`
	Unit    flex `json:"unit"`
}

func (c *ChannelTemp) EmitMetrics() {
	ch, name := string(c.Channel), string(c.Name)
	emitBattery(batteryBinary, "wn34", ch, string(c.Battery), string(c.Voltage))
	v, u, ok := parseNumber(string(c.Temp))
	if !ok {
		return
	}
	if string(c.Unit) != "" {
		u = string(c.Unit)
	}
	send("probeTemperature", channelTags(ch, name, u), v)
}

// ChannelLeaf is a WN35 leaf wetness sensor.
type ChannelLeaf struct {
	Channel  flex `json:"channel"`
	Name     flex `json:"name"`
	Battery  flex `json:"battery"`
	Voltage  flex `json:"voltage"`
	Humidity flex `json:"humidity"`
}

func (c *ChannelLeaf) EmitMetrics() {
	ch, name := string(c.Channel), string(c.Name)
	emitBattery(batteryLevel, "wn35", ch, string(c.Battery), string(c.Voltage))
	channelEmit("leafWetness", ch, name, string(c.Humidity), "%")
}
