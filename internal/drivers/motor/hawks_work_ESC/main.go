package hawks_work_ESC

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drone"
	"time"

	"github.com/rs/zerolog/log"
)

func Main(
	ctx context.Context,
	s *config.Device,
	taskQueue *chan drone.Task,
) error {
	for {
		task := <-*taskQueue
		log.Info().Interface("task", task)

		time.Sleep(500 * time.Millisecond)
	}
}
