package base

import (
	"droneOS/internal/config"
	"droneOS/internal/protocol"
	"droneOS/internal/utils"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/joystick"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	var msg protocol.Message
	err := json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Debugf("%+v", msg)

	output, err := utils.CallFunctionByName(BaseFuncMap, msg.Cmd, nil)
	if err != nil {
		log.Fatal(err)
	} else {
		data, err := json.Marshal(output)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_, err = w.Write(data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func Main(s *config.Config) {
	http.HandleFunc("/", handler)

	log.Infof("HTTP server listening on port %d", s.Base.Port)
	log.Fatal(
		http.ListenAndServe(
			fmt.Sprintf("%s:%d", s.Base.Host, s.Base.Port),
			nil,
		),
	)

	// TODO: this initialization should be generalized
	//       to allow user defined joystick code
	// initialize joystick
	go func() {
		joystickAdaptor := joystick.NewAdaptor()
		stick := joystick.NewDriver(joystickAdaptor, s.Base.Joystick)

		work := func() {
			// buttons
			stick.On(joystick.SquarePress, func(data interface{}) {
				fmt.Println("square_press")
			})
			stick.On(joystick.SquareRelease, func(data interface{}) {
				fmt.Println("square_release")
			})
			stick.On(joystick.TrianglePress, func(data interface{}) {
				fmt.Println("triangle_press")
			})
			stick.On(joystick.TriangleRelease, func(data interface{}) {
				fmt.Println("triangle_release")
			})
			stick.On(joystick.CirclePress, func(data interface{}) {
				fmt.Println("circle_press")
			})
			stick.On(joystick.CircleRelease, func(data interface{}) {
				fmt.Println("circle_release")
			})
			stick.On(joystick.XPress, func(data interface{}) {
				fmt.Println("x_press")
			})
			stick.On(joystick.XRelease, func(data interface{}) {
				fmt.Println("x_release")
			})
			stick.On(joystick.StartPress, func(data interface{}) {
				fmt.Println("start_press")
			})
			stick.On(joystick.StartRelease, func(data interface{}) {
				fmt.Println("start_release")
			})
			stick.On(joystick.SelectPress, func(data interface{}) {
				fmt.Println("select_press")
			})
			stick.On(joystick.SelectRelease, func(data interface{}) {
				fmt.Println("select_release")
			})

			// joysticks
			stick.On(joystick.LeftX, func(data interface{}) {
				fmt.Println("left_x", data)
			})
			stick.On(joystick.LeftY, func(data interface{}) {
				fmt.Println("left_y", data)
			})
			stick.On(joystick.RightX, func(data interface{}) {
				fmt.Println("right_x", data)
			})
			stick.On(joystick.RightY, func(data interface{}) {
				fmt.Println("right_y", data)
			})

			// triggers
			stick.On(joystick.R1Press, func(data interface{}) {
				fmt.Println("R1Press", data)
			})
			stick.On(joystick.R2Press, func(data interface{}) {
				fmt.Println("R2Press", data)
			})
			stick.On(joystick.L1Press, func(data interface{}) {
				fmt.Println("L1Press", data)
			})
			stick.On(joystick.L2Press, func(data interface{}) {
				fmt.Println("L2Press", data)
			})
		}

		robot := gobot.NewRobot("joystickBot",
			[]gobot.Connection{joystickAdaptor},
			[]gobot.Device{stick},
			work,
		)

		robot.Start()
	}()
}
