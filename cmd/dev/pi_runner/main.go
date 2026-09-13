package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/tarm/serial"
	"golang.org/x/term"
)

const (
	defaultBaud    = 115200
	defaultTimeout = 90 * time.Second
	readTimeout    = 200 * time.Millisecond
	maxBuffer      = 64 * 1024
)

type options struct {
	baud       int
	command    string
	device     string
	file       string
	password   string
	poke       time.Duration
	remote     string
	timeout    time.Duration
	user       string
	verbose    bool
	waitMarker string
}

type serialRunner struct {
	port    io.ReadWriter
	mirror  io.Writer
	recent  bytes.Buffer
	scratch []byte
}

func main() {
	os.Exit(runCLI())
}

func runCLI() (exitCode int) {
	opts, mode, err := parseFlags(os.Args[1:])
	if err != nil {
		reportCLIError(err)
		return 2
	}

	if mode == "list" {
		if err := listDevices(os.Stdout); err != nil {
			reportCLIError(err)
			return 1
		}
		return 0
	}

	device, err := resolveDevice(opts.device)
	if err != nil {
		reportCLIError(err)
		return 1
	}

	port, err := serial.OpenPort(&serial.Config{
		Name:        device,
		Baud:        opts.baud,
		ReadTimeout: readTimeout,
	})
	if err != nil {
		reportCLIError(fmt.Errorf("open serial device %s: %w", device, err))
		return 1
	}
	defer func() {
		if err := port.Close(); err != nil {
			reportCLIError(fmt.Errorf("close serial device %s: %w", device, err))
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch mode {
	case "console":
		if err := writeFormatted(os.Stderr, "serial console on %s at %d baud\n", device, opts.baud); err != nil {
			reportCLIError(fmt.Errorf("write console status: %w", err))
			return 1
		}
		if err := runConsole(ctx, port, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
			reportCLIError(err)
			return 1
		}
	case "loopback":
		if err := runLoopback(ctx, port, opts); err != nil {
			reportCLIError(err)
			return 1
		}
	case "wait":
		if err := runWait(ctx, port, opts); err != nil {
			reportCLIError(err)
			return 1
		}
	case "exec":
		if err := runExec(ctx, port, opts); err != nil {
			reportCLIError(err)
			return 1
		}
	case "upload":
		if err := runUpload(ctx, port, opts); err != nil {
			reportCLIError(err)
			return 1
		}
	default:
		reportCLIError(fmt.Errorf("unsupported mode %q", mode))
		return 2
	}
	return 0
}

func reportCLIError(err error) {
	if writeErr := writeFormatted(os.Stderr, "%v\n", err); writeErr != nil {
		log.Error().
			Err(writeErr).
			Str("original_error", err.Error()).
			Msg("failed to write CLI error")
	}
}

func writeFormatted(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

func parseFlags(args []string) (options, string, error) {
	opts := options{
		baud:       envInt("DRONEOS_SERIAL_BAUD", defaultBaud),
		device:     envString("DRONEOS_SERIAL_DEVICE", "auto"),
		file:       envString("DRONEOS_UPLOAD_FILE", ""),
		password:   envString("DRONEOS_PI_PASSWORD", ""),
		poke:       envDuration("DRONEOS_SERIAL_POKE_INTERVAL", 2*time.Second),
		remote:     envString("DRONEOS_UPLOAD_REMOTE", ""),
		timeout:    envDuration("DRONEOS_SERIAL_TIMEOUT", defaultTimeout),
		user:       envString("DRONEOS_PI_USER", "admin"),
		waitMarker: envString("DRONEOS_SERIAL_WAIT_MARKER", "login:"),
	}

	leading := newFlagSet(&opts)
	if err := leading.Parse(args); err != nil {
		return opts, "", fmt.Errorf("%v\n%s", err, usage())
	}

	rest := leading.Args()
	if len(rest) == 0 {
		return opts, "", errors.New(usage())
	}
	mode := rest[0]

	trailing := newFlagSet(&opts)
	if err := trailing.Parse(rest[1:]); err != nil {
		return opts, "", fmt.Errorf("%v\n%s", err, usage())
	}
	commandSet := flagWasSet(leading, "command") || flagWasSet(trailing, "command")
	fileSet := flagWasSet(leading, "file") || flagWasSet(trailing, "file")
	remoteSet := flagWasSet(leading, "remote") || flagWasSet(trailing, "remote")
	rest = trailing.Args()
	if len(rest) > 0 {
		if mode != "exec" || opts.command != "" {
			return opts, "", fmt.Errorf("unexpected arguments: %s\n%s", strings.Join(rest, " "), usage())
		}
		opts.command = strings.Join(rest, " ")
	}

	if opts.baud <= 0 {
		return opts, "", fmt.Errorf("invalid baud rate: %d", opts.baud)
	}
	if opts.timeout <= 0 {
		return opts, "", fmt.Errorf("invalid timeout: %s", opts.timeout)
	}
	if commandSet && mode != "exec" {
		return opts, "", errors.New("--command is only valid with exec mode")
	}
	if fileSet && mode != "upload" {
		return opts, "", errors.New("--file is only valid with upload mode")
	}
	if remoteSet && mode != "upload" {
		return opts, "", errors.New("--remote is only valid with upload mode")
	}
	if mode == "wait" && opts.waitMarker == "" {
		return opts, "", errors.New("wait mode requires a non-empty --wait-marker")
	}
	if mode == "exec" && opts.command == "" {
		return opts, "", errors.New("exec mode requires --command or a command after exec")
	}
	if mode == "upload" && opts.file == "" {
		return opts, "", errors.New("upload mode requires --file or DRONEOS_UPLOAD_FILE")
	}
	if mode == "upload" && opts.remote == "" {
		return opts, "", errors.New("upload mode requires --remote or DRONEOS_UPLOAD_REMOTE")
	}
	return opts, mode, nil
}

func newFlagSet(opts *options) *flag.FlagSet {
	fs := flag.NewFlagSet("pi_runner", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&opts.baud, "baud", opts.baud, "serial baud rate")
	fs.StringVar(&opts.command, "command", opts.command, "command for exec mode")
	fs.StringVar(&opts.device, "serial", opts.device, "serial device path or auto")
	fs.StringVar(&opts.file, "file", opts.file, "local file for upload mode")
	fs.StringVar(&opts.password, "password", opts.password, "login password for exec or upload mode")
	fs.DurationVar(&opts.poke, "poke-interval", opts.poke, "interval for sending carriage returns in wait mode; 0 disables")
	fs.StringVar(&opts.remote, "remote", opts.remote, "remote destination path for upload mode")
	fs.DurationVar(&opts.timeout, "timeout", opts.timeout, "wait/login timeout")
	fs.StringVar(&opts.user, "user", opts.user, "login user for exec or upload mode")
	fs.BoolVar(&opts.verbose, "verbose", opts.verbose, "mirror login traffic during exec")
	fs.StringVar(&opts.waitMarker, "wait-marker", opts.waitMarker, "text to wait for in wait mode")
	return fs
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(current *flag.Flag) {
		found = found || current.Name == name
	})
	return found
}

func usage() string {
	return `usage:
  pi_runner list
  pi_runner [flags] loopback
  pi_runner [flags] wait
  pi_runner [flags] console
  pi_runner [flags] exec --command 'cd /home/admin/droneOS && go test ./...'
  pi_runner [flags] exec 'uname -a'
  pi_runner [flags] upload --file build/droneOS/drone.bin --remote /home/admin/drone.bin

flags:
  --serial path|auto          default: DRONEOS_SERIAL_DEVICE or auto
  --baud n                    default: DRONEOS_SERIAL_BAUD or 115200
  --user name                 default: DRONEOS_PI_USER or admin
  --password value            default: DRONEOS_PI_PASSWORD
  --poke-interval duration    default: DRONEOS_SERIAL_POKE_INTERVAL or 2s
  --timeout duration          default: DRONEOS_SERIAL_TIMEOUT or 90s
  --wait-marker text          default: DRONEOS_SERIAL_WAIT_MARKER or "login:"; required in wait mode
  --command text              command for exec mode only
  --file path                 local file for upload mode; default: DRONEOS_UPLOAD_FILE
  --remote path               remote destination for upload mode; default: DRONEOS_UPLOAD_REMOTE
  --verbose                   mirror login traffic during exec

When more than one USB serial adapter is attached, pass the stable
/dev/serial/by-id/... path with --serial or DRONEOS_SERIAL_DEVICE.`
}

func listDevices(w io.Writer) error {
	devices := serialCandidates()
	if len(devices) == 0 {
		return errors.New("no serial devices found under /dev/serial/by-id, /dev/ttyUSB*, or /dev/ttyACM*")
	}
	for _, device := range devices {
		if err := writeFormatted(w, "%s\n", device); err != nil {
			return fmt.Errorf("write serial device %q: %w", device, err)
		}
	}
	return nil
}

func resolveDevice(value string) (string, error) {
	return resolveSerialDevice(value, serialCandidates())
}

func resolveSerialDevice(value string, devices []string) (string, error) {
	if value != "" && value != "auto" {
		return value, nil
	}

	switch len(devices) {
	case 0:
		return "", errors.New("no serial devices found; connect the USB-UART adapter or set DRONEOS_SERIAL_DEVICE")
	case 1:
		return devices[0], nil
	default:
		return "", fmt.Errorf("multiple serial devices found; set DRONEOS_SERIAL_DEVICE or pass --serial:\n%s", strings.Join(devices, "\n"))
	}
}

func serialCandidates() []string {
	return serialCandidatesAt("/dev")
}

func serialCandidatesAt(devRoot string) []string {
	seen := map[string]bool{}
	var candidates []string
	add := func(values ...string) {
		for _, value := range values {
			resolved, err := filepath.EvalSymlinks(value)
			if err != nil || seen[resolved] {
				continue
			}
			if info, err := os.Stat(resolved); err == nil && !info.IsDir() {
				seen[resolved] = true
				candidates = append(candidates, value)
			}
		}
	}

	byIDPath := filepath.Join(devRoot, "serial", "by-id")
	if entries, err := os.ReadDir(byIDPath); err == nil {
		var byID []string
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 || entry.Type().IsRegular() {
				byID = append(byID, filepath.Join(byIDPath, entry.Name()))
			}
		}
		sort.Strings(byID)
		add(byID...)
	}

	for _, pattern := range []string{
		filepath.Join(devRoot, "ttyUSB*"),
		filepath.Join(devRoot, "ttyACM*"),
	} {
		matches, _ := filepath.Glob(pattern)
		sort.Strings(matches)
		add(matches...)
	}

	return candidates
}

func runConsole(ctx context.Context, port io.ReadWriter, input io.Reader, output io.Writer) (err error) {
	restoreInput, rawInput, err := makeConsoleInputRaw(input)
	if err != nil {
		return fmt.Errorf("configure console terminal: %w", err)
	}
	defer func() {
		if restoreErr := restoreInput(); err == nil && restoreErr != nil {
			err = fmt.Errorf("restore console terminal: %w", restoreErr)
		}
	}()

	errCh := make(chan error, 2)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := port.Read(buf)
			if n > 0 {
				written, writeErr := output.Write(buf[:n])
				if writeErr != nil {
					errCh <- writeErr
					return
				}
				if written != n {
					errCh <- io.ErrShortWrite
					return
				}
			}
			if err != nil && !errors.Is(err, io.EOF) {
				errCh <- err
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	go func() {
		if rawInput {
			errCh <- copyRawConsoleInput(port, input)
			return
		}
		_, copyErr := io.Copy(port, input)
		if copyErr != nil {
			errCh <- copyErr
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func makeConsoleInputRaw(input io.Reader) (restore func() error, raw bool, err error) {
	file, ok := input.(*os.File)
	if !ok {
		return func() error { return nil }, false, nil
	}
	fd := int(file.Fd())
	if !term.IsTerminal(fd) {
		return func() error { return nil }, false, nil
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, false, err
	}
	return func() error { return term.Restore(fd, state) }, true, nil
}

func copyRawConsoleInput(output io.Writer, input io.Reader) error {
	buf := make([]byte, 1024)
	for {
		n, readErr := input.Read(buf)
		if n > 0 {
			data := buf[:n]
			if interrupt := bytes.IndexByte(data, 0x03); interrupt >= 0 {
				data = data[:interrupt]
				readErr = context.Canceled
			}
			if len(data) > 0 {
				written, writeErr := output.Write(data)
				if writeErr != nil {
					return writeErr
				}
				if written != len(data) {
					return io.ErrShortWrite
				}
			}
		}
		if readErr != nil {
			return readErr
		}
	}
}

func runWait(ctx context.Context, port *serial.Port, opts options) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	runner := &serialRunner{
		port:    port,
		mirror:  os.Stdout,
		scratch: make([]byte, 1024),
	}

	pokeCtx, stopPoke := runner.startPoke(timeoutCtx, opts.poke)
	defer stopPoke()

	_, err := runner.expect(pokeCtx, opts.waitMarker)
	if err != nil {
		return fmt.Errorf("wait for %q: %w", opts.waitMarker, err)
	}
	if err := writeFormatted(os.Stderr, "\nfound %q\n", opts.waitMarker); err != nil {
		return fmt.Errorf("write wait status: %w", err)
	}
	return nil
}

func runLoopback(ctx context.Context, port *serial.Port, opts options) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	runner := &serialRunner{
		port:    port,
		mirror:  os.Stdout,
		scratch: make([]byte, 1024),
	}
	marker := fmt.Sprintf("__DRONEOS_LOOPBACK_%d__", time.Now().UnixNano())
	if err := runner.writeRaw("\r\n" + marker + "\r\n"); err != nil {
		return err
	}
	if _, err := runner.expect(timeoutCtx, marker); err != nil {
		return fmt.Errorf("loopback marker was not echoed; short adapter TX to RX and retry: %w", err)
	}
	if err := writeFormatted(os.Stderr, "\nserial loopback OK: %s\n", marker); err != nil {
		return fmt.Errorf("write loopback status: %w", err)
	}
	return nil
}

func runExec(ctx context.Context, port io.ReadWriter, opts options) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	var mirror io.Writer
	if opts.verbose {
		mirror = os.Stderr
	}
	runner := &serialRunner{
		port:    port,
		mirror:  mirror,
		scratch: make([]byte, 1024),
	}
	if err := runner.login(timeoutCtx, opts.user, opts.password); err != nil {
		return err
	}
	runner.reset()

	token := strconv.FormatInt(time.Now().UnixNano(), 10)
	readyMarker := "__DRONEOS_EXEC_READY_" + token + "__"
	failedMarker := "__DRONEOS_EXEC_FAILED_" + token + "__"
	exitMarker := "__DRONEOS_CMD_" + token + "__"
	restoreNeeded := false
	finished := false
	defer func() {
		if restoreNeeded && !finished {
			_ = runner.writeLine("stty echo")
		}
	}()

	setup := fmt.Sprintf(
		"if stty -echo; then printf '\\n%%s%%s\\n' %s %s; else printf '\\n%%s%%s\\n' %s %s; fi",
		shellQuote("__DRONEOS_EXEC_READY_"),
		shellQuote(token+"__"),
		shellQuote("__DRONEOS_EXEC_FAILED_"),
		shellQuote(token+"__"),
	)
	restoreNeeded = true
	if err := runner.writeLine(setup); err != nil {
		return fmt.Errorf("disable remote echo: %w", err)
	}
	response, err := runner.expect(timeoutCtx, readyMarker, failedMarker)
	if err != nil {
		return fmt.Errorf("wait for remote exec setup: %w", err)
	}
	if strings.Contains(response, failedMarker) {
		restoreNeeded = false
		return errors.New("remote terminal refused to disable echo for exec")
	}
	runner.reset()

	command := fmt.Sprintf(
		"sh -lc %s; status=$?; if ! stty echo; then status=1; fi; printf '\\n%sEXIT:%%s\\n' \"$status\"",
		shellQuote(opts.command),
		exitMarker,
	)
	if err := runner.writeLine(command); err != nil {
		return err
	}
	if _, err := runner.expect(timeoutCtx, exitMarker+"EXIT:"); err != nil {
		return fmt.Errorf("wait for command exit marker: %w", err)
	}
	status, err := runner.waitExitStatus(timeoutCtx, exitMarker)
	if err != nil {
		return err
	}

	cleaned := stripAfterMarker(normalizeSerialOutput(runner.recent.String()), exitMarker)
	if err := writeFormatted(os.Stdout, "%s", cleaned); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(cleaned) > 0 && !strings.HasSuffix(cleaned, "\n") {
		if err := writeFormatted(os.Stdout, "\n"); err != nil {
			return fmt.Errorf("terminate command output: %w", err)
		}
	}
	if status != 0 {
		return fmt.Errorf("remote command exited with status %d", status)
	}
	finished = true
	return nil
}

func runUpload(ctx context.Context, port io.ReadWriter, opts options) error {
	token, err := newUploadToken()
	if err != nil {
		return err
	}
	return runUploadWithToken(ctx, port, opts, token)
}

func runUploadWithToken(ctx context.Context, port io.ReadWriter, opts options, token string) error {
	source, err := os.Open(opts.file)
	if err != nil {
		return fmt.Errorf("open upload file %s: %w", opts.file, err)
	}
	defer source.Close()

	tempPath, err := uploadTempPath(opts.remote, token)
	if err != nil {
		return err
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	runner := &serialRunner{
		port:    port,
		scratch: make([]byte, 1024),
	}
	if err := runner.login(timeoutCtx, opts.user, opts.password); err != nil {
		return err
	}
	runner.reset()

	readyMarker := "__DRONEOS_UPLOAD_READY_" + token + "__"
	failedMarker := "__DRONEOS_UPLOAD_FAILED_" + token + "__"
	exitMarker := "__DRONEOS_UPLOAD_" + token + "__"
	delimiter := "__DRONEOS_UPLOAD_DATA_" + token + "__"
	lineWriter := &base64LineWriter{destination: serialWriter{runner: runner}}
	heredocOpen := false
	restoreNeeded := false
	finished := false
	defer func() {
		if finished {
			return
		}
		if heredocOpen {
			_ = lineWriter.Close()
			_ = runner.writeLine(delimiter)
		}
		if restoreNeeded {
			_ = runner.writeLine(uploadCleanupCommand(tempPath))
		}
	}()

	setup := fmt.Sprintf(
		"if stty -echo; then printf '\\n%%s%%s\\n' %s %s; else printf '\\n%%s%%s\\n' %s %s; fi",
		shellQuote("__DRONEOS_UPLOAD_READY_"),
		shellQuote(token+"__"),
		shellQuote("__DRONEOS_UPLOAD_FAILED_"),
		shellQuote(token+"__"),
	)
	restoreNeeded = true
	if err := runner.writeLine(setup); err != nil {
		return fmt.Errorf("disable remote echo: %w", err)
	}

	response, err := runner.expect(timeoutCtx, readyMarker, failedMarker)
	if err != nil {
		return fmt.Errorf("wait for remote upload setup: %w", err)
	}
	if strings.Contains(response, failedMarker) {
		restoreNeeded = false
		return errors.New("remote terminal refused to disable echo for upload")
	}
	runner.reset()

	heredocOpen = true
	header := fmt.Sprintf("base64 -d > %s <<%s", shellQuote(tempPath), shellQuote(delimiter))
	if err := runner.writeLine(header); err != nil {
		return fmt.Errorf("begin remote upload: %w", err)
	}

	digest, err := streamBase64Upload(source, lineWriter)
	if err != nil {
		return fmt.Errorf("stream upload file: %w", err)
	}
	if err := runner.writeLine(delimiter); err != nil {
		return fmt.Errorf("finish upload payload: %w", err)
	}
	heredocOpen = false

	finish := uploadFinishCommand(tempPath, opts.remote, hex.EncodeToString(digest), exitMarker)
	if err := runner.writeLine(finish); err != nil {
		return fmt.Errorf("verify uploaded file: %w", err)
	}
	if _, err := runner.expect(timeoutCtx, exitMarker+"EXIT:"); err != nil {
		return fmt.Errorf("wait for upload exit marker: %w", err)
	}
	status, err := runner.waitExitStatus(timeoutCtx, exitMarker)
	if err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("remote upload failed with status %d", status)
	}
	finished = true
	return nil
}

func newUploadToken() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate upload token: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func uploadTempPath(remote, token string) (string, error) {
	if remote == "" {
		return "", errors.New("upload mode requires a remote destination path")
	}
	if strings.IndexByte(remote, 0) >= 0 {
		return "", errors.New("remote destination path contains a null byte")
	}
	if strings.HasSuffix(remote, "/") || filepath.Base(remote) == "." || filepath.Base(remote) == ".." {
		return "", fmt.Errorf("remote destination must name a file: %s", remote)
	}
	return remote + ".droneos-upload-" + token + ".tmp", nil
}

func uploadFinishCommand(tempPath, remote, digest, marker string) string {
	quotedTemp := shellQuote(tempPath)
	return fmt.Sprintf(
		`if actual=$(sha256sum -- %s) && [ "${actual%%%% *}" = %s ] && chmod 0755 -- %s && mv -f -- %s %s; then status=0; else rm -f -- %s; status=1; fi; if ! stty echo; then status=1; fi; printf '\n%sEXIT:%%s\n' "$status"`,
		quotedTemp,
		shellQuote(digest),
		quotedTemp,
		quotedTemp,
		shellQuote(remote),
		quotedTemp,
		marker,
	)
}

func uploadCleanupCommand(tempPath string) string {
	return fmt.Sprintf("rm -f -- %s; stty echo", shellQuote(tempPath))
}

func streamBase64Upload(source io.Reader, lines *base64LineWriter) ([]byte, error) {
	digest := sha256.New()
	encoded := base64.NewEncoder(base64.StdEncoding, lines)
	_, copyErr := io.Copy(encoded, io.TeeReader(source, digest))
	closeErr := encoded.Close()
	lineErr := lines.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if lineErr != nil {
		return nil, lineErr
	}
	return digest.Sum(nil), nil
}

type serialWriter struct {
	runner *serialRunner
}

func (w serialWriter) Write(data []byte) (int, error) {
	return w.runner.write(data)
}

type base64LineWriter struct {
	destination io.Writer
	line        [76]byte
	length      int
}

func (w *base64LineWriter) Write(data []byte) (int, error) {
	consumed := 0
	for len(data) > 0 {
		n := copy(w.line[w.length:], data)
		w.length += n
		consumed += n
		data = data[n:]
		if w.length != len(w.line) {
			continue
		}
		if err := writeAll(w.destination, w.line[:]); err != nil {
			return consumed, err
		}
		if err := writeAll(w.destination, []byte{'\r'}); err != nil {
			return consumed, err
		}
		w.length = 0
	}
	return consumed, nil
}

func (w *base64LineWriter) Close() error {
	if w.length == 0 {
		return nil
	}
	if err := writeAll(w.destination, w.line[:w.length]); err != nil {
		return err
	}
	if err := writeAll(w.destination, []byte{'\r'}); err != nil {
		return err
	}
	w.length = 0
	return nil
}

func writeAll(destination io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := destination.Write(data)
		if written > 0 {
			data = data[written:]
		}
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func (r *serialRunner) login(ctx context.Context, user, password string) error {
	if err := r.writeRaw("\r"); err != nil {
		return fmt.Errorf("wake login prompt: %w", err)
	}
	text, err := r.expect(ctx, "login:", "Password:", "$ ", "# ")
	if err != nil {
		return fmt.Errorf("wait for login prompt: %w", err)
	}

	if strings.Contains(text, "$ ") || strings.Contains(text, "# ") {
		return r.confirmShell(ctx)
	}

	if strings.Contains(text, "login:") {
		if user == "" {
			return errors.New("login prompt received but no user was configured")
		}
		if err := r.writeLine(user); err != nil {
			return err
		}
		if _, err := r.expect(ctx, "Password:"); err != nil {
			return fmt.Errorf("wait for password prompt: %w", err)
		}
	}

	if password == "" {
		return errors.New("password prompt received but no password was configured; set DRONEOS_PI_PASSWORD or pass --password")
	}
	if err := r.writeLine(password); err != nil {
		return err
	}

	return r.confirmShell(ctx)
}

func (r *serialRunner) confirmShell(ctx context.Context) error {
	marker := fmt.Sprintf("__DRONEOS_READY_%d__", time.Now().UnixNano())
	if err := r.writeLine("printf '\\n" + marker + "\\n'"); err != nil {
		return err
	}
	if _, err := r.expect(ctx, marker); err != nil {
		return fmt.Errorf("login succeeded but shell marker was not observed: %w", err)
	}
	return nil
}

func (r *serialRunner) waitExitStatus(ctx context.Context, marker string) (int, error) {
	prefix := marker + "EXIT:"
	for {
		text := r.recent.String()
		offset := 0
		for {
			start := strings.Index(text[offset:], prefix)
			if start < 0 {
				break
			}
			start += offset + len(prefix)
			line := text[start:]
			end := strings.IndexAny(line, "\r\n")
			if end < 0 {
				break
			}
			status, err := strconv.Atoi(strings.TrimSpace(line[:end]))
			if err == nil {
				return status, nil
			}
			offset = start + end + 1
		}

		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("read command exit status: %w", context.Cause(ctx))
		default:
		}

		n, err := r.port.Read(r.scratch)
		if n > 0 {
			if appendErr := r.append(r.scratch[:n]); appendErr != nil {
				return 0, appendErr
			}
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
	}
}

func (r *serialRunner) expect(ctx context.Context, needles ...string) (string, error) {
	for {
		current := r.recent.String()
		for _, needle := range needles {
			if needle != "" && strings.Contains(current, needle) {
				return current, nil
			}
		}

		select {
		case <-ctx.Done():
			return current, context.Cause(ctx)
		default:
		}

		n, err := r.port.Read(r.scratch)
		if n > 0 {
			if appendErr := r.append(r.scratch[:n]); appendErr != nil {
				return current, appendErr
			}
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return current, err
		}
	}
}

func (r *serialRunner) append(data []byte) error {
	if r.mirror != nil {
		written, err := r.mirror.Write(data)
		if err != nil {
			return fmt.Errorf("mirror serial output: %w", err)
		}
		if written != len(data) {
			return fmt.Errorf("mirror serial output: %w", io.ErrShortWrite)
		}
	}
	written, err := r.recent.Write(data)
	if err != nil {
		return fmt.Errorf("buffer serial output: %w", err)
	}
	if written != len(data) {
		return fmt.Errorf("buffer serial output: %w", io.ErrShortWrite)
	}
	if r.recent.Len() <= maxBuffer {
		return nil
	}
	value := r.recent.Bytes()
	keep := append([]byte(nil), value[len(value)-maxBuffer:]...)
	r.recent.Reset()
	written, err = r.recent.Write(keep)
	if err != nil {
		return fmt.Errorf("trim serial output buffer: %w", err)
	}
	if written != len(keep) {
		return fmt.Errorf("trim serial output buffer: %w", io.ErrShortWrite)
	}
	return nil
}

func (r *serialRunner) reset() {
	r.recent.Reset()
}

func (r *serialRunner) startPoke(ctx context.Context, interval time.Duration) (context.Context, func()) {
	if interval <= 0 {
		return ctx, func() {}
	}

	pokeCtx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		poke := func() bool {
			if err := r.writeRaw("\r"); err != nil {
				cancel(fmt.Errorf("write serial wakeup: %w", err))
				return false
			}
			return true
		}

		select {
		case <-pokeCtx.Done():
			return
		default:
		}
		if !poke() {
			return
		}
		for {
			select {
			case <-pokeCtx.Done():
				return
			case <-ticker.C:
				if !poke() {
					return
				}
			}
		}
	}()

	return pokeCtx, func() {
		cancel(context.Canceled)
		<-done
	}
}

func (r *serialRunner) writeLine(line string) error {
	return r.writeRaw(line + "\r")
}

func (r *serialRunner) write(data []byte) (int, error) {
	written, err := r.port.Write(data)
	if err != nil {
		return written, err
	}
	if written != len(data) {
		return written, io.ErrShortWrite
	}
	return written, nil
}

func (r *serialRunner) writeRaw(value string) error {
	_, err := r.write([]byte(value))
	return err
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func normalizeSerialOutput(output string) string {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	return strings.ReplaceAll(output, "\r", "\n")
}

func stripCommandEcho(output, command string) string {
	output = normalizeSerialOutput(output)
	if idx := strings.Index(output, command); idx >= 0 {
		output = output[idx+len(command):]
	}
	return strings.TrimLeft(output, "\n")
}

func stripAfterMarker(output, marker string) string {
	if idx := strings.Index(output, marker+"EXIT:"); idx >= 0 {
		return strings.TrimRight(output[:idx], "\n")
	}
	return output
}

func envString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
