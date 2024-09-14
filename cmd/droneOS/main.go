package main

// github.com/thinkski/go-v4l2
import (
	"droneOS/internal/config"
	"droneOS/internal/gpio"
	"errors"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"math"
	"reflect"
	"runtime"
	"runtime/debug"
)

func base() {
	log.Info("base mode")
}

func drone() {
	log.Info("drone mode")
}

// Helper function to call a function by its name from the map
func callFunctionByName(funcMap map[string]interface{}, name string) error {
	fn, ok := funcMap[name]
	if !ok {
		return errors.New("function not found")
	}

	// Use reflection to call the function
	reflect.ValueOf(fn).Call(nil)
	return nil
}

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.DebugLevel)

	log.Info("started droneOS")

	mode := flag.String("mode", "base", "[base, drone]")
	configFile := flag.String("config-file", "configs/config.yaml", "config file location")
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info(settings)

	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	// Map of function names to functions
	modeMap := map[string]interface{}{
		"base":  base,
		"drone": drone,
	}

	gpio.Init()

	for {
		err := callFunctionByName(modeMap, *mode)
		if err != nil {
			fmt.Println(err)
		}
		runtime.GC()
	}
}
