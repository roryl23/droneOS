//go:build linux
// +build linux

package radio

import (
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stianeikeland/go-rpio/v4"
	"github.com/tarm/serial"
)

const (
	// GPIO pins for Waveshare LoRa HAT on Raspberry Pi
	M0_PIN = 17 // GPIO 17 (Physical Pin 11) - Mode 0
	M1_PIN = 27 // GPIO 27 (Physical Pin 13) - Mode 1

	// Serial configuration
	SERIAL_DEVICE = "/dev/ttyS0" // Pi's hardware UART
	BAUD_RATE     = 9600
)

type LoRaHAT struct {
	serial *serial.Port
	m0, m1 rpio.Pin
	log    zerolog.Logger
	mode   string // "config", "tx", "rx"
}

func NewLoRaHAT() (*LoRaHAT, error) {
	// Initialize logging
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05"
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	hatLog := log.Logger.With().
		Str("component", "lora_hat").
		Str("platform", "raspberry_pi_linux").
		Logger()

	hatLog.Info().Msg("Initializing LoRa HAT on Raspberry Pi")

	// Initialize GPIO
	if err := rpio.Open(); err != nil {
		hatLog.Error().Err(err).Msg("Failed to open GPIO")
		return nil, err
	}

	// Configure mode pins
	m0 := rpio.Pin(M0_PIN)
	m1 := rpio.Pin(M1_PIN)
	m0.Output()
	m1.Output()

	// Open serial port
	cfg := &serial.Config{
		Name: SERIAL_DEVICE,
		Baud: BAUD_RATE,
	}
	ser, err := serial.OpenPort(cfg)
	if err != nil {
		rpio.Close()
		hatLog.Error().Err(err).Str("device", SERIAL_DEVICE).Msg("Failed to open serial port")
		return nil, err
	}

	hat := &LoRaHAT{
		serial: ser,
		m0:     m0,
		m1:     m1,
		log:    hatLog,
	}

	// Set to configuration mode (M0=0, M1=1)
	hat.setMode("config")

	// Configure LoRa parameters (example configuration command)
	if err := hat.configureLoRa(); err != nil {
		hat.Close()
		return nil, err
	}

	// Set to normal mode (M0=0, M1=0)
	hat.setMode("tx")

	hatLog.Info().Msg("LoRa HAT initialized successfully")
	return hat, nil
}

func (h *LoRaHAT) setMode(mode string) {
	h.mode = mode
	switch mode {
	case "config":
		h.m0.Low()
		h.m1.High()
	case "tx", "rx":
		h.m0.Low()
		h.m1.Low()
	default:
		h.log.Warn().Str("mode", mode).Msg("Unknown mode, defaulting to TX")
		h.m0.Low()
		h.m1.Low()
	}

	time.Sleep(100 * time.Millisecond)
	h.log.Debug().Str("mode", mode).Msg("Mode set")
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
		0x16,       // Power 22 dBm
		0x01, 0x04, // Other parameters
	}

	_, err := h.serial.Write(configCmd)
	if err != nil {
		h.log.Error().Err(err).Msg("Failed to send configuration command")
		return err
	}

	time.Sleep(100 * time.Millisecond)
	h.log.Info().Msg("LoRa configuration sent")
	return nil
}

func (h *LoRaHAT) Send(data []byte) error {
	h.log.Info().
		Str("payload", string(data)).
		Int("length", len(data)).
		Msg("Sending LoRa packet")

	_, err := h.serial.Write(data)
	if err != nil {
		h.log.Error().Err(err).Msg("Serial write failed")
		return err
	}

	h.log.Info().Str("payload", string(data)).Msg("LoRa packet sent")
	return nil
}

func (h *LoRaHAT) Receive() ([]byte, error) {
	buf := make([]byte, 256)
	n, err := h.serial.Read(buf)
	if err != nil {
		return nil, err
	}

	data := make([]byte, n)
	copy(data, buf[:n])

	if n > 0 {
		h.log.Info().
			Str("payload", string(data)).
			Int("length", n).
			Msg("LoRa packet received")
	}

	return data, nil
}

func (h *LoRaHAT) Close() {
	h.serial.Close()
	rpio.Close()
	h.log.Info().Msg("LoRa HAT resources cleaned up")
}

func main() {
	hat, err := NewLoRaHAT()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize LoRa HAT")
	}
	defer hat.Close()

	log.Info().Msg("=== Raspberry Pi LoRa HAT Application ===")

	counter := 0
	for {
		counter++
		payload := fmt.Sprintf("Pi-LoRa #%03d @ %s", counter, time.Now().Format("15:04:05"))

		if err := hat.Send([]byte(payload)); err != nil {
			log.Warn().Err(err).Msg("Send failed")
		}

		// Check for received data
		if data, err := hat.Receive(); err == nil && len(data) > 0 {
			log.Info().Str("received", string(data)).Msg("Incoming packet")
		}

		time.Sleep(10 * time.Second)
	}
}
