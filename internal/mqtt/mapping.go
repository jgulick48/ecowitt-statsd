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

// DeviceClass returns the Home Assistant device_class for a metric, or "".
// name may include the ecowitt. prefix.
func DeviceClass(name, unit string) string {
	switch baseName(name) {
	case "feelsLike", "outdoorTemperature", "dewPoint", "indoorTemperature", "channelTemperature":
		return "temperature"
	case "outdoorHumidity", "indoorHumidity", "channelHumidity":
		return "humidity"
	case "absolutePressure", "relativePressure":
		return "atmospheric_pressure"
	case "windSpeed", "windGust", "maxWindGust":
		return "wind_speed"
	case "rainRate":
		return "precipitation_intensity"
	case "rainEvent", "rainDay", "rainWeek", "rainMonth", "rainYear":
		return "precipitation"
	case "solarIrradiance":
		switch unit {
		case "W/m2", "W/m²":
			return "irradiance"
		case "lux":
			return "illuminance"
		}
	}
	return ""
}

// HAUnit normalizes an Ecowitt unit for Home Assistant. Empty means omit.
func HAUnit(name, unit string) string {
	base := baseName(name)
	switch base {
	case "windDirection":
		return "°"
	case "uvIndex":
		return "UV index"
	}
	if unit == "" {
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

// StateClass returns the Home Assistant state_class for a metric.
func StateClass(name string) string {
	switch baseName(name) {
	case "rainDay", "rainWeek", "rainMonth", "rainYear", "rainEvent":
		return "total_increasing"
	}
	return "measurement"
}

// humanName builds a display name such as "Garage Temperature".
func humanName(name, channel, label string) string {
	base := baseName(name)
	if label != "" {
		switch base {
		case "channelTemperature":
			return label + " Temperature"
		case "channelHumidity":
			return label + " Humidity"
		}
	}
	var words []string
	for _, w := range strings.Split(ObjectName(base), "_") {
		if w != "" {
			words = append(words, strings.ToUpper(w[:1])+w[1:])
		}
	}
	n := strings.Join(words, " ")
	if channel != "" {
		n += " CH" + channel
	}
	return n
}
