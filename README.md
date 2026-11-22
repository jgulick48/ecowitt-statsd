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
e.g. `ecowitt_192_168_1_50`; `<objectId>` is the snake_case metric name, plus `_ch<N>` for channel sensors,
e.g. `outdoor_temperature` or `channel_temperature_ch2`):

- `<topicPrefix>/status`: `online` / `offline` (retained). It reflects gateway reachability, not just the broker
  connection: `online` once the gateway is up, `offline` while it is down (and before the first successful scan),
  so Home Assistant marks every sensor unavailable. `offline` is also the last-will message. On reconnect the
  current state is republished. `ecowitt.up` and `ecowitt.lastSuccessTimestamp` get no Home Assistant entities.
- `<topicPrefix>/<objectId>/state`: the value as a plain number.
- `<discoveryPrefix>/sensor/<deviceId>/<objectId>/config`: retained discovery config, sent the first time
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

## Releasing

Images are built on the shared build machine (`casp-build-01`) through a buildx builder on that Docker context. They are pushed using your local Docker Hub login.

```sh
make docker-push      # tests, remote multi-arch build, push :<date> and :latest, then git tag <date> and push the tag
make docker.build     # remote multi-arch build only (no push), as a smoke test
make version          # show the date-coded version that would be used
make help             # list all targets
```

The version is date-coded in UTC, e.g. `2026.11.15-1432`. The same string is used for the image tag (`jgulick48/ecowitt-statsd:2026.11.15-1432`) and the git tag. `docker-push` refuses to run if tracked files have uncommitted changes or if the tag already exists. You can override `IMAGE`, `PLATFORMS`, `BUILD_CONTEXT`, `BUILDER` or `VERSION` on the command line.
