package radio

import (
	"droneOS/internal/config"
	"droneOS/internal/protocol"
	"net/http"
	"time"
)

func Main(s *config.Config) {
	client := &http.Client{
		Timeout: 10 * time.Millisecond,
	}
	for {
		if !s.Drone.Radio.AlwaysUse {
			status, err := protocol.CheckWiFi(s, *client)
			if err != nil || status == false {
				//log.Info("WiFi not connected, using radio...")
			} else {
				//log.Info("WiFi connected, using WiFi...")
			}
		} else {
			//log.Info("Always using radio...")
		}
	}
}
