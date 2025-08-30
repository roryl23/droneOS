package hawks_work_ESC

import (
	"droneOS/internal/config"
	"droneOS/internal/drone/output"
	"github.com/rs/zerolog/log"
	"time"
)

func Main(
	s *config.Device,
	taskQueue *chan output.Task,
) error {
	for {
		task := <-*taskQueue
		log.Info().Interface("task", task)

		time.Sleep(500 * time.Millisecond)
	}
}
