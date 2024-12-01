package base

import (
	"droneOS/internal/config"
	"droneOS/internal/protocol"
	"fmt"
	log "github.com/sirupsen/logrus"
	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/joystick"
	"net"
)

func Main(s *config.Config) {
	// initialize joystick
	log.Info("initialize joystick")
	joystickAdaptor := joystick.NewAdaptor()
	err := joystickAdaptor.Connect()
	if err != nil {
		log.Fatal("failed to connect to joystick: ", err)
	}
	defer joystickAdaptor.Finalize()
	j := joystick.NewDriver(joystickAdaptor, s.Base.Joystick)

	work := func() {
		// buttons
		j.On(joystick.APress, func(data interface{}) {
			log.Info("a release")
			// TODO: send over the wire to drone
		})
		j.On(joystick.ARelease, func(data interface{}) {
			log.Info("a release")
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
			log.Fatal("failed to start robot: ", err)
		}
	}()

	// Start TCP server
	addr := fmt.Sprintf("%s:%d", s.Base.Host, s.Base.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("error starting TCP server: %v", err)
	}
	defer listener.Close()
	log.Infof("TCP server listening on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Errorf("error accepting connection: %v", err)
		}
		protocol.TCPHandler(conn)
	}
}
