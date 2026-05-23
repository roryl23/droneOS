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
	verbose         bool
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
	piUser := flag.String("pi-user", envOr("DRONEOS_PI_USER", "root"), "Raspberry Pi SSH user (root required for /opt)")
	piPort := flag.String("pi-port", envOr("DRONEOS_PI_PORT", "22"), "Raspberry Pi SSH port")
	piDir := flag.String("pi-dir", envOr("DRONEOS_PI_DIR", "/opt/droneOS"), "Remote deploy directory")
	piBinName := flag.String("pi-bin-name", envOr("DRONEOS_PI_BIN", "drone.bin"), "Remote drone binary name")
	output := flag.String("output", envOr("DRONEOS_PI_OUT", filepath.Join("build", "droneOS", "drone.bin")), "Local output path for drone binary")
	goCmd := flag.String("go-cmd", envOr("DRONEOS_GO_CMD", "go"), "Go command to use")
	verbose := flag.Bool("verbose", false, "Enable verbose output for all commands")

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
		verbose:         *verbose,
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
	// Allow non-root users with sudo access
	needsSudo := requiresRoot(cfg.piDir) && strings.TrimSpace(cfg.piUser) != "root"

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
	if err := ensureRemoteDir(ctx, projectDir, cfg, needsSudo); err != nil {
		return err
	}
	if err := stopRemote(ctx, projectDir, cfg); err != nil {
		return err
	}
	if err := copyFiles(ctx, projectDir, cfg, needsSudo); err != nil {
		return err
	}

	if err := runRemote(ctx, projectDir, cfg, needsSudo); err != nil {
		return err
	}

	// Keep base station running in foreground until interrupted
	fmt.Println("\n✓ Base station running in foreground (Ctrl+C to stop)")
	select {
	case <-ctx.Done():
		fmt.Println("\n✓ Shutting down base station...")
		return nil
	case err := <-baseCmd.done:
		return fmt.Errorf("base station exited unexpectedly: %w", err)
	}
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

	if cfg.verbose {
		fmt.Printf("Building drone binary: %s build -o %s ./cmd/drone/main.go\n", cfg.goCmd, cfg.output)
		fmt.Printf("Environment: CGO_ENABLED=1 GOOS=linux GOARCH=%s\n", cfg.arch)
	}

	cmd := exec.CommandContext(ctx, cfg.goCmd, "build", "-o", cfg.output, "./cmd/drone/main.go")
	cmd.Dir = projectDir
	cmd.Env = env
	if cfg.verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build drone: %w", err)
	}

	return nil
}

func ensureRemoteDir(ctx context.Context, projectDir string, cfg config, needsSudo bool) error {
	sshHost := formatSSHHost(cfg.piUser, cfg.piHost)
	remoteCmd := "mkdir -pv " + shellEscape(cfg.piDir)
	if needsSudo {
		remoteCmd = "sudo " + remoteCmd
	}
	if cfg.verbose {
		fmt.Printf("Creating remote directory: ssh -v %s '%s'\n", sshHost, remoteCmd)
	}

	sshArgs := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=/tmp/ssh-%r@%h:%p",
		"-o", "ControlPersist=300",
		"-p", cfg.piPort,
	}
	if cfg.verbose {
		sshArgs = append(sshArgs, "-v")
	}
	sshArgs = append(sshArgs, sshHost, remoteCmd)

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	cmd.Dir = projectDir
	if cfg.verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create remote dir: %w", err)
	}
	return nil
}

