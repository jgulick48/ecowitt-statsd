package mqtt

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/jgulick48/ecowitt-statsd/internal/models"
)

type msg struct {
	topic    string
	retained bool
	payload  string
}

type fakePub struct {
	mu   sync.Mutex
	msgs []msg
	err  error
}

func (f *fakePub) Publish(topic string, qos byte, retained bool, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.msgs = append(f.msgs, msg{topic, retained, string(payload)})
	return nil
}

func newTest() (*Client, *fakePub) {
	f := &fakePub{}
	return NewClient(models.MQTTConfig{}, f, "ecowitt_192_168_1_50"), f
}

func TestDiscoveryAndState(t *testing.T) {
	c, f := newTest()
	c.SendGaugeMetric("ecowitt.outdoorTemperature", []string{"unit:F"}, 72.5)
	if len(f.msgs) != 2 {
		t.Fatalf("got %d msgs", len(f.msgs))
	}
	d := f.msgs[0]
	if d.topic != "homeassistant/sensor/ecowitt_192_168_1_50/outdoor_temperature/config" || !d.retained {
		t.Fatalf("bad discovery %+v", d)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(d.payload), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"name": "Outdoor Temperature", "unique_id": "ecowitt_192_168_1_50_outdoor_temperature",
		"object_id": "outdoor_temperature", "state_topic": "ecowitt/outdoor_temperature/state",
		"availability_topic": "ecowitt/status", "unit_of_measurement": "°F",
		"device_class": "temperature", "state_class": "measurement",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %s", k, got[k], v)
		}
	}
	dev := got["device"].(map[string]interface{})
	if dev["name"] != "Ecowitt Gateway" || dev["manufacturer"] != "Ecowitt" || dev["model"] != "Gateway" ||
		dev["identifiers"].([]interface{})[0] != "ecowitt_192_168_1_50" {
		t.Errorf("bad device %v", dev)
	}
	s := f.msgs[1]
	if s.topic != "ecowitt/outdoor_temperature/state" || s.payload != "72.5" || s.retained {
		t.Fatalf("bad state %+v", s)
	}
}

func TestChannelDiscovery(t *testing.T) {
	c, f := newTest()
	c.SendGaugeMetric("ecowitt.channelTemperature", []string{"channel:2", "name:Garage", "unit:F"}, 60)
	var got map[string]interface{}
	_ = json.Unmarshal([]byte(f.msgs[0].payload), &got)
	if f.msgs[0].topic != "homeassistant/sensor/ecowitt_192_168_1_50/channel_temperature_ch2/config" {
		t.Fatalf("topic %s", f.msgs[0].topic)
	}
	if got["name"] != "Garage Temperature" || got["unique_id"] != "ecowitt_192_168_1_50_channel_temperature_ch2" {
		t.Fatalf("bad payload %v", got)
	}
	if f.msgs[1].topic != "ecowitt/channel_temperature_ch2/state" {
		t.Fatalf("state topic %s", f.msgs[1].topic)
	}
}

func TestOmitsEmptyUnitAndSkipsUnknown(t *testing.T) {
	c, f := newTest()
	c.SendGaugeMetric("ecowitt.", nil, 1)
	if len(f.msgs) != 0 {
		t.Fatal("unknown metric published")
	}
	c.SendGaugeMetric("ecowitt.indoorHumidity", nil, 40)
	var got map[string]interface{}
	_ = json.Unmarshal([]byte(f.msgs[0].payload), &got)
	if _, ok := got["unit_of_measurement"]; ok {
		t.Error("unit should be omitted")
	}
	if got["device_class"] != "humidity" {
		t.Error("want humidity")
	}
}

