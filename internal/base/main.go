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
	// initialize joystick
	log.Info("initialize joystick")
	joystickAdaptor := joystick.NewAdaptor()
	err := joystickAdaptor.Connect()
	if err != nil {
		log.Fatal("failed to connect to joystick: ", err)
	}
	j := joystick.NewDriver(joystickAdaptor, s.Base.Joystick)
	//j := joystick.NewDriver(joystickAdaptor, "./platforms/joystick/configs/xbox360_power_a_mini_proex.json")

	work := func() {
		// buttons
		j.On(joystick.APress, func(data interface{}) {
			log.Info("a press")
		})
		j.On(joystick.ARelease, func(data interface{}) {
			log.Info("a release")
		})
	}
	robot := gobot.NewRobot("joystickBot",
		[]gobot.Connection{joystickAdaptor},
		[]gobot.Device{j},
		work,
	)
	go robot.Start()

	http.HandleFunc("/", handler)
	log.Infof("HTTP server listening on port %d", s.Base.Port)
	log.Fatal(http.ListenAndServe(
		fmt.Sprintf("%s:%d", s.Base.Host, s.Base.Port),
		nil,
	))
}
