package joystick

import (
	log "github.com/sirupsen/logrus"
	"gobot.io/x/gobot/platforms/raspi"
)

func Main() {
	rpi := raspi.NewAdaptor()
	log.Info("rpi: ", rpi)
}
