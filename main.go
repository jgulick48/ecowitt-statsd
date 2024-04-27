package main

import (
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
)

var configLocation = flag.String("configFile", "/var/lib/ecowitt-statsd/config.json", "Location for the configuration file.")

func main() {
	config := LoadClientConfig(*configLocation)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan bool, 1)
	go func() {
		sig := <-sigs
		fmt.Println()
		fmt.Println(sig)
		done <- true
	}()
	var err error
	statsdClient, err := statsd.New(fmt.Sprintf(config.StatsServer))
	if err != nil {
		log.Printf("Error creating stats client %s", err.Error())
	}
	metricsClient := metrics.NewClient(statsdClient, config.DefaultTags)
	client := ecowitt.NewClient(config.Host, 10*time.Second, http.DefaultClient, metricsClient)
	client.StartScan()
	fmt.Println("Running application")
	<-done
	client.StopScan()
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
