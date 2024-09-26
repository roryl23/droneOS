package main

import (
	"droneOS/internal/config"
	"droneOS/internal/input/sensor"
	"time"
)

func Main(s *config.Config, eventCh *chan sensor.Event) {
	for {
		//log.Info("sensor plugin GT_U7 is running")
		time.Sleep(500 * time.Millisecond)
	}
}
