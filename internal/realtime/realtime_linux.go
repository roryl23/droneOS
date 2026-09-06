//go:build linux

// Package realtime configures explicitly opted-in Linux real-time execution.
package realtime

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	EnvEnable   = "DRONEOS_RT_ENABLE"
	EnvStrict   = "DRONEOS_RT_STRICT"
	EnvPolicy   = "DRONEOS_RT_POLICY"
	EnvPriority = "DRONEOS_RT_PRIORITY"
	EnvCPU      = "DRONEOS_RT_CPU"
	EnvMlock    = "DRONEOS_RT_MLOCK"
)

// Policy selects the Linux real-time scheduler policy.
type Policy string

const (
	PolicyFIFO Policy = "fifo"
	PolicyRR   Policy = "rr"
)

// Config is the real-time execution configuration read from the environment.
type Config struct {
	Enabled  bool
	Strict   bool
	Policy   Policy
	Priority int
	CPU      int
	Mlock    bool
}

// Report receives non-strict real-time setup and restoration failures.
type Report func(error)

// KernelStatus describes the result of a PREEMPT_RT kernel check.
type KernelStatus struct {
	Enabled bool
	Source  string
}

// LoadConfig reads and validates the DRONEOS_RT_* environment variables.
func LoadConfig() (Config, error) {
	return parseConfig(os.Getenv)
}

func parseConfig(getenv func(string) string) (Config, error) {
	config := Config{
		Policy:   PolicyFIFO,
		Priority: 20,
		CPU:      -1,
	}

	var err error
	if config.Enabled, err = parseBool(getenv, EnvEnable, false); err != nil {
		return Config{}, err
	}
	if config.Strict, err = parseBool(getenv, EnvStrict, false); err != nil {
		return Config{}, err
	}
	if config.Mlock, err = parseBool(getenv, EnvMlock, false); err != nil {
		return Config{}, err
	}

	if value := strings.TrimSpace(getenv(EnvPolicy)); value != "" {
		config.Policy = Policy(strings.ToLower(value))
	}
	if value := strings.TrimSpace(getenv(EnvPriority)); value != "" {
		config.Priority, err = strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be an integer from 1 through 99: %w", EnvPriority, err)
		}
	}
	if value := strings.TrimSpace(getenv(EnvCPU)); value != "" {
		config.CPU, err = strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be -1 or a non-negative CPU number: %w", EnvCPU, err)
		}
	}

	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func parseBool(getenv func(string) string, name string, defaultValue bool) (bool, error) {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	return parsed, nil
}

// Validate confirms that Config can be applied by the Linux scheduling APIs.
func (config Config) Validate() error {
	switch config.Policy {
	case PolicyFIFO, PolicyRR:
	default:
		return fmt.Errorf("%s must be %q or %q, got %q", EnvPolicy, PolicyFIFO, PolicyRR, config.Policy)
	}
	if config.Priority < 1 || config.Priority > 99 {
		return fmt.Errorf("%s must be from 1 through 99, got %d", EnvPriority, config.Priority)
	}
	if config.CPU < -1 {
		return fmt.Errorf("%s must be -1 or a non-negative CPU number, got %d", EnvCPU, config.CPU)
	}
	return nil
}

// Runtime prepares process-level memory locking and runs callbacks on configured RT threads.
type Runtime struct {
	config Config
	report Report
	system systemCalls

	prepareOnce sync.Once
	prepareErr  error
}

type systemCalls struct {
	mlockall         func(int) error
	schedGetAttr     func(int, uint) (*unix.SchedAttr, error)
	schedSetAttr     func(int, *unix.SchedAttr, uint) error
	schedGetaffinity func(int, *unix.CPUSet) error
	schedSetaffinity func(int, *unix.CPUSet) error
}

var linuxSystemCalls = systemCalls{
	mlockall:         unix.Mlockall,
	schedGetAttr:     unix.SchedGetAttr,
	schedSetAttr:     unix.SchedSetAttr,
	schedGetaffinity: unix.SchedGetaffinity,
	schedSetaffinity: unix.SchedSetaffinity,
}

// New creates a Runtime for a validated real-time configuration.
func New(config Config, report Report) (*Runtime, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return newRuntime(config, report, linuxSystemCalls), nil
}

func newRuntime(config Config, report Report, system systemCalls) *Runtime {
	return &Runtime{
		config: config,
		report: report,
		system: system,
	}
}

// Prepare applies process-wide preparation once. Memory is locked only when RT is enabled and Mlock is selected.
func (r *Runtime) Prepare() error {
	if !r.config.Enabled || !r.config.Mlock {
		return nil
	}

	r.prepareOnce.Do(func() {
		if err := r.system.mlockall(unix.MCL_CURRENT | unix.MCL_FUTURE); err != nil {
			r.prepareErr = fmt.Errorf("mlockall: %w", err)
			if !r.config.Strict {
				r.reportError(r.prepareErr)
				r.prepareErr = nil
			}
		}
	})
	return r.prepareErr
}

