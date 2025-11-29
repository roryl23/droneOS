package controller

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/veandco/go-sdl2/sdl"
	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/joystick"
)

const (
	JoystickName = "xbox360"
)

func Xbox360Interface(
	ctx context.Context,
	cCh *chan Event[any],
) error {
	logger := zerolog.Ctx(ctx)
	for {
		adaptor := joystick.NewAdaptor()
		if err := adaptor.Connect(); err != nil {
			logger.Debug().Msg("no joystick found, waiting...")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(250 * time.Millisecond):
				continue
			}
		}

		logger.Info().Msg("Xbox 360 connected")

		// device appeared — try to use it
		driver := joystick.NewDriver(adaptor, JoystickName)

		driver.On(joystick.APress, func(data any) {
			*cCh <- Event[any]{
				Action:  TAKEOFF,
				Payload: true,
			}
		})
		driver.On(joystick.BPress, func(data any) {
			*cCh <- Event[any]{
				Action:  LAND,
				Payload: true,
			}
		})
		driver.On(joystick.LeftX, func(data any) {
			if v, ok := data.(int16); ok {
				*cCh <- Event[any]{
					Action:  MOVE_X,
					Payload: v,
				}
			} else {
				logger.Warn().
					Interface("payload", data).
					Msg("unexpected data type for LeftX")
			}
		})
		driver.On(joystick.LeftY, func(data any) {
			if v, ok := data.(int16); ok {
				*cCh <- Event[any]{
					Action:  MOVE_Y,
					Payload: v,
				}
			} else {
				logger.Warn().
					Interface("payload", data).
					Msg("unexpected data type for LeftY")
			}
		})
		driver.On(joystick.RightX, func(data any) {
			if v, ok := data.(int16); ok {
				*cCh <- Event[any]{
					Action:  ROTATE,
					Payload: v,
				}
			} else {
				logger.Warn().
					Interface("payload", data).
					Msg("unexpected data type for RightX")
			}
		})
		driver.On(joystick.RightY, func(data any) {
			if v, ok := data.(int16); ok {
				*cCh <- Event[any]{
					Action:  ADJUST_ALTITUDE,
					Payload: v,
				}
			} else {
				logger.Warn().
					Interface("payload", data).
					Msg("unexpected data type for RightY")
			}
		})

		robot := gobot.NewRobot("joystick",
			[]gobot.Connection{adaptor},
			[]gobot.Device{driver},
		)

		if err := robot.Start(false); err != nil {
			adaptor.Finalize()
			continue
		}
		defer robot.Stop()

		disconnect := make(chan struct{})
		go func() {
			for {
				if sdl.NumJoysticks() < 1 {
					close(disconnect)
					return
				}
				time.Sleep(250 * time.Millisecond)
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-disconnect:
			continue // retry on unplug
		}
	}
}
