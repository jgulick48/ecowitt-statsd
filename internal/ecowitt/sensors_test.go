package ecowitt

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

type emit struct {
	name  string
	tags  []string
	value float64
}

type tagRecorder struct{ emits []emit }

func (r *tagRecorder) SendGaugeMetric(name string, tags []string, v float64) {
	r.emits = append(r.emits, emit{name, append([]string(nil), tags...), v})
}

func (r *tagRecorder) has(name string, tags []string, v float64) bool {
	want := strings.Join(tags, ",")
	for _, e := range r.emits {
		if e.name == name && strings.Join(e.tags, ",") == want && e.value == v {
			return true
		}
	}
	return false
}

func scanBody(t *testing.T, body string) *tagRecorder {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get_livedata_info" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	rec := &tagRecorder{}
	c := NewClientWithOptions(strings.TrimPrefix(srv.URL, "http://"), http.DefaultClient, rec, Options{}).(*client)
	if err := c.scanMetrics(); err != nil {
		t.Fatal(err)
	}
	return rec
}

func fullScan(t *testing.T) *tagRecorder {
	b, err := os.ReadFile("testdata/livedata_full.json")
	if err != nil {
		t.Fatal(err)
	}
	return scanBody(t, string(b))
}

func TestFullFixtureEmits(t *testing.T) {
	rec := fullScan(t)
	lt, err := time.ParseInLocation("2006-01-02T15:04:05", "2026-11-20T14:03:11", time.Local)
	if err != nil {
		t.Fatal(err)
	}
	ch := func(c, n, u string) []string { return []string{"channel:" + c, "name:" + n, "unit:" + u} }
	bt := func(s, c, u string) []string { return []string{"sensor:" + s, "channel:" + c, "unit:" + u} }
	u := func(x string) []string { return []string{"unit:" + x} }
	tests := []struct {
		name  string
		tags  []string
		value float64
	}{
		// common_list
		{"ecowitt.outdoorTemperature", u("F"), 72.5},
		{"ecowitt.outdoorHumidity", u(""), 55},
		{"ecowitt.windChill", u("F"), 72.5},
		{"ecowitt.heatIndex", u("F"), 74},
		{"ecowitt.vaporPressureDeficit", u("kPa"), 0.612},
		{"ecowitt.windDirectionAvg10m", u(""), 205},
		{"ecowitt.uvRadiation", u("µW/m²"), 45.2},
		{"ecowitt.solarIrradiance", u("W/m2"), 412.3},
		// rain and piezo
		{"ecowitt.rainYear", u("in"), 20.1},
		{"ecowitt.rainRate", u("in/Hr"), 0},
		{"ecowitt.piezoRainEvent", u("in"), 0},
		{"ecowitt.piezoRainRate", u("in/Hr"), 0},
		{"ecowitt.piezoRainDay", u("in"), 0.1},
		{"ecowitt.piezoRainWeek", u("in"), 0.4},
		{"ecowitt.piezoRainMonth", u("in"), 1.1},
		{"ecowitt.piezoRainYear", u("in"), 19.8},
		{"ecowitt.piezoRaining", u(""), 0},
		// wh25
		{"ecowitt.indoorTemperature", u("F"), 70.1},
		{"ecowitt.indoorHumidity", u(""), 40},
		{"ecowitt.absolutePressure", u("inHg"), 29.8},
		{"ecowitt.relativePressure", u("inHg"), 29.95},
		// lightning
		{"ecowitt.lightningDistance", u("mi"), 12},
		{"ecowitt.lightningCount", u(""), 3},
		{"ecowitt.lightningLastStrikeTimestamp", u(""), float64(lt.Unix())},
		// co2
		{"ecowitt.co2", u("ppm"), 512},
		{"ecowitt.co2Avg24h", u("ppm"), 498},
		{"ecowitt.co2Pm25", u("µg/m³"), 5},
		{"ecowitt.co2Pm10", u("µg/m³"), 6},
		{"ecowitt.co2Pm25Aqi", u(""), 21},
		{"ecowitt.co2Pm10Aqi", u(""), 6},
		{"ecowitt.co2Temperature", u("F"), 71.2},
		{"ecowitt.co2Humidity", u("%"), 41},
		// channel sensors
		{"ecowitt.pm25", ch("1", "", "µg/m³"), 6},
		{"ecowitt.pm25Aqi", ch("1", "", ""), 25},
		{"ecowitt.pm25Avg24hAqi", ch("1", "", ""), 22},
		{"ecowitt.leak", ch("1", "Basement", ""), 0},
		{"ecowitt.leak", ch("2", "", ""), 1},
		{"ecowitt.soilMoisture", ch("1", "Garden", "%"), 38},
		{"ecowitt.probeTemperature", ch("1", "Pool", "F"), 78.4},
		{"ecowitt.leafWetness", ch("1", "", "%"), 12},
		// WH31 unchanged
		{"ecowitt.channelTemperature", []string{"channel:2", "name:Garage", "unit:F"}, 61},
		{"ecowitt.channelHumidity", []string{"channel:2", "name:Garage"}, 48},
		// batteries
		{"ecowitt.batteryLow", bt("wh31", "2", ""), 0},
		{"ecowitt.batteryLow", bt("wn34", "1", ""), 0},
		{"ecowitt.batteryVoltage", bt("wn34", "1", "V"), 1.48},
		{"ecowitt.batteryLow", bt("wh55", "1", ""), 0},
		{"ecowitt.batteryLevel", bt("wh55", "1", "%"), 80},
		{"ecowitt.batteryLow", bt("wh55", "2", ""), 1},
		{"ecowitt.batteryLevel", bt("wh55", "2", "%"), 20},
		{"ecowitt.batteryLevel", bt("wh57", "", "%"), 80},
		{"ecowitt.batteryLow", bt("wh45", "", ""), 0},
		{"ecowitt.batteryLevel", bt("wh41", "1", "%"), 100},
		{"ecowitt.batteryLevel", bt("wh51", "1", "%"), 100},
		{"ecowitt.batteryVoltage", bt("wh51", "1", "V"), 1.5},
		{"ecowitt.batteryLevel", bt("wn35", "1", "%"), 100},
		{"ecowitt.batteryVoltage", bt("wn35", "1", "V"), 1.45},
		{"ecowitt.batteryLevel", bt("wh40", "", "%"), 100},
		{"ecowitt.batteryVoltage", bt("wh40", "", "V"), 3.12},
		{"ecowitt.batteryLevel", bt("ws90", "", "%"), 100},
		{"ecowitt.batteryVoltage", bt("ws90", "", "V"), 3.1},
	}
	for _, tc := range tests {
		if !rec.has(tc.name, tc.tags, tc.value) {
			t.Errorf("missing %s %v = %v", tc.name, tc.tags, tc.value)
		}
	}
	// DC powered WH45: low=0 and no level emitted.
	for _, e := range rec.emits {
		if e.name == "ecowitt.batteryLevel" && len(e.tags) > 0 && e.tags[0] == "sensor:wh45" {
			t.Errorf("DC powered sensor must not emit batteryLevel: %+v", e)
		}
	}
}

