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
e.g. `ecowitt_192_168_1_50`; `<objectId>` is the snake_case metric name, plus `_ch<N>` for channel sensors,
e.g. `outdoor_temperature` or `channel_temperature_ch2`):

- `<topicPrefix>/status`: `online` / `offline` (retained; `offline` is also the last-will message).
- `<topicPrefix>/<objectId>/state`: the value as a plain number.
- `<discoveryPrefix>/sensor/<deviceId>/<objectId>/config`: retained discovery config, sent the first time
  a sensor is seen and again after every reconnect.

If the broker cannot be reached at startup, the error is logged and the daemon continues without MQTT.
