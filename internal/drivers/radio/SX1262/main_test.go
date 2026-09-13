//go:build linux
// +build linux

package SX1262

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"testing"

	"droneOS/internal/config"
)

func TestResolveSerialDeviceGPIOExplicitUART(t *testing.T) {
	device, useGPIO, err := resolveSerialDevice(&config.Radio{UartDevice: os.DevNull})
	if err != nil {
		t.Fatalf("resolveSerialDevice() error = %v", err)
	}
	if device != os.DevNull {
		t.Fatalf("resolveSerialDevice() device = %q, want %q", device, os.DevNull)
	}
	if !useGPIO {
		t.Fatal("resolveSerialDevice() useGPIO = false, want true")
	}
}

func TestResolveSerialDeviceGPIODefaultUART(t *testing.T) {
	device, useGPIO, err := resolveSerialDevice(&config.Radio{})
	if err != nil {
		t.Fatalf("resolveSerialDevice() error = %v", err)
	}
	if device != SERIAL_DEVICE {
		t.Fatalf("resolveSerialDevice() device = %q, want %q", device, SERIAL_DEVICE)
	}
	if !useGPIO {
		t.Fatal("resolveSerialDevice() useGPIO = false, want true")
	}
}

func TestResolveSerialDeviceUSBPrecedesUART(t *testing.T) {
	device, useGPIO, err := resolveSerialDevice(&config.Radio{
		UsbId:      os.DevNull,
		UartDevice: "/missing/uart",
	})
	if err != nil {
		t.Fatalf("resolveSerialDevice() error = %v", err)
	}
	if device != os.DevNull {
		t.Fatalf("resolveSerialDevice() device = %q, want %q", device, os.DevNull)
	}
	if useGPIO {
		t.Fatal("resolveSerialDevice() useGPIO = true, want false")
	}
}

func TestResolveSerialDeviceRejectsNonCharacterUART(t *testing.T) {
	device := t.TempDir() + "/uart"
	if err := os.WriteFile(device, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	_, useGPIO, err := resolveSerialDevice(&config.Radio{UartDevice: device})
	if err == nil {
		t.Fatal("resolveSerialDevice() error = nil, want character-device validation error")
	}
	if !useGPIO {
		t.Fatal("resolveSerialDevice() useGPIO = false, want true")
	}
}

func TestReceivePreservesFragmentedPrefix(t *testing.T) {
	readErr := errors.New("serial read interrupted")
	hat := &LoRaHAT{serial: &scriptedSerial{steps: []serialReadStep{
		{data: []byte{0, 0}},
		{data: []byte{0, 3}},
		{err: readErr},
		{data: []byte("one")},
	}}}

	requireNoFrame(t, hat)
	requireNoFrame(t, hat)
	if _, err := hat.Receive(); !errors.Is(err, readErr) {
		t.Fatalf("Receive() error = %v, want %v", err, readErr)
	}

	got, err := hat.Receive()
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	want := framed([]byte("one"))
	if !bytes.Equal(got, want) {
		t.Fatalf("Receive() = %x, want %x", got, want)
	}
}

func TestReceivePreservesQueuedFrameAfterFragmentedPayload(t *testing.T) {
	readErr := errors.New("serial read interrupted")
	first := framed([]byte("one"))
	second := framed([]byte("two"))
	hat := &LoRaHAT{serial: &scriptedSerial{steps: []serialReadStep{
		{data: first[:4]},
		{data: first[4:5]},
		{err: readErr},
		{data: append(first[5:], second...)},
	}}}

	requireNoFrame(t, hat)
	requireNoFrame(t, hat)
	if _, err := hat.Receive(); !errors.Is(err, readErr) {
		t.Fatalf("Receive() error = %v, want %v", err, readErr)
	}

	got, err := hat.Receive()
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	if !bytes.Equal(got, first) {
		t.Fatalf("first Receive() = %x, want %x", got, first)
	}

	got, err = hat.Receive()
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	if !bytes.Equal(got, second) {
		t.Fatalf("second Receive() = %x, want %x", got, second)
	}
}

func requireNoFrame(t *testing.T, hat *LoRaHAT) {
	t.Helper()

	got, err := hat.Receive()
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	if got != nil {
		t.Fatalf("Receive() = %x, want no frame", got)
	}
}

func framed(payload []byte) []byte {
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame, uint32(len(payload)))
	copy(frame[4:], payload)
	return frame
}

type serialReadStep struct {
	data []byte
	err  error
}

type scriptedSerial struct {
	steps      []serialReadStep
	step       int
	pending    []byte
	pendingErr error
}

func (s *scriptedSerial) Read(p []byte) (int, error) {
	for len(s.pending) == 0 && s.step < len(s.steps) {
		step := s.steps[s.step]
		s.step++
		s.pending = step.data
		s.pendingErr = step.err
		if len(s.pending) == 0 {
			err := s.pendingErr
			s.pendingErr = nil
			return 0, err
		}
	}
	if len(s.pending) == 0 {
		return 0, nil
	}

	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	if len(s.pending) == 0 && s.pendingErr != nil {
		err := s.pendingErr
		s.pendingErr = nil
		return n, err
	}
	return n, nil
}

func (s *scriptedSerial) Write(p []byte) (int, error) {
	return len(p), nil
}

func (s *scriptedSerial) Close() error {
	return nil
}
