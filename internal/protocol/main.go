package protocol

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/utils"
	"fmt"
	"net"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func ping(ctx context.Context, m Message) Message {
	return Message{
		ID:   m.ID,
		Cmd:  m.Cmd,
		Data: "pong",
	}
}

func debugLog(ctx context.Context, m Message) Message {
	logger := zerolog.Ctx(ctx)
	if m.Data != "" {
		logger.Debug().Str("remote", m.Data).Msg("drone debug")
	}
	return Message{
		ID:   m.ID,
		Cmd:  m.Cmd,
		Data: "ok",
	}
}

// CheckWiFi Request base to determine whether the WiFi connection is operational
func CheckWiFi(
	ctx context.Context,
	s *config.Config,
) (bool, error) {
	addr := fmt.Sprintf("%s:%d", s.Base.Host, s.Base.Port)
	transport := &WiFiTransport{
		Addr:    addr,
		Timeout: 500 * time.Millisecond,
	}
	msg := Message{
		ID:  s.Drone.ID,
		Cmd: "ping",
	}
	resp, err := transport.Send(ctx, msg)
	if err != nil {
		return false, fmt.Errorf("wifi ping failed: %w", err)
	}
	if resp.Data != "pong" {
		return false, fmt.Errorf("invalid response: %s", resp.Data)
	}
	return true, nil
}

// FuncMap Map of function names to functions
var FuncMap = map[string]any{
	"ping":           ping,
	"debug_log":      debugLog,
	"next_command":   nextControllerCommand,
	"controller_ack": controllerAck,
}

// TCPHandler handles TCP connections and messages
func TCPHandler(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	msg, err := DecodeMessage(conn)
	if err != nil {
		log.Error().Err(err).
			Msg("error decoding message")
		return
	}
	log.Debug().Interface("msg", msg)

	output, err := utils.CallFunctionByName(ctx, FuncMap, msg.Cmd, msg)
	if err != nil {
		log.Error().Err(err).
			Msg("error executing command")
		return
	}

	response, ok := output[0].Interface().(Message)
	if !ok {
		log.Error().Msg("unexpected response type")
		return
	}

	data, err := EncodeMessage(response)
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
