package protocol

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog"
)

type DeviceStateReport struct {
	DroneID   int            `json:"droneId"`
	Timestamp int64          `json:"timestamp"`
	USB       USBState       `json:"usb"`
	GPIO      []GPIOPinState `json:"gpio"`
	Errors    []string       `json:"errors,omitempty"`
}

type USBState struct {
	Devices []USBDevice `json:"devices"`
}

type USBDevice struct {
	SysfsPath    string   `json:"sysfsPath,omitempty"`
	BusNum       string   `json:"busNum,omitempty"`
	DevNum       string   `json:"devNum,omitempty"`
	VendorID     string   `json:"vendorId,omitempty"`
	ProductID    string   `json:"productId,omitempty"`
	Product      string   `json:"product,omitempty"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	Serial       string   `json:"serial,omitempty"`
	Driver       string   `json:"driver,omitempty"`
	Interfaces   []string `json:"interfaces,omitempty"`
}

type GPIOPinState struct {
	Chip      string `json:"chip"`
	Offset    int    `json:"offset"`
	Name      string `json:"name,omitempty"`
	Consumer  string `json:"consumer,omitempty"`
	Used      bool   `json:"used"`
	Direction string `json:"direction,omitempty"`
	ActiveLow bool   `json:"activeLow,omitempty"`
	Drive     string `json:"drive,omitempty"`
	Bias      string `json:"bias,omitempty"`
	Value     *int   `json:"value,omitempty"`
}

func deviceState(ctx context.Context, msg Message) Message {
	logger := zerolog.Ctx(ctx)
	if msg.Data == "" {
		return Message{ID: msg.ID, Cmd: msg.Cmd, Data: "empty"}
	}

	var report DeviceStateReport
	if err := json.Unmarshal([]byte(msg.Data), &report); err != nil {
		logger.Warn().Err(err).
			Msg("invalid device state report")
		return Message{ID: msg.ID, Cmd: msg.Cmd, Data: "invalid"}
	}

	logger.Info().
		Int("drone_id", report.DroneID).
		Int("usb_devices", len(report.USB.Devices)).
		Int("gpio_pins", len(report.GPIO)).
		Msg("device state report")

	if len(report.Errors) > 0 {
		logger.Warn().
			Strs("errors", report.Errors).
			Msg("device state report errors")
	}

	logger.Debug().
		Interface("report", report).
		Msg("device state detail")

	return Message{ID: msg.ID, Cmd: msg.Cmd, Data: "ok"}
}
