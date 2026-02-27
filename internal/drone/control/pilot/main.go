package pilot

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drivers/sensor"
	"droneOS/internal/drone"
	"droneOS/internal/drone/control"
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
	taskQueue *chan drone.Task,
) {
	_ = ctx
	_ = s
	_ = sensorEvents
	_ = taskQueue
	lastLog := time.Time{}
	for {
		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)
		priorityMutex.Lock(priority)

		if time.Since(lastLog) >= 5*time.Second {
			log.Debug().Msg("controller algorithm pilot running")
			lastLog = time.Now()
		}
		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)

		priorityMutex.Unlock()
		if time.Since(lastLog) >= 5*time.Second {
			log.Debug().Msg("controller algorithm pilot finished")
			lastLog = time.Now()
		}
	}
}
