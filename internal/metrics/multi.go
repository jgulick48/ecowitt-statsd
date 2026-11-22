package metrics

type multiClient struct {
	clients []Client
}

// NewMultiClient returns a Client that forwards every gauge to each non-nil client.
func NewMultiClient(clients ...Client) Client {
	m := &multiClient{}
	for _, c := range clients {
		if c != nil {
			m.clients = append(m.clients, c)
		}
	}
	return m
}

func (m *multiClient) SendGaugeMetric(name string, tags []string, value float64) {
	for _, c := range m.clients {
		c.SendGaugeMetric(name, tags, value)
	}
}

// StatusReceiver is implemented by outputs that react to gateway up/down transitions.
type StatusReceiver interface{ SetGatewayStatus(up bool) }

// SetGatewayStatus forwards the status to every wrapped client that implements StatusReceiver.
func (m *multiClient) SetGatewayStatus(up bool) {
	for _, c := range m.clients {
		if sr, ok := c.(StatusReceiver); ok {
			sr.SetGatewayStatus(up)
		}
	}
}
