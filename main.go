package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/jgulick48/ecowitt-statsd/internal/ecowitt"
	"github.com/jgulick48/ecowitt-statsd/internal/health"
	"github.com/jgulick48/ecowitt-statsd/internal/metrics"
	"github.com/jgulick48/ecowitt-statsd/internal/models"
	ecomqtt "github.com/jgulick48/ecowitt-statsd/internal/mqtt"
	"github.com/jgulick48/ecowitt-statsd/internal/prometheus"
)

var configLocation = flag.String("configFile", "/var/lib/ecowitt-statsd/config.json", "Location for the configuration file.")

var (
	healthcheck    = flag.Bool("healthcheck", false, "Perform an HTTP health check against -healthcheckURL and exit 0 (healthy) or 1.")
	healthcheckURL = flag.String("healthcheckURL", "http://127.0.0.1:9090/healthz", "URL used by -healthcheck.")
)

// runHealthcheck GETs url and returns the process exit code: 0 on HTTP 200,
// 1 otherwise.
func runHealthcheck(url string) int {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck failed:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck failed: status", resp.StatusCode)
		return 1
	}
	return 0
}

// gatewayTimeout is 80% of the poll interval, clamped to [2s, 30s].
func gatewayTimeout(interval time.Duration) time.Duration {
	t := interval * 8 / 10
	if t < 2*time.Second {
		return 2 * time.Second
	}
	if t > 30*time.Second {
		return 30 * time.Second
	}
	return t
}

// effectiveListenAddress picks listenAddress, then prometheus.listenAddress,
// then :9090.
func effectiveListenAddress(c models.EcowittConfiguration) string {
	if c.ListenAddress != "" {
		return c.ListenAddress
	}
	if c.Prometheus.ListenAddress != "" {
		return c.Prometheus.ListenAddress
	}
	return ":9090"
}

func main() {
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck(*healthcheckURL))
	}
	startTime := time.Now()
	config := LoadClientConfig(*configLocation)
	pollInterval, err := config.PollIntervalDuration()
	if err != nil {
		log.Fatalf("Invalid configuration: %s", err.Error())
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan bool, 1)
	go func() {
		sig := <-sigs
		fmt.Println()
		fmt.Println(sig)
		done <- true
	}()
	var clients []metrics.Client
	if config.StatsServer != "" {
		statsdClient, err := statsd.New(config.StatsServer)
		if err != nil {
			log.Printf("Error creating stats client %s", err.Error())
		} else {
			clients = append(clients, metrics.NewClient(statsdClient, config.DefaultTags))
		}
	}
	mux := http.NewServeMux()
	if config.Prometheus.Enabled {
		promClient := prometheus.NewClient(config.DefaultTags)
		clients = append(clients, promClient)
		mux.Handle("/metrics", promClient.Handler())
	}
	var mqttClient *ecomqtt.Client
	if config.MQTT.Enabled {
		mc, err := ecomqtt.Connect(config.MQTT, ecomqtt.DeviceID(config.Host))
		if err != nil {
			log.Printf("Error connecting to MQTT, continuing without it: %s", err.Error())
		} else {
			mqttClient = mc
			clients = append(clients, mc)
		}
	}
	metricsClient := metrics.NewMultiClient(clients...)
	// A bounded timeout makes a hung gateway count as a failed scan instead of
	// blocking the poll loop forever.
	gatewayHTTP := &http.Client{Timeout: gatewayTimeout(pollInterval)}
	client := ecowitt.NewClientWithOptions(config.Host, gatewayHTTP, metricsClient, ecowitt.Options{
		TickRate:         pollInterval,
		FailureThreshold: config.FailureThreshold,
	})
	mux.Handle("/healthz", health.Healthz(client.Health, pollInterval, startTime, time.Now))
	mux.Handle("/readyz", health.Readyz(client.Health))
	addr := effectiveListenAddress(config)
	httpServer := &http.Server{Addr: addr, Handler: mux}
	go func() {
		log.Printf("Serving HTTP on %s (/healthz, /readyz, metrics enabled: %v)", addr, config.Prometheus.Enabled)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error %s", err.Error())
		}
	}()
	go client.StartScan()
	fmt.Println("Running application")
	<-done
	client.StopScan()
	if mqttClient != nil {
		mqttClient.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down HTTP server %s", err.Error())
	}
	fmt.Println("exiting")
}

func LoadClientConfig(filename string) models.EcowittConfiguration {
	log.Printf("Loading configuration from %s", filename)
	if filename == "" {
		filename = "./config.json"
	}
	configFile, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("No config file found. Making new IDs")
		fmt.Println(err)
	}
	var config models.EcowittConfiguration
	err = json.Unmarshal(configFile, &config)
	if err != nil {
		log.Printf("Invliad config file provided")
		fmt.Println(err)
	}
	return config
}
