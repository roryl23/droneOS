package drone

// github.com/thinkski/go-v4l2
import (
	"droneOS/internal/config"
	"droneOS/internal/control"
	"droneOS/internal/input/sensor"
	"droneOS/internal/output"
	"droneOS/internal/utils"
	log "github.com/sirupsen/logrus"
	"math"
	"runtime"
	"runtime/debug"
	"time"
)

func Main(s *config.Config) {
	// disable automatic garbage collection,
	// we handle this in the perpetual loop below
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	// initialize and run sensors
	sensorEventChannels := make([]chan sensor.Event, len(s.Drone.Sensors))
	for i := range sensorEventChannels {
		sensorEventChannels[i] = make(chan sensor.Event)
	}

	for index, device := range s.Drone.Sensors {
		go func() {
			_, err := utils.CallFunctionByName(
				SensorFuncMap,
				device.Name,
				s.Drone.Sensors[index],
				&sensorEventChannels,
			)
			if err != nil {
				log.Error(err)
			}
		}()
	}

	// initialize and run control algorithms
	taskQueue := make(chan output.Task)
	priorityMutex := control.NewPriorityMutex()
	for index, name := range s.Drone.ControlAlgorithmPriority {
		go func() {
			_, err := utils.CallFunctionByName(
				ControlFuncMap,
				name,
				s,
				index+1,
				priorityMutex,
				&sensorEventChannels[index],
				&taskQueue,
			)
			if err != nil {
				log.Error(err)
			}
		}()
	}

	// main loop that runs forever
	for {
		// handle output according to current task queue
		task := <-taskQueue
		for _, device := range s.Drone.Outputs {
			if device.Name == task.Name {
				go func() {
					_, err := utils.CallFunctionByName(
						OutputFuncMap,
						device.Name,
						s,
						&taskQueue,
					)
					if err != nil {
						log.Error(err)
					}
				}()
			}
		}

		runtime.GC()
		time.Sleep(500 * time.Millisecond)
	}
}
