package drone

import (
	"bytes"
	"droneOS/internal/base"
	"droneOS/internal/config"
	"encoding/json"
	"errors"
	"fmt"
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
	resp.Body.Close()
	return nil
}
