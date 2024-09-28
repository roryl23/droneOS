package main

import (
	"droneOS/internal/base"
	"droneOS/internal/config"
	"droneOS/internal/drone"
	"droneOS/internal/gpio"
	"droneOS/internal/utils"
	"flag"
	log "github.com/sirupsen/logrus"
)

var funcMap = map[string]interface{}{
	"base":  base.Main,
	"drone": drone.Main,
}

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.DebugLevel)
	log.Info("started droneOS")

	mode := flag.String("mode", "base", "[base, drone]")
	configFile := flag.String("config-file", "configs/config.yaml", "config file location")
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info(settings)

	chips := gpio.Init()
	log.Info("Available chips: ", chips)

	log.Fatal(utils.CallFunctionByName(funcMap, *mode, &settings))
}
