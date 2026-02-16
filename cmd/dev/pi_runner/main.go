package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type config struct {
	configFile      string
	droneConfigFile string
	arch            string
	goarm           string
	cc              string
	piHost          string
	piUser          string
	piPort          string
	piDir           string
	piBinName       string
	output          string
	goCmd           string
}

type processHandle struct {
	cmd  *exec.Cmd
	done chan error
}

func main() {
	cfg := parseFlags()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() config {
	configFile := flag.String("config-file", "./configs/config.yaml", "Path to local config file for base station")
	droneConfigFile := flag.String("drone-config-file", "", "Path to local config file for drone (defaults to config-file)")
	arch := flag.String("arch", envOr("DRONEOS_PI_ARCH", "arm64"), "Target GOARCH for Raspberry Pi: arm64 or arm")
	goarm := flag.String("goarm", envOr("DRONEOS_PI_GOARM", ""), "Target GOARM for Raspberry Pi (used when arch=arm)")
	cc := flag.String("cc", envOr("DRONEOS_PI_CC", ""), "C compiler for CGO cross-compile")
	piHost := flag.String("pi-host", envOr("DRONEOS_PI_HOST", "raspberrypi.local"), "Raspberry Pi SSH host or IP")
	piUser := flag.String("pi-user", envOr("DRONEOS_PI_USER", "pi"), "Raspberry Pi SSH user")
	piPort := flag.String("pi-port", envOr("DRONEOS_PI_PORT", "22"), "Raspberry Pi SSH port")
	piDir := flag.String("pi-dir", envOr("DRONEOS_PI_DIR", "/home/pi/droneOS"), "Remote deploy directory")
	piBinName := flag.String("pi-bin-name", envOr("DRONEOS_PI_BIN", "drone.bin"), "Remote drone binary name")
	output := flag.String("output", envOr("DRONEOS_PI_OUT", filepath.Join("build", "droneOS", "drone.pi")), "Local output path for drone binary")
	goCmd := flag.String("go-cmd", envOr("DRONEOS_GO_CMD", "go"), "Go command to use")

	flag.Parse()

	cfg := config{
		configFile:      *configFile,
		droneConfigFile: *droneConfigFile,
		arch:            *arch,
		goarm:           *goarm,
		cc:              *cc,
		piHost:          *piHost,
		piUser:          *piUser,
		piPort:          *piPort,
		piDir:           *piDir,
		piBinName:       *piBinName,
		output:          *output,
		goCmd:           *goCmd,
	}
	if cfg.droneConfigFile == "" {
		cfg.droneConfigFile = cfg.configFile
	}
	return cfg
}

func run(ctx context.Context, cfg config) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	if err := requireFile(cfg.configFile); err != nil {
		return fmt.Errorf("base config: %w", err)
	}
	if err := requireFile(cfg.droneConfigFile); err != nil {
		return fmt.Errorf("drone config: %w", err)
	}
	if err := requireCommand(cfg.goCmd); err != nil {
		return err
	}
	if err := requireCommand("ssh"); err != nil {
		return err
	}
	if err := requireCommand("scp"); err != nil {
		return err
	}

	arch := normalizeArch(cfg.arch)
	if arch == "" {
		return fmt.Errorf("unsupported arch %q (use arm64 or arm)", cfg.arch)
	}
	cfg.arch = arch
	if cfg.cc == "" {
		cfg.cc = defaultCC(cfg.arch)
	}
	if cfg.goarm == "" && cfg.arch == "arm" {
		cfg.goarm = "5"
	}
	if cfg.cc != "" {
		if err := requireCommand(cfg.cc); err != nil {
			return err
		}
	}

	baseCmd, err := startBase(projectDir, cfg.goCmd, cfg.configFile)
	if err != nil {
		return err
	}
	defer baseCmd.stop(5 * time.Second)

	time.Sleep(750 * time.Millisecond)

	if err := buildDrone(ctx, projectDir, cfg); err != nil {
		return err
	}
	if err := ensureRemoteDir(ctx, projectDir, cfg); err != nil {
		return err
	}
	if err := copyFiles(ctx, projectDir, cfg); err != nil {
		return err
	}

	return runRemote(ctx, projectDir, cfg)
}

