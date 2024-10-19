package protocol

import (
	"droneOS/internal/utils"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"net"
)

// TCPHandler handles TCP connections and messages
func TCPHandler(conn net.Conn) {
	defer conn.Close()

	var msg Message
	decoder := json.NewDecoder(conn)
	err := decoder.Decode(&msg)
	if err != nil {
		log.Errorf("error decoding message: %v", err)
		return
	}
	log.Debugf("%+v", msg)

	output, err := utils.CallFunctionByName(FuncMap, msg.Cmd, nil)
	if err != nil {
		log.Errorf("error executing command: %v", err)
		return
	}

	data, err := json.Marshal(output)
	if err != nil {
		log.Errorf("error marshaling response: %v", err)
		return
	}

	_, err = conn.Write(data)
	if err != nil {
		log.Errorf("error writing response: %v", err)
		return
	}
}
