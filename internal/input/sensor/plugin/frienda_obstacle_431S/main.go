package frienda_obstacle_431S

import (
	"droneOS/internal/config"
	"droneOS/internal/input/sensor"
	"time"
)

func Main(
	s *config.Config,
	eCh *[]chan sensor.Event,
) {
	name := "frienda_obstacle_431S"
	for {
		for _, ch := range *eCh {
			ch <- sensor.Event{
				Name: name,
				Data: 0,
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}
