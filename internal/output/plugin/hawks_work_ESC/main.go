package main

import (
	log "github.com/sirupsen/logrus"
	"time"
)

func Main(i interface{}) {
	for {
		log.Info("output plugin hawks_work_ESC: ", i)
		time.Sleep(100 * time.Millisecond)
	}
}
