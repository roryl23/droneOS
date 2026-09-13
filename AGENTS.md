# droneOS Agents Guide

This repository is a Go codebase for two cooperating runtimes: a base station and a Raspberry Pi drone. The code is map-driven and hardware-oriented, so compile success does not prove that a configured driver or control loop is safe to run on the target device.

## Current Architecture

- Base station entrypoint: `cmd/base/main.go`
  - Reads `configs/config.yaml` via `-config-file`.
  - Starts a TCP server on `0.0.0.0:<base.port>`.
  - Optionally starts an Xbox 360 controller interface and queues controller commands.
  - Optionally starts the configured radio driver and serves protocol requests through `protocol.ServeRadio`.
- Drone entrypoint: `cmd/drone/main.go`
  - Reads the same config file and uses `drone.*` plus `base.host` and `base.port`.
  - Logging is completely disabled unless `drone.enableLogging` is true.
  - Starts WiFi polling, controller polling over WiFi, device-state reporting over WiFi, and radio ping fallback/keepalive.
  - Starts configured sensors, control loops, and output tasks through reflection maps in this file.
- Shared protocol lives in `internal/protocol`.
  - Every message is JSON encoded, prefixed by a 4-byte big-endian length, and capped at 64 KiB.
  - `protocol.Message` JSON field names are currently `Id`, `Cmd`, and `Data`.
  - TCP handling is one request per connection; radio handling loops on a `protocol.RadioLink`.
- Hardware abstractions live under `internal/drivers`.
  - GPIO helpers resolve configured pins and validate/request lines through `go-gpiocdev`.
  - The SX1262 radio driver is Linux-only and implements `protocol.RadioLink`.
  - Most sensor, motor, and control packages are still stubs or partial implementations.

## Repository Map

- `cmd/base/main.go`: base TCP server, controller queue, radio server startup.
- `cmd/drone/main.go`: drone orchestration, plugin maps, WiFi/controller/device-reporting loops.
- `cmd/dev/pi_runner/main.go`: host-side serial helper for listing USB-UART adapters, waiting for a login prompt, opening a console, and executing commands through the Alpine getty.
- `internal/config/config.go`: YAML struct contract for `configs/config.yaml`.
- `internal/protocol/codec.go`: length-prefixed JSON framing helpers.
- `internal/protocol/transport.go`: WiFi, radio, and `AutoTransport` implementations.
- `internal/protocol/main.go`: command handler map for `ping`, `device_state`, `debug_log`, `next_command`, and `controller_ack`.
- `internal/protocol/controller.go`: in-memory base-side controller command queue.
- `internal/protocol/device_state.go`: base-side parsing/logging of drone hardware reports.
- `internal/drone/device_scan.go`: drone-side USB/GPIO/configured-device scanning.
- `internal/drone/device_detect.go`: device-specific detection registry; currently real probes are limited.
- `internal/drivers/gpio/*`: Raspberry Pi 40-pin layout, pin resolution, and GPIO validation.
- `internal/drivers/radio/SX1262/main.go`: LoRa HAT/USB serial radio link.
- `internal/drivers/camera/OV5647/main.go` and `internal/drone/battery.go`: empty placeholders today.
- `build.sh`: static Linux build wrapper for base or drone binaries.
- `build_image.sh`: destructive Alpine Raspberry Pi SD-card image builder; drone images install a validated PREEMPT_RT kernel bundle by default.
- `build_rt_kernel.sh`: builds Alpine's Raspberry Pi kernel with `CONFIG_PREEMPT_RT=y` in an emulated `aarch64` Alpine container and emits a complete diskless-media kernel bundle.
- `run.sh`: runs prebuilt `build/droneOS/base.bin` and `build/droneOS/drone.bin` together.
- `sync_pi.sh`: rsync source tree to a development Pi over SSH.
- `pi_runner.sh`: thin wrapper around `go run ./cmd/dev/pi_runner`; use it for UART console bring-up and scripted serial command execution.
- `.buildenv`: legacy build flags, not sourced by the current shell scripts.
- `configs/.config`: legacy generated Linux 6.6 configuration retained as reference; the Alpine RT builder derives its configuration from the matching `linux-rpi` aport instead.

