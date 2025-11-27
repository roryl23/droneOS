package controller

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/joystick"
)

const (
	JoystickName = "xbox360"
)

func Xbox360Interfaceold(ctx context.Context, cCh *chan Event[any]) error {
	joystickAdaptor := joystick.NewAdaptor()
	err := joystickAdaptor.Connect()
	if err != nil {
		return err
	}
	defer func(joystickAdaptor *joystick.Adaptor) {
		err := joystickAdaptor.Finalize()
		if err != nil {
			log.Warn().Err(err).
				Msg("failed to finalize joystick adaptor")
		}
	}(joystickAdaptor)
	j := joystick.NewDriver(joystickAdaptor, JoystickName)

	inputHandler := func() {
		j.On(joystick.APress, func(data any) {
			*cCh <- Event[any]{
				Action:  TAKEOFF,
				Payload: true,
			}
		})
		j.On(joystick.BPress, func(data any) {
			*cCh <- Event[any]{
				Action:  LAND,
				Payload: true,
			}
		})
		j.On(joystick.LeftX, func(data any) {
			if v, ok := data.(int16); ok {
				*cCh <- Event[any]{
					Action:  MOVE_X,
					Payload: v,
				}
			} else {
				log.Warn().
					Interface("payload", data).
					Msg("unexpected data type for LeftX")
			}
		})
		j.On(joystick.LeftY, func(data any) {
			if v, ok := data.(int16); ok {
				*cCh <- Event[any]{
					Action:  MOVE_Y,
					Payload: v,
				}
			} else {
				log.Warn().
					Interface("payload", data).
					Msg("unexpected data type for LeftY")
			}
		})
		j.On(joystick.RightX, func(data any) {
			if v, ok := data.(int16); ok {
				*cCh <- Event[any]{
					Action:  ROTATE,
					Payload: v,
				}
			} else {
				log.Warn().
					Interface("payload", data).
					Msg("unexpected data type for RightX")
			}
		})
		j.On(joystick.RightY, func(data any) {
			if v, ok := data.(int16); ok {
				*cCh <- Event[any]{
					Action:  ADJUST_ALTITUDE,
					Payload: v,
				}
			} else {
				log.Warn().
					Interface("payload", data).
					Msg("unexpected data type for RightY")
			}
		})
	}

	robot := gobot.NewRobot("joystickBot",
		[]gobot.Connection{joystickAdaptor},
		[]gobot.Device{j},
		inputHandler,
	)

	if err := robot.Start(false); err != nil {
		return err
	}
	defer robot.Stop()

	// block until context canceled
	<-ctx.Done()
	return nil
}

func Xbox360Interface(ctx context.Context, cCh *chan Event[any]) error {
	for {
		adaptor := joystick.NewAdaptor()
		if err := adaptor.Connect(); err != nil {
			log.Info().Msg("no joystick found, waiting for plug-in...")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second): // poll every 2s
				continue
			}
		}

		// Device appeared — try to use it
		driver := joystick.NewDriver(adaptor, JoystickName)

		// Set up all your event handlers exactly as before
		driver.On(joystick.APress, func(_ any) {
			*cCh <- Event[any]{Action: TAKEOFF, Payload: true}
		})
		driver.On(joystick.BPress, func(_ any) {
			*cCh <- Event[any]{Action: LAND, Payload: true}
		})

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
				log.Warn().
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
				log.Warn().
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
				log.Warn().
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
				log.Warn().
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
			continue // retry loop
		}
		defer robot.Stop()

		// Blocks until device disappears or ctx cancelled
		<-ctx.Done()
		return nil
	}
}
