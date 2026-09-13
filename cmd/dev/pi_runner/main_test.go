package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

func TestCopyRawConsoleInputForwardsTerminalResponseAndStopsOnInterrupt(t *testing.T) {
	const cursorPosition = "\x1b[31;11R"
	var output bytes.Buffer

	err := copyRawConsoleInput(&output, strings.NewReader(cursorPosition+"\x03ignored"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("copyRawConsoleInput() error = %v, want context.Canceled", err)
	}
	if got := output.String(); got != cursorPosition {
		t.Fatalf("copyRawConsoleInput() output = %q, want %q", got, cursorPosition)
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
	t.Setenv("DRONEOS_UPLOAD_FILE", "")
	t.Setenv("DRONEOS_UPLOAD_REMOTE", "")
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
			name: "file outside upload",
			args: []string{"--file", "build/droneOS/drone.bin", "wait"},
			want: "--file is only valid with upload mode",
		},
		{
			name: "remote outside upload",
			args: []string{"--remote", "/home/admin/drone.bin", "console"},
			want: "--remote is only valid with upload mode",
		},
		{
			name: "upload missing file",
			args: []string{"upload", "--remote", "/home/admin/drone.bin"},
			want: "upload mode requires --file or DRONEOS_UPLOAD_FILE",
		},
		{
			name: "upload missing remote",
			args: []string{"upload", "--file", "build/droneOS/drone.bin"},
			want: "upload mode requires --remote or DRONEOS_UPLOAD_REMOTE",
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

func TestParseFlagsSupportsUploadInputs(t *testing.T) {
	t.Run("environment defaults", func(t *testing.T) {
		t.Setenv("DRONEOS_UPLOAD_FILE", "build/droneOS/drone.bin")
		t.Setenv("DRONEOS_UPLOAD_REMOTE", "/home/admin/drone.bin")

		opts, mode, err := parseFlags([]string{"upload"})
		if err != nil {
			t.Fatal(err)
		}
		if mode != "upload" || opts.file != "build/droneOS/drone.bin" || opts.remote != "/home/admin/drone.bin" {
			t.Fatalf("parseFlags() = mode %q, file %q, remote %q", mode, opts.file, opts.remote)
		}
	})
	t.Run("flags after mode", func(t *testing.T) {
		t.Setenv("DRONEOS_UPLOAD_FILE", "")
		t.Setenv("DRONEOS_UPLOAD_REMOTE", "")

		opts, mode, err := parseFlags([]string{"upload", "--file", "build/droneOS/drone.bin", "--remote", "/home/admin/drone.bin"})
		if err != nil {
			t.Fatal(err)
		}
		if mode != "upload" || opts.file != "build/droneOS/drone.bin" || opts.remote != "/home/admin/drone.bin" {
			t.Fatalf("parseFlags() = mode %q, file %q, remote %q", mode, opts.file, opts.remote)
		}
	})
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

type uploadPort struct {
	responses      bytes.Buffer
	writes         [][]byte
	user           string
	exitStatus     int
	failPayload    bool
	payloadFailed  bool
	payloadStarted bool
	delimiter      string
	token          string
}

func (p *uploadPort) Read(data []byte) (int, error) {
	if p.responses.Len() == 0 {
		return 0, io.EOF
	}
	return p.responses.Read(data)
}

func (p *uploadPort) Write(data []byte) (int, error) {
	p.writes = append(p.writes, append([]byte(nil), data...))
	text := string(data)
	switch {
	case text == "\r":
		p.reply("login:")
	case text == p.user+"\r":
		p.reply("Password:")
	case strings.Contains(text, "__DRONEOS_UPLOAD_READY_"):
		p.reply("__DRONEOS_UPLOAD_READY_" + p.token + "__\r\n")
	case strings.Contains(text, "base64 -d"):
		p.delimiter = heredocDelimiter(text)
		p.payloadStarted = true
	case strings.Contains(text, "__DRONEOS_READY_"):
		p.reply(markerIn(text, "__DRONEOS_READY_") + "\r\n")
	case p.failPayload && p.payloadStarted && !p.payloadFailed && text != p.delimiter+"\r" && !strings.Contains(text, "rm -f --"):
		p.payloadFailed = true
		return 0, errors.New("serial payload write failed")
	case strings.Contains(text, "sha256sum --"):
		p.reply(uploadExitMarker(text) + "EXIT:" + strconv.Itoa(p.exitStatus) + "\r\n")
	}
	return len(data), nil
}

func (p *uploadPort) reply(text string) {
	_, _ = p.responses.WriteString(text)
}

func markerIn(text, prefix string) string {
	start := strings.Index(text, prefix)
	if start < 0 {
		return ""
	}
	end := strings.Index(text[start+len(prefix):], "__")
	if end < 0 {
		return ""
	}
	return text[start : start+len(prefix)+end+2]
}

func heredocDelimiter(text string) string {
	start := strings.Index(text, "<<'")
	if start < 0 {
		return ""
	}
	value := text[start+3:]
	end := strings.IndexByte(value, '\'')
	if end < 0 {
		return ""
	}
	return value[:end]
}

func uploadExitMarker(text string) string {
	start := strings.Index(text, "__DRONEOS_UPLOAD_")
	if start < 0 {
		return ""
	}
	end := strings.Index(text[start:], "EXIT:")
	if end < 0 {
		return ""
	}
	return text[start : start+end]
}

func uploadPayload(writes [][]byte, delimiter string) []byte {
	start := -1
	end := -1
	for index, write := range writes {
		text := string(write)
		if strings.Contains(text, "base64 -d") {
			start = index
			continue
		}
		if start >= 0 && text == delimiter+"\r" {
			end = index
			break
		}
	}
	if start < 0 || end < 0 {
		return nil
	}
	var encoded bytes.Buffer
	for _, write := range writes[start+1 : end] {
		_, _ = encoded.Write(bytes.ReplaceAll(write, []byte{'\r'}, nil))
	}
	return encoded.Bytes()
}

func TestRunUploadStreamsPayload(t *testing.T) {
	payload := bytes.Repeat([]byte{0, 1, 2, 3, 0xfe, 0xff, '\r', '\n'}, 1024)
	source := filepath.Join(t.TempDir(), "drone.bin")
	if err := os.WriteFile(source, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	remote := "/home/admin/firmware;$(touch not-run) 'quoted'.bin"
	token := "deterministic"
	port := &uploadPort{token: token, user: "admin"}
	opts := options{
		file:     source,
		password: "secret",
		remote:   remote,
		timeout:  time.Second,
		user:     "admin",
	}

	if err := runUploadWithToken(context.Background(), port, opts, token); err != nil {
		t.Fatal(err)
	}
	encoded := uploadPayload(port.writes, port.delimiter)
	got, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("uploaded payload = %d bytes, want %d bytes", len(got), len(payload))
	}
	lineCount := 0
	for _, write := range port.writes {
		if len(write) == 76 {
			lineCount++
		}
	}
	if lineCount < 2 {
		t.Fatalf("base64 payload used %d complete lines, want a multi-chunk transfer", lineCount)
	}
}

func TestRunUploadCleansFailedTransfer(t *testing.T) {
	source := filepath.Join(t.TempDir(), "drone.bin")
	if err := os.WriteFile(source, bytes.Repeat([]byte{1}, 128), 0o600); err != nil {
		t.Fatal(err)
	}
	token := "deterministic"
	remote := "/home/admin/drone.bin"
	port := &uploadPort{failPayload: true, token: token, user: "admin"}
	opts := options{
		file:     source,
		password: "secret",
		remote:   remote,
		timeout:  time.Second,
		user:     "admin",
	}

	err := runUploadWithToken(context.Background(), port, opts, token)
	if err == nil || !strings.Contains(err.Error(), "stream upload file") {
		t.Fatalf("runUploadWithToken() error = %v, want streaming failure", err)
	}
	if !port.payloadFailed {
		t.Fatal("upload did not attempt to stream the payload")
	}
}

func TestUploadFinishCommandPreservesDestinationOnHashMismatch(t *testing.T) {
	directory := t.TempDir()
	remote := filepath.Join(directory, "drone.bin")
	tempPath := remote + ".tmp"
	if err := os.WriteFile(remote, []byte("known-good"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tempPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := "__DRONEOS_UPLOAD_TEST__"
	output, err := exec.Command("sh", "-c", uploadFinishCommand(tempPath, remote, strings.Repeat("0", 64), marker)).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(remote)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "known-good" {
		t.Fatalf("remote destination = %q, want original content", got)
	}
	if _, err := os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt temporary file remains: %v", err)
	}
	if !strings.Contains(string(output), marker+"EXIT:1") {
		t.Fatalf("upload finish output = %q, want failed marker", output)
	}
}

func TestUploadFinishCommandCommitsVerifiedExecutable(t *testing.T) {
	directory := t.TempDir()
	remote := filepath.Join(directory, "drone.bin")
	tempPath := remote + ".tmp"
	payload := []byte("verified drone binary")
	if err := os.WriteFile(tempPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	if _, err := exec.Command("sh", "-c", uploadFinishCommand(tempPath, remote, hex.EncodeToString(digest[:]), "__DRONEOS_UPLOAD_TEST__")).CombinedOutput(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(remote)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("remote destination = %q, want committed payload", got)
	}
	info, err := os.Stat(remote)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("remote mode = %o, want 755", info.Mode().Perm())
	}
	if _, err := os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("verified temporary file remains: %v", err)
	}
}

func TestUploadCleanupRemovesTemporaryFile(t *testing.T) {
	tempPath := filepath.Join(t.TempDir(), "drone.bin.tmp")
	if err := os.WriteFile(tempPath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = exec.Command("sh", "-c", uploadCleanupCommand(tempPath)).CombinedOutput()
	if _, err := os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed temporary file remains: %v", err)
	}
}

func TestShellQuotePreservesSpecialRemotePath(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	remote := "/home/admin/firmware;$(touch " + marker + ") 'quoted' $HOME"
	output, err := exec.Command("sh", "-c", "printf %s "+shellQuote(remote)).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != remote {
		t.Fatalf("shellQuote() output = %q, want %q", output, remote)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shell-special path executed command: %v", err)
	}
}

func TestWaitExitStatusSkipsEchoedMarkerTemplate(t *testing.T) {
	marker := "__DRONEOS_CMD_TEST__"
	runner := &serialRunner{
		port:    &oneReadPort{},
		scratch: make([]byte, 16),
	}
	_, _ = runner.recent.WriteString(marker + "EXIT:%s\r\n" + marker + "EXIT:17\r\n")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	status, err := runner.waitExitStatus(ctx, marker)
	if err != nil {
		t.Fatal(err)
	}
	if status != 17 {
		t.Fatalf("waitExitStatus() = %d, want 17", status)
	}
}

type execOutputPort struct {
	responses bytes.Buffer
	user      string
}

func (p *execOutputPort) Read(data []byte) (int, error) {
	if p.responses.Len() == 0 {
		return 0, io.EOF
	}
	return p.responses.Read(data)
}

func (p *execOutputPort) Write(data []byte) (int, error) {
	text := string(data)
	switch {
	case text == "\r":
		p.reply("login:")
	case text == p.user+"\r":
		p.reply("Password:")
	case strings.Contains(text, "__DRONEOS_EXEC_READY_"):
		marker := splitMarker(text, "__DRONEOS_EXEC_READY_")
		p.reply(strings.Repeat("wrapped setup ", 128) + "'__DRONEOS_EXEC_READY_' '" + strings.TrimPrefix(marker, "__DRONEOS_EXEC_READY_") + "'\r\n")
		p.reply(marker + "\r\n")
	case strings.Contains(text, "__DRONEOS_READY_"):
		p.reply(markerIn(text, "__DRONEOS_READY_") + "\r\n")
	case strings.Contains(text, "__DRONEOS_CMD_"):
		marker := markerIn(text, "__DRONEOS_CMD_")
		p.reply("observable command output\r\n" + marker + "EXIT:0\r\n")
	}
	return len(data), nil
}

func (p *execOutputPort) reply(text string) {
	_, _ = p.responses.WriteString(text)
}

func splitMarker(text, prefix string) string {
	quotedPrefix := shellQuote(prefix)
	start := strings.Index(text, quotedPrefix)
	if start < 0 {
		return ""
	}
	suffix := text[start+len(quotedPrefix):]
	open := strings.IndexByte(suffix, '\'')
	if open < 0 {
		return ""
	}
	suffix = suffix[open+1:]
	close := strings.IndexByte(suffix, '\'')
	if close < 0 {
		return ""
	}
	return prefix + suffix[:close]
}

func TestRunExecRetainsOutputAfterWrappedSetupEcho(t *testing.T) {
	port := &execOutputPort{user: "admin"}
	opts := options{
		command:  "printf ignored",
		password: "secret",
		timeout:  time.Second,
		user:     "admin",
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	originalOutput := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = originalOutput
	}()

	if err := runExec(context.Background(), port, opts); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "observable command output\n" {
		t.Fatalf("runExec() output = %q, want observable command output", output)
	}
}
