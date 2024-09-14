package main

// github.com/thinkski/go-v4l2
import (
	"droneOS/internal/config"
	"flag"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.DebugLevel)

	log.Info("started droneOS")

	configFile := flag.String("config-file", "configs/config.yaml", "config file location")
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info(settings)

	//debug.SetGCPercent(-1)
	//debug.SetMemoryLimit(math.MaxInt64)
	//
	//gpio.Init()
	//
	//runtime.GC()
}
