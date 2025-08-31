package controller

import "github.com/rs/zerolog/log"

const (
	MOVE_X          = "MOVE_X"
	MOVE_Y          = "MOVE_Y"
	ROTATE          = "ROTATE"
	ADJUST_ALTITUDE = "ADJUST_ALTITUDE"
	HOVER           = "HOVER"
	TAKEOFF         = "TAKEOFF"
	LAND            = "LAND"
	EMERGENCY_STOP  = "EMERGENCY_STOP"
	RETURN_TO_HOME  = "RETURN_TO_HOME"
)

type Event[T any] struct {
	Action  string
	Payload T
}

func EventHandler(eCh chan Event[any]) {
	for {
		e := <-eCh
		switch v := e.Payload.(type) {
		// buttons
		case bool:
			println("Action:", e.Action, "Payload:", v)
		// axes movement
		case int16:
			println("Action:", e.Action, "Payload:", v)
		default:
			println("Action:", e.Action, "Payload:", v)
			log.Info().Str("addr", addr).Msg("TCP server listening")
		}
	}
}
