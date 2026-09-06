//go:build linux

package realtime

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestParseConfigDefaults(t *testing.T) {
	config, err := parseConfig(func(string) string { return "" })
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	want := Config{
		Policy:   PolicyFIFO,
		Priority: 20,
		CPU:      -1,
	}
	if !reflect.DeepEqual(config, want) {
		t.Fatalf("parseConfig() = %#v, want %#v", config, want)
	}
}

func TestParseConfigValidation(t *testing.T) {
	tests := []struct {
		name  string
		value map[string]string
		want  string
	}{
		{
			name:  "invalid enabled flag",
			value: map[string]string{EnvEnable: "sometimes"},
			want:  EnvEnable,
		},
		{
			name:  "invalid policy",
			value: map[string]string{EnvPolicy: "deadline"},
			want:  EnvPolicy,
		},
		{
			name:  "priority too low",
			value: map[string]string{EnvPriority: "0"},
			want:  EnvPriority,
		},
		{
			name:  "priority too high",
			value: map[string]string{EnvPriority: "100"},
			want:  EnvPriority,
		},
		{
			name:  "invalid CPU",
			value: map[string]string{EnvCPU: "-2"},
			want:  EnvCPU,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseConfig(func(name string) string { return test.value[name] })
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseConfig() error = %v, want an error mentioning %s", err, test.want)
			}
		})
	}
}

