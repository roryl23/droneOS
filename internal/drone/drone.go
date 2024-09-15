package drone

import (
	"droneOS/internal/config"
	"droneOS/internal/gpio"
	log "github.com/sirupsen/logrus"
	"time"
)

func Main(settings *config.Config) {
	chips := gpio.Init()
	log.Info("Available chips: ", chips)
	for {
		pluginFunctions := config.LoadPlugins(settings)
		for _, plugin := range pluginFunctions {
			plugin(settings)
			time.Sleep(time.Second * 1) //TODO: get rid of this
		}
	}
}