// Run invokes callback on a pinned OS thread with the configured RT scheduling settings.
func (r *Runtime) Run(callback func() error) error {
	if callback == nil {
		return errors.New("realtime callback is nil")
	}
	if !r.config.Enabled {
		return callback()
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	state, setupErr := r.setupThread()
	if setupErr != nil {
		restoreErr := state.restore(r.system)
		if r.config.Strict {
			return errors.Join(setupErr, restoreErr)
		}
		r.reportError(setupErr)
		if restoreErr != nil {
			r.reportError(restoreErr)
		}
		return errors.Join(callback(), restoreErr)
	}

	callbackErr := callback()
	restoreErr := state.restore(r.system)
	if restoreErr != nil && !r.config.Strict {
		r.reportError(restoreErr)
	}
	return errors.Join(callbackErr, restoreErr)
}

type threadState struct {
	scheduler        *unix.SchedAttr
	affinity         unix.CPUSet
	restoreScheduler bool
	restoreAffinity  bool
}

func (r *Runtime) setupThread() (threadState, error) {
	var state threadState

	originalScheduler, err := r.system.schedGetAttr(0, 0)
	if err != nil {
		return state, fmt.Errorf("read scheduler: %w", err)
	}
	state.scheduler = originalScheduler

	if r.config.CPU >= 0 {
		if err := r.system.schedGetaffinity(0, &state.affinity); err != nil {
			return state, fmt.Errorf("read CPU affinity: %w", err)
		}

		var affinity unix.CPUSet
		affinity.Set(r.config.CPU)
		if err := r.system.schedSetaffinity(0, &affinity); err != nil {
			return state, fmt.Errorf("set CPU affinity to %d: %w", r.config.CPU, err)
		}
		state.restoreAffinity = true
	}

	if err := r.system.schedSetAttr(0, &unix.SchedAttr{
		Policy:   uint32(r.schedulerPolicy()),
		Priority: uint32(r.config.Priority),
	}, 0); err != nil {
		return state, fmt.Errorf("set %s scheduler with priority %d: %w", r.config.Policy, r.config.Priority, err)
	}
	state.restoreScheduler = true
	return state, nil
}

func (r *Runtime) schedulerPolicy() int {
	if r.config.Policy == PolicyRR {
		return unix.SCHED_RR
	}
	return unix.SCHED_FIFO
}

func (state threadState) restore(system systemCalls) error {
	var restoreErr error
	if state.restoreScheduler {
		if err := system.schedSetAttr(0, state.scheduler, 0); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("restore scheduler: %w", err))
		}
	}
	if state.restoreAffinity {
		if err := system.schedSetaffinity(0, &state.affinity); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("restore CPU affinity: %w", err))
		}
	}
	return restoreErr
}

func (r *Runtime) reportError(err error) {
	if r.report != nil {
		r.report(err)
	}
}

// DetectKernelRealtime checks the live kernel's PREEMPT_RT status. A false result with a non-nil error is unknown, not non-RT.
func DetectKernelRealtime() (KernelStatus, error) {
	return detectKernelRealtime(os.ReadFile, kernelRelease)
}

func detectKernelRealtime(readFile func(string) ([]byte, error), release func() (string, error)) (KernelStatus, error) {
	const sysfsPath = "/sys/kernel/realtime"
	if content, err := readFile(sysfsPath); err == nil {
		enabled, parseErr := parseKernelRealtimeValue(content)
		if parseErr == nil {
			return KernelStatus{Enabled: enabled, Source: sysfsPath}, nil
		}
	}

	kernelRelease, releaseErr := release()
	if releaseErr != nil {
		return KernelStatus{}, fmt.Errorf("determine kernel release: %w", releaseErr)
	}
	configPath := "/boot/config-" + kernelRelease
	content, err := readFile(configPath)
	if err != nil {
		return KernelStatus{}, fmt.Errorf("read %s: %w", configPath, err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == "CONFIG_PREEMPT_RT=y" {
			return KernelStatus{Enabled: true, Source: configPath}, nil
		}
	}
	return KernelStatus{Enabled: false, Source: configPath}, nil
}

func parseKernelRealtimeValue(content []byte) (bool, error) {
	value := strings.TrimSpace(string(content))
	switch value {
	case "1", "y", "Y", "true", "TRUE", "True":
		return true, nil
	case "0", "n", "N", "false", "FALSE", "False":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected /sys/kernel/realtime value %q", value)
	}
}

func kernelRelease() (string, error) {
	var name unix.Utsname
	if err := unix.Uname(&name); err != nil {
		return "", err
	}

	release := make([]byte, 0, len(name.Release))
	for _, character := range name.Release {
		if character == 0 {
			break
		}
		release = append(release, byte(character))
	}
	if len(release) == 0 {
		return "", errors.New("empty kernel release")
	}
	return string(release), nil
}
