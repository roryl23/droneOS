package pilot

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drone/control"
	"droneOS/internal/drone/input/sensor"
	"droneOS/internal/drone/output"
	"math/rand"
	"time"

	"github.com/rs/zerolog/log"
)

func Main(
	ctx context.Context,
	s *config.Config,
	priority int,
	priorityMutex *control.PriorityMutex,
	sensorEvents *chan sensor.Event,
	taskQueue *chan output.Task,
) {
	for {
		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)
		priorityMutex.Lock(priority)

		log.Info().Msg("controller algorithm pilot running")
		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)

		priorityMutex.Unlock()
		log.Info().Msg("controller algorithm pilot finished")
	}
}
