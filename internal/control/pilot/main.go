package main

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
	sensorEvent *chan sensor.Event,
	tq chan output.Task,
) {
	for {
		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)
		priorityMutex.Lock(priority)

		log.Info("control algorithm pilot running")
		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)

		priorityMutex.Unlock()
		log.Info("control algorithm pilot finished")
	}
}
