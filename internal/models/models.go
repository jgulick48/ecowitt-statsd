package models

type PrometheusConfig struct {
	Enabled       bool   `json:"enabled"`
	ListenAddress string `json:"listenAddress"`
}

type EcowittConfiguration struct {
	StatsServer string           `json:"statsServer"`
	Host        string           `json:"host"`
	Port        int              `json:"port"`
	DefaultTags []string         `json:"defaultTags"`
	Prometheus  PrometheusConfig `json:"prometheus"`
}
