package hawks_work_ESC

import (
	"droneOS/internal/config"
	"droneOS/internal/output"
	log "github.com/sirupsen/logrus"
	"time"
)

func Main(
	s *config.Device,
	taskQueue *chan output.Task,
) error {
	for {
		task := <-*taskQueue
		log.Info("task: ", task)

		time.Sleep(500 * time.Millisecond)
	}
}
