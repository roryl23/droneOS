package drone

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"droneOS/internal/config"
	"droneOS/internal/drivers/gpio"
)

type DetectStatus string

const (
	DetectDetected    DetectStatus = "detected"
	DetectNotDetected DetectStatus = "not_detected"
	DetectUnknown     DetectStatus = "unknown"
	DetectError       DetectStatus = "error"
)

type DetectResult struct {
	Status DetectStatus
	Method string
	Reason string
}

type DeviceDetectFunc func(ctx context.Context, device *config.Device, pins []gpio.ResolvedPin) DetectResult

type DetectRegistry struct {
	Sensors map[string]DeviceDetectFunc
	Outputs map[string]DeviceDetectFunc
}

func DefaultDetectRegistry() *DetectRegistry {
	return &DetectRegistry{
		Sensors: map[string]DeviceDetectFunc{
			"frienda_obstacle_431S": detectNoProbe("gpio"),
			"GT_U7":                 detectGTU7,
			"HC_SR04":               detectNoProbe("gpio"),
			"MPU_6050":              detectMPU6050,
		},
		Outputs: map[string]DeviceDetectFunc{
			"hawks_work_ESC": detectNoProbe("gpio"),
			"MG90S":          detectNoProbe("gpio"),
		},
	}
}

func (r *DetectRegistry) DetectSensor(
	ctx context.Context,
	device *config.Device,
	pins []gpio.ResolvedPin,
) DetectResult {
	if r == nil {
		return DetectResult{Status: DetectUnknown, Method: "config", Reason: "no detector registry"}
	}
	fn := r.Sensors[device.Name]
	if fn == nil {
		return DetectResult{Status: DetectUnknown, Method: "config", Reason: "no detector for device"}
	}
	return fn(ctx, device, pins)
}

func (r *DetectRegistry) DetectOutput(
	ctx context.Context,
	device *config.Device,
	pins []gpio.ResolvedPin,
) DetectResult {
	if r == nil {
		return DetectResult{Status: DetectUnknown, Method: "config", Reason: "no detector registry"}
	}
	fn := r.Outputs[device.Name]
	if fn == nil {
		return DetectResult{Status: DetectUnknown, Method: "config", Reason: "no detector for device"}
	}
	return fn(ctx, device, pins)
}

func detectNoProbe(method string) DeviceDetectFunc {
	return func(ctx context.Context, device *config.Device, pins []gpio.ResolvedPin) DetectResult {
		_ = ctx
		_ = pins
		m := method
		if m == "" {
			m = "config"
		}
		return DetectResult{
			Status: DetectUnknown,
			Method: m,
			Reason: "no device probe implemented",
		}
	}
}

func detectGTU7(ctx context.Context, device *config.Device, pins []gpio.ResolvedPin) DetectResult {
	_ = ctx
	_ = pins
	serialDevice := configString(device.Config, "serialDevice", "")
	if serialDevice == "" {
		return DetectResult{
			Status: DetectUnknown,
			Method: "uart",
			Reason: "serialDevice not configured",
		}
	}
	info, err := os.Stat(serialDevice)
	if err != nil {
		return DetectResult{
			Status: DetectNotDetected,
			Method: "uart",
			Reason: fmt.Sprintf("serial device missing: %s", err.Error()),
		}
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return DetectResult{
			Status: DetectError,
			Method: "uart",
			Reason: "serialDevice is not a character device",
		}
	}
	return DetectResult{
		Status: DetectUnknown,
		Method: "uart",
		Reason: "serial device present; no UART probe implemented",
	}
}

func detectMPU6050(ctx context.Context, device *config.Device, pins []gpio.ResolvedPin) DetectResult {
	_ = ctx
	_ = pins
	bus := configInt(device.Config, "i2cBus", 1)
	addr := configInt(device.Config, "i2cAddress", 0x68)
	who, err := readI2CRegister(bus, addr, 0x75)
	if err != nil {
		return DetectResult{
			Status: DetectError,
			Method: "i2c",
			Reason: fmt.Sprintf("i2c probe failed: %s", err.Error()),
		}
	}
	if who == 0x68 || who == 0x69 {
		return DetectResult{
			Status: DetectDetected,
			Method: "i2c",
			Reason: fmt.Sprintf("WHO_AM_I=0x%02X", who),
		}
	}
	return DetectResult{
		Status: DetectNotDetected,
		Method: "i2c",
		Reason: fmt.Sprintf("unexpected WHO_AM_I=0x%02X", who),
	}
}

func configInt(values map[string]any, key string, fallback int) int {
	if values == nil {
		return fallback
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 0, 64)
		if err != nil {
			return fallback
		}
		return int(parsed)
	default:
		return fallback
	}
}

func configString(values map[string]any, key, fallback string) string {
	if values == nil {
		return fallback
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback
		}
		return v
	default:
		return fallback
	}
}
