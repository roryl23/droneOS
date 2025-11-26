package controller

import (
	"context"

	"github.com/rs/zerolog/log"
)

// these constants are used to identify the actions
// that can be performed by the controller,
// providing an abstraction layer between arbitrary
// controllers and the drone code.
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

func EventHandler(ctx context.Context, eCh chan Event[any]) {
	for {
		e := <-eCh
		switch v := e.Payload.(type) {
		// buttons
		case bool:
			log.Info().
				Str("action", e.Action).
				Interface("payload", v)
		// axes movement
		case int16:
			log.Info().
				Str("action", e.Action).
				Interface("payload", v)
		default:
			log.Warn().
				Str("action", e.Action).
				Interface("payload", v).
				Msg("unknown payload type")
		}
	}
}
