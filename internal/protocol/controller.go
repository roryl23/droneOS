package protocol

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog"
)

type ControllerCommand struct {
	Action  string `json:"action"`
	Payload any    `json:"payload"`
}

type ControllerAck struct {
	Action  string `json:"action"`
	Status  string `json:"status"`
	Payload any    `json:"payload,omitempty"`
	Error   string `json:"error,omitempty"`
}

var controllerQueue = make(chan ControllerCommand, 100)
var controllerLongPoll = 2 * time.Second

func EnqueueControllerCommand(cmd ControllerCommand) bool {
	select {
	case controllerQueue <- cmd:
		return true
	default:
		return false
	}
}

func nextControllerCommand(ctx context.Context, msg Message) Message {
	select {
	case cmd := <-controllerQueue:
		payload, err := json.Marshal(cmd)
		if err != nil {
			return Message{ID: msg.ID, Cmd: "controller_cmd", Data: ""}
		}
		return Message{ID: msg.ID, Cmd: "controller_cmd", Data: string(payload)}
	case <-ctx.Done():
		return Message{ID: msg.ID, Cmd: "controller_cmd", Data: ""}
	case <-time.After(controllerLongPoll):
		return Message{ID: msg.ID, Cmd: "controller_cmd", Data: ""}
	}
}

func controllerAck(ctx context.Context, msg Message) Message {
	logger := zerolog.Ctx(ctx)
	if msg.Data == "" {
		return Message{ID: msg.ID, Cmd: msg.Cmd, Data: "ok"}
	}

	var ack ControllerAck
	if err := json.Unmarshal([]byte(msg.Data), &ack); err != nil {
		logger.Warn().Err(err).Msg("invalid controller ack")
		return Message{ID: msg.ID, Cmd: msg.Cmd, Data: "invalid"}
	}

	event := logger.Info().
		Str("action", ack.Action).
		Str("status", ack.Status)
	if ack.Error != "" {
		event.Str("error", ack.Error)
	}
	if ack.Payload != nil {
		event.Interface("payload", ack.Payload)
	}
	event.Msg("controller ack")

	return Message{ID: msg.ID, Cmd: msg.Cmd, Data: "ok"}
}
