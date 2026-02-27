package obstacle_avoidance

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
	motor := "hawks_work_ESC"
	lastLog := time.Time{}
	for {
		sensorEvent := <-*sensorEvents
		if time.Since(lastLog) >= 5*time.Second {
			log.Debug().Interface("sensorEvent", sensorEvent)
			lastLog = time.Now()
		}

		priorityMutex.Lock(priority)

		task := drone.Task{
			Name: motor,
			Data: 0,
		}
		*taskQueue <- task

		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)
		priorityMutex.Unlock()
	}
}
