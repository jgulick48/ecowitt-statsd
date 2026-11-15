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
