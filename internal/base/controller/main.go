package controller

import (
	"context"

	"github.com/rs/zerolog"
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

func EventHandler(ctx context.Context, eCh <-chan Event[any]) {
	logger := zerolog.Ctx(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-eCh:
			if !ok {
				return // channel closed
			}

			ev := logger.Info().
				Str("action", e.Action)

			switch payload := e.Payload.(type) {
			case bool:
				ev.Bool("payload", payload)
			case int16:
				ev.Int16("payload", payload)
			default:
				logger.Warn().
					Str("action", e.Action).
					Interface("payload", payload).
					Msg("unknown payload type")
				continue
			}
		}
	}
}