func TestRunDisabledDoesNotCallSystem(t *testing.T) {
	config := Config{Policy: PolicyFIFO, Priority: 20, CPU: -1}
	runtime := newRuntime(config, nil, systemCalls{
		mlockall: func(int) error {
			t.Fatal("mlockall called while disabled")
			return nil
		},
		schedGetAttr: func(int, uint) (*unix.SchedAttr, error) {
			t.Fatal("sched_getattr called while disabled")
			return nil, nil
		},
	})

	called := false
	if err := runtime.Run(func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !called {
		t.Fatal("Run() did not invoke callback while disabled")
	}
}

func TestRunStrictSetupFailureSkipsCallback(t *testing.T) {
	setupErr := errors.New("missing CAP_SYS_NICE")
	runtime := newRuntime(Config{
		Enabled:  true,
		Strict:   true,
		Policy:   PolicyFIFO,
		Priority: 20,
		CPU:      -1,
	}, nil, systemCalls{
		schedGetAttr: func(int, uint) (*unix.SchedAttr, error) {
			return nil, setupErr
		},
	})

	called := false
	err := runtime.Run(func() error {
		called = true
		return nil
	})
	if !errors.Is(err, setupErr) {
		t.Fatalf("Run() error = %v, want setup error", err)
	}
	if called {
		t.Fatal("Run() invoked callback after strict setup failure")
	}
}

func TestRunBestEffortSetupFailureReportsAndContinues(t *testing.T) {
	setupErr := errors.New("missing CAP_SYS_NICE")
	var reported []error
	runtime := newRuntime(Config{
		Enabled:  true,
		Policy:   PolicyFIFO,
		Priority: 20,
		CPU:      -1,
	}, func(err error) {
		reported = append(reported, err)
	}, systemCalls{
		schedGetAttr: func(int, uint) (*unix.SchedAttr, error) {
			return nil, setupErr
		},
	})

	called := false
	if err := runtime.Run(func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("Run() error = %v, want nil after best-effort setup failure", err)
	}
	if !called {
		t.Fatal("Run() did not invoke callback after best-effort setup failure")
	}
	if len(reported) != 1 || !errors.Is(reported[0], setupErr) {
		t.Fatalf("reported errors = %v, want setup error", reported)
	}
}

func TestRunRestoresSchedulerAndAffinityInReverseSetupOrder(t *testing.T) {
	var calls []string
	originalScheduler := &unix.SchedAttr{}
	originalAffinity := unix.CPUSet{}
	originalAffinity.Set(1)

	runtime := newRuntime(Config{
		Enabled:  true,
		Policy:   PolicyRR,
		Priority: 37,
		CPU:      3,
	}, nil, systemCalls{
		schedGetAttr: func(pid int, flags uint) (*unix.SchedAttr, error) {
			if pid != 0 || flags != 0 {
				t.Fatalf("SchedGetAttr(%d, %d), want (0, 0)", pid, flags)
			}
			calls = append(calls, "get-scheduler")
			return originalScheduler, nil
		},
		schedSetAttr: func(pid int, attr *unix.SchedAttr, flags uint) error {
			if pid != 0 || flags != 0 {
				t.Fatalf("SchedSetAttr(%d, _, %d), want (0, _, 0)", pid, flags)
			}
			switch {
			case attr == originalScheduler:
				calls = append(calls, "restore-scheduler")
			case attr.Policy == unix.SCHED_RR && attr.Priority == 37:
				calls = append(calls, "set-scheduler")
			default:
				t.Fatalf("unexpected scheduler attributes: %#v", attr)
			}
			return nil
		},
		schedGetaffinity: func(pid int, affinity *unix.CPUSet) error {
			if pid != 0 {
				t.Fatalf("SchedGetaffinity(%d, _), want pid 0", pid)
			}
			calls = append(calls, "get-affinity")
			*affinity = originalAffinity
			return nil
		},
		schedSetaffinity: func(pid int, affinity *unix.CPUSet) error {
			if pid != 0 {
				t.Fatalf("SchedSetaffinity(%d, _), want pid 0", pid)
			}
			switch {
			case affinity.IsSet(3) && affinity.Count() == 1:
				calls = append(calls, "set-affinity")
			case affinity.IsSet(1) && affinity.Count() == 1:
				calls = append(calls, "restore-affinity")
			default:
				t.Fatalf("unexpected affinity mask")
			}
			return nil
		},
	})

	if err := runtime.Run(func() error {
		calls = append(calls, "callback")
		return nil
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{
		"get-scheduler",
		"get-affinity",
		"set-affinity",
		"set-scheduler",
		"callback",
		"restore-scheduler",
		"restore-affinity",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("system call order = %v, want %v", calls, want)
	}
}

func TestRunPreservesCallbackAndRestorationErrors(t *testing.T) {
	callbackErr := errors.New("control failed")
	restoreErr := errors.New("cannot restore scheduler")
	setCalls := 0
	runtime := newRuntime(Config{
		Enabled:  true,
		Strict:   true,
		Policy:   PolicyFIFO,
		Priority: 20,
		CPU:      -1,
	}, nil, systemCalls{
		schedGetAttr: func(int, uint) (*unix.SchedAttr, error) {
			return &unix.SchedAttr{}, nil
		},
		schedSetAttr: func(int, *unix.SchedAttr, uint) error {
			setCalls++
			if setCalls == 2 {
				return restoreErr
			}
			return nil
		},
	})

	err := runtime.Run(func() error { return callbackErr })
	if !errors.Is(err, callbackErr) {
		t.Fatalf("Run() error = %v, does not preserve callback error", err)
	}
	if !errors.Is(err, restoreErr) {
		t.Fatalf("Run() error = %v, does not preserve restoration error", err)
	}
}

func TestPrepareMlockallOnce(t *testing.T) {
	calls := 0
	runtime := newRuntime(Config{
		Enabled:  true,
		Policy:   PolicyFIFO,
		Priority: 20,
		CPU:      -1,
		Mlock:    true,
	}, nil, systemCalls{
		mlockall: func(flags int) error {
			calls++
			if flags != unix.MCL_CURRENT|unix.MCL_FUTURE {
				t.Fatalf("Mlockall flags = %d, want current|future", flags)
			}
			return nil
		},
	})

	if err := runtime.Prepare(); err != nil {
		t.Fatalf("first Prepare() error = %v", err)
	}
	if err := runtime.Prepare(); err != nil {
		t.Fatalf("second Prepare() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("Mlockall calls = %d, want 1", calls)
	}
}

func TestDetectKernelRealtimePrefersSysfsAndFallsBackToBootConfig(t *testing.T) {
	status, err := detectKernelRealtime(func(path string) ([]byte, error) {
		if path != "/sys/kernel/realtime" {
			t.Fatalf("readFile(%q), expected sysfs path only", path)
		}
		return []byte("1\n"), nil
	}, func() (string, error) {
		t.Fatal("release lookup called despite sysfs result")
		return "", nil
	})
	if err != nil || !status.Enabled || status.Source != "/sys/kernel/realtime" {
		t.Fatalf("sysfs status = %#v, %v", status, err)
	}

	bootPath := "/boot/config-test-kernel"
	status, err = detectKernelRealtime(func(path string) ([]byte, error) {
		switch path {
		case "/sys/kernel/realtime":
			return nil, errors.New("not present")
		case bootPath:
			return []byte("# CONFIG_PREEMPT_RT is not set\n"), nil
		default:
			t.Fatalf("unexpected path %q", path)
			return nil, nil
		}
	}, func() (string, error) {
		return "test-kernel", nil
	})
	if err != nil || status.Enabled || status.Source != bootPath {
		t.Fatalf("boot config status = %#v, %v", status, err)
	}
}
