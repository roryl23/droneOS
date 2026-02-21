package main

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drivers/gpio"
	"droneOS/internal/drivers/motor/MG90S"
	"droneOS/internal/drivers/motor/hawks_work_ESC"
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
	"droneOS/internal/protocol"
	"droneOS/internal/utils"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

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

type wifiDebugWriter struct {
	ctx       context.Context
	status    *atomic.Bool
	transport *protocol.WiFiTransport
	queue     chan string
	droneID   int
}

type gpioPinLog struct {
	Name           string `json:"name,omitempty"`
	Scheme         string `json:"scheme,omitempty"`
	Number         int    `json:"number,omitempty"`
	ConfigChip     string `json:"configChip,omitempty"`
	ConfigOffset   int    `json:"configOffset,omitempty"`
	ResolvedChip   string `json:"resolvedChip,omitempty"`
	ResolvedOffset int    `json:"resolvedOffset,omitempty"`
	Direction      string `json:"direction,omitempty"`
	ActiveLow      *bool  `json:"activeLow,omitempty"`
	Bias           string `json:"bias,omitempty"`
	Drive          string `json:"drive,omitempty"`
	Used           bool   `json:"used,omitempty"`
	Consumer       string `json:"consumer,omitempty"`
	Error          string `json:"error,omitempty"`
}

func newWiFiDebugWriter(ctx context.Context, status *atomic.Bool, addr string, droneID int) *wifiDebugWriter {
	writer := &wifiDebugWriter{
		ctx:       ctx,
		status:    status,
		transport: &protocol.WiFiTransport{Addr: addr, Timeout: 500 * time.Millisecond},
		queue:     make(chan string, 200),
		droneID:   droneID,
	}
	go writer.loop()
	return writer
}

func (w *wifiDebugWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level != zerolog.DebugLevel {
		return len(p), nil
	}
	if !w.status.Load() {
		return len(p), nil
	}
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}
	select {
	case w.queue <- msg:
	default:
	}
	return len(p), nil
}

func (w *wifiDebugWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w *wifiDebugWriter) loop() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case msg := <-w.queue:
			if !w.status.Load() {
				continue
			}
			_, _ = w.transport.Send(w.ctx, protocol.Message{
				ID:   w.droneID,
				Cmd:  "debug_log",
				Data: msg,
			})
		}
	}
}

func logGPIOPins(kind, name string, pins []config.Pin, statuses []gpio.PinStatus) {
	if len(pins) == 0 {
		return
	}

	logPins := make([]gpioPinLog, 0, len(pins))
	for i, pin := range pins {
		logPin := gpioPinLog{
			Name:         pin.Name,
			Scheme:       pin.Scheme,
			Number:       pin.Number,
			ConfigChip:   pin.Chip,
			ConfigOffset: pin.Offset,
			Direction:    pin.Direction,
			ActiveLow:    pin.ActiveLow,
			Bias:         pin.Bias,
			Drive:        pin.Drive,
		}
		if i < len(statuses) {
			status := statuses[i]
			logPin.ResolvedChip = status.Resolved.Chip
			logPin.ResolvedOffset = status.Resolved.Offset
			logPin.Used = status.Used
			logPin.Consumer = status.Consumer
			if status.Err != nil {
				logPin.Error = status.Err.Error()
			}
		}
		logPins = append(logPins, logPin)
	}

	log.Debug().
		Str("device", name).
		Str("kind", kind).
		Interface("pins", logPins).
		Msg("gpio device config")
}

func preflightPins(layout gpio.Layout, kind, name string, pins []config.Pin) {
	if len(pins) == 0 {
		return
	}

	statuses, _ := gpio.ValidatePins(layout, pins)
	logGPIOPins(kind, name, pins, statuses)
	for _, status := range statuses {
		if status.Err != nil {
			log.Warn().Err(status.Err).
				Str("device", name).
				Str("kind", kind).
				Str("chip", status.Resolved.Chip).
				Int("offset", status.Resolved.Offset).
				Msg("gpio pin validation failed")
			continue
		}
		if status.Used {
			log.Warn().
				Str("device", name).
				Str("kind", kind).
				Str("chip", status.Resolved.Chip).
				Int("offset", status.Resolved.Offset).
				Str("consumer", status.Consumer).
				Msg("gpio pin already in use")
		}
	}
}

func preflightDevices(layout gpio.Layout, kind string, devices []config.Device) {
	for i := range devices {
		preflightPins(layout, kind, devices[i].Name, devices[i].Pins)
	}
}

func startWiFiPoller(ctx context.Context, settings *config.Config, status *atomic.Bool) {
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		defer ticker.Stop()
		wasConnected := status.Load()
		lastHeartbeat := time.Time{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			ok, err := protocol.CheckWiFi(ctx, settings)
			if err != nil {
				if wasConnected {
					status.Store(false)
					wasConnected = false
					log.Debug().Msg("wifi disconnected")
				} else {
					status.Store(false)
				}
				log.Debug().Err(err).Msg("wifi check failed")
				continue
			}
			if ok {
				status.Store(true)
				if !wasConnected {
					wasConnected = true
					log.Debug().Msg("wifi connected")
				}
				if time.Since(lastHeartbeat) >= 15*time.Second {
					lastHeartbeat = time.Now()
					log.Debug().Msg("wifi debug heartbeat")
				}
			} else {
				status.Store(false)
				if wasConnected {
					wasConnected = false
					log.Debug().Msg("wifi disconnected")
				}
			}
		}
	}()
}

