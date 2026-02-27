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
	_ = ctx
	_ = s
	lastLog := time.Time{}
	for {
		task := <-*taskQueue
		if time.Since(lastLog) >= 5*time.Second {
			log.Debug().Interface("task", task)
			lastLog = time.Now()
		}

		time.Sleep(500 * time.Millisecond)
	}
}