func TestDiscoveryOnceAndReset(t *testing.T) {
	c, f := newTest()
	for i := 0; i < 3; i++ {
		c.SendGaugeMetric("ecowitt.windSpeed", []string{"unit:mph"}, float64(i))
	}
	if len(f.msgs) != 4 { // 1 discovery + 3 states
		t.Fatalf("got %d msgs", len(f.msgs))
	}
	c.ResetDiscovery()
	c.SendGaugeMetric("ecowitt.windSpeed", []string{"unit:mph"}, 9)
	if len(f.msgs) != 6 || f.msgs[4].retained != true {
		t.Fatalf("discovery not resent: %d", len(f.msgs))
	}
}

func TestPublishErrorRetriesDiscoveryAndNilPublisher(t *testing.T) {
	c, f := newTest()
	f.err = errors.New("boom")
	c.SendGaugeMetric("ecowitt.windSpeed", nil, 1)
	f.err = nil
	c.SendGaugeMetric("ecowitt.windSpeed", nil, 1)
	if len(f.msgs) != 2 {
		t.Fatalf("got %d", len(f.msgs))
	}
	NewClient(models.MQTTConfig{}, nil, "x").SendGaugeMetric("ecowitt.windSpeed", nil, 1)
}

func TestMappings(t *testing.T) {
	tests := []struct{ name, unit, class, haUnit string }{
		{"outdoorTemperature", "F", "temperature", "°F"},
		{"dewPoint", "C", "temperature", "°C"},
		{"channelTemperature", "F", "temperature", "°F"},
		{"outdoorHumidity", "%", "humidity", "%"},
		{"channelHumidity", "", "humidity", ""},
		{"relativePressure", "inHg", "atmospheric_pressure", "inHg"},
		{"absolutePressure", "hPa", "atmospheric_pressure", "hPa"},
		{"absolutePressure", "mmHg", "atmospheric_pressure", "mmHg"},
		{"windSpeed", "knots", "wind_speed", "kn"},
		{"windGust", "km/h", "wind_speed", "km/h"},
		{"maxWindGust", "m/s", "wind_speed", "m/s"},
		{"windSpeed", "mph", "wind_speed", "mph"},
		{"windDirection", "", "", "°"},
		{"windDirection", "deg", "", "°"},
		{"rainRate", "in/Hr", "precipitation_intensity", "in/h"},
		{"rainRate", "mm/Hr", "precipitation_intensity", "mm/h"},
		{"rainDay", "in", "precipitation", "in"},
		{"rainYear", "mm", "precipitation", "mm"},
		{"solarIrradiance", "W/m2", "irradiance", "W/m²"},
		{"solarIrradiance", "lux", "illuminance", "lx"},
		{"solarIrradiance", "Klux", "", "Klux"},
		{"solarIrradiance", "fc", "", "fc"},
		{"uvIndex", "", "", "UV index"},
		{"ecowitt.uvIndex", "x", "", "UV index"},
	}
	for _, tc := range tests {
		if got := DeviceClass(tc.name, tc.unit); got != tc.class {
			t.Errorf("DeviceClass(%s,%s)=%q want %q", tc.name, tc.unit, got, tc.class)
		}
		if got := HAUnit(tc.name, tc.unit); got != tc.haUnit {
			t.Errorf("HAUnit(%s,%s)=%q want %q", tc.name, tc.unit, got, tc.haUnit)
		}
	}
}

func TestStateClassAndIDs(t *testing.T) {
	for _, n := range []string{"rainDay", "rainWeek", "rainMonth", "rainYear", "rainEvent"} {
		if StateClass(n) != "total_increasing" {
			t.Errorf("%s", n)
		}
	}
	if StateClass("rainRate") != "measurement" || StateClass("ecowitt.windGust") != "measurement" {
		t.Error("measurement")
	}
	if ObjectName("ecowitt.outdoorTemperature") != "outdoor_temperature" {
		t.Error("object name")
	}
	if DeviceID("192.168.1.50") != "ecowitt_192_168_1_50" || DeviceID("gw.local") != "ecowitt_gw_local" {
		t.Error("device id")
	}
}