## Config Contracts

- Optional role environment files are local and may contain secrets:
  - `build_image.sh` loads project-root `.image.env` as shell syntax before resolving its environment-backed defaults. In addition to image, credential, WiFi, UART, architecture, and cache settings listed by `bash build_image.sh --help`, it supports `ENABLE_REALTIME_KERNEL`, `RT_KERNEL_BUNDLE`, `RT_KERNEL_CACHE_DIR`, `RT_APORTS_REF`, `RT_KERNEL_FORCE_REBUILD`, `RT_BUILD_CONTAINER_IMAGE`, and the `DRONEOS_RT_*` service settings. Assignments from that file govern image variables and are exported to child commands; positional hostname, development-credential, and WiFi arguments still win.
  - The base and drone processes optionally load `.base.env` and `.drone.env`, respectively, from their working directory before flag parsing. A missing file is allowed; a malformed present file must stop startup with a clear error.
  - Both use `DRONEOS_CONFIG_FILE` as the default for `--config-file`; an explicit flag wins. Drone also accepts `DRONEOS_DISABLE_GC` plus `DRONEOS_RT_ENABLE`, `DRONEOS_RT_STRICT`, `DRONEOS_RT_POLICY`, `DRONEOS_RT_PRIORITY`, `DRONEOS_RT_CPU`, and `DRONEOS_RT_MLOCK`.
  - Realtime scheduling applies only to configured control-algorithm goroutines. Each enabled worker locks to its OS thread before applying optional affinity and `SCHED_FIFO`/`SCHED_RR`; memory locking is process-wide and opt-in.
  - Go dotenv loading must preserve values already set in the process environment.
- Default config: `configs/config.yaml`.
- `base.host` and `base.port` are the address the drone uses for WiFi commands and device reports.
- `base.controller` is looked up in `internal/controller/funcmap.go`; current value is usually `xbox360`.
- `base.radio` and `drone.radio` map to `config.Radio`:
  - `name`: registered radio driver, currently `SX1262` or empty/`none`.
  - `alwaysUse`: drone radio pings continue even when WiFi is up if true.
  - `usbId`: empty (with `usbScan` false) selects GPIO UART mode; `auto`/`scan`/`usb` variants scan USB serial devices; a path uses that char device.
  - `usbScan`: true also triggers USB serial scan.
  - `uartDevice`: GPIO-UART device only. Empty keeps the `/dev/ttyS0` compatibility default; USB selection through `usbId` or `usbScan` ignores it. The current Waveshare drone bench config explicitly uses `/dev/ttyAMA0`.
  - `pins`: optional pin metadata for GPIO preflight logging/validation.
- `drone.alwaysUseRadio` exists in the struct but current runtime code reads `drone.radio.alwaysUse`.
- `drone.gpioLayout` supports `rpi-40` aliases. Pin schemes are `bcm`, `physical`, or `chip`.
- `config.Pin` also supports `direction`, `activeLow`, `bias`, and `drive`; see `internal/drivers/gpio/validate.go`.
- `config.Device.config` is a free-form map used by probes, for example `serialDevice`, `i2cBus`, and `i2cAddress`.

## Reflection And Plugin Contracts

`utils.CallFunctionByName` prepends `context.Context` to the arguments provided by the caller and invokes the selected function with `reflect.Call`. A wrong signature often still compiles and then panics only when the config path is exercised.

- Register new radio drivers in both `cmd/base/main.go` and `cmd/drone/main.go` if both runtimes need them.
- Radio driver signature expected by both entrypoints:
  - `func(context.Context, *config.Radio) (protocol.RadioLink, error)`
- Drone sensor call site currently passes:
  - `context.Context`
  - `*config.Device`
  - `*[]chan sensor.Event`
- Current sensor stubs are not fully consistent with that call site. Reconcile signatures and add a runtime test before enabling a sensor in config.
- Control loop signature expected by `cmd/drone/main.go`:
  - `func(context.Context, *config.Config, int, *control.PriorityMutex, *chan sensor.Event, *chan drone.Task)`
