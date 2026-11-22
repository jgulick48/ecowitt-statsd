// Package health provides liveness and readiness HTTP handlers.
package health

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jgulick48/ecowitt-statsd/internal/ecowitt"
)

type response struct {
	Status              string  `json:"status"`
	GatewayUp           bool    `json:"gatewayUp"`
	LastSuccess         *string `json:"lastSuccess"`
	LastAttempt         *string `json:"lastAttempt"`
	ConsecutiveFailures int     `json:"consecutiveFailures"`
	LastError           string  `json:"lastError"`
}

func ts(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func write(w http.ResponseWriter, code int, status string, h ecowitt.Health) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(response{
		Status:              status,
		GatewayUp:           h.Up,
		LastSuccess:         ts(h.LastSuccess),
		LastAttempt:         ts(h.LastAttempt),
		ConsecutiveFailures: h.ConsecutiveFailures,
		LastError:           h.LastError,
	})
}

// Healthz is the liveness handler: it reports ok while the poll loop is alive,
// meaning the last scan attempt (successful or not) happened within
// 3*pollInterval+10s of now. Before the first attempt, start is the reference.
// A down gateway does not make it fail.
func Healthz(h func() ecowitt.Health, pollInterval time.Duration, start time.Time, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s := h()
		ref := s.LastAttempt
		if ref.IsZero() {
			ref = start
		}
		if now().Sub(ref) <= 3*pollInterval+10*time.Second {
			write(w, http.StatusOK, "ok", s)
			return
		}
		write(w, http.StatusServiceUnavailable, "unhealthy", s)
	})
}

// Readyz is the readiness handler: 200 when the gateway is up, else 503.
func Readyz(h func() ecowitt.Health) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s := h()
		if s.Up {
			write(w, http.StatusOK, "ok", s)
			return
		}
		write(w, http.StatusServiceUnavailable, "not_ready", s)
	})
}
