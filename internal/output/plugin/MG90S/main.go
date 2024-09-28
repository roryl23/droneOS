package MG90S

import (
	"droneOS/internal/config"
	"droneOS/internal/output"
	"time"
)

func Main(
	s *config.Device,
	taskQueue *chan output.Task,
) error {
	for {
		//log.Info("output plugin MG90S running. Input: ", i)
		time.Sleep(500 * time.Millisecond)
	}
}
