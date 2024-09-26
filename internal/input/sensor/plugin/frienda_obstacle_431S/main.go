package main

import (
	"droneOS/internal/config"
	"droneOS/internal/input/sensor"
	log "github.com/sirupsen/logrus"
	"time"
)

func Main(s *config.Config, eventCh *chan sensor.Event) {
	for {
		log.Info("sensor plugin frienda_obstacle_431S is running")
		time.Sleep(100 * time.Millisecond)
	}
}
