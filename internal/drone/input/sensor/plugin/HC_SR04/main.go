package HC_SR04

import (
	"droneOS/internal/config"
	"droneOS/internal/drone/input/sensor"
	"github.com/rs/zerolog/log"
	"time"
)

func Main(
	s *config.Device,
	eventCh *chan sensor.Event,
) {
	for {
		log.Info().Msg("sensor plugin HC_SR04 is running")
		time.Sleep(500 * time.Millisecond)
	}
}
