package sensor

import (
	"droneOS/internal/config"
)

type Event struct {
	Type string
}

type Sensor struct {
	Name string
	Main func(c *config.Config, eCh *chan Event)
}
