package hawks_work_ESC

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drone/output"
	"time"

	"github.com/rs/zerolog/log"
)

func Main(
	ctx context.Context,
	s *config.Device,
	taskQueue *chan output.Task,
) error {
	for {
		task := <-*taskQueue
		log.Info().Interface("task", task)

		time.Sleep(500 * time.Millisecond)
	}
}
