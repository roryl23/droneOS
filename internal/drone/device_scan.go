package drone

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"droneOS/internal/config"
	"droneOS/internal/drivers/gpio"
	"droneOS/internal/protocol"

	"github.com/warthog618/go-gpiocdev"
)

const usbSysfsPath = "/sys/bus/usb/devices"

var defaultDetectRegistry = DefaultDetectRegistry()

func CollectDeviceState(
	ctx context.Context,
	settings *config.Config,
) (protocol.DeviceStateReport, []error) {
	report := protocol.DeviceStateReport{
		Timestamp: time.Now().Unix(),
	}
	if settings != nil {
		report.DroneID = settings.Drone.ID
	}

	var errs []error

	usbDevices, usbErrs := scanUSBDevices()
	if len(usbErrs) > 0 {
		errs = append(errs, usbErrs...)
	}
	report.USB = protocol.USBState{Devices: usbDevices}

	gpioPins, gpioErrs := scanGPIOPins()
	if len(gpioErrs) > 0 {
		errs = append(errs, gpioErrs...)
	}
	report.GPIO = gpioPins

	devices, deviceErrs := scanConfiguredDevices(ctx, settings, defaultDetectRegistry)
	if len(deviceErrs) > 0 {
		errs = append(errs, deviceErrs...)
	}
	report.Devices = devices

	if len(errs) > 0 {
		report.Errors = errorsToStrings(errs)
	}

	return report, errs
}

func scanConfiguredDevices(
	ctx context.Context,
	settings *config.Config,
	registry *DetectRegistry,
) ([]protocol.DeviceProbe, []error) {
	if settings == nil {
		return nil, nil
	}
	layout, err := gpio.LayoutByName(settings.Drone.GPIOLayout)
	var errs []error
	if err != nil {
		errs = append(errs, err)
		layout = gpio.DefaultLayout()
	}

	if registry == nil {
		registry = defaultDetectRegistry
	}

	var probes []protocol.DeviceProbe

	for i := range settings.Drone.Sensors {
		device := &settings.Drone.Sensors[i]
		probe, probeErrs := probeDevice(ctx, "sensor", device, layout, registry.DetectSensor)
		if len(probeErrs) > 0 {
			errs = append(errs, probeErrs...)
		}
		probes = append(probes, probe)
	}

	for i := range settings.Drone.Outputs {
		device := &settings.Drone.Outputs[i]
		probe, probeErrs := probeDevice(ctx, "output", device, layout, registry.DetectOutput)
		if len(probeErrs) > 0 {
			errs = append(errs, probeErrs...)
		}
		probes = append(probes, probe)
	}

	return probes, errs
}

type detectFunc func(context.Context, *config.Device, []gpio.ResolvedPin) DetectResult

func probeDevice(
	ctx context.Context,
	kind string,
	device *config.Device,
	layout gpio.Layout,
	detect detectFunc,
) (protocol.DeviceProbe, []error) {
	probe := protocol.DeviceProbe{
		Name: device.Name,
		Kind: kind,
		Detect: protocol.DeviceDetect{
			Status: string(DetectUnknown),
			Method: "config",
			Reason: "no device probe configured",
		},
	}

	pinStatuses, pinErrs := gpio.InspectPins(layout, device.Pins)
	if len(pinStatuses) > 0 {
		probe.Pins = make([]protocol.DeviceProbePin, 0, len(pinStatuses))
	}

	resolvedPins := make([]gpio.ResolvedPin, 0, len(pinStatuses))
	for _, status := range pinStatuses {
		pin := protocol.DeviceProbePin{
			Name:      status.Resolved.Name,
			Chip:      status.Resolved.Chip,
			Offset:    status.Resolved.Offset,
			Used:      status.Used,
			Consumer:  status.Consumer,
			Direction: status.Resolved.Direction,
			ActiveLow: status.Resolved.ActiveLow,
			Drive:     status.Resolved.Drive,
			Bias:      status.Resolved.Bias,
		}
		if status.Err != nil {
			pin.Error = status.Err.Error()
		} else {
			resolvedPins = append(resolvedPins, status.Resolved)
		}
		probe.Pins = append(probe.Pins, pin)
	}

	if detect != nil {
		result := detect(ctx, device, resolvedPins)
		if result.Status == "" {
			result.Status = DetectUnknown
		}
		if result.Method == "" {
			result.Method = "config"
		}
		probe.Detect = protocol.DeviceDetect{
			Status: string(result.Status),
			Method: result.Method,
			Reason: result.Reason,
		}
	}

	if len(pinErrs) > 0 {
		probe.Detect = protocol.DeviceDetect{
			Status: string(DetectError),
			Method: "gpio",
			Reason: pinErrs[0].Error(),
		}
	} else if probe.Detect.Status == string(DetectUnknown) && len(device.Pins) > 0 {
		probe.Detect.Method = "gpio"
		if probe.Detect.Reason == "" || probe.Detect.Reason == "no device probe configured" {
			probe.Detect.Reason = "pins inspected only"
		}
	}

	return probe, pinErrs
}

