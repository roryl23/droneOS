package drone

import (
	"droneOS/internal/config"
	"droneOS/internal/gpio"
	log "github.com/sirupsen/logrus"
	"net"
	"time"
)

func Main(s *config.Config) {
	baseHost := net.ParseIP(s.Base.Host)
	if baseHost == nil {
		log.Errorf("Invalid IP address for base host: %s", s.Base.Host)
	}

	chips := gpio.Init()
	log.Info("Available chips: ", chips)

	for {
		// try to connect to base

		pluginFunctions := config.LoadPlugins(s)
		for _, plugin := range pluginFunctions {
			plugin(s)
			time.Sleep(time.Second * 1) //TODO: get rid of this
		}
	}
}
