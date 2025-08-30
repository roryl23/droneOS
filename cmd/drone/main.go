package main

import (
	"droneOS/internal/config"
	"droneOS/internal/drone/control"
	"droneOS/internal/drone/input/sensor"
	"droneOS/internal/drone/output"
	"droneOS/internal/gpio"
	"droneOS/internal/utils"
	"flag"
	"math"
	"runtime"
	"runtime/debug"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	configFile := flag.String(
		"config-file",
		"configs/config.yaml",
		"config file location",
	)
	flag.Parse()
	settings := config.GetConfig(*configFile)
	log.Info().Interface("settings", settings)

	chips := gpio.Init()
	log.Info().Interface("chips", chips)

	// disable automatic garbage collection,
	// we handle this in the perpetual loop below
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	// initialize and run sensors
	sensorEventChannels := make([]chan sensor.Event, len(settings.Drone.Sensors))
	for i := range sensorEventChannels {
		sensorEventChannels[i] = make(chan sensor.Event)
	}

	for index, device := range settings.Drone.Sensors {
		go func() {
			_, err := utils.CallFunctionByName(
				SensorFuncMap,
				device.Name,
				&settings.Drone.Sensors[index],
				&sensorEventChannels,
			)
			if err != nil {
				log.Error().Err(err).Msg("error calling sensor")
			}
		}()
	}

	// initialize and run control algorithms
	taskQueue := make(chan output.Task)
	priorityMutex := control.NewPriorityMutex()
	for index, name := range settings.Drone.ControlAlgorithmPriority {
		go func() {
			_, err := utils.CallFunctionByName(
				ControlFuncMap,
				name,
				&settings,
				index+1,
				priorityMutex,
				&sensorEventChannels[index],
				&taskQueue,
			)
			if err != nil {
				log.Error().Err(err).Msg("error calling control algorithm")
			}
		}()
	}

	// main loop that runs forever
	log.Info().Msg("starting main loop")
	for {
		// handle output according to current task queue
		task := <-taskQueue
		for _, device := range settings.Drone.Outputs {
			if device.Name == task.Name {
				go func() {
					_, err := utils.CallFunctionByName(
						OutputFuncMap,
						device.Name,
						&settings,
						&taskQueue,
					)
					if err != nil {
						log.Error().Err(err).Msg("error calling output")
					}
				}()
			}
		}
		runtime.GC()
	}
}
