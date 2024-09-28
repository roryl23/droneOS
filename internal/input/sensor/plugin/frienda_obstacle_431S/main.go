package frienda_obstacle_431S

import (
	"droneOS/internal/config"
	"droneOS/internal/input/sensor"
	"time"
)

func Main(s *config.Config, eCh *[]chan sensor.Event) {
	for {
		//log.Info("sensor plugin frienda_obstacle_431S is running")
		time.Sleep(500 * time.Millisecond)
	}
}