- Output task dispatch currently passes:
  - `context.Context`
  - `*config.Config`
  - `*chan drone.Task`
- Current motor stubs expect `*config.Device`, so output dispatch needs signature cleanup or tests before output tasks are enabled.
- Protocol command handlers registered in `internal/protocol/main.go` should accept `(context.Context, protocol.Message)` and return `protocol.Message`.
- Controller interfaces registered in `internal/controller/funcmap.go` should match `func(context.Context, *chan controller.Event[any]) error`.

## Runtime Behavior To Preserve

- Use `context.Context` for shutdown in all long-running loops. Several current stubs sleep forever; new code should select on `ctx.Done()` where practical.
- Use zerolog (`github.com/rs/zerolog/log` or `zerolog.Ctx(ctx)`).
- Be careful with log volume. Radio receive errors and device reports already throttle or use debug logs to avoid flooding.
- Do not assume `AutoTransport` is the active main path. It exists in `internal/protocol/transport.go`, but current base/drone flows mostly use `WiFiTransport`, `RadioTransport`, and `ServeRadio` directly.
- WiFi status is determined by a `ping` request to the base. Controller polling and device-state reports only use WiFi in the current code.
- `protocol.ServeRadio` decodes a framed request, dispatches through `protocol.FuncMap`, then sends a framed response on the same radio link.
- `StartDeviceReporter` scans USB sysfs, GPIO `LineInfo` metadata, configured sensors, and configured outputs every 10 seconds, but only sends reports when WiFi is connected. GPIO inventory must never request unowned lines to sample values: closing such requests can reset firmware-configured alternate functions and break UART/WiFi.

## Hardware Notes

- `internal/drivers/radio/SX1262` has a `//go:build linux` tag.
- GPIO mode keeps `/dev/ttyS0` as its compatibility default and uses GPIO BCM 22/27 for M0/M1. On the Waveshare bench, `dtoverlay=disable-bt` makes `/dev/ttyAMA0` the Pi primary UART, so the explicit `radio.uartDevice` must select `/dev/ttyAMA0`.
- SX1262 USB mode scans `/dev/serial/by-id`, `/dev/ttyUSB*`, and `/dev/ttyACM*`, then chooses the first sorted valid device; `uartDevice` has no effect in USB mode.
- GPIO-mode LoRa startup selects transparent normal mode without persisting radio parameters; physical modules must already have matching settings and compatible mode/interface jumpers.
- GPIO detection and validation can fail on non-Pi hosts due missing `/dev/gpiochip*`; keep hardware-specific checks optional or isolated in tests.
- Device detection is partial:
  - MPU-6050 probes I2C `WHO_AM_I`.
  - GT-U7 checks configured `serialDevice` presence only.
  - Other configured sensors/outputs mostly report GPIO/config inspection, not true device detection.

## Build And Deployment

- Local runs:
  - `go run ./cmd/base/main.go --config-file ./configs/config.yaml`
  - `go run ./cmd/drone/main.go --config-file ./configs/config.yaml`
- Static binary builds:
  - `bash build.sh drone arm64`
  - `bash build.sh base amd64`
  - Output defaults to `build/droneOS/<type>.bin`.
  - `build.sh` does not run `go mod tidy`; dependency changes should be explicit.
- `run.sh` assumes both binaries already exist under `build/droneOS/`.
- `build_image.sh` repartitions and formats the target block device. Do not run it casually during verification.
- `build_image.sh` loads optional project-root `.image.env` before resolving image defaults. The file uses shell syntax and can contain secrets; keep the ignored local file private and start from `.image.env.example`.
- Production image builds copy the selected optional runtime role file—`.base.env` for base images or `.drone.env` for drone images—into `/opt/droneOS` with restrictive permissions. The selected file remains optional.
- `sync_pi.sh` transfers `.drone.env` when present for development source sync but excludes `.image.env` and `.base.env` to avoid copying unrelated secrets. The supported Pi flow cross-builds the drone on the workstation and deploys it through the serial runner's upload mode; source synchronization remains optional when SSH is available, but development images do not preload Go.
- Alpine image modes:
  - `BUILD_MODE=prod` builds and enables the `droneOS` OpenRC service, disables WiFi by default, and does not add network configuration.
  - `BUILD_MODE=dev` skips embedding the app binary, disables the `droneOS` service, enables WiFi/SSH, and preloads SSH/rsync/network tools including Alpine's OpenRC service package for `sshd` and `doas` for the wheel-group development user. WiFi startup disables `wlan0` power saving for stable SSH sessions. Its credentials are for trusted networks only.
