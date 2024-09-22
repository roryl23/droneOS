package protocol

import (
	"bytes"
	"droneOS/internal/config"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

func CheckWiFi(s *config.Config, c http.Client) (bool, error) {
	msg := Message{
		ID:  s.Drone.ID,
		Cmd: "ping",
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return false, errors.New(fmt.Sprintf("error encoding JSON: %s", err))
	}

	resp, err := c.Post(
		fmt.Sprintf("http://%s:%d", s.Base.Host, s.Base.Port),
		"application/json",
		bytes.NewBuffer(msgBytes),
	)
	if err != nil {
		return false, errors.New(fmt.Sprintf("error sending request: %s", err))
	}
	defer resp.Body.Close()

	data := make([]byte, 1024)
	n, err := resp.Body.Read(data)
	if err != nil && err != io.EOF {
		return false, errors.New(fmt.Sprintf("error reading request: %s", err))
	} else {
		if n > 0 {
			data = data[:n]
			var response Message
			err = json.Unmarshal(data, &response)
			if err != nil {
				return false, errors.New(fmt.Sprintf("error decoding JSON: %s", err))
			}
			if response.Data != "pong" {
				return false, errors.New(fmt.Sprintf("invalid response: %s", response.Data))
			} else {
				return true, nil
			}
		}
	}
	return false, nil
}