func TestNoEmptyMetricNameAndConsistentTagKeys(t *testing.T) {
	rec := fullScan(t)
	keys := map[string]string{}
	for _, e := range rec.emits {
		if e.name == "ecowitt." || !strings.HasPrefix(e.name, "ecowitt.") || len(e.name) <= len("ecowitt.") {
			t.Errorf("bad metric name %q", e.name)
		}
		var ks []string
		for _, tag := range e.tags {
			k, _, _ := strings.Cut(tag, ":")
			ks = append(ks, k)
		}
		sort.Strings(ks)
		got := strings.Join(ks, ",")
		if prev, ok := keys[e.name]; ok && prev != got {
			t.Errorf("%s emitted with key sets %q and %q", e.name, prev, got)
		}
		keys[e.name] = got
	}
	if len(rec.emits) < 60 {
		t.Errorf("only %d emits", len(rec.emits))
	}
}

func TestUnknownIDEmitsNothingAndTolerance(t *testing.T) {
	body := `{"common_list":[{"id":"0xZZ","val":"1"},{"id":"0x02","val":"abc"},{"id":5,"val":7},{"id":"0x07"}],
	 "lightning":[{"distance":"--","count":"","date":"nope"}],
	 "ch_leak":[{"channel":"1","status":""}],"co2":[{"battery":"x","voltage":"??"}],
	 "ch_soil":[{"channel":1,"humidity":38,"battery":5}],"rain":[{}],"piezoRain":[{"id":"srain_piezo","val":"x"}]}`
	rec := scanBody(t, body)
	for _, e := range rec.emits {
		if e.name == "ecowitt." || e.name == "ecowitt.outdoorTemperature" {
			t.Errorf("unexpected %+v", e)
		}
	}
	if !rec.has("ecowitt.soilMoisture", []string{"channel:1", "name:", "unit:%"}, 38) {
		t.Error("numeric json values should be accepted")
	}
	if !rec.has("ecowitt.batteryLevel", []string{"sensor:wh51", "channel:1", "unit:%"}, 100) {
		t.Error("numeric battery should be accepted")
	}
}

