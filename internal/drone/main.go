package drone

// github.com/thinkski/go-v4l2
import (
	"container/heap"
	"droneOS/internal/config"
	"droneOS/internal/control"
	"droneOS/internal/input/sensor"
	"droneOS/internal/output"
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

	// initialize and run sensors
	sensorEventChannel := make(chan sensor.Event)
	sensorPlugins := sensor.LoadPlugins(s)
	for _, sensorPlugin := range sensorPlugins {
		go sensorPlugin.Main(s, &sensorEventChannel)
	}

	// create a priority queue and initialize
	pq := make(output.Queue, 0)
	heap.Init(&pq)

	outputPlugins := output.LoadPlugins(s)
	go output.Main(&pq, outputPlugins)

	controlAlgorithmPlugins := control.LoadPlugins(s)
	for priority, controlAlgorithm := range controlAlgorithmPlugins {
		go controlAlgorithm.Main(s, priority, &sensorEventChannel, &pq)
	}

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

		runtime.GC()
		time.Sleep(500 * time.Millisecond)
	}
}
