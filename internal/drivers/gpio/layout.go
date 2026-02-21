package gpio

import (
	"fmt"
	"strings"

	"droneOS/internal/config"
)

const (
	SchemeBCM      = "bcm"
	SchemePhysical = "physical"
	SchemeChip     = "chip"

	defaultGPIOChip = "gpiochip0"
)

type Layout struct {
	Name          string
	PhysicalToBCM map[int]int
}

type ResolvedPin struct {
	Name      string
	Chip      string
	Offset    int
	Direction string
	ActiveLow *bool
	Bias      string
	Drive     string
}

func DefaultLayout() Layout {
	return Layout{
		Name:          "rpi-40",
		PhysicalToBCM: rpi40PhysicalToBCM,
	}
}

func LayoutByName(name string) (Layout, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "rpi-40", "rpi40", "raspi-40", "raspberrypi-40":
		return DefaultLayout(), nil
	default:
		return Layout{}, fmt.Errorf("unknown gpio layout %q", name)
	}
}

func ResolvePin(layout Layout, pin config.Pin) (ResolvedPin, error) {
	scheme := strings.ToLower(strings.TrimSpace(pin.Scheme))
	if scheme == "" {
		if strings.TrimSpace(pin.Chip) != "" || pin.Offset != 0 {
			scheme = SchemeChip
		} else if pin.Number != 0 {
			scheme = SchemeBCM
		} else {
			return ResolvedPin{}, fmt.Errorf("pin %q missing scheme/number/chip", pin.Name)
		}
	}

	resolved := ResolvedPin{
		Name:      pin.Name,
		Direction: pin.Direction,
		ActiveLow: pin.ActiveLow,
		Bias:      pin.Bias,
		Drive:     pin.Drive,
	}

	switch scheme {
	case SchemeChip:
		if strings.TrimSpace(pin.Chip) == "" {
			return ResolvedPin{}, fmt.Errorf("pin %q scheme %q requires chip", pin.Name, scheme)
		}
		resolved.Chip = pin.Chip
		resolved.Offset = pin.Offset
	case SchemeBCM:
		resolved.Chip = pin.Chip
		if resolved.Chip == "" {
			resolved.Chip = defaultGPIOChip
		}
		resolved.Offset = pin.Number
	case SchemePhysical:
		bcm, ok := layout.PhysicalToBCM[pin.Number]
		if !ok {
			return ResolvedPin{}, fmt.Errorf("pin %q physical %d not mapped for layout %q", pin.Name, pin.Number, layout.Name)
		}
		resolved.Chip = pin.Chip
		if resolved.Chip == "" {
			resolved.Chip = defaultGPIOChip
		}
		resolved.Offset = bcm
	default:
		return ResolvedPin{}, fmt.Errorf("pin %q has unsupported scheme %q", pin.Name, scheme)
	}

	return resolved, nil
}

func ResolvePins(layout Layout, pins []config.Pin) ([]ResolvedPin, []error) {
	if layout.Name == "" {
		layout = DefaultLayout()
	}

	resolved := make([]ResolvedPin, 0, len(pins))
	var errs []error
	for _, pin := range pins {
		rp, err := ResolvePin(layout, pin)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		resolved = append(resolved, rp)
	}

	return resolved, errs
}

var rpi40PhysicalToBCM = map[int]int{
	3:  2,
	5:  3,
	7:  4,
	8:  14,
	10: 15,
	11: 17,
	12: 18,
	13: 27,
	15: 22,
	16: 23,
	18: 24,
	19: 10,
	21: 9,
	22: 25,
	23: 11,
	24: 8,
	26: 7,
	27: 0,
	28: 1,
	29: 5,
	31: 6,
	32: 12,
	33: 13,
	35: 19,
	36: 16,
	37: 26,
	38: 20,
	40: 21,
}