func scanUSBDevices() ([]protocol.USBDevice, []error) {
	entries, err := os.ReadDir(usbSysfsPath)
	if err != nil {
		return nil, []error{fmt.Errorf("read usb sysfs: %w", err)}
	}

	interfaces := map[string][]string{}
	for _, entry := range entries {
		name := entry.Name()
		if strings.Contains(name, ":") {
			base := strings.SplitN(name, ":", 2)[0]
			interfaces[base] = append(interfaces[base], name)
		}
	}

	devices := make([]protocol.USBDevice, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.Contains(name, ":") {
			continue
		}
		path := filepath.Join(usbSysfsPath, name)

		vendor := readSysfsValue(path, "idVendor")
		product := readSysfsValue(path, "idProduct")
		if vendor == "" && product == "" {
			continue
		}

		device := protocol.USBDevice{
			SysfsPath:    path,
			VendorID:     vendor,
			ProductID:    product,
			Product:      readSysfsValue(path, "product"),
			Manufacturer: readSysfsValue(path, "manufacturer"),
			Serial:       readSysfsValue(path, "serial"),
			BusNum:       readSysfsValue(path, "busnum"),
			DevNum:       readSysfsValue(path, "devnum"),
			Driver:       readSysfsSymlink(path, "driver"),
			Interfaces:   interfaces[name],
		}
		sort.Strings(device.Interfaces)
		devices = append(devices, device)
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].SysfsPath < devices[j].SysfsPath
	})

	return devices, nil
}

func scanGPIOPins() ([]protocol.GPIOPinState, []error) {
	chips := gpiocdev.Chips()
	if len(chips) == 0 {
		return nil, nil
	}

	var states []protocol.GPIOPinState
	var errs []error

	for _, chipName := range chips {
		chip, err := gpiocdev.NewChip(chipName)
		if err != nil {
			errs = append(errs, fmt.Errorf("open gpio chip %s: %w", chipName, err))
			continue
		}

		lineCount := chip.Lines()
		for offset := 0; offset < lineCount; offset++ {
			info, err := chip.LineInfo(offset)
			if err != nil {
				errs = append(errs, fmt.Errorf("gpio line info %s:%d: %w", chipName, offset, err))
				continue
			}

			state := protocol.GPIOPinState{
				Chip:      chipName,
				Offset:    offset,
				Name:      strings.TrimSpace(info.Name),
				Consumer:  strings.TrimSpace(info.Consumer),
				Used:      info.Used,
				Direction: gpioDirection(info.Config.Direction),
				ActiveLow: info.Config.ActiveLow,
				Drive:     gpioDrive(info.Config.Drive),
				Bias:      gpioBias(info.Config.Bias),
			}

			if !info.Used {
				if value, err := readGPIOValue(chipName, offset); err == nil {
					state.Value = &value
				}
			}

			states = append(states, state)
		}
		_ = chip.Close()
	}

	sort.Slice(states, func(i, j int) bool {
		if states[i].Chip == states[j].Chip {
			return states[i].Offset < states[j].Offset
		}
		return states[i].Chip < states[j].Chip
	})

	return states, errs
}

func readGPIOValue(chip string, offset int) (int, error) {
	line, err := gpiocdev.RequestLine(chip, offset, gpiocdev.AsIs)
	if err != nil {
		return 0, err
	}
	defer line.Close()
	return line.Value()
}

func readSysfsValue(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readSysfsSymlink(dir, name string) string {
	target, err := os.Readlink(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

func errorsToStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		out = append(out, err.Error())
	}
	return out
}

func gpioDirection(dir gpiocdev.LineDirection) string {
	switch dir {
	case gpiocdev.LineDirectionInput:
		return "input"
	case gpiocdev.LineDirectionOutput:
		return "output"
	default:
		return "unknown"
	}
}

func gpioDrive(drive gpiocdev.LineDrive) string {
	switch drive {
	case gpiocdev.LineDriveOpenDrain:
		return "open_drain"
	case gpiocdev.LineDriveOpenSource:
		return "open_source"
	default:
		return "push_pull"
	}
}

func gpioBias(bias gpiocdev.LineBias) string {
	switch bias {
	case gpiocdev.LineBiasPullUp:
		return "pull_up"
	case gpiocdev.LineBiasPullDown:
		return "pull_down"
	case gpiocdev.LineBiasDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}
