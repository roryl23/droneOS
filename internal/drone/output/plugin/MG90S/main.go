package MG90S

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drone/output"
	"time"
)

func Main(
	ctx context.Context,
	s *config.Device,
	taskQueue *chan output.Task,
) error {
	for {
		//log.Info("output plugin MG90S running. Input: ", i)
		time.Sleep(500 * time.Millisecond)
	}
}
