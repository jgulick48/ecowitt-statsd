# ecowitt-statsd
Small Daemon that reads data from ecowitt gateway and publishes them to a statsd/prometheus endpoint.

## Configuration
The daemon reads a JSON file (default `/var/lib/ecowitt-statsd/config.json`, override with `-configFile`).

```json
{
  "host": "192.168.1.50",
  "port": 80,
  "statsServer": "127.0.0.1:8125",
  "defaultTags": ["location:home"],
  "prometheus": {
    "enabled": true,
    "listenAddress": ":9090"
  },
  "mqtt": {
    "enabled": false,
    "broker": "tcp://192.168.1.10:1883"
  }
}
```

- `statsServer`: statsd address. If empty, statsd output is disabled.
- `defaultTags`: `key:value` tags added to every metric (statsd tags / Prometheus labels).
- `prometheus.enabled`: serve a Prometheus endpoint.
- `prometheus.listenAddress`: listen address, defaults to `:9090`.
- `failureThreshold`: number of consecutive failed scans (request error, non-200 status, unreadable body or
  invalid JSON) before the gateway is considered down. Defaults to `3`. Any successful scan marks it up again.

## Gateway health
Every scan emits `ecowitt.up` (`1` when the gateway is up, `0` when down; before the first successful scan it
is `0`) and, after a successful scan, `ecowitt.lastSuccessTimestamp` (Unix seconds). In Prometheus these are
`ecowitt_up` and `ecowitt_last_success_timestamp`.

When the gateway goes down, Prometheus drops all sensor series (everything except `ecowitt_up` and
`ecowitt_last_success_timestamp`) so stale values are not served; they reappear on the first scan after
recovery. MQTT/Home Assistant is notified too, see below.

## Prometheus /metrics endpoint
When enabled, metrics are served at `http://<host>:9090/metrics`. Names are converted to
snake_case with dots as underscores, e.g. `ecowitt.outdoorTemperature` becomes
`ecowitt_outdoor_temperature{unit="F"}`. Tags become labels. Go runtime and process metrics are included.
A metric's label key set is fixed by its first sample; samples with a different set are logged and dropped.

```yaml
scrape_configs:
  - job_name: ecowitt
    static_configs:
      - targets: ["localhost:9090"]
```

## MQTT / Home Assistant
When `mqtt.enabled` is true, every gauge is published to MQTT and registered in Home Assistant through
MQTT Discovery, so sensors appear automatically under one "Ecowitt Gateway" device.

```json
"mqtt": {
  "enabled": true,
  "broker": "tcp://192.168.1.10:1883",
  "username": "user",
  "password": "secret",
  "clientId": "ecowitt-statsd",
  "topicPrefix": "ecowitt",
  "discoveryPrefix": "homeassistant",
  "deviceName": "Ecowitt Gateway",
  "retain": false
}
```

Only `enabled` and `broker` are required; the values shown for `clientId`, `topicPrefix`,
`discoveryPrefix` and `deviceName` are the defaults, and `retain` (retain state messages) defaults to false.

Topics (`<deviceId>` is `ecowitt_` plus the gateway host with non-alphanumerics replaced by `_`,
e.g. `ecowitt_192_168_1_50`; `<objectId>` is the snake_case metric name, plus `_<sensor>` and `_ch<N>` for battery and channel sensors,
e.g. `outdoor_temperature`, `channel_temperature_ch2` or `battery_level_wh41_ch1`):

- `<topicPrefix>/status`: `online` / `offline` (retained). It reflects gateway reachability, not just the broker
  connection: `online` once the gateway is up, `offline` while it is down (and before the first successful scan),
  so Home Assistant marks every sensor unavailable. `offline` is also the last-will message. On reconnect the
  current state is republished. `ecowitt.up` and `ecowitt.lastSuccessTimestamp` get no Home Assistant entities.
- `<topicPrefix>/<objectId>/state`: the value as a plain number.
- `<discoveryPrefix>/<sensor|binary_sensor>/<deviceId>/<objectId>/config`: retained discovery config, sent the first time
  a sensor is seen and again after every reconnect.

