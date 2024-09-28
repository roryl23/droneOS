package sensor

import (
	"droneOS/internal/config"
)

type Event struct {
	Name string
	Data interface{}
}

type Sensor struct {
	Name string
	Main func(c *config.Config, eCh *chan Event)
}
