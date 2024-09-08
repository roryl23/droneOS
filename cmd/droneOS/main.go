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
	log.Info("started")

	configFile := flag.String("config-file", "configs/config.yaml", "config file location")
	flag.Parse()
	log.Debug(*configFile)
	config := config.GetConfig(*configFile)
	log.Info(config)

	level, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		log.Fatalf("invalid log level: %s", err)
	}
	log.SetLevel(level)

	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	gpio.Init()

	runtime.GC()
}
