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

		log.Info("control algorithm obstacle_avoidance running")

		//motor := "hawks_work_ESC"
		//motorInput := make([]uint8, 4)
		//motorInput[0] = 0
		//motorInput[1] = 0
		//motorInput[2] = 0
		//motorInput[3] = 0
		//
		//task := &output.Task{
		//	Priority: priority,
		//	Name:     motor,
		//	Input:    motorInput,
		//}
		//tq <- *task

		time.Sleep(time.Duration(rand.Intn(200-100+1)+100) * time.Millisecond)
		priorityMutex.Unlock()
		log.Info("control algorithm obstacle_avoidance finished")
	}
}
