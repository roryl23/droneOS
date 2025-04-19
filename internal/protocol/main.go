package protocol

import (
	"droneOS/internal/utils"
	"encoding/json"
	"github.com/rs/zerolog/log"
	"net"
)

// TCPHandler handles TCP connections and messages
func TCPHandler(conn net.Conn) {
	defer conn.Close()

	var msg Message
	decoder := json.NewDecoder(conn)
	err := decoder.Decode(&msg)
	if err != nil {
		log.Error().Err(err).Msg("error decoding message")
		return
	}
	log.Debug().Interface("msg", msg)

	output, err := utils.CallFunctionByName(FuncMap, msg.Cmd, nil)
	if err != nil {
		log.Error().Err(err).Msg("error executing command")
		return
	}

	data, err := json.Marshal(output)
	if err != nil {
		log.Error().Err(err).Msg("error marshaling response")
		return
	}

	_, err = conn.Write(data)
	if err != nil {
		log.Error().Err(err).Msg("error writing response")
		return
	}
}
