package frienda_obstacle_431S

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drivers/sensor"
	"time"
)

func Main(
	ctx context.Context,
	s *config.Device,
	eCh *[]chan sensor.Event,
) {
	//name := "frienda_obstacle_431S"
	for {

		// send event to all channels
		//for _, ch := range *eCh {
		//	ch <- sensor.Event{
		//		Name: name,
		//		Data: 0,
		//	}
		//}
		time.Sleep(500 * time.Millisecond)
	}
}
