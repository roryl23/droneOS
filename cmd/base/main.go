package main

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/controller"
	"droneOS/internal/drivers/radio/SX1262"
	"droneOS/internal/protocol"
	"droneOS/internal/utils"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var RadioFuncMap = map[string]any{
	"SX1262": SX1262.Main,
}

func radio(ctx context.Context, s *config.Config) {
	// Start TCP server
	addr := fmt.Sprintf("%s:%d", s.Base.Host, s.Base.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).
			Msg("error starting TCP server")
	}
	defer listener.Close()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	log.Info().
		Str("addr", addr).
		Msg("TCP server listening")

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error().Err(err).
				Msg("error accepting connection")
			continue
		}
		go protocol.TCPHandler(ctx, conn)
	}
}

func initRadioLink(ctx context.Context, name string, cfg *config.Radio) (protocol.RadioLink, error) {
	if strings.TrimSpace(name) == "" || strings.EqualFold(name, "none") {
		return nil, nil
	}
	outputs, err := utils.CallFunctionByName(ctx, RadioFuncMap, name, cfg)
	if err != nil {
		return nil, err
	}
	if len(outputs) < 2 {
		return nil, fmt.Errorf("radio driver %q returned %d values", name, len(outputs))
	}
	if errVal, ok := outputs[1].Interface().(error); ok && errVal != nil {
		return nil, errVal
	}
	link, ok := outputs[0].Interface().(protocol.RadioLink)
	if !ok || link == nil {
		return nil, fmt.Errorf("radio driver %q returned unexpected link type", name)
	}
	return link, nil
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
	controllerName := strings.TrimSpace(settings.Base.Controller)
	if controllerName == "" || strings.EqualFold(controllerName, "none") {
		log.Info().Msg("controller disabled")
	} else {
		ctrlFnAny, ok := controller.FuncMap[controllerName]
		if !ok {
			log.Error().Str("controller", controllerName).
				Msg("unknown controller")
		} else if ctrlFn, ok := ctrlFnAny.(func(context.Context, *chan controller.Event[any]) error); !ok {
			log.Error().Str("controller", controllerName).
				Msg("controller has unexpected signature")
		} else {
			go func() {
				for {
					err := ctrlFn(ctx, &controllerChannel)
					if err == nil || errors.Is(err, context.Canceled) {
						return // normal shutdown
					}
					log.Warn().Err(err).
						Msg("controller interface failed, retrying in 1 second...")
					select {
					case <-ctx.Done():
						return // shutdown requested
					case <-time.After(1 * time.Second):
						// retry
					}
				}
			}()
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case ev := <-controllerChannel:
						queued := protocol.EnqueueControllerCommand(protocol.ControllerCommand{
							Action:  ev.Action,
							Payload: ev.Payload,
						})
						if !queued {
							log.Warn().Str("action", ev.Action).
								Msg("controller command queue full")
							continue
						}
						log.Info().Str("action", ev.Action).
							Interface("payload", ev.Payload).
							Msg("controller command queued")
					}
				}
			}()
		}
	}

	radioLink, err := initRadioLink(ctx, settings.Base.Radio.Name, &settings.Base.Radio)
	if err != nil {
		log.Error().Err(err).Msg("error initializing radio")
	} else if radioLink != nil {
		go protocol.ServeRadio(ctx, radioLink)
	}

	radio(ctx, &settings)
}
