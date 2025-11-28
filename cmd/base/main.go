package main

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/controller"
	"droneOS/internal/protocol"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func radio(ctx context.Context, s *config.Config) {
	// Start TCP server
	addr := fmt.Sprintf("%s:%d", s.Base.Host, s.Base.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).
			Msg("error starting TCP server")
	}
	defer listener.Close()
	log.Info().
		Str("addr", addr).
		Msg("TCP server listening")

	client := &http.Client{
		Timeout: 10 * time.Millisecond,
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Error().Err(err).
				Msg("error accepting connection")
		}
		protocol.TCPHandler(ctx, conn)

		if !s.Drone.Radio.AlwaysUse {
			status, err := protocol.CheckWiFi(ctx, s, client)
			if err != nil || status == false {
				//log.Info("WiFi not connected, using radio...")
			} else {
				//log.Info("WiFi connected, using WiFi...")
			}
		} else {
			//log.Info("Always using radio...")
		}
	}
}

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
			log.Warn().Err(err).
				Msg("controller.Xbox360Interface failed, retrying in 1 second...")
			select {
			case <-ctx.Done():
				return // shutdown requested
			case <-time.After(1 * time.Second):
				// retry
			}
		}
	}()
	go controller.EventHandler(ctx, controllerChannel)

	radio(ctx, &settings)
}
