package MG90S

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drone"
	"time"
)

func Main(
	ctx context.Context,
	s *config.Device,
	taskQueue *chan drone.Task,
) error {
	for {
		//log.Info("output plugin MG90S running. Input: ", i)
		time.Sleep(500 * time.Millisecond)
	}
}
