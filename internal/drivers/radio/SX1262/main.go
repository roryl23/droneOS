//go:build linux
// +build linux

package SX1262

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"droneOS/internal/config"
	"droneOS/internal/protocol"

	"github.com/rs/zerolog"
	"github.com/tarm/serial"
	"github.com/warthog618/go-gpiocdev"
)

const (
	// GPIO pins for Waveshare LoRa HAT on Raspberry Pi
	M0_PIN = 22 // GPIO 22 (BCM22, Physical Pin 15) - Mode 0
	M1_PIN = 27 // GPIO 27 (BCM27, Physical Pin 13) - Mode 1

	// Serial configuration
	SERIAL_DEVICE = "/dev/ttyS0" // Pi's hardware UART (GPIO 14 TX, GPIO 15 RX)
	BAUD_RATE     = 9600

	maxFramePayloadSize = 64 * 1024
	maxFrameSize        = 4 + maxFramePayloadSize
)

type LoRaHAT struct {
	serial       io.ReadWriteCloser
	m0           *gpiocdev.Line
	m1           *gpiocdev.Line
	log          zerolog.Logger
	receiveBuf   [maxFrameSize]byte
	receiveStart int
	receiveEnd   int
}

func NewLoRaHAT(ctx context.Context, serialDevice string, useGPIO bool) (*LoRaHAT, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().Msg("Initializing LoRa HAT on Raspberry Pi")

	// Configure mode pins
	var m0 *gpiocdev.Line
	var m1 *gpiocdev.Line
	var err error
	if useGPIO {
		m0, err = gpiocdev.RequestLine("gpiochip0", M0_PIN, gpiocdev.AsOutput(0))
		if err != nil {
			logger.Error().Err(err).Msg("Failed to open GPIO line M0")
			return nil, err
		}
		m1, err = gpiocdev.RequestLine("gpiochip0", M1_PIN, gpiocdev.AsOutput(0))
		if err != nil {
			_ = m0.Close()
			logger.Error().Err(err).Msg("Failed to open GPIO line M1")
			return nil, err
		}
	}

	// Open serial port
	// Use longer timeout for USB devices to reduce kernel pressure
	readTimeout := 200 * time.Millisecond
	if !useGPIO {
		// USB serial devices need much longer timeouts to reduce polling
		readTimeout = 1000 * time.Millisecond
	}
	cfg := &serial.Config{
		Name:        serialDevice,
		Baud:        BAUD_RATE,
		ReadTimeout: readTimeout,
	}
	ser, err := serial.OpenPort(cfg)
	if err != nil {
		if m0 != nil {
			_ = m0.Close()
		}
		if m1 != nil {
			_ = m1.Close()
		}
		mode := radioMode(useGPIO)
		openErr := fmt.Errorf("open LoRa serial port %q in %s mode: %w", serialDevice, mode, err)
		logger.Error().Err(openErr).Str("device", serialDevice).Str("mode", mode).Msg("Failed to open serial port")
		return nil, openErr
	}

	hat := &LoRaHAT{
		serial: ser,
		m0:     m0,
		m1:     m1,
		log:    *logger,
	}

	// In USB mode, the physical jumpers control M0/M1.
	if useGPIO {
		hat.setTransparentMode()
		logger.Info().Msg("LoRa GPIO mode ready: M0=LOW, M1=LOW (transparent transmission mode)")
	} else {
		logger.Info().Msg("LoRa in USB mode - ensure jumpers are set: UART=A, M0=GND, M1=GND for transmission")
	}

	logger.Info().Msg("LoRa HAT initialized successfully")
	return hat, nil
}

func (h *LoRaHAT) setTransparentMode() {
	h.setLine(h.m0, 0)
	h.setLine(h.m1, 0)
	time.Sleep(100 * time.Millisecond)
	h.log.Debug().Msg("LoRa transparent transmission mode set")
}

func (h *LoRaHAT) setLine(line *gpiocdev.Line, value int) {
	if line == nil {
		return
	}
	if err := line.SetValue(value); err != nil {
		h.log.Error().Err(err).Msg("Failed to set GPIO line")
	}
}

func (h *LoRaHAT) Send(data []byte) error {
	// Only log at debug level to avoid flooding logs with pings/pongs
	h.log.Debug().
		Int("length", len(data)).
		Msg("Sending LoRa packet")

	_, err := h.serial.Write(data)
	if err != nil {
		h.log.Error().Err(err).Msg("Serial write failed")
		return err
	}

	return nil
}

