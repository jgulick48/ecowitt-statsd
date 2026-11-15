package metrics

import "testing"

type fake struct {
	names  []string
	values []float64
}

func (f *fake) SendGaugeMetric(name string, tags []string, value float64) {
	f.names = append(f.names, name)
	f.values = append(f.values, value)
}

func TestNewMultiClientFansOutAndSkipsNil(t *testing.T) {
	a, b := &fake{}, &fake{}
	var nilClient Client
	m := NewMultiClient(a, nilClient, b)
	m.SendGaugeMetric("x", nil, 1.5)
	m.SendGaugeMetric("y", []string{"k:v"}, 2)
	for _, f := range []*fake{a, b} {
		if len(f.names) != 2 || f.names[0] != "x" || f.values[1] != 2 {
			t.Fatalf("unexpected forwarding: %+v", f)
		}
	}
}

func TestNewMultiClientEmpty(t *testing.T) {
	NewMultiClient().SendGaugeMetric("x", nil, 1)
}
