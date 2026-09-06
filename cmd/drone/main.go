package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

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
	"droneOS/internal/envfile"
	"droneOS/internal/protocol"
	"droneOS/internal/realtime"
	"droneOS/internal/utils"

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

// Disabled for minimal testing
/*
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
*/

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
				// No commands available - sleep to avoid tight loop
				time.Sleep(50 * time.Millisecond)
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
	if err := envfile.LoadOptional(".drone.env"); err != nil {
		fmt.Fprintf(os.Stderr, "drone startup: failed to load .drone.env: %v\n", err)
		os.Exit(1)
	}

	configFileDefault := os.Getenv("DRONEOS_CONFIG_FILE")
	if configFileDefault == "" {
		configFileDefault = "configs/config.yaml"
	}
	configFile := flag.String(
		"config-file",
		configFileDefault,
		"config file location",
	)
	flag.Parse()
	settings := config.GetConfig(*configFile)

	// Configure logging based on config setting
	if settings.Drone.EnableLogging {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		setLogLevel(settings.Drone.LogLevel)
		log.Logger = log.Output(os.Stdout)
	} else {
		// Disable ALL logging - operate entirely in memory
		zerolog.SetGlobalLevel(zerolog.Disabled)
		log.Logger = zerolog.New(io.Discard).With().Timestamp().Logger()
	}
	startupError := func(message string, err error) {
		fmt.Fprintf(os.Stderr, "drone startup: %s: %v\n", message, err)
		log.Error().Err(err).Msg(message)
	}

	rtConfig, err := realtime.LoadConfig()
	if err != nil {
		startupError("invalid realtime configuration", err)
		os.Exit(1)
	}
	rt, err := realtime.New(rtConfig, func(err error) {
		log.Warn().Err(err).Msg("realtime setup issue; continuing")
	})
	if err != nil {
		startupError("invalid realtime configuration", err)
		os.Exit(1)
	}
	kernelStatus, kernelErr := realtime.DetectKernelRealtime()
	if kernelErr != nil {
		if rtConfig.Enabled && rtConfig.Strict {
			startupError("cannot verify PREEMPT_RT kernel", kernelErr)
			os.Exit(1)
		}
		log.Warn().Err(kernelErr).Msg("PREEMPT_RT kernel status is unavailable")
	} else {
		log.Info().
			Bool("preempt_rt", kernelStatus.Enabled).
			Str("source", kernelStatus.Source).
			Msg("detected PREEMPT_RT kernel status")
		if rtConfig.Enabled && rtConfig.Strict && !kernelStatus.Enabled {
			startupError(
				"realtime scheduling requires a PREEMPT_RT kernel",
				fmt.Errorf("%s reports realtime disabled", kernelStatus.Source),
			)
			os.Exit(1)
		}
	}
	if err := rt.Prepare(); err != nil {
		startupError("realtime process preparation failed", err)
		os.Exit(1)
	}
	if err := rt.Run(func() error { return nil }); err != nil {
		startupError("realtime scheduler preflight failed", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,  // ctrl+C
		syscall.SIGTERM, // docker stop, systemd
	)
	defer stop() // restores default signal behavior

	// Ensure clean shutdown with filesystem sync
	defer func() {
		log.Info().Msg("initiating graceful shutdown")
		syscall.Sync() // Force filesystem sync before exit
		time.Sleep(100 * time.Millisecond)
	}()

	wifiConnected := &atomic.Bool{}

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

	// Optional GC disable for specialized profiling/tuning.
	if strings.EqualFold(os.Getenv("DRONEOS_DISABLE_GC"), "1") ||
		strings.EqualFold(os.Getenv("DRONEOS_DISABLE_GC"), "true") {
		debug.SetGCPercent(-1)
		debug.SetMemoryLimit(math.MaxInt64)
		log.Warn().Msg("GC disabled (DRONEOS_DISABLE_GC=1)")
	}

	startWiFiPoller(ctx, &settings, wifiConnected)
	startControllerPoller(ctx, &settings, wifiConnected)
	drone.StartDeviceReporter(ctx, &settings, wifiConnected)

	// initialize radio link (used for base comms when WiFi is unavailable)
	radioLink, err := initRadioLink(ctx, settings.Drone.Radio.Name, &settings.Drone.Radio)
	if err != nil {
		log.Error().Err(err).Msg("error initializing radio")
		radioLink = nil
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
		index := index   // capture loop variable
		device := device // capture loop variable
		go func() {
			_, err := utils.CallFunctionByName(
				ctx,
				SensorFuncMap,
				device.Name,
				&settings.Drone.Sensors[index],
				&sensorEventChannels,
			)
			if err != nil {
				log.Error().Err(err).Msg("error initializing sensors")
				return
			}
		}()
	}

	// initialize and run control algorithms
	taskQueue := make(chan drone.Task)
	priorityMutex := control.NewPriorityMutex()
	for index, name := range settings.Drone.ControlAlgorithmPriority {
		index := index // capture loop variable
		name := name   // capture loop variable
		go func() {
			// Each control algorithm gets its corresponding sensor channel
			// Make sure we have enough sensor channels
			if index >= len(sensorEventChannels) {
				log.Error().
					Int("index", index).
					Int("available", len(sensorEventChannels)).
					Msg("not enough sensor channels for control algorithm")
				return
			}
			err := rt.Run(func() error {
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
				return err
			})
			if err != nil {
				log.Error().Err(err).Msg("error initializing control algorithms")
				return
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
					log.Error().Err(err).Str("task", t.Name).Msg("error calling task")
				}
			}(task)
		}
	}
}

func setLogLevel(level string) {
	level = strings.TrimSpace(level)
	if level == "" {
		level = "warn"
	}
	switch strings.ToLower(level) {
	case "panic":
		zerolog.SetGlobalLevel(zerolog.PanicLevel)
	case "fatal":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "warn", "warning":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}
}
