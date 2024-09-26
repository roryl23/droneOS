package main

import (
	log "github.com/sirupsen/logrus"
	"time"
)

func Main(i interface{}) error {
	for {
		log.Info("output plugin hawks_work_ESC running. Input: ", i)
		time.Sleep(500 * time.Millisecond)
	}
}
