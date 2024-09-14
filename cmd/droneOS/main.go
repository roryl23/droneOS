package droneOS

// github.com/thinkski/go-v4l2
import (
	"droneOS/internal/config"
	"droneOS/internal/gpio"
	"flag"
	log "github.com/sirupsen/logrus"
	"math"
	"runtime"
	"runtime/debug"
)

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	log.Info("started droneOS")

	configFile := flag.String("config-file", "configs/config.yaml", "config file location")
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info(settings)

	level, err := log.ParseLevel(settings.LogLevel)
	if err != nil {
		log.Fatalf("invalid log level: %s", err)
	}
	log.SetLevel(level)

	log.Debug(*configFile)

	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	gpio.Init()

	runtime.GC()
}
