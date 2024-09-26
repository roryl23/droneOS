package main

import (
	"droneOS/internal/config"
	"droneOS/internal/input/sensor"
	"time"
)

func Main(s *config.Config, eventCh *chan sensor.Event) {
	for {
		//log.Info("sensor plugin MPU_6050 is running")
		time.Sleep(500 * time.Millisecond)
	}
}