func copyFiles(ctx context.Context, projectDir string, cfg config, needsSudo bool) error {
	sshHost := formatSSHHost(cfg.piUser, cfg.piHost)
	remoteBin := path.Join(cfg.piDir, cfg.piBinName)
	remoteConfig := path.Join(cfg.piDir, filepath.Base(cfg.droneConfigFile))

	// Build base SCP args
	scpArgs := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=/tmp/ssh-%r@%h:%p",
		"-o", "ControlPersist=300",
		"-P", cfg.piPort,
	}
	if cfg.verbose {
		scpArgs = append(scpArgs, "-v")
	}

	// Build base SSH args
	sshArgs := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=/tmp/ssh-%r@%h:%p",
		"-o", "ControlPersist=300",
		"-p", cfg.piPort,
	}
	if cfg.verbose {
		sshArgs = append(sshArgs, "-v")
	}

	// If using sudo, copy to /tmp first, then move with sudo
	if needsSudo {
		tmpBin := "/tmp/" + cfg.piBinName
		tmpConfig := "/tmp/" + filepath.Base(cfg.droneConfigFile)

		if cfg.verbose {
			fmt.Printf("Copying binary to temp: scp -v %s %s:%s\n", cfg.output, sshHost, tmpBin)
		}
		cmdArgs := append(scpArgs, cfg.output, fmt.Sprintf("%s:%s", sshHost, tmpBin))
		cmd := exec.CommandContext(ctx, "scp", cmdArgs...)
		cmd.Dir = projectDir
		if cfg.verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("copy drone binary to tmp: %w", err)
		}

		if cfg.verbose {
			fmt.Printf("Copying config to temp: scp -v %s %s:%s\n", cfg.droneConfigFile, sshHost, tmpConfig)
		}
		cmdArgs = append(scpArgs, cfg.droneConfigFile, fmt.Sprintf("%s:%s", sshHost, tmpConfig))
		cmd = exec.CommandContext(ctx, "scp", cmdArgs...)
		cmd.Dir = projectDir
		if cfg.verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("copy drone config to tmp: %w", err)
		}

		// Move files from /tmp to target directory with sudo
		moveCmd := fmt.Sprintf("sudo mv -v %s %s && sudo mv -v %s %s",
			shellEscape(tmpBin), shellEscape(remoteBin),
			shellEscape(tmpConfig), shellEscape(remoteConfig))
		if cfg.verbose {
			fmt.Printf("Moving files to target: ssh -v %s '%s'\n", sshHost, moveCmd)
		}
		cmdArgs = append(sshArgs, sshHost, moveCmd)
		cmd = exec.CommandContext(ctx, "ssh", cmdArgs...)
		cmd.Dir = projectDir
		if cfg.verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("move files to target dir: %w", err)
		}
	} else {
		if cfg.verbose {
			fmt.Printf("Copying binary: scp -v %s %s:%s\n", cfg.output, sshHost, remoteBin)
		}
		cmdArgs := append(scpArgs, cfg.output, fmt.Sprintf("%s:%s", sshHost, remoteBin))
		cmd := exec.CommandContext(ctx, "scp", cmdArgs...)
		cmd.Dir = projectDir
		if cfg.verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("copy drone binary: %w", err)
		}

		if cfg.verbose {
			fmt.Printf("Copying config: scp -v %s %s:%s\n", cfg.droneConfigFile, sshHost, remoteConfig)
		}
		cmdArgs = append(scpArgs, cfg.droneConfigFile, fmt.Sprintf("%s:%s", sshHost, remoteConfig))
		cmd = exec.CommandContext(ctx, "scp", cmdArgs...)
		cmd.Dir = projectDir
		if cfg.verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("copy drone config: %w", err)
		}
	}
	return nil
}

func stopRemote(ctx context.Context, projectDir string, cfg config) error {
	sshHost := formatSSHHost(cfg.piUser, cfg.piHost)

	// Use sudo only if not logged in as root
	sudoPrefix := "sudo "
	if strings.TrimSpace(cfg.piUser) == "root" {
		sudoPrefix = ""
	}

	remoteCmd := fmt.Sprintf("%ssystemctl stop droneOS.service || true", sudoPrefix)
	if cfg.verbose {
		fmt.Printf("Stopping remote service: ssh -v %s '%s'\n", sshHost, remoteCmd)
	}

	sshArgs := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=/tmp/ssh-%r@%h:%p",
		"-o", "ControlPersist=300",
		"-p", cfg.piPort,
	}
	if cfg.verbose {
		sshArgs = append(sshArgs, "-v")
	}
	sshArgs = append(sshArgs, sshHost, remoteCmd)

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	cmd.Dir = projectDir
	if cfg.verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: stop remote drone failed: %v\n", err)
		return nil
	}
	return nil
}