func (h *LoRaHAT) Receive() ([]byte, error) {
	if frame := h.nextFrame(); frame != nil {
		return frame, nil
	}

	h.compactReceiveBuffer()
	n, err := h.serial.Read(h.receiveBuf[h.receiveEnd:])
	if n > 0 {
		h.receiveEnd += n
	}
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}

	return h.nextFrame(), nil
}

func (h *LoRaHAT) nextFrame() []byte {
	for h.receiveEnd-h.receiveStart >= 4 {
		payloadLength := binary.BigEndian.Uint32(h.receiveBuf[h.receiveStart : h.receiveStart+4])
		if payloadLength == 0 || payloadLength > maxFramePayloadSize {
			h.receiveStart++
			continue
		}

		frameLength := 4 + int(payloadLength)
		if h.receiveEnd-h.receiveStart < frameLength {
			return nil
		}

		frame := make([]byte, frameLength)
		copy(frame, h.receiveBuf[h.receiveStart:h.receiveStart+frameLength])
		h.receiveStart += frameLength
		if h.receiveStart == h.receiveEnd {
			h.receiveStart = 0
			h.receiveEnd = 0
		}

		h.log.Debug().Int("length", frameLength).Msg("LoRa packet received")
		return frame
	}

	return nil
}

func (h *LoRaHAT) compactReceiveBuffer() {
	if h.receiveStart == 0 {
		return
	}
	copy(h.receiveBuf[:], h.receiveBuf[h.receiveStart:h.receiveEnd])
	h.receiveEnd -= h.receiveStart
	h.receiveStart = 0
}

func (h *LoRaHAT) Close() {
	if h.serial != nil {
		_ = h.serial.Close()
	}
	if h.m0 != nil {
		_ = h.m0.Close()
	}
	if h.m1 != nil {
		_ = h.m1.Close()
	}
	h.log.Info().Msg("LoRa HAT resources cleaned up")
}

func Main(
	ctx context.Context,
	s *config.Radio,
) (protocol.RadioLink, error) {
	logger := zerolog.Ctx(ctx)

	serialDevice, useGPIO, err := resolveSerialDevice(s)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to resolve LoRa serial device")
		return nil, err
	}
	hat, err := NewLoRaHAT(ctx, serialDevice, useGPIO)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to initialize LoRa HAT")
		return nil, err
	}

	logger.Info().Msg("LoRa HAT ready")
	_ = s
	return hat, nil
}

func resolveSerialDevice(cfg *config.Radio) (string, bool, error) {
	if cfg == nil {
		return SERIAL_DEVICE, true, nil
	}
	usbID := strings.TrimSpace(cfg.UsbId)
	if isAutoUSB(usbID) || (usbID == "" && cfg.UsbScan) {
		dev, err := findUSBSerialDevice()
		if err != nil {
			return "", false, err
		}
		return dev, false, nil
	}
	if usbID != "" {
		dev, err := validateSerialDevice(usbID)
		return dev, false, err
	}
	uartDevice := strings.TrimSpace(cfg.UartDevice)
	if uartDevice == "" {
		return SERIAL_DEVICE, true, nil
	}
	dev, err := validateSerialDevice(uartDevice)
	return dev, true, err
}

func validateSerialDevice(device string) (string, error) {
	info, err := os.Stat(device)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return "", fmt.Errorf("%s is not a character device", device)
	}
	return device, nil
}

func radioMode(useGPIO bool) string {
	if useGPIO {
		return "GPIO UART"
	}
	return "USB"
}

func isAutoUSB(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "scan", "usb", "usb-auto", "usb-scan":
		return true
	default:
		return false
	}
}

func findUSBSerialDevice() (string, error) {
	candidates := make([]string, 0, 8)

	if entries, err := os.ReadDir("/dev/serial/by-id"); err == nil {
		for _, entry := range entries {
			candidates = append(candidates, filepath.Join("/dev/serial/by-id", entry.Name()))
		}
	}
	if len(candidates) == 0 {
		if matches, _ := filepath.Glob("/dev/ttyUSB*"); len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
		if matches, _ := filepath.Glob("/dev/ttyACM*"); len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
	}

	valid := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		resolved := candidate
		if info, err := os.Lstat(candidate); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if target, err := filepath.EvalSymlinks(candidate); err == nil {
				resolved = target
			}
		}
		info, err := os.Stat(resolved)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeCharDevice == 0 {
			continue
		}
		valid = append(valid, candidate)
	}
	if len(valid) == 0 {
		return "", fmt.Errorf("no USB serial devices found")
	}
	sort.Strings(valid)
	return valid[0], nil
}