- Dev image package fetch supports host `apk`, then Docker, then Podman via `APK_FETCH_CONTAINER_IMAGE`; do not add Go because the Pi root tmpfs cannot reliably install it, and do not add packages such as `linux-firmware-brcm` or `wireless-regdb` that write `/lib/firmware` because diskless Alpine supplies that read-only tree from the modloop. The stock Alpine modloop or complete RT kernel bundle must carry Pi Zero 2 W `brcmfmac` firmware and regulatory data.
- `ENABLE_UART_CONSOLE` defaults to `0` for production images and `1` for development images. When enabled in either mode, `UART_CONSOLE_TTY`, every space-separated `UART_CONSOLE_EXTRA_TTYS` value, and `UART_CONSOLE_BAUD` must be valid.
- Dev image UART-console defaults are `UART_CONSOLE_TTY=ttyAMA0`, `UART_CONSOLE_EXTRA_TTYS=""`, and `UART_CONSOLE_BAUD=115200`. With the default `dtoverlay=disable-bt`, `/dev/ttyAMA0` is the Pi primary UART; do not add extra console/getty TTYs for this bench.
- Waveshare HAT selector topology is fixed: **A** connects workstation host USB to the LoRa module (base radio); **B** connects the Pi primary UART to the LoRa module (drone run); **C** connects the host CP2102 USB to the Pi primary UART (console and provisioning). Put both selector caps on the same selected row and power the hardware off before moving either cap. C and B share `/dev/ttyAMA0`, so a serial console cannot remain connected during a radio run.
- Stage the persistent drone binary, explicit `/dev/ttyAMA0` GPIO-radio config, log destination, and a boot-time service/start path while selector C is fitted; on Alpine development images, include the staged files in LBU and commit before rebooting. Then power off, select B with both caps, boot, and use a base on selector A. For recovery or further serial provisioning, power off and restore both caps to C before connecting the Pi console.
- `UART_CONSOLE_EXTRA_TTYS` is a space-separated explicit opt-in list. It appends extra `console=<tty>,<baud>` tokens and adds extra serial gettys in the dev overlay; it must remain empty on the shared-UART Waveshare bench.
- `write_boot_config` must preserve multiple `console=` tokens; use token-level appending for UART console entries.
- `configs/config-kernel8.txt` is reference boot config, but `build_image.sh` writes boot settings into the extracted Alpine boot partition.
- `ENABLE_REALTIME_KERNEL` defaults to `1` for `kernel8 drone` images and `0` for base images. RT builds currently support only `aarch64`; the builder validates `CONFIG_PREEMPT_RT=y` and installs kernel, initramfs, modloop/modules, DTBs, and overlays as one bundle.
- `build_rt_kernel.sh` must keep using the Alpine release's `linux-rpi` aport and `update-kernel`; do not restore the historical Raspberry Pi OS kernel copier or external 6.6 RT patch flow. Linux 6.12 and newer contain PREEMPT_RT in mainline.
- For dev Pis, use `sync_pi.sh <host> [remote-dir]`; override with `DRONEOS_PI_USER`, `DRONEOS_PI_PORT`, `DRONEOS_PI_DIR`, and `DRONEOS_RSYNC_DELETE`.
- `pi_runner.sh` supports `list`, `console`, `loopback`, `wait`, `exec`, and `upload`. Automatic discovery sorts `/dev/serial/by-id` before `/dev/ttyUSB*` and `/dev/ttyACM*`, resolves and deduplicates aliases by device target, and keeps the stable by-id name as the canonical candidate. One canonical candidate is selected automatically; distinct devices require `--serial` or `DRONEOS_SERIAL_DEVICE`. On the two-adapter bench, both CP2102 devices report serial `0001`, so by-id is ambiguous: while both caps are on C, explicitly use the verified `/dev/ttyUSB0` Pi-console adapter; selector A's base radio remains at its configured `/dev/serial/by-path` path. Recheck after reconnecting hardware.
- Interactive `pi_runner.sh console` places terminal stdin in raw mode, transparently forwards terminal replies such as ANSI cursor-position reports, restores the workstation terminal on every exit path, and keeps `Ctrl-C` as the local exit command. Piped and other non-TTY input remains unchanged.
- `pi_runner.sh loopback` writes a marker and waits for it to echo back; use it with the adapter TX and RX pins temporarily shorted to prove host-side serial input/output before debugging Pi wiring.
- `pi_runner.sh wait` defaults to 115200 baud and sends carriage returns every two seconds to rediscover a getty prompt; override with `DRONEOS_SERIAL_BAUD`, `DRONEOS_SERIAL_POKE_INTERVAL`, and `DRONEOS_SERIAL_TIMEOUT`.
- `pi_runner.sh exec` logs in through the serial getty with `DRONEOS_PI_USER` and `DRONEOS_PI_PASSWORD`, then runs a shell command. It proves serial input/output and is the automation hook for agent-driven Pi checks when SSH is unavailable.
- `pi_runner.sh upload` logs in with the same credentials and requires local and remote paths from `--file <local-binary>` plus `--remote <remote-path>` or `DRONEOS_UPLOAD_FILE` plus `DRONEOS_UPLOAD_REMOTE`; explicit path flags are rejected in every other mode and may precede or follow `upload`. It streams a base64 payload to a safely quoted remote temporary file with terminal echo disabled, verifies local and remote SHA-256 values, atomically installs and chmods the destination to `0755` only after a match, restores echo on every reachable exit path, and removes failed temporary files. Use a longer shared `--timeout` or `DRONEOS_SERIAL_TIMEOUT` for C-selector transfers and never send through selector A's base-radio adapter.

