package main

import (
	"container/heap"
	"droneOS/internal/config"
	"droneOS/internal/input/sensor"
	"droneOS/internal/output"
	log "github.com/sirupsen/logrus"
	"time"
)

func Main(s *config.Config, priority int, sensorEvent chan sensor.Event, pq *output.Queue) {
	log.Info("Starting obstacle_avoidance plugin...")

	for {

		motor := "hawks_work_ESC"
		motorInput := make([]uint8, 4)
		motorInput[0] = 0
		motorInput[1] = 0
		motorInput[2] = 0
		motorInput[3] = 0

		task := &output.Task{
			Priority: priority,
			Name:     motor,
			Input:    motorInput,
		}
		heap.Push(pq, task)

		time.Sleep(100 * time.Millisecond)
	}
}
