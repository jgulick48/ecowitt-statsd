package ecowitt

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const liveJSON = `{"common_list":[{"id":"0x02","val":"72.5","unit":"F"}],"wh25":[],"ch_aisle":[],"rain":[]}`

type recorder struct {
	statuses []bool
	gauges   map[string][]float64
}

func (r *recorder) SendGaugeMetric(name string, _ []string, v float64) {
	if r.gauges == nil {
		r.gauges = map[string][]float64{}
	}
	r.gauges[name] = append(r.gauges[name], v)
}
func (r *recorder) SetGatewayStatus(up bool) { r.statuses = append(r.statuses, up) }

func newGateway(t *testing.T) (*httptest.Server, *atomic.Value, string) {
	var mode atomic.Value
	mode.Store("ok")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load().(string) {
		case "500":
			http.Error(w, "boom", 500)
		case "badjson":
			w.Write([]byte("{nope"))
		default:
			w.Write([]byte(liveJSON))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &mode, strings.TrimPrefix(srv.URL, "http://")
}

func TestHealthTransitions(t *testing.T) {
	_, mode, addr := newGateway(t)
	rec := &recorder{}
	c := NewClientWithOptions(addr, http.DefaultClient, rec, Options{TickRate: 1}).(*client)

	if h := c.Health(); h.Up || !h.LastSuccess.IsZero() || h.ConsecutiveFailures != 0 {
		t.Fatalf("initial: %+v", h)
	}
	if err := c.scanMetrics(); err != nil {
		t.Fatal(err)
	}
	h := c.Health()
	if !h.Up || h.LastSuccess.IsZero() || h.ConsecutiveFailures != 0 {
		t.Fatalf("after success: %+v", h)
	}
	if len(rec.statuses) != 1 || !rec.statuses[0] {
		t.Fatalf("statuses %v", rec.statuses)
	}

	mode.Store("500")
	for i := 1; i <= 2; i++ {
		if err := c.scanMetrics(); err == nil {
			t.Fatal("expected error")
		}
		h = c.Health()
		if !h.Up || h.ConsecutiveFailures != i || h.LastError == "" {
			t.Fatalf("below threshold %d: %+v", i, h)
		}
	}
	if len(rec.statuses) != 1 {
		t.Fatalf("no transition expected yet: %v", rec.statuses)
	}
	mode.Store("badjson")
	c.scanMetrics()
	h = c.Health()
	if h.Up || h.ConsecutiveFailures != 3 {
		t.Fatalf("at threshold: %+v", h)
	}
	if len(rec.statuses) != 2 || rec.statuses[1] {
		t.Fatalf("statuses %v", rec.statuses)
	}
	mode.Store("500")
	c.scanMetrics() // still down, no extra notification
	if len(rec.statuses) != 2 {
		t.Fatalf("statuses %v", rec.statuses)
	}

	mode.Store("ok")
	c.scanMetrics()
	h = c.Health()
	if !h.Up || h.ConsecutiveFailures != 0 || h.LastError != "" {
		t.Fatalf("recovery: %+v", h)
	}
	c.scanMetrics() // steady state, no notification
	want := []bool{true, false, true}
	if len(rec.statuses) != len(want) {
		t.Fatalf("statuses %v", rec.statuses)
	}
	for i := range want {
		if rec.statuses[i] != want[i] {
			t.Fatalf("statuses %v", rec.statuses)
		}
	}
}

func TestUpGaugeEmittedEachScan(t *testing.T) {
	_, mode, addr := newGateway(t)
	rec := &recorder{}
	c := NewClientWithOptions(addr, http.DefaultClient, rec, Options{FailureThreshold: 2}).(*client)
	c.scanMetrics()
	mode.Store("500")
	c.scanMetrics()
	c.scanMetrics()
	up := rec.gauges["ecowitt.up"]
	want := []float64{1, 1, 0}
	if len(up) != len(want) {
		t.Fatalf("up %v", up)
	}
	for i := range want {
		if up[i] != want[i] {
			t.Fatalf("up %v", up)
		}
	}
	if ts := rec.gauges["ecowitt.lastSuccessTimestamp"]; len(ts) != 1 || ts[0] <= 0 {
		t.Fatalf("timestamp %v", ts)
	}
}

func TestDefaultThreshold(t *testing.T) {
	c := NewClient("x", 1, http.DefaultClient, &recorder{}).(*client)
	if c.failureThreshold != DefaultFailureThreshold {
		t.Fatal(c.failureThreshold)
	}
}
