package main

import (
	"droneOS/internal/base"
	"droneOS/internal/config"
	"droneOS/internal/drone"
	"droneOS/internal/gpio"
	"droneOS/internal/utils"
	"flag"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var funcMap = map[string]interface{}{
	"base":  base.Main,
	"drone": drone.Main,
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	mode := flag.String("mode", "base", "[base, drone]")
	log.Info().Str("mode", *mode)
	configFile := flag.String("config-file", "configs/config.yaml", "config file location")
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info().Interface("settings", settings)

	chips := gpio.Init()
	log.Info().Interface("chips", chips)

	log.Info().Msg("started droneOS")
	result, err := utils.CallFunctionByName(funcMap, *mode, &settings)
	if err != nil {
		log.Fatal().Err(err).Interface("result", result)
	}
}
