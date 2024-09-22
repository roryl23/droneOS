package protocol

import (
	"bytes"
	"droneOS/internal/config"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
)

func PingBaseWiFi(s *config.Config) error {
	msg := Message{
		ID:  s.Drone.ID,
		Cmd: "ping",
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return errors.New(fmt.Sprintf("error encoding JSON: %s", err))
	}

	resp, err := http.Post(
		fmt.Sprintf("http://%s:%d", s.Base.Host, s.Base.Port),
		"application/json",
		bytes.NewBuffer(msgBytes),
	)
	if err != nil {
		return errors.New(fmt.Sprintf("error sending request: %s", err))
	}
	defer resp.Body.Close()

	data := make([]byte, 1024)
	n, err := resp.Body.Read(data)
	if err != nil && err != io.EOF {
		log.Error("error reading data: ", err)
	} else {
		if n > 0 {
			data = data[:n]
			dataString := string(data)
			log.Info(dataString)
			var response Message
			err = json.Unmarshal(data, &response)
			if err != nil {
				return errors.New(fmt.Sprintf("error decoding JSON: %s", err))
			}
			if response.Data != "pong" {
				return errors.New(fmt.Sprintf("invalid response: %s", response.Data))
			}
		}
	}
	return nil
}
