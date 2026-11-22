package mqtt

import (
	"encoding/json"
	"testing"
)

func TestNewMappings(t *testing.T) {
	tests := []struct{ name, unit, class, haUnit string }{
		{"windChill", "F", "temperature", "°F"},
		{"heatIndex", "C", "temperature", "°C"},
		{"co2Temperature", "F", "temperature", "°F"},
		{"probeTemperature", "F", "temperature", "°F"},
		{"co2Humidity", "%", "humidity", "%"},
		{"co2Humidity", "", "humidity", "%"},
		{"soilMoisture", "%", "moisture", "%"},
		{"leafWetness", "", "moisture", "%"},
		{"vaporPressureDeficit", "kPa", "pressure", "kPa"},
		{"windDirectionAvg10m", "", "", "°"},
		{"uvRadiation", "W/m2", "irradiance", "W/m²"},
		{"uvRadiation", "µW/m²", "", "µW/m²"},
		{"rainHour", "in", "precipitation", "in"},
		{"rainTotal", "mm", "precipitation", "mm"},
		{"piezoRainEvent", "in", "precipitation", "in"},
		{"piezoRainYear", "in", "precipitation", "in"},
		{"piezoRainRate", "in/Hr", "precipitation_intensity", "in/h"},
		{"lightningDistance", "mi", "distance", "mi"},
		{"lightningDistance", "km", "distance", "km"},
		{"lightningCount", "", "", ""},
		{"lightningLastStrikeTimestamp", "", "timestamp", ""},
		{"co2", "ppm", "carbon_dioxide", "ppm"},
		{"co2Avg24h", "ppm", "carbon_dioxide", "ppm"},
		{"pm25", "µg/m³", "pm25", "µg/m³"},
		{"co2Pm25", "µg/m³", "pm25", "µg/m³"},
		{"co2Pm10", "µg/m³", "pm10", "µg/m³"},
		{"co2Pm25Aqi", "", "aqi", ""},
		{"co2Pm10Aqi", "", "aqi", ""},
		{"pm25Aqi", "", "aqi", ""},
		{"pm25Avg24hAqi", "", "aqi", ""},
		{"batteryLevel", "%", "battery", "%"},
		{"batteryVoltage", "V", "voltage", "V"},
		{"batteryLow", "", "battery", ""},
		{"leak", "", "moisture", ""},
		{"piezoRaining", "", "moisture", ""},
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

func TestComponentAndStateClass(t *testing.T) {
	for _, n := range []string{"leak", "batteryLow", "piezoRaining", "ecowitt.leak"} {
		if Component(n) != "binary_sensor" || StateClass(n) != "" {
			t.Errorf("%s", n)
		}
	}
	for _, n := range []string{"outdoorTemperature", "batteryLevel", "lightningCount"} {
		if Component(n) != "sensor" {
			t.Errorf("%s", n)
		}
	}
	for _, n := range []string{"rainHour", "rainTotal", "piezoRainDay", "piezoRainYear", "lightningCount"} {
		if StateClass(n) != "total_increasing" {
			t.Errorf("%s", n)
		}
	}
	if StateClass("piezoRainRate") != "measurement" || StateClass("lightningLastStrikeTimestamp") != "" {
		t.Error("state class")
	}
	if EntityCategory("batteryLow") != "diagnostic" || EntityCategory("windSpeed") != "" {
		t.Error("entity category")
	}
}

func discoveryOf(t *testing.T, m msg) map[string]interface{} {
	t.Helper()
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(m.payload), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestBinarySensorDiscovery(t *testing.T) {
	c, f := newTest()
	c.SendGaugeMetric("ecowitt.leak", []string{"channel:1", "name:Basement", "unit:"}, 1)
	if f.msgs[0].topic != "homeassistant/binary_sensor/ecowitt_192_168_1_50/leak_ch1/config" {
		t.Fatalf("topic %s", f.msgs[0].topic)
	}
	got := discoveryOf(t, f.msgs[0])
	if got["payload_on"] != "1" || got["payload_off"] != "0" || got["device_class"] != "moisture" || got["name"] != "Basement Leak" {
		t.Errorf("payload %v", got)
	}
	for _, k := range []string{"state_class", "unit_of_measurement", "entity_category"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s must be absent", k)
		}
	}
	if got["default_entity_id"] != "binary_sensor.ecowitt_192_168_1_50_leak_ch1" {
		t.Errorf("entity id %v", got["default_entity_id"])
	}
	if f.msgs[1].topic != "ecowitt/leak_ch1/state" || f.msgs[1].payload != "1" {
		t.Errorf("state %+v", f.msgs[1])
	}
}

func TestBatteryDiscovery(t *testing.T) {
	c, f := newTest()
	c.SendGaugeMetric("ecowitt.batteryLevel", []string{"sensor:wh41", "channel:1", "unit:%"}, 80)
	c.SendGaugeMetric("ecowitt.batteryLow", []string{"sensor:wh31", "channel:2", "unit:"}, 0)
	c.SendGaugeMetric("ecowitt.batteryVoltage", []string{"sensor:ws90", "channel:", "unit:V"}, 3.1)
	wantTopics := []string{
		"homeassistant/sensor/ecowitt_192_168_1_50/battery_level_wh41_ch1/config",
		"homeassistant/binary_sensor/ecowitt_192_168_1_50/battery_low_wh31_ch2/config",
		"homeassistant/sensor/ecowitt_192_168_1_50/battery_voltage_ws90/config",
	}
	for i, idx := range []int{0, 2, 4} {
		if f.msgs[idx].topic != wantTopics[i] {
			t.Errorf("topic %s want %s", f.msgs[idx].topic, wantTopics[i])
		}
		if discoveryOf(t, f.msgs[idx])["entity_category"] != "diagnostic" {
			t.Errorf("%s: entity_category", wantTopics[i])
		}
	}
	lvl := discoveryOf(t, f.msgs[0])
	if lvl["name"] != "WH41 CH1 Battery" || lvl["device_class"] != "battery" || lvl["unit_of_measurement"] != "%" || lvl["state_class"] != "measurement" {
		t.Errorf("level %v", lvl)
	}
	if n := discoveryOf(t, f.msgs[2])["name"]; n != "WH31 CH2 Battery Low" {
		t.Errorf("name %v", n)
	}
	v := discoveryOf(t, f.msgs[4])
	if v["name"] != "WS90 Battery Voltage" || v["device_class"] != "voltage" || v["unit_of_measurement"] != "V" {
		t.Errorf("voltage %v", v)
	}
}

func TestLightningTimestampState(t *testing.T) {
	c, f := newTest()
	c.SendGaugeMetric("ecowitt.lightningLastStrikeTimestamp", []string{"unit:"}, 1763647391)
	got := discoveryOf(t, f.msgs[0])
	if got["device_class"] != "timestamp" {
		t.Errorf("class %v", got["device_class"])
	}
	for _, k := range []string{"state_class", "unit_of_measurement"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s must be absent", k)
		}
	}
	if f.msgs[1].payload != "2025-11-20T14:03:11Z" {
		t.Errorf("state %q", f.msgs[1].payload)
	}
}

func TestLightningCountDiscovery(t *testing.T) {
	c, f := newTest()
	c.SendGaugeMetric("ecowitt.lightningCount", []string{"unit:"}, 3)
	got := discoveryOf(t, f.msgs[0])
	if got["state_class"] != "total_increasing" {
		t.Errorf("%v", got)
	}
	if _, ok := got["unit_of_measurement"]; ok {
		t.Error("unit")
	}
	if _, ok := got["device_class"]; ok {
		t.Error("class")
	}
}
