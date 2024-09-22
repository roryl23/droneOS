package drone

// github.com/thinkski/go-v4l2
import (
	"droneOS/internal/config"
	"droneOS/internal/protocol"
	log "github.com/sirupsen/logrus"
	"math"
	"net"
	"runtime"
	"runtime/debug"
	"time"
)

func Main(s *config.Config) {
	// disable automatic garbage collection
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	baseHost := net.ParseIP(s.Base.Host)
	if baseHost == nil {
		log.Fatalf("Invalid IP address for base host: %s", s.Base.Host)
	}

	for {
		status, err := protocol.CheckWiFi(s)
		if err != nil || status == false {
			// TODO: send messages over radio
		}

		pluginFunctions := config.LoadPlugins(s)
		for _, plugin := range pluginFunctions {
			plugin(s)
		}

		runtime.GC()
		time.Sleep(time.Second * 1) //TODO: get rid of this
	}
}
