package main

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drivers/gpio"
	"droneOS/internal/drivers/motor/MG90S"
	"droneOS/internal/drivers/motor/hawks_work_ESC"
	"droneOS/internal/drivers/radio"
	"droneOS/internal/drivers/radio/SX1262"
	"droneOS/internal/drivers/sensor"
	"droneOS/internal/drivers/sensor/GT_U7"
	"droneOS/internal/drivers/sensor/HC_SR04"
	"droneOS/internal/drivers/sensor/MPU_6050"
	"droneOS/internal/drivers/sensor/frienda_obstacle_431S"
	"droneOS/internal/drone"
	"droneOS/internal/drone/control"
	"droneOS/internal/drone/control/obstacle_avoidance"
	"droneOS/internal/drone/control/pilot"
	"droneOS/internal/utils"
	"flag"
	"math"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var RadioFuncMap = map[string]any{
	"SX1262": SX1262.Main,
}

var SensorFuncMap = map[string]any{
	"frienda_obstacle_431S": frienda_obstacle_431S.Main,
	"GT_U7":                 GT_U7.Main,
	"HC_SR04":               HC_SR04.Main,
	"MPU_6050":              MPU_6050.Main,
}

var ControlFuncMap = map[string]any{
	"obstacle_avoidance": obstacle_avoidance.Main,
	"pilot":              pilot.Main,
}

var MotorFuncMap = map[string]any{
	"hawks_work_ESC": hawks_work_ESC.Main,
	"MG90S":          MG90S.Main,
}

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

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,  // ctrl+C
		syscall.SIGTERM, // docker stop, systemd
	)
	defer stop() // restores default signal behavior

	chips := gpio.Init()
	log.Info().Interface("chips", chips)

	// disable automatic garbage collection,
	// we handle this in the perpetual loop below
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	// initialize and run radio
	radioEventChannel := make(chan radio.Event)
	go func() {
		_, err := utils.CallFunctionByName(
			ctx,
			RadioFuncMap,
			settings.Drone.Radio.Name,
			&settings.Drone.Radio,
			&radioEventChannel,
		)
		if err != nil {
			log.Error().Err(err).
				Msg("error initializing radio")
		}
	}()

	// initialize and run sensors
	sensorEventChannels := make(
		[]chan sensor.Event,
		len(settings.Drone.Sensors),
	)
	for i := range sensorEventChannels {
		sensorEventChannels[i] = make(chan sensor.Event)
	}
	for index, device := range settings.Drone.Sensors {
		go func() {
			_, err := utils.CallFunctionByName(
				ctx,
				SensorFuncMap,
				device.Name,
				&settings.Drone.Sensors[index],
				&sensorEventChannels,
			)
			if err != nil {
				log.Fatal().Err(err).
					Msg("error initializing sensors")
			}
		}()
	}

	// initialize and run control algorithms
	taskQueue := make(chan drone.Task)
	priorityMutex := control.NewPriorityMutex()
	for index, name := range settings.Drone.ControlAlgorithmPriority {
		go func() {
			_, err := utils.CallFunctionByName(
				ctx,
				ControlFuncMap,
				name,
				&settings,
				index+1,
				priorityMutex,
				&sensorEventChannels[index],
				&taskQueue,
			)
			if err != nil {
				log.Fatal().Err(err).
					Msg("error initializing control algorithms")
			}
		}()
	}

	// main loop that runs forever
	log.Info().Msg("starting main loop")
	for {
		// handle output according to current task queue
		task := <-taskQueue
		go func() {
			_, err := utils.CallFunctionByName(
				ctx,
				MotorFuncMap,
				task.Name,
				&settings,
				&taskQueue,
			)
			if err != nil {
				log.Fatal().Err(err).Str("task", task.Name).
					Msg("error calling task")
			}
		}()
		runtime.GC()
	}
}
