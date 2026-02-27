package gpio

import (
	"fmt"
	"strings"

	"droneOS/internal/config"

	"github.com/warthog618/go-gpiocdev"
)

type PinStatus struct {
	Resolved ResolvedPin
	Used     bool
	Consumer string
	Err      error
}

func InspectPins(layout Layout, pins []config.Pin) ([]PinStatus, []error) {
	if layout.Name == "" {
		layout = DefaultLayout()
	}

	statuses := make([]PinStatus, 0, len(pins))
	var errs []error

	for _, pin := range pins {
		resolved, err := ResolvePin(layout, pin)
		if err != nil {
			errs = append(errs, err)
			statuses = append(statuses, PinStatus{
				Resolved: ResolvedPin{Name: pin.Name},
				Err:      err,
			})
			continue
		}

		status := PinStatus{Resolved: resolved}
		chip, err := gpiocdev.NewChip(resolved.Chip)
		if err != nil {
			status.Err = fmt.Errorf("open gpio chip %s: %w", resolved.Chip, err)
			errs = append(errs, status.Err)
			statuses = append(statuses, status)
			continue
		}

		info, err := chip.LineInfo(resolved.Offset)
		if err != nil {
			status.Err = fmt.Errorf("gpio line info %s:%d: %w", resolved.Chip, resolved.Offset, err)
			errs = append(errs, status.Err)
			_ = chip.Close()
			statuses = append(statuses, status)
			continue
		}

		status.Used = info.Used
		status.Consumer = strings.TrimSpace(info.Consumer)
		_ = chip.Close()
		statuses = append(statuses, status)
	}

	return statuses, errs
}

func ValidatePins(layout Layout, pins []config.Pin) ([]PinStatus, []error) {
	statuses, errs := InspectPins(layout, pins)
	for i := range statuses {
		if statuses[i].Err != nil || statuses[i].Used {
			continue
		}
		opts := lineReqOptions(statuses[i].Resolved)
		if len(opts) == 0 {
			continue
		}
		line, err := gpiocdev.RequestLine(
			statuses[i].Resolved.Chip,
			statuses[i].Resolved.Offset,
			opts...,
		)
		if err != nil {
			statuses[i].Err = fmt.Errorf(
				"request gpio line %s:%d: %w",
				statuses[i].Resolved.Chip,
				statuses[i].Resolved.Offset,
				err,
			)
			errs = append(errs, statuses[i].Err)
			continue
		}
		_ = line.Close()
	}

	return statuses, errs
}

func lineReqOptions(pin ResolvedPin) []gpiocdev.LineReqOption {
	var opts []gpiocdev.LineReqOption

	if pin.ActiveLow != nil {
		if *pin.ActiveLow {
			opts = append(opts, gpiocdev.AsActiveLow)
		} else {
			opts = append(opts, gpiocdev.AsActiveHigh)
		}
	}

	bias := strings.ToLower(strings.TrimSpace(pin.Bias))
	switch bias {
	case "pull_up":
		opts = append(opts, gpiocdev.WithPullUp)
	case "pull_down":
		opts = append(opts, gpiocdev.WithPullDown)
	case "disabled":
		opts = append(opts, gpiocdev.WithBiasDisabled)
	case "as_is":
		opts = append(opts, gpiocdev.WithBiasAsIs)
	}

	drive := strings.ToLower(strings.TrimSpace(pin.Drive))
	hasDrive := false
	switch drive {
	case "open_drain":
		opts = append(opts, gpiocdev.AsOpenDrain)
		hasDrive = true
	case "open_source":
		opts = append(opts, gpiocdev.AsOpenSource)
		hasDrive = true
	case "push_pull":
		opts = append(opts, gpiocdev.AsPushPull)
		hasDrive = true
	}

	direction := strings.ToLower(strings.TrimSpace(pin.Direction))
	switch direction {
	case "input":
		opts = append(opts, gpiocdev.AsInput)
	case "output":
		if !hasDrive {
			opts = append(opts, gpiocdev.AsOutput(0))
		}
	}

	return opts
}
