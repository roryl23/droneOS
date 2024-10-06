package xboxone

import (
	"droneOS/internal/protocol"
	log "github.com/sirupsen/logrus"
	"gobot.io/x/gobot/platforms/raspi"
)

func Main(
	bCh *chan protocol.Message,
) {
	rpi := raspi.NewAdaptor()
	log.Info("rpi: ", rpi)
}
