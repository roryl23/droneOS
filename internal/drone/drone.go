package drone

import (
	"droneOS/internal/config"
	"time"
)

func Main(c *config.Config) {
	for {
		pluginFunctions := config.LoadPlugins(c)
		for _, plugin := range pluginFunctions {
			plugin()
			time.Sleep(time.Second * 1) //TODO: get rid of this
		}
	}
}
