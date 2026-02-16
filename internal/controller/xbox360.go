package controller

import (
	"context"
	"errors"
	"time"

	"github.com/roryl23/xpad-go"
	"github.com/rs/zerolog"
)

const (
	xpadScanInterval  = 250 * time.Millisecond
	xpadEventInterval = 250 * time.Millisecond
	maxInt16          = int32(1<<15 - 1)
	minInt16          = int32(-1 << 15)
)

func Xbox360Interface(
	ctx context.Context,
	cCh *chan Event[any],
) error {
	logger := zerolog.Ctx(ctx)
	for {
		info, err := waitForXpad(ctx, logger)
		if err != nil {
			return err
		}

		dev, err := xpad.OpenDevice(info)
		if err != nil {
			logger.Warn().Err(err).
				Msg("failed to open xpad device, retrying")
			if !sleepOrDone(ctx, xpadScanInterval) {
				return ctx.Err()
			}
			continue
		}
		logger.Info().
			Str("name", info.Name).
			Str("path", info.Path).
			Msg("Xbox 360 connected")

		if err := readXpadEvents(ctx, dev, cCh); err != nil {
			_ = dev.Close()
			if errors.Is(err, context.Canceled) {
				return err
			}
			logger.Warn().Err(err).
				Msg("xpad device disconnected, retrying")
			continue
		}
		_ = dev.Close()
	}
}

func waitForXpad(ctx context.Context, logger *zerolog.Logger) (xpad.DeviceInfo, error) {
	for {
		infos, err := xpad.FindXpadDevices()
		if err != nil {
			logger.Warn().Err(err).
				Msg("failed to scan for xpad devices")
		} else if len(infos) > 0 {
			return infos[0], nil
		} else {
			logger.Debug().Msg("no xpad controller found, waiting...")
		}

		if !sleepOrDone(ctx, xpadScanInterval) {
			return xpad.DeviceInfo{}, ctx.Err()
		}
	}
}

func readXpadEvents(ctx context.Context, dev *xpad.Device, cCh *chan Event[any]) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		ev, err := dev.ReadEvent(xpadEventInterval)
		if err != nil {
			if errors.Is(err, xpad.ErrTimeout) {
				continue
			}
			return err
		}

		switch ev.Kind {
		case xpad.EVKey:
			if ev.Value == 0 {
				continue
			}
			switch ev.Code {
			case xpad.BTNA:
				sendEvent(cCh, TAKEOFF, true)
			case xpad.BTNB:
				sendEvent(cCh, LAND, true)
			}
		case xpad.EVAbs:
			value := clampInt32ToInt16(ev.Value)
			switch ev.Code {
			case xpad.ABSX:
				sendEvent(cCh, MOVE_X, value)
			case xpad.ABSY:
				sendEvent(cCh, MOVE_Y, value)
			case xpad.ABSRX:
				sendEvent(cCh, ROTATE, value)
			case xpad.ABSRY:
				sendEvent(cCh, ADJUST_ALTITUDE, value)
			}
		default:
			// ignore other event types (EV_SYN, etc.)
		}
	}
}

func sendEvent(cCh *chan Event[any], action string, payload any) {
	*cCh <- Event[any]{
		Action:  action,
		Payload: payload,
	}
}

func clampInt32ToInt16(value int32) int16 {
	if value > maxInt16 {
		return int16(maxInt16)
	}
	if value < minInt16 {
		return int16(minInt16)
	}
	return int16(value)
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
