package pilot

import (
	"droneOS/internal/config"
	"droneOS/internal/drone/control"
	"droneOS/internal/input/sensor"
	"droneOS/internal/output"
	"github.com/rs/zerolog/log"
	"math/rand"
	"time"
)

func Main(
	s *config.Config,
	priority int,
	priorityMutex *control.PriorityMutex,
	sensorEvents *chan sensor.Event,
	taskQueue *chan output.Task,
) {
	for {
		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)
		priorityMutex.Lock(priority)

		log.Info().Msg("control algorithm pilot running")
		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)

		priorityMutex.Unlock()
		log.Info().Msg("control algorithm pilot finished")
	}
}
