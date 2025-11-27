package main

import (
	"context"
	"droneOS/internal/base/controller"
	"droneOS/internal/config"
	"droneOS/internal/protocol"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := zerolog.New(os.Stdout)
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,  // ctrl+C
		syscall.SIGTERM, // docker stop, systemd
	)
	defer stop() // restores default signal behavior
	ctx = logger.WithContext(ctx)

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
		for {
			err := controller.Xbox360Interface(ctx, &controllerChannel)
			if err == nil {
				return
			}

			log.Warn().Err(err).
				Msg("controller failed, retrying in 5 seconds...")

			select {
			case <-ctx.Done():
				return // shutdown requested
			case <-time.After(5 * time.Second):
				// retry
			}
		}
	}()
	go controller.EventHandler(ctx, controllerChannel)

	// Start TCP server
	addr := fmt.Sprintf("%s:%d", settings.Base.Host, settings.Base.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).
			Msg("error starting TCP server")
	}
	defer listener.Close()
	log.Info().
		Str("addr", addr).
		Msg("TCP server listening")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Error().Err(err).
				Msg("error accepting connection")
		}
		protocol.TCPHandler(ctx, conn)
	}
}
