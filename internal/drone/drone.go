package drone

// github.com/thinkski/go-v4l2
import (
	"droneOS/internal/config"
	"droneOS/internal/protocol"
	log "github.com/sirupsen/logrus"
	"math"
	"net/http"
	"runtime"
	"runtime/debug"
	"time"
)

func Main(s *config.Config) {
	// disable automatic garbage collection,
	// we handle this in the perpetual loop below
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	client := &http.Client{
		Timeout: 10 * time.Millisecond,
	}
	for {
		if !s.Drone.AlwaysUseRadio {
			status, err := protocol.CheckWiFi(s, *client)
			if err != nil || status == false {
				log.Info("WiFi not connected, using radio...")
			} else {
				log.Info("WiFi connected, using WiFi...")
			}
		} else {
			log.Info("Always using radio...")
		}

		pluginFunctions := config.LoadPlugins(s)
		for _, plugin := range pluginFunctions {
			plugin(s)
		}

		runtime.GC()
		time.Sleep(500 * time.Millisecond)
	}
}
