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
	opts, mode, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if mode == "list" {
		if err := listDevices(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	device, err := resolveDevice(opts.device)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	port, err := serial.OpenPort(&serial.Config{
		Name:        device,
		Baud:        opts.baud,
		ReadTimeout: readTimeout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "open serial device %s: %v\n", device, err)
		os.Exit(1)
	}
	defer port.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch mode {
	case "console":
		fmt.Fprintf(os.Stderr, "serial console on %s at %d baud\n", device, opts.baud)
		if err := runConsole(ctx, port, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "loopback":
		if err := runLoopback(ctx, port, opts); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "wait":
		if err := runWait(ctx, port, opts); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "exec":
		if err := runExec(ctx, port, opts); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unsupported mode %q\n", mode)
		os.Exit(2)
	}
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
		fmt.Fprintln(w, device)
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

	stopPoke := runner.startPoke(timeoutCtx, opts.poke)
	defer stopPoke()

	_, err := runner.expect(timeoutCtx, opts.waitMarker)
	if err != nil {
		return fmt.Errorf("wait for %q: %w", opts.waitMarker, err)
	}
	fmt.Fprintf(os.Stderr, "\nfound %q\n", opts.waitMarker)
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
	fmt.Fprintf(os.Stderr, "\nserial loopback OK: %s\n", marker)
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
	fmt.Print(cleaned)
	if len(cleaned) > 0 && !strings.HasSuffix(cleaned, "\n") {
		fmt.Println()
	}

	if status != 0 {
		return fmt.Errorf("remote command exited with status %d", status)
	}
	return nil
}

func (r *serialRunner) login(ctx context.Context, user, password string) error {
	_ = r.writeRaw("\r")
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
			return 0, fmt.Errorf("read command exit status: %w", ctx.Err())
		default:
		}

		n, err := r.port.Read(r.scratch)
		if n > 0 {
			r.append(r.scratch[:n])
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
			return current, ctx.Err()
		default:
		}

		n, err := r.port.Read(r.scratch)
		if n > 0 {
			r.append(r.scratch[:n])
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return current, err
		}
	}
}

func (r *serialRunner) append(data []byte) {
	if r.mirror != nil {
		_, _ = r.mirror.Write(data)
	}
	_, _ = r.recent.Write(data)
	if r.recent.Len() <= maxBuffer {
		return
	}
	value := r.recent.Bytes()
	keep := append([]byte(nil), value[len(value)-maxBuffer:]...)
	r.recent.Reset()
	_, _ = r.recent.Write(keep)
}

func (r *serialRunner) reset() {
	r.recent.Reset()
}

func (r *serialRunner) startPoke(ctx context.Context, interval time.Duration) func() {
	if interval <= 0 {
		return func() {}
	}

	pokeCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		_ = r.writeRaw("\r")
		for {
			select {
			case <-pokeCtx.Done():
				return
			case <-ticker.C:
				_ = r.writeRaw("\r")
			}
		}
	}()

	return func() {
		cancel()
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
