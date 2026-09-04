package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type notifyingEOFReader struct {
	read chan struct{}
}

func (r *notifyingEOFReader) Read([]byte) (int, error) {
	close(r.read)
	return 0, io.EOF
}

type transientEOFReader struct {
	eof     chan struct{}
	release chan struct{}
	step    int
}

func (r *transientEOFReader) Read(p []byte) (int, error) {
	switch r.step {
	case 0:
		r.step++
		close(r.eof)
		return 0, io.EOF
	case 1:
		r.step++
		return copy(p, "login:"), nil
	default:
		<-r.release
		return 0, io.EOF
	}
}

type channelWriter struct {
	writes chan string
}

func (w *channelWriter) Write(p []byte) (int, error) {
	w.writes <- string(p)
	return len(p), nil
}

func TestRunConsoleSurvivesInputAndSerialEOF(t *testing.T) {
	serialInput := &transientEOFReader{
		eof:     make(chan struct{}),
		release: make(chan struct{}),
	}
	port := struct {
		io.Reader
		io.Writer
	}{
		Reader: serialInput,
		Writer: io.Discard,
	}
	input := &notifyingEOFReader{read: make(chan struct{})}
	output := &channelWriter{writes: make(chan string, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runConsole(ctx, port, input, output)
	}()

	select {
	case <-input.read:
	case <-time.After(time.Second):
		t.Fatal("console did not read input")
	}
	select {
	case <-serialInput.eof:
	case <-time.After(time.Second):
		t.Fatal("console did not encounter serial EOF")
	}
	select {
	case got := <-output.writes:
		if got != "login:" {
			t.Fatalf("console output = %q, want %q", got, "login:")
		}
	case <-time.After(time.Second):
		t.Fatal("console stopped reading after serial EOF")
	}
	select {
	case err := <-done:
		t.Fatalf("console returned after transient EOF: %v", err)
	default:
	}

	cancel()
	close(serialInput.release)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runConsole() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("console did not stop after context cancellation")
	}
}

func TestSerialCandidatesPreferStableByIDAlias(t *testing.T) {
	devRoot := t.TempDir()
	byIDDir := filepath.Join(devRoot, "serial", "by-id")
	if err := os.MkdirAll(byIDDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ttyUSB0 := filepath.Join(devRoot, "ttyUSB0")
	if err := os.WriteFile(ttyUSB0, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"usb-a", "usb-z"} {
		if err := os.Symlink(filepath.Join("..", "..", "ttyUSB0"), filepath.Join(byIDDir, name)); err != nil {
			t.Fatal(err)
		}
	}

	candidates := serialCandidatesAt(devRoot)
	want := filepath.Join(byIDDir, "usb-a")
	if len(candidates) != 1 || candidates[0] != want {
		t.Fatalf("serialCandidatesAt() = %q, want only %q", candidates, want)
	}
	device, err := resolveSerialDevice("auto", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if device != want {
		t.Fatalf("auto-selected device = %q, want %q", device, want)
	}

	if err := os.WriteFile(filepath.Join(devRoot, "ttyUSB1"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveSerialDevice("auto", serialCandidatesAt(devRoot)); err == nil || !strings.Contains(err.Error(), "multiple serial devices") {
		t.Fatalf("auto-select with distinct adapters error = %v, want explicit selection error", err)
	}
}

func TestParseFlagsRejectsModeSpecificInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "command outside exec",
			args: []string{"--command", "uname -a", "wait"},
			want: "--command is only valid with exec mode",
		},
		{
			name: "empty command outside exec",
			args: []string{"--command", "", "console"},
			want: "--command is only valid with exec mode",
		},
		{
			name: "empty wait marker",
			args: []string{"--wait-marker", "", "wait"},
			want: "wait mode requires a non-empty --wait-marker",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := parseFlags(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseFlags(%q) error = %v, want %q", test.args, err, test.want)
			}
		})
	}
}

func TestParseFlagsSupportsModeFollowingFlags(t *testing.T) {
	t.Setenv("DRONEOS_SERIAL_DEVICE", "")
	tests := []struct {
		name        string
		args        []string
		wantCommand string
		wantDevice  string
	}{
		{
			name:        "command flag after exec",
			args:        []string{"exec", "--command", "uname -a"},
			wantCommand: "uname -a",
			wantDevice:  "auto",
		},
		{
			name:        "leading serial and following command",
			args:        []string{"--serial", "/dev/ttyUSB0", "exec", "--command", "uname -a"},
			wantCommand: "uname -a",
			wantDevice:  "/dev/ttyUSB0",
		},
		{
			name:        "positional command",
			args:        []string{"exec", "uname -a"},
			wantCommand: "uname -a",
			wantDevice:  "auto",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts, mode, err := parseFlags(test.args)
			if err != nil {
				t.Fatal(err)
			}
			if mode != "exec" {
				t.Fatalf("mode = %q, want exec", mode)
			}
			if opts.command != test.wantCommand {
				t.Fatalf("command = %q, want %q", opts.command, test.wantCommand)
			}
			if opts.device != test.wantDevice {
				t.Fatalf("device = %q, want %q", opts.device, test.wantDevice)
			}
		})
	}
}

type shortReadWriter struct{}

func (shortReadWriter) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (shortReadWriter) Write(value []byte) (int, error) {
	return len(value) - 1, nil
}

func TestWriteRawReportsShortWrite(t *testing.T) {
	runner := &serialRunner{port: shortReadWriter{}}
	if err := runner.writeRaw("x"); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("writeRaw() error = %v, want io.ErrShortWrite", err)
	}
}

type oneReadPort struct {
	data []byte
}

func (p *oneReadPort) Read(value []byte) (int, error) {
	if len(p.data) == 0 {
		return 0, io.EOF
	}
	n := copy(value, p.data)
	p.data = p.data[n:]
	return n, nil
}

func (p *oneReadPort) Write(value []byte) (int, error) {
	return len(value), nil
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestExpectPropagatesMirrorError(t *testing.T) {
	mirrorErr := errors.New("mirror failed")
	runner := &serialRunner{
		port:    &oneReadPort{data: []byte("login:")},
		mirror:  errorWriter{err: mirrorErr},
		scratch: make([]byte, 16),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := runner.expect(ctx, "login:"); !errors.Is(err, mirrorErr) {
		t.Fatalf("expect() error = %v, want mirror error", err)
	}
}

type writeErrorPort struct {
	err error
}

func (p writeErrorPort) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (p writeErrorPort) Write([]byte) (int, error) {
	return 0, p.err
}

func TestStartPokePropagatesWriteError(t *testing.T) {
	writeErr := errors.New("write failed")
	runner := &serialRunner{
		port:    writeErrorPort{err: writeErr},
		scratch: make([]byte, 16),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	pokeCtx, stop := runner.startPoke(ctx, time.Hour)
	defer stop()
	if _, err := runner.expect(pokeCtx, "never"); !errors.Is(err, writeErr) {
		t.Fatalf("expect() error = %v, want poke write error", err)
	}
}
