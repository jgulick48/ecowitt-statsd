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

## Environment variables
Every setting can also be supplied through environment variables. The JSON file is optional: if it is missing, the daemon runs from environment variables alone. When both are present, environment variables override the file (only when set and non-empty). The gateway host is required, either as `host` in the file or `ECOWITT_HOST`; startup fails otherwise.

| Variable | JSON field |
| --- | --- |
| `ECOWITT_HOST` | `host` |
| `ECOWITT_STATS_SERVER` | `statsServer` |
| `ECOWITT_DEFAULT_TAGS` (comma-separated) | `defaultTags` |
| `ECOWITT_PROMETHEUS_ENABLED` | `prometheus.enabled` |
| `ECOWITT_PROMETHEUS_LISTEN_ADDRESS` | `prometheus.listenAddress` |
| `ECOWITT_MQTT_ENABLED` | `mqtt.enabled` |
| `ECOWITT_MQTT_BROKER` | `mqtt.broker` |
| `ECOWITT_MQTT_USERNAME` (or `_FILE`) | `mqtt.username` |
| `ECOWITT_MQTT_PASSWORD` (or `_FILE`) | `mqtt.password` |
| `ECOWITT_MQTT_CLIENT_ID` | `mqtt.clientId` |
| `ECOWITT_MQTT_TOPIC_PREFIX` | `mqtt.topicPrefix` |
| `ECOWITT_MQTT_DISCOVERY_PREFIX` | `mqtt.discoveryPrefix` |
| `ECOWITT_MQTT_DEVICE_NAME` | `mqtt.deviceName` |
| `ECOWITT_MQTT_RETAIN` | `mqtt.retain` |
| `ECOWITT_MQTT_TLS_CA_FILE` | `mqtt.tls.caFile` |
| `ECOWITT_MQTT_TLS_CERT_FILE` | `mqtt.tls.certFile` |
| `ECOWITT_MQTT_TLS_KEY_FILE` | `mqtt.tls.keyFile` |
| `ECOWITT_MQTT_TLS_SERVER_NAME` | `mqtt.tls.serverName` |
| `ECOWITT_MQTT_TLS_INSECURE_SKIP_VERIFY` | `mqtt.tls.insecureSkipVerify` |

Booleans are parsed as Go `strconv.ParseBool` values (`true`, `false`, `1`, `0`, ...); an invalid value is a startup error naming the variable.

Docker run:

```sh
docker run -d \
  -e ECOWITT_HOST=192.168.1.50 \
  -e ECOWITT_STATS_SERVER=statsd:8125 \
  -e ECOWITT_PROMETHEUS_ENABLED=true \
  -e ECOWITT_DEFAULT_TAGS=location:home,site:backyard \
  -p 9090:9090 jgulick48/ecowitt-statsd
```

docker-compose:

```yaml
services:
  ecowitt:
    image: jgulick48/ecowitt-statsd
    environment:
      ECOWITT_HOST: 192.168.1.50
      ECOWITT_PROMETHEUS_ENABLED: "true"
    ports:
      - "9090:9090"
```

### Docker secrets
`ECOWITT_MQTT_PASSWORD` and `ECOWITT_MQTT_USERNAME` have `_FILE` variants that read the value from a file (trailing newlines are trimmed). Setting both a variable and its `_FILE` variant is an error.

```yaml
services:
  ecowitt:
    image: jgulick48/ecowitt-statsd
    environment:
      ECOWITT_HOST: 192.168.1.50
      ECOWITT_MQTT_ENABLED: "true"
      ECOWITT_MQTT_BROKER: ssl://mqtt:8883
      ECOWITT_MQTT_USERNAME: ecowitt
      ECOWITT_MQTT_PASSWORD_FILE: /run/secrets/mqtt_password
    secrets:
      - mqtt_password
secrets:
  mqtt_password:
    file: ./mqtt_password.txt
```

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
