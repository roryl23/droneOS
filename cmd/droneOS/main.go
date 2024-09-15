package main

// github.com/thinkski/go-v4l2
import (
	"droneOS/internal/base"
	"droneOS/internal/config"
	"droneOS/internal/drone"
	"droneOS/internal/gpio"
	"errors"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"math"
	"reflect"
	"runtime/debug"
)

// Helper function to call a function by its name from the map
func callFunctionByName(s *config.Config, funcMap map[string]interface{}, name string) error {
	fn, ok := funcMap[name]
	if !ok {
		return errors.New("mode not found")
	}
	input := []reflect.Value{reflect.ValueOf(s)}
	reflect.ValueOf(fn).Call(input)
	return nil
}

// Map of function names to functions
var modeMap = map[string]interface{}{
	"base":  base.Main,
	"drone": drone.Main,
}

func main() {
	// disable automatic garbage collection
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.DebugLevel)
	log.Info("started droneOS")

	mode := flag.String("mode", "base", "[base, drone]")
	configFile := flag.String("config-file", "configs/config.yaml", "config file location")
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info(settings)

	gpio.Init()
	for {
		err := callFunctionByName(&settings, modeMap, *mode)
		if err != nil {
			fmt.Println(err)
		}
	}
}