## Verification

Run focused checks for the files you touch. Useful commands:

- `gofmt -w <go files>`
- `go test ./...`
- `go test ./cmd/dev/pi_runner`
- `go build ./cmd/base ./cmd/drone ./cmd/dev/pi_runner`
- `bash -n build.sh build_rt_kernel.sh setup.sh sync_pi.sh pi_runner.sh build_image.sh`
- `shellcheck build.sh build_rt_kernel.sh setup.sh sync_pi.sh pi_runner.sh build_image.sh` if `shellcheck` is installed
- `git diff --check`

Local Go tests cover WiFi transport behavior, realtime environment validation/setup sequencing, and the UART runner's transient-EOF handling, raw-console cursor-position forwarding and interrupt behavior, mode-specific flag rejection, short-write reporting, and canonical by-id alias selection. They do not validate measured PREEMPT_RT latency, configured hardware/control plugins, real GPIO, LoRa hardware, OpenRC behavior, Alpine SD-card boot, or controller hardware. Kernel-image verification must additionally inspect the generated bundle and boot the target Pi.

## Contribution Guidelines

- Keep changes small and tied to the runtime layer that owns the behavior.
- Prefer adding drivers/controls in new package directories, then registering them in the exact entrypoint maps that call them.
- Agents MUST update `README.md` in the same change, without waiting for a separate request or confirmation, whenever user-facing commands, setup, configuration/environment behavior, runtime contracts, architecture, supported hardware or status, dependencies, build/deploy behavior, safety caveats, or verification workflows change. Internal-only refactors MUST trigger a README accuracy check but MUST NOT cause churn when its user-facing claims remain correct.
- Avoid hiding durable behavior in ad hoc shell flags when it belongs in `configs/config.yaml` or a documented image-builder environment variable.
- Preserve existing generated/build outputs in `build/` as ignored artifacts; do not commit binaries or SD-card images.
- Do not rewrite unrelated IDE or editor metadata while doing code work.
