package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func write(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFileOnly(t *testing.T) {
	p := write(t, "c.json", `{"host":"h1","statsServer":"s:8125","mqtt":{"broker":"tcp://b","tls":{"caFile":"/ca"}}}`)
	c, err := Load(p, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "h1" || c.StatsServer != "s:8125" || c.MQTT.Broker != "tcp://b" || c.MQTT.TLS.CAFile != "/ca" {
		t.Fatalf("unexpected %+v", c)
	}
}

func TestEnvOnlyNoFile(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "missing.json"), env(map[string]string{
		"ECOWITT_HOST": "gw", "ECOWITT_PROMETHEUS_ENABLED": "true", "ECOWITT_PROMETHEUS_LISTEN_ADDRESS": ":1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "gw" || !c.Prometheus.Enabled || c.Prometheus.ListenAddress != ":1" {
		t.Fatalf("unexpected %+v", c)
	}
}

func TestEmptyPathEnvOnly(t *testing.T) {
	if _, err := Load("", env(map[string]string{"ECOWITT_HOST": "gw"})); err != nil {
		t.Fatal(err)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	p := write(t, "c.json", `{"host":"file","statsServer":"keep"}`)
	c, err := Load(p, env(map[string]string{"ECOWITT_HOST": "envhost", "ECOWITT_STATS_SERVER": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "envhost" || c.StatsServer != "keep" {
		t.Fatalf("unexpected %+v", c)
	}
}

func TestNestedMQTTAndTLS(t *testing.T) {
	c, err := Load("", env(map[string]string{
		"ECOWITT_HOST": "h", "ECOWITT_MQTT_ENABLED": "1", "ECOWITT_MQTT_BROKER": "ssl://b:8883",
		"ECOWITT_MQTT_USERNAME": "u", "ECOWITT_MQTT_PASSWORD": "p", "ECOWITT_MQTT_CLIENT_ID": "cid",
		"ECOWITT_MQTT_TOPIC_PREFIX": "tp", "ECOWITT_MQTT_DISCOVERY_PREFIX": "dp", "ECOWITT_MQTT_DEVICE_NAME": "dn",
		"ECOWITT_MQTT_RETAIN": "true", "ECOWITT_MQTT_TLS_CA_FILE": "/ca", "ECOWITT_MQTT_TLS_CERT_FILE": "/crt",
		"ECOWITT_MQTT_TLS_KEY_FILE": "/key", "ECOWITT_MQTT_TLS_SERVER_NAME": "sn", "ECOWITT_MQTT_TLS_INSECURE_SKIP_VERIFY": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	m := c.MQTT
	if !m.Enabled || m.Broker != "ssl://b:8883" || m.Username != "u" || m.Password != "p" || m.ClientID != "cid" ||
		m.TopicPrefix != "tp" || m.DiscoveryPrefix != "dp" || m.DeviceName != "dn" || !m.Retain ||
		m.TLS.CAFile != "/ca" || m.TLS.CertFile != "/crt" || m.TLS.KeyFile != "/key" || m.TLS.ServerName != "sn" || !m.TLS.InsecureSkipVerify {
		t.Fatalf("unexpected %+v", m)
	}
}

func TestTags(t *testing.T) {
	c, err := Load("", env(map[string]string{"ECOWITT_HOST": "h", "ECOWITT_DEFAULT_TAGS": "a:b, c:d ,e:f"}))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a:b", "c:d", "e:f"}; !reflect.DeepEqual(c.DefaultTags, want) {
		t.Fatalf("got %v", c.DefaultTags)
	}
}

func TestBadBool(t *testing.T) {
	_, err := Load("", env(map[string]string{"ECOWITT_HOST": "h", "ECOWITT_MQTT_RETAIN": "maybe"}))
	if err == nil || !strings.Contains(err.Error(), "ECOWITT_MQTT_RETAIN") {
		t.Fatalf("got %v", err)
	}
}

func TestFileSecrets(t *testing.T) {
	pw := write(t, "pw", "s3cret\n\n")
	us := write(t, "us", "bob \r\n")
	c, err := Load("", env(map[string]string{
		"ECOWITT_HOST": "h", "ECOWITT_MQTT_PASSWORD_FILE": pw, "ECOWITT_MQTT_USERNAME_FILE": us,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.MQTT.Password != "s3cret" || c.MQTT.Username != "bob" {
		t.Fatalf("unexpected %+v", c.MQTT)
	}
}

func TestBothSetErrorsWithoutSecret(t *testing.T) {
	pw := write(t, "pw", "filesecret\n")
	_, err := Load("", env(map[string]string{
		"ECOWITT_HOST": "h", "ECOWITT_MQTT_PASSWORD": "envsecret", "ECOWITT_MQTT_PASSWORD_FILE": pw,
	}))
	if err == nil || !strings.Contains(err.Error(), "ECOWITT_MQTT_PASSWORD") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "envsecret") || strings.Contains(err.Error(), "filesecret") {
		t.Fatalf("error leaks secret: %v", err)
	}
}

func TestMissingSecretFile(t *testing.T) {
	_, err := Load("", env(map[string]string{"ECOWITT_HOST": "h", "ECOWITT_MQTT_PASSWORD_FILE": "/nonexistent/pw"}))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInvalidJSON(t *testing.T) {
	p := write(t, "c.json", `{not json`)
	if _, err := Load(p, env(map[string]string{"ECOWITT_HOST": "h"})); err == nil {
		t.Fatal("expected error")
	}
}

func TestOtherReadError(t *testing.T) {
	if _, err := Load(t.TempDir(), env(map[string]string{"ECOWITT_HOST": "h"})); err == nil {
		t.Fatal("expected error reading a directory")
	}
}

func TestMissingHost(t *testing.T) {
	_, err := Load("", env(nil))
	want := `gateway host is required (set "host" in the config file or ECOWITT_HOST)`
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}
