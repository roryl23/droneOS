package base

import (
	"droneOS/internal/config"
	"droneOS/internal/protocol"
	"droneOS/internal/utils"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
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

	// initialize joystick
	go func() {
		_, err := utils.CallFunctionByName(
			BaseFuncMap,
			s.Base.Joystick,
			&sensorEventChannels,
		)
		if err != nil {
			log.Error(err)
		}
	}()
}
