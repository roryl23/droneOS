package main

import (
	"droneOS/internal/config"
	"droneOS/internal/protocol"
	"flag"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/joystick"
	"net"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	configFile := flag.String("config-file", "configs/config.yaml", "config file location")
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info().Interface("settings", settings)

	// initialize joystick
	log.Info().Msg("initialize joystick")
	joystickAdaptor := joystick.NewAdaptor()
	err := joystickAdaptor.Connect()
	if err != nil {
		log.Warn().Err(err).Msg("failed to connect to joystick")
	}
	defer joystickAdaptor.Finalize()
	j := joystick.NewDriver(joystickAdaptor, settings.Base.Joystick)

	work := func() {
		// buttons
		j.On(joystick.APress, func(data interface{}) {
			log.Info().Msg("a press")
			// TODO: send over the wire to drone
		})
		j.On(joystick.ARelease, func(data interface{}) {
			log.Info().Msg("a release")
		})
	}
	robot := gobot.NewRobot("joystickBot",
		[]gobot.Connection{joystickAdaptor},
		[]gobot.Device{j},
		work,
	)
	go func() {
		err := robot.Start()
		if err != nil {
			log.Fatal().Err(err).Msg("failed to start robot")
		}
	}()

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
