package main

import (
	"bytes"
	"context"
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
	password   string
	poke       time.Duration
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
		password:   envString("DRONEOS_PI_PASSWORD", ""),
		poke:       envDuration("DRONEOS_SERIAL_POKE_INTERVAL", 2*time.Second),
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
	if mode == "wait" && opts.waitMarker == "" {
		return opts, "", errors.New("wait mode requires a non-empty --wait-marker")
	}
	if mode == "exec" && opts.command == "" {
		return opts, "", errors.New("exec mode requires --command or a command after exec")
	}
	return opts, mode, nil
}

func newFlagSet(opts *options) *flag.FlagSet {
	fs := flag.NewFlagSet("pi_runner", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&opts.baud, "baud", opts.baud, "serial baud rate")
	fs.StringVar(&opts.command, "command", opts.command, "command for exec mode")
	fs.StringVar(&opts.device, "serial", opts.device, "serial device path or auto")
	fs.StringVar(&opts.password, "password", opts.password, "login password for exec mode")
	fs.DurationVar(&opts.poke, "poke-interval", opts.poke, "interval for sending carriage returns in wait mode; 0 disables")
	fs.DurationVar(&opts.timeout, "timeout", opts.timeout, "wait/login timeout")
	fs.StringVar(&opts.user, "user", opts.user, "login user for exec mode")
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

flags:
  --serial path|auto          default: DRONEOS_SERIAL_DEVICE or auto
  --baud n                    default: DRONEOS_SERIAL_BAUD or 115200
  --user name                 default: DRONEOS_PI_USER or admin
  --password value            default: DRONEOS_PI_PASSWORD
  --poke-interval duration    default: DRONEOS_SERIAL_POKE_INTERVAL or 2s
  --timeout duration          default: DRONEOS_SERIAL_TIMEOUT or 90s
  --wait-marker text          default: DRONEOS_SERIAL_WAIT_MARKER or "login:"; required in wait mode
  --command text              command for exec mode only
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

func runConsole(ctx context.Context, port io.ReadWriter, input io.Reader, output io.Writer) error {
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
		_, err := io.Copy(port, input)
		if err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
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

func runExec(ctx context.Context, port *serial.Port, opts options) error {
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

	marker := fmt.Sprintf("__DRONEOS_CMD_%d__", time.Now().UnixNano())
	command := fmt.Sprintf("sh -lc %s; status=$?; printf '\\n%sEXIT:%%s\\n' \"$status\"", shellQuote(opts.command), marker)

	if err := runner.writeLine(command); err != nil {
		return err
	}

	output, err := runner.expect(timeoutCtx, marker+"EXIT:")
	if err != nil {
		return fmt.Errorf("wait for command exit marker: %w", err)
	}

	status, err := runner.waitExitStatus(timeoutCtx, marker)
	if err != nil {
		return err
	}

	cleaned := stripCommandEcho(output, command)
	cleaned = stripAfterMarker(cleaned, marker)
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
		start := strings.LastIndex(text, prefix)
		if start >= 0 {
			start += len(prefix)
			line := text[start:]
			if idx := strings.IndexAny(line, "\r\n"); idx >= 0 {
				line = strings.TrimSpace(line[:idx])
				status, err := strconv.Atoi(line)
				if err != nil {
					return 0, fmt.Errorf("parse command exit status %q: %w", line, err)
				}
				return status, nil
			}
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

func (r *serialRunner) writeRaw(value string) error {
	n, err := r.port.Write([]byte(value))
	if err != nil {
		return err
	}
	if n != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func stripCommandEcho(output, command string) string {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
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
