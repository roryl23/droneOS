package HC_SR04

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drivers/sensor"
	"time"

	"github.com/rs/zerolog/log"
)

func Main(
	ctx context.Context,
	s *config.Device,
	eventCh *chan sensor.Event,
) {
	for {
		log.Info().Msg("sensor plugin HC_SR04 is running")
		time.Sleep(500 * time.Millisecond)
	}
}
