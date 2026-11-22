package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	appconfig "github.com/jgulick48/ecowitt-statsd/internal/config"
	"github.com/jgulick48/ecowitt-statsd/internal/ecowitt"
	"github.com/jgulick48/ecowitt-statsd/internal/metrics"
	ecomqtt "github.com/jgulick48/ecowitt-statsd/internal/mqtt"
	"github.com/jgulick48/ecowitt-statsd/internal/prometheus"
)

var configLocation = flag.String("configFile", "/var/lib/ecowitt-statsd/config.json", "Location for the configuration file.")

func main() {
	flag.Parse()
	config, err := appconfig.Load(*configLocation, os.Getenv)
	if err != nil {
		log.Fatalf("Error loading configuration: %s", err)
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
	var httpServer *http.Server
	if config.Prometheus.Enabled {
		addr := config.Prometheus.ListenAddress
		if addr == "" {
			addr = ":9090"
		}
		promClient := prometheus.NewClient(config.DefaultTags)
		clients = append(clients, promClient)
		mux := http.NewServeMux()
		mux.Handle("/metrics", promClient.Handler())
		httpServer = &http.Server{Addr: addr, Handler: mux}
		go func() {
			log.Printf("Serving prometheus metrics on %s/metrics", addr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Prometheus HTTP server error %s", err.Error())
			}
		}()
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
	client := ecowitt.NewClient(config.Host, 10*time.Second, http.DefaultClient, metricsClient)
	go client.StartScan()
	fmt.Println("Running application")
	<-done
	client.StopScan()
	if mqttClient != nil {
		mqttClient.Close()
	}
	if httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down HTTP server %s", err.Error())
		}
	}
	fmt.Println("exiting")
}
