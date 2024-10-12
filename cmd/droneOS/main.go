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

	mode := flag.String("mode", "base", "[base, drone]")
	log.Info("mode: ", *mode)
	configFile := flag.String("config-file", "configs/config.yaml", "config file location")
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info(settings)

	chips := gpio.Init()
	log.Info("available chips: ", chips)

	log.Info("started droneOS")
	result, err := utils.CallFunctionByName(funcMap, *mode, &settings)
	if err != nil {
		log.Fatal("result: ", result, " error: ", err)
	}
}
