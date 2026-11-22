package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jgulick48/ecowitt-statsd/internal/ecowitt"
)

var (
	start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	poll  = 10 * time.Second // grace = 40s
)

func get(t *testing.T, h http.Handler) (int, map[string]interface{}, string) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	var m map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("bad json %q: %v", rec.Body.String(), err)
	}
	return rec.Code, m, rec.Header().Get("Content-Type")
}

func handlers(h ecowitt.Health, now time.Time) (http.Handler, http.Handler) {
	f := func() ecowitt.Health { return h }
	return Healthz(f, poll, start, func() time.Time { return now }), Readyz(f)
}

func TestOK(t *testing.T) {
	at := start.Add(time.Minute)
	hz, rz := handlers(ecowitt.Health{Up: true, LastSuccess: at, LastAttempt: at}, at.Add(5*time.Second))
	for _, h := range []http.Handler{hz, rz} {
		code, m, ct := get(t, h)
		if code != 200 || m["status"] != "ok" || m["gatewayUp"] != true || ct != "application/json" {
			t.Fatalf("%d %v %s", code, m, ct)
		}
		if m["lastSuccess"] != "2026-01-01T00:01:00Z" || m["lastAttempt"] != "2026-01-01T00:01:00Z" {
			t.Fatalf("%v", m)
		}
	}
}

func TestStale(t *testing.T) {
	at := start.Add(time.Minute)
	hz, _ := handlers(ecowitt.Health{Up: true, LastAttempt: at}, at.Add(41*time.Second))
	code, m, _ := get(t, hz)
	if code != 503 || m["status"] != "unhealthy" {
		t.Fatalf("%d %v", code, m)
	}
	hz, _ = handlers(ecowitt.Health{Up: true, LastAttempt: at}, at.Add(40*time.Second))
	if code, _, _ := get(t, hz); code != 200 {
		t.Fatalf("boundary %d", code)
	}
}

func TestGatewayDown(t *testing.T) {
	at := start.Add(time.Minute)
	hz, rz := handlers(ecowitt.Health{LastAttempt: at, ConsecutiveFailures: 5, LastError: "boom"}, at.Add(time.Second))
	code, m, _ := get(t, hz)
	if code != 200 || m["status"] != "ok" || m["lastSuccess"] != nil || m["lastError"] != "boom" || m["consecutiveFailures"] != float64(5) {
		t.Fatalf("healthz %d %v", code, m)
	}
	code, m, _ = get(t, rz)
	if code != 503 || m["status"] != "not_ready" || m["gatewayUp"] != false {
		t.Fatalf("readyz %d %v", code, m)
	}
}

func TestBeforeFirstAttempt(t *testing.T) {
	hz, rz := handlers(ecowitt.Health{}, start.Add(30*time.Second))
	code, m, _ := get(t, hz)
	if code != 200 || m["lastAttempt"] != nil {
		t.Fatalf("within grace %d %v", code, m)
	}
	if code, _, _ := get(t, rz); code != 503 {
		t.Fatalf("readyz %d", code)
	}
	hz, rz = handlers(ecowitt.Health{}, start.Add(41*time.Second))
	if code, m, _ := get(t, hz); code != 503 || m["status"] != "unhealthy" {
		t.Fatalf("past grace %d %v", code, m)
	}
	if code, _, _ := get(t, rz); code != 503 {
		t.Fatalf("readyz %d", code)
	}
}
