package obstacle_avoidance

import (
	"droneOS/internal/config"
	"droneOS/internal/control"
	"droneOS/internal/input/sensor"
	"droneOS/internal/output"
	log "github.com/sirupsen/logrus"
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
	motor := "hawks_work_ESC"
	for {
		sensorEvent := <-*sensorEvents
		log.Info("sensorEvent: ", sensorEvent)

		priorityMutex.Lock(priority)

		task := output.Task{
			Name: motor,
			Data: 0,
		}
		*taskQueue <- task

		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)
		priorityMutex.Unlock()
	}
}
