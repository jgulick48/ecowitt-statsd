package models

import (
	"strings"
	"testing"
	"time"
)

func TestPollIntervalDuration(t *testing.T) {
	ok := map[string]time.Duration{"": 10 * time.Second, "10s": 10 * time.Second, "1m": time.Minute, "2s": 2 * time.Second}
	for in, want := range ok {
		got, err := EcowittConfiguration{PollInterval: in}.PollIntervalDuration()
		if err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := (EcowittConfiguration{PollInterval: "abc"}).PollIntervalDuration(); err == nil {
		t.Error("invalid should error")
	}
	_, err := EcowittConfiguration{PollInterval: "1s"}.PollIntervalDuration()
	if err == nil || !strings.Contains(err.Error(), "minimum") {
		t.Errorf("too small: %v", err)
	}
}
