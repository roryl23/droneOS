//go:build linux
// +build linux

package SX1262

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/protocol"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
)

type LoRaHAT struct {
	serial *serial.Port
	m0     *gpiocdev.Line
	m1     *gpiocdev.Line
	log    zerolog.Logger
	mode   string // "config", "tx", "rx"
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
		logger.Error().Err(err).Str("device", serialDevice).Msg("Failed to open serial port")
		return nil, err
	}

	hat := &LoRaHAT{
		serial: ser,
		m0:     m0,
		m1:     m1,
		log:    *logger,
	}

	// Only configure via software if using GPIO mode
	// In USB mode, the physical jumpers control M0/M1
	if useGPIO {
		logger.Info().Msg("Configuring LoRa in GPIO mode")

		// Set to configuration mode (M0=LOW, M1=HIGH)
		hat.setMode("config")

		// Configure LoRa parameters
		if err := hat.configureLoRa(); err != nil {
			hat.Close()
			return nil, err
		}

		// Set to transmission mode (M0=LOW, M1=LOW)
		hat.setMode("tx")

		// Give extra time for mode to settle
		time.Sleep(200 * time.Millisecond)

		logger.Info().Msg("LoRa configured in GPIO mode: M0=LOW, M1=LOW (TX/RX mode)")
	} else {
		logger.Info().Msg("LoRa in USB mode - ensure jumpers are set: UART=A, M0=GND, M1=GND for transmission")
		// In USB mode, assume jumpers are physically set correctly
		// No software configuration needed
	}

	logger.Info().Msg("LoRa HAT initialized successfully")
	return hat, nil
}

func (h *LoRaHAT) setMode(mode string) {
	h.mode = mode
	switch mode {
	case "config":
		h.setLine(h.m0, 0)
		h.setLine(h.m1, 1)
	case "tx", "rx":
		h.setLine(h.m0, 0)
		h.setLine(h.m1, 0)
	default:
		h.log.Warn().Str("mode", mode).Msg("Unknown mode, defaulting to TX")
		h.setLine(h.m0, 0)
		h.setLine(h.m1, 0)
	}

	time.Sleep(100 * time.Millisecond)
	h.log.Debug().Str("mode", mode).Msg("Mode set")
}

func (h *LoRaHAT) setLine(line *gpiocdev.Line, value int) {
	if line == nil {
		return
	}
	if err := line.SetValue(value); err != nil {
		h.log.Error().Err(err).Msg("Failed to set GPIO line")
	}
}

func (h *LoRaHAT) configureLoRa() error {
	h.log.Info().Msg("Configuring LoRa parameters")

	// Example configuration bytes (adapt from Waveshare documentation)
	// This is a placeholder - refer to your HAT's register map
	configCmd := []byte{
		0xC0, 0x00, 0x09, // Write to register 0x00, 9 bytes
		0x00, 0x00, // Address 0x0000
		0x00,       // Network ID 0
		0x17,       // Channel 23 (915 MHz)
		0x04,       // Air data rate 4.8K
		0x0D,       // Power 13 dBm (~30mA TX current, was 0x16 = 22 dBm ~140mA)
		0x01, 0x04, // Other parameters
	}

	_, err := h.serial.Write(configCmd)
	if err != nil {
		h.log.Error().Err(err).
			Msg("Failed to send configuration command")
		return err
	}

	time.Sleep(100 * time.Millisecond)

	// Drain the input buffer to discard any echo or config response
	h.drainSerialBuffer()

	h.log.Info().
		Msg("LoRa configuration sent")
	return nil
}

// drainSerialBuffer reads and discards all available data from the serial port
func (h *LoRaHAT) drainSerialBuffer() {
	buf := make([]byte, 256)
	totalDiscarded := 0

	// Read until no more data is immediately available
	// Limit iterations to prevent tight loop on USB devices
	for i := 0; i < 3; i++ {
		n, err := h.serial.Read(buf)
		if err != nil || n == 0 {
			break
		}
		totalDiscarded += n
		// Longer sleep for USB devices to avoid overwhelming the kernel
		time.Sleep(100 * time.Millisecond)
	}

	if totalDiscarded > 0 {
		h.log.Debug().Int("bytes", totalDiscarded).Msg("Drained serial buffer")
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
	// Try to read the 4-byte length prefix
	lengthBytes := make([]byte, 4)
	n, err := h.serial.Read(lengthBytes)

	// Handle timeout or no data (common, don't log)
	if err != nil || n == 0 {
		// Add small delay to prevent tight polling loop
		time.Sleep(10 * time.Millisecond)
		return []byte{}, nil
	}

	// If we got partial length bytes, try to complete the read
	if n < 4 {
		remaining := lengthBytes[n:]
		n2, err := io.ReadFull(h.serial, remaining)
		if err != nil {
			// Don't log every incomplete read - too noisy
			h.drainSerialBuffer()
			return []byte{}, nil
		}
		n += n2
	}

	// Parse the length
	length := binary.BigEndian.Uint32(lengthBytes)
	if length == 0 || length > 64*1024 {
		// Only log invalid lengths occasionally to avoid log spam
		h.drainSerialBuffer()
		return []byte{}, nil
	}

	// Read the complete payload
	payload := make([]byte, length)
	if _, err := io.ReadFull(h.serial, payload); err != nil {
		// Don't log - this can happen during normal operation
		h.drainSerialBuffer()
		return []byte{}, nil
	}

	// Return the complete frame (length prefix + payload)
	frame := make([]byte, 4+length)
	copy(frame[:4], lengthBytes)
	copy(frame[4:], payload)

	// Only log successful receives at debug level
	h.log.Debug().
		Int("length", len(frame)).
		Msg("LoRa packet received")

	return frame, nil
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
	if usbID == "" {
		return SERIAL_DEVICE, true, nil
	}
	info, err := os.Stat(usbID)
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return "", false, fmt.Errorf("%s is not a character device", usbID)
	}
	return usbID, false, nil
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