If the broker cannot be reached at startup, the daemon keeps retrying in the background every 30 seconds.
State updates are dropped while it is disconnected, so the gateway polling never stalls.

### TLS / mutual TLS

For a TLS broker, use an `ssl://` URL and add a `tls` block:

```json
"mqtt": {
  "enabled": true,
  "broker": "ssl://mqtt.example.com:8883",
  "tls": {
    "caFile": "/var/lib/ecowitt-statsd/ca.pem",
    "certFile": "/var/lib/ecowitt-statsd/client.crt",
    "keyFile": "/var/lib/ecowitt-statsd/client.key",
    "serverName": "mqtt.example.com",
    "insecureSkipVerify": false
  }
}
```

- `caFile`: PEM bundle of CA certificates to trust, for a private CA. It is added to the system CAs, so
  public CAs are still trusted when it is set.
- `certFile` / `keyFile`: client certificate and private key for mutual TLS. Set both or neither.
- `serverName`: overrides the name used for SNI and certificate verification, useful when connecting by IP
  to a certificate issued for a hostname.
- `insecureSkipVerify`: skips server certificate verification. Insecure; use only for testing.

All `tls` fields are optional. The settings only apply when the broker URL uses a TLS scheme (`ssl://`,
`tls://`, `mqtts://`, `mqtt+ssl://`, `tcps://` or `wss://`); with `tcp://` they are ignored and a warning
is logged. In Docker, mount the files into the container, e.g. under `/var/lib/ecowitt-statsd/`, and
reference those paths. Invalid TLS settings (unreadable files, mismatched key) disable MQTT with an error
in the log.

## Metrics

Metric names are prefixed with `ecowitt.` (`ecowitt_` in Prometheus). Every metric carries a `unit` tag (empty
when it has no unit) except as noted. The "HA entity" column is the Home Assistant entity type created through
MQTT discovery.

| Metric | Source section | Unit | HA entity |
|---|---|---|---|
| `outdoorTemperature`, `feelsLike`, `dewPoint`, `windChill`, `heatIndex` | `common_list` | gateway unit (F/C) | sensor (temperature) |
| `outdoorHumidity`, `indoorHumidity` | `common_list` / `wh25` | % | sensor (humidity) |
| `indoorTemperature` | `wh25` / `common_list` | gateway unit | sensor (temperature) |
| `absolutePressure`, `relativePressure` | `wh25` / `common_list` | gateway unit | sensor (atmospheric_pressure) |
| `vaporPressureDeficit` | `common_list` (`5`) | kPa | sensor (pressure) |
| `windSpeed`, `windGust`, `maxWindGust` | `common_list` | gateway unit | sensor (wind_speed) |
| `windDirection`, `windDirectionAvg10m` | `common_list` | degrees | sensor |
| `solarIrradiance` | `common_list` | W/m2 | sensor (irradiance) |
| `uvRadiation` | `common_list` (`0x16`) | uW/m2 | sensor (no class) |
| `uvIndex` | `common_list` | UV index | sensor |
| `rainEvent`, `rainDay`, `rainWeek`, `rainMonth`, `rainYear`, `rainHour`, `rainTotal` | `rain` / `common_list` | in/mm | sensor (precipitation, total_increasing) |
| `rainRate` | `rain` | in/Hr, mm/Hr | sensor (precipitation_intensity) |
| `piezoRainEvent`, `piezoRainDay`, `piezoRainWeek`, `piezoRainMonth`, `piezoRainYear` | `piezoRain` | in/mm | sensor (precipitation, total_increasing) |
| `piezoRainRate` | `piezoRain` | in/Hr, mm/Hr | sensor (precipitation_intensity) |
| `piezoRaining` | `piezoRain` (`srain_piezo`) | 0/1 | binary_sensor (moisture) |
| `lightningDistance` | `lightning` | mi/km | sensor (distance) |
| `lightningCount` | `lightning` | none | sensor (total_increasing) |
| `lightningLastStrikeTimestamp` | `lightning` | Unix seconds (HA state is ISO-8601 UTC) | sensor (timestamp) |
| `co2`, `co2Avg24h` | `co2` (WH45) | ppm | sensor (carbon_dioxide) |
| `co2Pm25`, `co2Pm10` | `co2` | ug/m3 | sensor (pm25 / pm10) |
| `co2Pm25Aqi`, `co2Pm10Aqi` | `co2` (RealAQI) | none | sensor (aqi) |
| `co2Temperature`, `co2Humidity` | `co2` | F or C, % | sensor (temperature / humidity) |
| `pm25` | `ch_pm25` (WH41/43) | ug/m3 | sensor (pm25) |
| `pm25Aqi`, `pm25Avg24hAqi` | `ch_pm25` | none | sensor (aqi) |
| `leak` | `ch_leak` (WH55) | 0/1 (1 = status other than Normal) | binary_sensor (moisture) |
| `soilMoisture` | `ch_soil` (WH51) | % | sensor (moisture) |
| `probeTemperature` | `ch_temp` (WN34) | F or C | sensor (temperature) |
| `leafWetness` | `ch_leaf` (WN35) | % | sensor (moisture) |
| `channelTemperature`, `channelHumidity` | `ch_aisle` (WH31) | F or C, % | sensor |
| `batteryLow` | every section with a battery field | 0/1 | binary_sensor (battery), diagnostic |
| `batteryLevel` | level-type sensors | % | sensor (battery), diagnostic |
| `batteryVoltage` | any section with a `voltage` field | V | sensor (voltage), diagnostic |

