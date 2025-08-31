package main

import (
	"droneOS/internal/base/controller"
	"droneOS/internal/config"
	"droneOS/internal/protocol"
	"droneOS/internal/utils"
	"flag"
	"fmt"
	"net"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	configFile := flag.String(
		"config-file",
		"configs/config.yaml",
		"config file location",
	)
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info().Interface("settings", settings)

	// initialize the configured controller interface and handler
	controllerChannel := make(chan controller.Event[any])
	go func() {
		output, err := utils.CallFunctionByName(
			controller.FuncMap,
			settings.Base.Controller,
			&controllerChannel,
		)
		if err != nil {
			log.Error().
				Err(err).
				Interface("output", output).
				Msg("error initializing controller")
			return
		}
	}()
	go controller.EventHandler(controllerChannel)

	// Start TCP server
	addr := fmt.Sprintf("%s:%d", settings.Base.Host, settings.Base.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).Msg("error starting TCP server")
	}
	defer listener.Close()
	log.Info().Str("addr", addr).Msg("TCP server listening")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Error().Err(err).Msg("error accepting connection")
		}
		protocol.TCPHandler(conn)
	}
}
