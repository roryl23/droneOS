package main

import (
	"droneOS/internal/base"
	"droneOS/internal/config"
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

	log.Info().Msg("started droneOS - base")
	base.Main(&settings)
}
