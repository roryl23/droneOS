package drone

import (
	"bytes"
	"droneOS/internal/config"
	"droneOS/internal/gpio"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"time"
)

type Message struct {
	Text string `json:"text"`
	ID   int    `json:"id"`
}

func attemptBaseConnection(s *config.Config) {
	msg := Message{
		Text: "Hello, Server!",
		ID:   1,
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}

	resp, err := http.Post(
		fmt.Sprintf("http://%s:%d", s.Base.Host, s.Base.Port),
		"application/json",
		bytes.NewBuffer(msgBytes),
	)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()
	fmt.Println("Server responded with status:", resp.Status)
}

func Main(s *config.Config) {
	baseHost := net.ParseIP(s.Base.Host)
	if baseHost == nil {
		log.Errorf("Invalid IP address for base host: %s", s.Base.Host)
	}

	chips := gpio.Init()
	log.Info("Available chips: ", chips)

	for {
		attemptBaseConnection(s)
		pluginFunctions := config.LoadPlugins(s)
		for _, plugin := range pluginFunctions {
			plugin(s)
			time.Sleep(time.Second * 1) //TODO: get rid of this
		}
	}
}