func runRemote(ctx context.Context, projectDir string, cfg config, needsSudo bool) error {
	sshHost := formatSSHHost(cfg.piUser, cfg.piHost)
	remoteBin := path.Join(cfg.piDir, cfg.piBinName)

	// Use sudo only if not logged in as root
	sudoPrefix := "sudo "
	if strings.TrimSpace(cfg.piUser) == "root" {
		sudoPrefix = ""
	}

	// Build the complete deployment command sequence
	var remoteCmd string
	if needsSudo {
		// Non-root user managing /opt directory
		remoteCmd = fmt.Sprintf(
			"%ssystemctl stop droneOS.service && "+
				"sudo chmod +x %s && "+
				"sudo chown root:root %s && "+
				"%ssystemctl start droneOS.service && "+
				"echo 'droneOS service restarted successfully'",
			sudoPrefix,
			shellEscape(remoteBin),
			shellEscape(remoteBin),
			sudoPrefix,
		)
	} else {
		// Root user - no sudo needed
		remoteCmd = fmt.Sprintf(
			"%ssystemctl stop droneOS.service && "+
				"chmod +x %s && "+
				"%ssystemctl start droneOS.service && "+
				"echo 'droneOS service restarted successfully'",
			sudoPrefix,
			shellEscape(remoteBin),
			sudoPrefix,
		)
	}

	if cfg.verbose {
		fmt.Printf("Deploying and restarting service: ssh -v %s '%s'\n", sshHost, remoteCmd)
	}

	sshArgs := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=/tmp/ssh-%r@%h:%p",
		"-o", "ControlPersist=300",
		"-p", cfg.piPort,
	}
	if cfg.verbose {
		sshArgs = append(sshArgs, "-v")
	}
	sshArgs = append(sshArgs, sshHost, remoteCmd)

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("deploy and restart remote drone service: %w", err)
	}

	// Give the service a moment to start
	time.Sleep(500 * time.Millisecond)

	// Verify service started successfully
	checkStatusCmd := fmt.Sprintf("%ssystemctl is-active droneOS.service", sudoPrefix)
	if cfg.verbose {
		fmt.Printf("Checking service status: ssh -v %s '%s'\n", sshHost, checkStatusCmd)
	}

	checkArgs := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=/tmp/ssh-%r@%h:%p",
		"-o", "ControlPersist=300",
		"-p", cfg.piPort,
	}
	if cfg.verbose {
		checkArgs = append(checkArgs, "-v")
	}
	checkArgs = append(checkArgs, sshHost, checkStatusCmd)

	checkCmd := exec.CommandContext(ctx, "ssh", checkArgs...)
	checkCmd.Dir = projectDir
	output, err := checkCmd.Output()
	if err != nil || string(output) != "active\n" {
		fmt.Println("\n⚠ Warning: droneOS service may not have started correctly")
		fmt.Printf("Check status with: ssh -p %s %s 'sudo systemctl status droneOS.service'\n", cfg.piPort, sshHost)
	} else {
		fmt.Println("\n✓ Binary deployed and droneOS service is running")
	}

	fmt.Printf("To view logs: ssh -p %s %s 'sudo journalctl -u droneOS.service -f'\n", cfg.piPort, sshHost)
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

func requiresRoot(remoteDir string) bool {
	dir := path.Clean(strings.TrimSpace(remoteDir))
	return dir == "/opt" || strings.HasPrefix(dir, "/opt/")
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
