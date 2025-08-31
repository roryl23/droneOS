package controller

import (
	"github.com/rs/zerolog/log"
	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/joystick"
)

const (
	JoystickName = "xbox360"
)

func Xbox360(cCh *chan Event[any]) {
	joystickAdaptor := joystick.NewAdaptor()
	err := joystickAdaptor.Connect()
	if err != nil {
		log.Warn().Err(err).Msg("failed to connect to controller")
	}
	defer func(joystickAdaptor *joystick.Adaptor) {
		err := joystickAdaptor.Finalize()
		if err != nil {
			log.Fatal().Err(err).Msg("failed to finalize joystick adaptor")
		}
	}(joystickAdaptor)
	j := joystick.NewDriver(joystickAdaptor, JoystickName)

	inputHandler := func() {
		j.On(joystick.APress, func(data interface{}) {
			*cCh <- Event[any]{
				Action:  TAKEOFF,
				Payload: true,
			}
		})
		j.On(joystick.BPress, func(data interface{}) {
			*cCh <- Event[any]{
				Action:  LAND,
				Payload: true,
			}
		})
		j.On(joystick.LeftX, func(data interface{}) {
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
		j.On(joystick.LeftY, func(data interface{}) {
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
		j.On(joystick.RightX, func(data interface{}) {
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
		j.On(joystick.RightY, func(data interface{}) {
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
	defer func(robot *gobot.Robot) {
		err := robot.Stop()
		if err != nil {
			log.Fatal().Err(err).Msg("failed to stop robot")
		}
	}(robot)
	err = robot.Start()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start robot")
	}
}