func TestBatterySemantics(t *testing.T) {
	cases := []struct {
		kind       batteryKind
		bat        string
		low, level float64
		hasLevel   bool
	}{
		{batteryBinary, "0", 0, 0, false},
		{batteryBinary, "1", 1, 0, false},
		{batteryLevel, "0", 1, 0, true},
		{batteryLevel, "1", 1, 20, true},
		{batteryLevel, "2", 0, 40, true},
		{batteryLevel, "5", 0, 100, true},
		{batteryLevel, "6", 0, 0, false},
		{batteryLevel, "9", 0, 100, true},
	}
	for _, tc := range cases {
		rec := &tagRecorder{}
		metricsClient = rec
		emitBattery(tc.kind, "x", "", tc.bat, "")
		var low, level *float64
		for i := range rec.emits {
			switch rec.emits[i].name {
			case "ecowitt.batteryLow":
				low = &rec.emits[i].value
			case "ecowitt.batteryLevel":
				level = &rec.emits[i].value
			}
		}
		if low == nil || *low != tc.low {
			t.Errorf("%v %s: low %v want %v", tc.kind, tc.bat, low, tc.low)
		}
		if (level != nil) != tc.hasLevel || (level != nil && *level != tc.level) {
			t.Errorf("%v %s: level %v want %v/%v", tc.kind, tc.bat, level, tc.level, tc.hasLevel)
		}
	}
}

func TestMinimalPayloadUnchanged(t *testing.T) {
	rec := scanBody(t, liveJSON)
	if len(rec.emits) != 3 { // outdoorTemperature + up + lastSuccessTimestamp
		t.Fatalf("emits %+v", rec.emits)
	}
	if !rec.has("ecowitt.outdoorTemperature", []string{"unit:F"}, 72.5) {
		t.Errorf("emits %+v", rec.emits)
	}
}

func TestLightningTimestampFormats(t *testing.T) {
	if _, ok := parseLightningTime("garbage", ""); ok {
		t.Error("garbage parsed")
	}
	a, ok1 := parseLightningTime("", "11/20/2026 14:03:11")
	b, ok2 := parseLightningTime("2026-11-20T14:03:11", "")
	if !ok1 || !ok2 || !a.Equal(b) {
		t.Errorf("%v %v", a, b)
	}
	z, ok := parseLightningTime("2026-11-20T14:03:11Z", "")
	if !ok || !z.Equal(time.Date(2026, 11, 20, 14, 3, 11, 0, time.UTC)) {
		t.Errorf("%v", z)
	}
}
