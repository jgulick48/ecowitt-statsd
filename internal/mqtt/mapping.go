package mqtt

import (
	"strings"
	"unicode"
)

// ObjectName converts a gauge name such as ecowitt.outdoorTemperature to
// snake_case without the ecowitt. prefix (outdoor_temperature).
func ObjectName(name string) string {
	name = strings.TrimPrefix(name, "ecowitt.")
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1])) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// DeviceID builds the HA device identifier from the gateway host.
func DeviceID(host string) string {
	var b strings.Builder
	b.WriteString("ecowitt_")
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// baseName strips the ecowitt. prefix, keeping camelCase.
func baseName(name string) string { return strings.TrimPrefix(name, "ecowitt.") }

// Component returns the Home Assistant platform for a metric: binary_sensor
// for on/off metrics, sensor otherwise.
func Component(name string) string {
	switch baseName(name) {
	case "leak", "batteryLow", "piezoRaining":
		return "binary_sensor"
	}
	return "sensor"
}

// IsBinary reports whether the metric is published as a binary_sensor.
func IsBinary(name string) bool { return Component(name) == "binary_sensor" }

// IsBattery reports whether the metric describes a sensor battery.
func IsBattery(name string) bool {
	switch baseName(name) {
	case "batteryLow", "batteryLevel", "batteryVoltage":
		return true
	}
	return false
}

// EntityCategory returns the HA entity_category for a metric, or "".
func EntityCategory(name string) string {
	if IsBattery(name) {
		return "diagnostic"
	}
	return ""
}

// IsTimestamp reports whether the metric is a Unix timestamp that is published
// to Home Assistant as an ISO-8601 state.
func IsTimestamp(name string) bool { return baseName(name) == "lightningLastStrikeTimestamp" }

// DeviceClass returns the Home Assistant device_class for a metric, or "".
// name may include the ecowitt. prefix.
func DeviceClass(name, unit string) string {
	base := baseName(name)
	if strings.HasSuffix(base, "Aqi") {
		return "aqi"
	}
	switch base {
	case "feelsLike", "outdoorTemperature", "dewPoint", "indoorTemperature", "channelTemperature",
		"windChill", "heatIndex", "co2Temperature", "probeTemperature":
		return "temperature"
	case "outdoorHumidity", "indoorHumidity", "channelHumidity", "co2Humidity":
		return "humidity"
	case "soilMoisture", "leafWetness", "leak", "piezoRaining":
		return "moisture"
	case "absolutePressure", "relativePressure":
		return "atmospheric_pressure"
	case "vaporPressureDeficit":
		return "pressure"
	case "windSpeed", "windGust", "maxWindGust":
		return "wind_speed"
	case "rainRate", "piezoRainRate":
		return "precipitation_intensity"
	case "rainEvent", "rainDay", "rainWeek", "rainMonth", "rainYear", "rainHour", "rainTotal",
		"piezoRainEvent", "piezoRainDay", "piezoRainWeek", "piezoRainMonth", "piezoRainYear",
		"piezoRainHour", "piezoRainTotal":
		return "precipitation"
	case "lightningDistance":
		return "distance"
	case "lightningLastStrikeTimestamp":
		return "timestamp"
	case "co2", "co2Avg24h":
		return "carbon_dioxide"
	case "pm25", "co2Pm25":
		return "pm25"
	case "co2Pm10":
		return "pm10"
	case "batteryLow":
		return "battery"
	case "batteryLevel":
		return "battery"
	case "batteryVoltage":
		return "voltage"
	case "solarIrradiance", "uvRadiation":
		switch unit {
		case "W/m2", "W/m²":
			return "irradiance"
		case "lux":
			if base == "solarIrradiance" {
				return "illuminance"
			}
		}
	}
	return ""
}

// HAUnit normalizes an Ecowitt unit for Home Assistant. Empty means omit.
func HAUnit(name, unit string) string {
	base := baseName(name)
	switch base {
	case "windDirection", "windDirectionAvg10m":
		return "°"
	case "uvIndex":
		return "UV index"
	case "co2Humidity", "soilMoisture", "leafWetness", "batteryLevel":
		return "%"
	case "batteryVoltage":
		return "V"
	case "co2", "co2Avg24h":
		return "ppm"
	case "pm25", "co2Pm25", "co2Pm10":
		return "µg/m³"
	case "leak", "batteryLow", "piezoRaining", "lightningCount", "lightningLastStrikeTimestamp":
		return ""
	}
	if strings.HasSuffix(base, "Aqi") || unit == "" {
		return ""
	}
	switch DeviceClass(name, unit) {
	case "temperature":
		switch unit {
		case "F":
			return "°F"
		case "C":
			return "°C"
		}
	case "wind_speed":
		if unit == "knots" {
			return "kn"
		}
	case "precipitation_intensity":
		switch unit {
		case "in/Hr":
			return "in/h"
		case "mm/Hr":
			return "mm/h"
		}
	case "irradiance":
		return "W/m²"
	case "illuminance":
		return "lx"
	}
	return unit
}

// StateClass returns the Home Assistant state_class for a metric, or "" when
// none is allowed (binary sensors and timestamps).
func StateClass(name string) string {
	if IsBinary(name) || IsTimestamp(name) {
		return ""
	}
	switch baseName(name) {
	case "rainDay", "rainWeek", "rainMonth", "rainYear", "rainEvent", "rainHour", "rainTotal",
		"piezoRainEvent", "piezoRainDay", "piezoRainWeek", "piezoRainMonth", "piezoRainYear",
		"piezoRainHour", "piezoRainTotal", "lightningCount":
		return "total_increasing"
	}
	return "measurement"
}

// humanName builds a display name such as "Garage Temperature" or
// "WH41 CH1 Battery".
func humanName(name, sensor, channel, label string) string {
	base := baseName(name)
	if IsBattery(name) {
		n := strings.ToUpper(sensor)
		if channel != "" {
			n += " CH" + channel
		}
		suffix := map[string]string{"batteryLevel": "Battery", "batteryLow": "Battery Low", "batteryVoltage": "Battery Voltage"}[base]
		return strings.TrimSpace(n + " " + suffix)
	}
	var words []string
	for _, w := range strings.Split(ObjectName(base), "_") {
		if w != "" {
			words = append(words, strings.ToUpper(w[:1])+w[1:])
		}
	}
	if label != "" {
		switch base {
		case "channelTemperature":
			return label + " Temperature"
		case "channelHumidity":
			return label + " Humidity"
		}
		return label + " " + strings.Join(words, " ")
	}
	n := strings.Join(words, " ")
	if channel != "" {
		n += " CH" + channel
	}
	return n
}
