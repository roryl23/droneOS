package protocol

import (
	"bytes"
	"context"
	"droneOS/internal/config"
	"droneOS/internal/utils"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/rs/zerolog/log"
)

func ping(m Message) Message {
	return Message{
		ID:   m.ID,
		Cmd:  m.Cmd,
		Data: "pong",
	}
}

// CheckWiFi Request base to determine whether the WiFi connection is operational
func CheckWiFi(
	ctx context.Context,
	s *config.Config,
	c http.Client,
) (bool, error) {
	msg := Message{
		ID:  s.Drone.ID,
		Cmd: "ping",
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return false, fmt.Errorf("error encoding JSON: %s", err)
	}

	resp, err := c.Post(
		fmt.Sprintf("http://%s:%d", s.Base.Host, s.Base.Port),
		"application/json",
		bytes.NewBuffer(msgBytes),
	)
	if err != nil {
		return false, fmt.Errorf("error sending request: %s", err)
	}
	defer resp.Body.Close()

	data := make([]byte, 1024)
	n, err := resp.Body.Read(data)
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("error reading request: %s", err)
	} else {
		if n > 0 {
			data = data[:n]
			var response Message
			err = json.Unmarshal(data, &response)
			if err != nil {
				return false, fmt.Errorf("error decoding JSON: %s", err)
			}
			if response.Data != "pong" {
				return false, fmt.Errorf("invalid response: %s", response.Data)
			} else {
				return true, nil
			}
		}
	}
	return false, nil
}

// FuncMap Map of function names to functions
var FuncMap = map[string]any{
	"ping": ping,
}

// TCPHandler handles TCP connections and messages
func TCPHandler(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	var msg Message
	decoder := json.NewDecoder(conn)
	err := decoder.Decode(&msg)
	if err != nil {
		log.Error().Err(err).
			Msg("error decoding message")
		return
	}
	log.Debug().Interface("msg", msg)

	output, err := utils.CallFunctionByName(ctx, FuncMap, msg.Cmd, nil)
	if err != nil {
		log.Error().Err(err).
			Msg("error executing command")
		return
	}

	data, err := json.Marshal(output)
	if err != nil {
		log.Error().Err(err).
			Msg("error marshaling response")
		return
	}

	_, err = conn.Write(data)
	if err != nil {
		log.Error().Err(err).
			Msg("error writing response")
		return
	}
}
