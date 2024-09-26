package main

import (
	"droneOS/internal/config"
	"droneOS/internal/input/sensor"
	"time"
)

func Main(s *config.Config, eventCh *chan sensor.Event) {
	for {
		//log.Info("sensor plugin HC_SR04 is running")
		time.Sleep(100 * time.Millisecond)
	}
}
