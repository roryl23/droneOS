package drone

import (
	"bytes"
	"droneOS/internal/base"
	"droneOS/internal/config"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
)

func pingBaseWiFi(s *config.Config) error {
	msg := base.Message{
		ID:   s.Drone.ID,
		Type: "ping",
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		errors.New("error encoding JSON")
	}

	resp, err := http.Post(
		fmt.Sprintf("http://%s:%d", s.Base.Host, s.Base.Port),
		"application/json",
		bytes.NewBuffer(msgBytes),
	)
	if err != nil {
		return errors.New("error sending request")
	}

	responseString := make([]byte, 1024)
	bytes, err := resp.Body.Read(responseString)
	if err != nil {
		return errors.New(fmt.Sprintf("error reading response: %s", err))
	}
	responseString = responseString[:bytes]
	log.Debug(fmt.Sprintf("base response: %s", responseString))

	resp.Body.Close()
	return nil
}
