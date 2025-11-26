package GT_U7

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drone/input/sensor"
	"time"
)

func Main(
	ctx context.Context,
	s *config.Device,
	eventCh *chan sensor.Event,
) {
	for {
		//log.Info("sensor plugin GT_U7 is running")
		time.Sleep(500 * time.Millisecond)
	}
}