Channel sensors (`ch_*`) are tagged `channel` and `name` (empty when unnamed) plus `unit`.

### Batteries
Battery metrics are tagged `sensor` (`wh31`, `wn34`, `wh41`, `wh55`, `wh57`, `wh45`, `wh51`, `wn35`, `wh40` for
the rain gauge, `ws90` for the piezo rain / WS90, `wh25`), `channel` (empty for single sensors) and `unit`.

- Binary sensors (WH31, WN34, WH25): the gateway reports `0` = OK, `1` = low. Only `batteryLow` is emitted.
- Level sensors (WH41, WH55, WH57, WH45, WH51, WN35, WH40, WS90): `0`-`5`, where `5` is full and `1` or less is
  low, and `6` means DC/USB powered. `batteryLow` is `1` when the level is `1` or less (and `0` for `6`);
  `batteryLevel` is the level times 20 as a percentage (0-100) and is not emitted for DC powered sensors.
- `batteryVoltage` is emitted whenever the gateway reports a `voltage` for the sensor.

In Home Assistant the object IDs include the sensor and channel, e.g. `battery_level_wh41_ch1`,
`battery_low_wh31_ch2` or `battery_voltage_ws90`.

Section and battery formats are based on Ecowitt's documented local API format and can vary by gateway model and
firmware. Missing or unparseable fields are skipped (and logged) instead of failing the scan.

### Unknown sensor IDs
`common_list` IDs that are not mapped are not published. Each unknown ID is logged once with its raw value
(`Unknown sensor id ...`). Please report these in an issue so they can be added.

## Releasing

Images are built on the shared build machine (`casp-build-01`) through a buildx builder on that Docker context. They are pushed using your local Docker Hub login.

```sh
make docker-push      # tests, remote multi-arch build, push :<date> and :latest, then git tag <date> and push the tag
make docker.build     # remote multi-arch build only (no push), as a smoke test
make version          # show the date-coded version that would be used
make help             # list all targets
```

The version is date-coded in UTC, e.g. `2026.11.15-1432`. The same string is used for the image tag (`jgulick48/ecowitt-statsd:2026.11.15-1432`) and the git tag. `docker-push` refuses to run if tracked files have uncommitted changes or if the tag already exists. You can override `IMAGE`, `PLATFORMS`, `BUILD_CONTEXT`, `BUILDER` or `VERSION` on the command line.
