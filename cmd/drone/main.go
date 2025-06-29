package main

import (
	"droneOS/internal/config"
	"droneOS/internal/drone"
	"droneOS/internal/gpio"
	"flag"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	configFile := flag.String("config-file", "configs/config.yaml", "config file location")
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info().Interface("settings", settings)

	chips := gpio.Init()
	log.Info().Interface("chips", chips)

	log.Info().Msg("started droneOS - drone")
	drone.Main(&settings)
}
