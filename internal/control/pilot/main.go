package main

import (
	"droneOS/internal/config"
	"droneOS/internal/input/sensor"
	"droneOS/internal/output"
	"time"
)

func Main(s *config.Config, priority int, sensorEvent *chan sensor.Event, pq *output.Queue) {
	for {
		//log.Info("control algorithm pilot running")
		time.Sleep(500 * time.Millisecond)
	}
}
