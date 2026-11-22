package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jgulick48/ecowitt-statsd/internal/models"
)

func TestGatewayTimeout(t *testing.T) {
	cases := map[time.Duration]time.Duration{
		2 * time.Second:          2 * time.Second, // 1.6s clamped up
		5 * time.Second:          4 * time.Second,
		10 * time.Second:         8 * time.Second,
		time.Minute:              30 * time.Second, // 48s clamped down
		37500 * time.Millisecond: 30 * time.Second,
	}
	for in, want := range cases {
		if got := gatewayTimeout(in); got != want {
			t.Errorf("gatewayTimeout(%s)=%s want %s", in, got, want)
		}
	}
}

func TestEffectiveListenAddress(t *testing.T) {
	var c models.EcowittConfiguration
	if got := effectiveListenAddress(c); got != ":9090" {
		t.Fatal(got)
	}
	c.Prometheus.ListenAddress = ":9100"
	if got := effectiveListenAddress(c); got != ":9100" {
		t.Fatal(got)
	}
	c.ListenAddress = "127.0.0.1:8080"
	if got := effectiveListenAddress(c); got != "127.0.0.1:8080" {
		t.Fatal(got)
	}
}

func TestRunHealthcheck(t *testing.T) {
	code := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
	if got := runHealthcheck(srv.URL); got != 0 {
		t.Fatalf("200 -> %d", got)
	}
	code = http.StatusServiceUnavailable
	if got := runHealthcheck(srv.URL); got != 1 {
		t.Fatalf("503 -> %d", got)
	}
	srv.Close()
	if got := runHealthcheck(srv.URL); got != 1 {
		t.Fatalf("refused -> %d", got)
	}
}
