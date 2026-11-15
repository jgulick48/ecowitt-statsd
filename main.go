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
	"github.com/jgulick48/ecowitt-statsd/internal/metrics"
	"github.com/jgulick48/ecowitt-statsd/internal/models"
	"github.com/jgulick48/ecowitt-statsd/internal/prometheus"
)

var configLocation = flag.String("configFile", "/var/lib/ecowitt-statsd/config.json", "Location for the configuration file.")

func main() {
	flag.Parse()
	config :=LoadClientConfig(*configLocation)
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
	metricsClient := metrics.NewMultiClient(clients...)
	client := ecowitt.NewClient(config.Host, 10*time.Second, http.DefaultClient, metricsClient)
	go client.StartScan()
	fmt.Println("Running application")
	<-done
	client.StopScan()
	if httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down HTTP server %s", err.Error())
		}
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
