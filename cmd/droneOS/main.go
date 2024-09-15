package main

import (
	"droneOS/internal/base"
	"droneOS/internal/config"
	"droneOS/internal/drone"
	"droneOS/internal/gpio"
	"errors"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// funcMap Map of function names to functions
var funcMap = map[string]interface{}{
	"base":  base.Main,
	"drone": drone.Main,
}

// callFunctionByName Helper function to call a function by its name from the map
func callFunctionByName(name string, input interface{}) ([]reflect.Value, error) {
	fn, ok := funcMap[name]
	if !ok {
		return nil, errors.New(fmt.Sprintf("function not found: %s", name))
	}
	inputValue := []reflect.Value{reflect.ValueOf(input)}
	output := reflect.ValueOf(fn).Call(inputValue)
	return output, nil
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

	gpio.Init()
	log.Fatal(callFunctionByName(*mode, &settings))
}
