package models

type EcowittConfiguration struct {
	StatsServer string   `json:"statsServer"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	DefaultTags []string `json:"defaultTags"`
}
