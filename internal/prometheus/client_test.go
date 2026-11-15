package prometheus

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestConvertName(t *testing.T) {
	tests := map[string]string{
		"ecowitt.outdoorTemperature": "ecowitt_outdoor_temperature",
		"ecowitt.channelHumidity":    "ecowitt_channel_humidity",
		"ecowitt.rain":               "ecowitt_rain",
		"a-b c":                      "a_b_c",
		"HTTPServer":                 "http_server",
		"temp2F":                     "temp2_f",
		"1abc":                       "_1abc",
		"":                           "_",
	}
	for in, want := range tests {
		if got := ConvertName(in); got != want {
			t.Errorf("ConvertName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeLabelName(t *testing.T) {
	tests := map[string]string{"unit": "unit", "a.b-c": "a_b_c", "1x": "_1x", "": "_"}
	for in, want := range tests {
		if got := SanitizeLabelName(in); got != want {
			t.Errorf("SanitizeLabelName(%q) = %q, want %q", in, got, want)
		}
	}
}

func scrape(c *Client) string {
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	return rec.Body.String()
}

func TestHandlerOutput(t *testing.T) {
	c := NewClient([]string{"site:home"})
	c.SendGaugeMetric("ecowitt.outdoorTemperature", []string{"unit:F"}, 72.5)
	c.SendGaugeMetric("ecowitt.channelHumidity", []string{"channel:1", "name:Garage"}, 40)
	body := scrape(c)
	for _, want := range []string{
		`ecowitt_outdoor_temperature{site="home",unit="F"} 72.5`,
		`ecowitt_channel_humidity{channel="1",name="Garage",site="home"} 40`,
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("output missing %q:\n%s", want, body)
		}
	}
}

func TestHandlerOutputNoDefaultTags(t *testing.T) {
	c := NewClient(nil)
	c.SendGaugeMetric("ecowitt.outdoorTemperature", []string{"unit:F"}, 72.5)
	if body := scrape(c); !strings.Contains(body, `ecowitt_outdoor_temperature{unit="F"} 72.5`) {
		t.Errorf("unexpected output:\n%s", body)
	}
}

func TestMismatchedLabelsDropped(t *testing.T) {
	c := NewClient(nil)
	c.SendGaugeMetric("ecowitt.x", []string{"unit:F"}, 1)
	c.SendGaugeMetric("ecowitt.x", []string{"unit:F", "extra:y"}, 2)
	c.SendGaugeMetric("ecowitt.x", nil, 3)
	body := scrape(c)
	if !strings.Contains(body, `ecowitt_x{unit="F"} 1`) {
		t.Errorf("original sample missing:\n%s", body)
	}
	if strings.Contains(body, "extra") || strings.Contains(body, "} 3") || strings.Contains(body, "} 2") {
		t.Errorf("mismatched sample not dropped:\n%s", body)
	}
}

func TestConcurrentUse(t *testing.T) {
	c := NewClient(nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.SendGaugeMetric("ecowitt.y", []string{"unit:C"}, 1)
		}()
	}
	wg.Wait()
}