func startBase(projectDir, goCmd, configFile string) (*processHandle, error) {
	cmd := exec.Command(goCmd, "run", "./cmd/base/main.go", "--config-file", configFile)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start base: %w", err)
	}

	h := &processHandle{cmd: cmd, done: make(chan error, 1)}
	go func() {
		h.done <- cmd.Wait()
	}()
	return h, nil
}

func (p *processHandle) stop(timeout time.Duration) {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	select {
	case <-p.done:
		return
	default:
	}

	_ = p.cmd.Process.Signal(os.Interrupt)
	select {
	case <-p.done:
	case <-time.After(timeout):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
}

func buildDrone(ctx context.Context, projectDir string, cfg config) error {
	if err := os.MkdirAll(filepath.Dir(cfg.output), 0o755); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}

	env := append(os.Environ(),
		"CGO_ENABLED=1",
		"GOOS=linux",
		"GOARCH="+cfg.arch,
	)
	if cfg.goarm != "" {
		env = append(env, "GOARM="+cfg.goarm)
	}
	if cfg.cc != "" {
		env = append(env, "CC="+cfg.cc)
	}

	cmd := exec.CommandContext(ctx, cfg.goCmd, "build", "-o", cfg.output, "./cmd/drone/main.go")
	cmd.Dir = projectDir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build drone: %w", err)
	}

	return nil
}

func ensureRemoteDir(ctx context.Context, projectDir string, cfg config) error {
	sshHost := formatSSHHost(cfg.piUser, cfg.piHost)
	cmd := exec.CommandContext(ctx, "ssh", "-p", cfg.piPort, sshHost, "mkdir -p "+shellEscape(cfg.piDir))
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create remote dir: %w", err)
	}
	return nil
}

func copyFiles(ctx context.Context, projectDir string, cfg config) error {
	sshHost := formatSSHHost(cfg.piUser, cfg.piHost)
	target := fmt.Sprintf("%s:%s/", sshHost, cfg.piDir)
	cmd := exec.CommandContext(ctx, "scp", "-P", cfg.piPort, cfg.output, cfg.droneConfigFile, target)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy files: %w", err)
	}
	return nil
}

func runRemote(ctx context.Context, projectDir string, cfg config) error {
	sshHost := formatSSHHost(cfg.piUser, cfg.piHost)
	remoteBin := path.Join(cfg.piDir, cfg.piBinName)
	remoteConfig := path.Join(cfg.piDir, filepath.Base(cfg.droneConfigFile))
	remoteCmd := fmt.Sprintf("chmod +x %s && %s --config-file %s", shellEscape(remoteBin), shellEscape(remoteBin), shellEscape(remoteConfig))

	cmd := exec.CommandContext(ctx, "ssh", "-p", cfg.piPort, sshHost, remoteCmd)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("run remote drone: %w", err)
	}
	return nil
}

func formatSSHHost(user, host string) string {
	if user == "" || strings.Contains(host, "@") {
		return host
	}
	return user + "@" + host
}

func shellEscape(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func requireFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	return nil
}

func requireCommand(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("required command %q not found in PATH", name)
	}
	return nil
}

func normalizeArch(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "arm64", "aarch64":
		return "arm64"
	case "arm", "armhf", "armv7":
		return "arm"
	default:
		return ""
	}
}

func defaultCC(arch string) string {
	switch arch {
	case "arm64":
		return "aarch64-linux-gnu-gcc"
	case "arm":
		return "arm-linux-gnueabi-gcc"
	default:
		return ""
	}
}

func envOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