func startControllerPoller(ctx context.Context, settings *config.Config, status *atomic.Bool) {
	addr := fmt.Sprintf("%s:%d", settings.Base.Host, settings.Base.Port)
	transport := &protocol.WiFiTransport{Addr: addr, Timeout: 3 * time.Second}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !status.Load() {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			resp, err := transport.Send(ctx, protocol.Message{
				ID:  settings.Drone.ID,
				Cmd: "next_command",
			})
			if err != nil {
				log.Debug().Err(err).Msg("controller poll failed")
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if resp.Data == "" {
				continue
			}
			var cmd protocol.ControllerCommand
			if err := json.Unmarshal([]byte(resp.Data), &cmd); err != nil {
				log.Warn().Err(err).Msg("invalid controller command payload")
				continue
			}

			log.Info().Str("action", cmd.Action).
				Interface("payload", cmd.Payload).
				Msg("controller action received")

			ack := protocol.ControllerAck{
				Action:  cmd.Action,
				Status:  "taken",
				Payload: cmd.Payload,
			}
			data, err := json.Marshal(ack)
			if err != nil {
				log.Warn().Err(err).Msg("failed to encode controller ack")
				continue
			}
			_, err = transport.Send(ctx, protocol.Message{
				ID:   settings.Drone.ID,
				Cmd:  "controller_ack",
				Data: string(data),
			})
			if err != nil {
				log.Debug().Err(err).Msg("controller ack failed")
			}
		}
	}()
}

func initRadioLink(ctx context.Context, name string, cfg *config.Radio) (protocol.RadioLink, error) {
	if strings.TrimSpace(name) == "" || strings.EqualFold(name, "none") {
		return nil, nil
	}
	outputs, err := utils.CallFunctionByName(ctx, RadioFuncMap, name, cfg)
	if err != nil {
		return nil, err
	}
	if len(outputs) < 2 {
		return nil, fmt.Errorf("radio driver %q returned %d values", name, len(outputs))
	}
	if errVal, ok := outputs[1].Interface().(error); ok && errVal != nil {
		return nil, errVal
	}
	link, ok := outputs[0].Interface().(protocol.RadioLink)
	if !ok || link == nil {
		return nil, fmt.Errorf("radio driver %q returned unexpected link type", name)
	}
	return link, nil
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

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,  // ctrl+C
		syscall.SIGTERM, // docker stop, systemd
	)
	defer stop() // restores default signal behavior

	wifiConnected := &atomic.Bool{}
	wifiAddr := fmt.Sprintf("%s:%d", settings.Base.Host, settings.Base.Port)
	debugWriter := newWiFiDebugWriter(ctx, wifiConnected, wifiAddr, settings.Drone.ID)
	log.Logger = log.Output(zerolog.MultiLevelWriter(os.Stdout, debugWriter))

	log.Info().Interface("settings", settings)

	layout, err := gpio.LayoutByName(settings.Drone.GPIOLayout)
	if err != nil {
		log.Warn().Err(err).Msg("invalid gpio layout; using default")
		layout = gpio.DefaultLayout()
	}

	preflightDevices(layout, "sensor", settings.Drone.Sensors)
	preflightDevices(layout, "output", settings.Drone.Outputs)
	preflightPins(layout, "radio", settings.Drone.Radio.Name, settings.Drone.Radio.Pins)

	chips := gpio.Init()
	log.Info().Interface("chips", chips)

	// disable automatic garbage collection,
	// we handle this in the perpetual loop below
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	startWiFiPoller(ctx, &settings, wifiConnected)
	startControllerPoller(ctx, &settings, wifiConnected)
	drone.StartDeviceReporter(ctx, &settings, wifiConnected)

	// initialize radio link (used for base comms when WiFi is unavailable)
	radioLink, err := initRadioLink(ctx, settings.Drone.Radio.Name, &settings.Drone.Radio)
	if err != nil {
		log.Error().Err(err).Msg("error initializing radio")
	}
	var radioTransport *protocol.RadioTransport
	if radioLink != nil {
		radioTransport = &protocol.RadioTransport{
			Link:          radioLink,
			Timeout:       2 * time.Second,
			RetryInterval: 50 * time.Millisecond,
		}
	}

	if radioTransport != nil {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				if wifiConnected.Load() && !settings.Drone.Radio.AlwaysUse {
					continue
				}
				_, err := radioTransport.Send(ctx, protocol.Message{
					ID:  settings.Drone.ID,
					Cmd: "ping",
				})
				if err != nil {
					log.Debug().Err(err).Msg("radio ping failed")
				}
			}
		}()
	}

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
		select {
		case <-ctx.Done():
			log.Info().Msg("shutdown requested")
			return
		case task := <-taskQueue:
			// handle output according to current task queue
			go func(t drone.Task) {
				_, err := utils.CallFunctionByName(
					ctx,
					MotorFuncMap,
					t.Name,
					&settings,
					&taskQueue,
				)
				if err != nil {
					log.Fatal().Err(err).Str("task", t.Name).
						Msg("error calling task")
				}
			}(task)
			runtime.GC()
		}
	}
}
