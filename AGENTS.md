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
- `build_image.sh`: destructive Alpine Raspberry Pi SD-card image builder.
- `run.sh`: runs prebuilt `build/droneOS/base.bin` and `build/droneOS/drone.bin` together.
- `sync_pi.sh`: rsync source tree to a development Pi over SSH.
- `pi_runner.sh`: thin wrapper around `go run ./cmd/dev/pi_runner`; use it for UART console bring-up and scripted serial command execution.
- `.buildenv`: legacy build flags, not sourced by the current shell scripts.
- `configs/.config`: generated Linux kernel config; current Alpine image flow does not build a kernel from it.

## Config Contracts

- Optional role environment files are local and may contain secrets:
  - `build_image.sh` loads project-root `.image.env` as shell syntax before resolving its environment-backed defaults. It supports `BUILD_MODE`, `IMAGE_HOSTNAME`, `DEV_USER_NAME`, `DEV_USER_PASSWORD`, `WIFI_SSID`, `WIFI_PASSWORD`, `WIFI_COUNTRY`, `DEV_PROJECT_DIR`, `BUILD_DIR`, `ALPINE_MIRROR`, `ALPINE_BRANCH`, `ALPINE_ARCH`, `ALPINE_VERSION`, `ALPINE_TARBALL`, `ALPINE_TARBALL_URL`, `ALPINE_CACHE_DIR`, `APK_FETCH_CONTAINER_IMAGE`, `DISABLE_WIFI`, `DISABLE_BLUETOOTH`, `ENABLE_UART_CONSOLE`, `UART_CONSOLE_TTY`, `UART_CONSOLE_EXTRA_TTYS`, `UART_CONSOLE_BAUD`, `SKIP_KERNEL_BUILD`, `INSTALL_PISUGAR`, `GOARM`, and `MOUNT_BASE`. Assignments from that file govern image variables and are exported to child commands; positional hostname, development-credential, and WiFi arguments still win.
  - The base and drone processes optionally load `.base.env` and `.drone.env`, respectively, from their working directory before flag parsing. A missing file is allowed; a malformed present file must stop startup with a clear error.
  - Both use `DRONEOS_CONFIG_FILE` as the default for `--config-file`; an explicit flag wins. Drone also accepts `DRONEOS_DISABLE_GC=1` or `true`.
  - Go dotenv loading must preserve values already set in the process environment.
- Default config: `configs/config.yaml`.
- `base.host` and `base.port` are the address the drone uses for WiFi commands and device reports.
- `base.controller` is looked up in `internal/controller/funcmap.go`; current value is usually `xbox360`.
- `base.radio` and `drone.radio` map to `config.Radio`:
  - `name`: registered radio driver, currently `SX1262` or empty/`none`.
  - `alwaysUse`: drone radio pings continue even when WiFi is up if true.
  - `usbId`: empty means GPIO UART mode; `auto`/`scan`/`usb` variants scan USB serial devices; a path uses that char device.
  - `usbScan`: with empty `usbId`, true also triggers USB serial scan.
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
- `StartDeviceReporter` scans USB sysfs, all GPIO chips, configured sensors, and configured outputs every 10 seconds, but only sends reports when WiFi is connected.

## Hardware Notes

- `internal/drivers/radio/SX1262` has a `//go:build linux` tag.
- SX1262 GPIO mode uses `/dev/ttyS0` by default and GPIO BCM 22/27 for M0/M1.
- SX1262 USB mode scans `/dev/serial/by-id`, `/dev/ttyUSB*`, and `/dev/ttyACM*`, then chooses the first sorted valid device.
- The LoRa driver currently sends a placeholder configuration command in GPIO mode; consult the actual module register map before changing radio parameters.
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
- `sync_pi.sh` transfers `.drone.env` when present for development source sync but excludes `.image.env` and `.base.env` to avoid copying unrelated secrets.
- Alpine image modes:
  - `BUILD_MODE=prod` builds and enables the `droneOS` OpenRC service, disables WiFi by default, and does not add network configuration.
  - `BUILD_MODE=dev` skips embedding the app binary, disables the `droneOS` service, enables WiFi/SSH, preloads dev APKs, and expects source sync/build on the Pi. Its credentials are for trusted networks only.
- Dev image package fetch supports host `apk`, then Docker, then Podman via `APK_FETCH_CONTAINER_IMAGE`.
- `ENABLE_UART_CONSOLE` defaults to `0` for production images and `1` for development images. When enabled in either mode, `UART_CONSOLE_TTY`, every space-separated `UART_CONSOLE_EXTRA_TTYS` value, and `UART_CONSOLE_BAUD` must be valid.
- Dev image UART console defaults are `UART_CONSOLE_TTY=ttyAMA0`, `UART_CONSOLE_EXTRA_TTYS=ttyS0`, and `UART_CONSOLE_BAUD=115200`.
- `UART_CONSOLE_EXTRA_TTYS` is a space-separated fallback list. It appends extra `console=<tty>,<baud>` tokens and adds extra serial gettys in the dev overlay; set it to an empty value only when another UART device must own that TTY during development.
- `write_boot_config` must preserve multiple `console=` tokens; use token-level appending for UART console entries.
- `configs/config-kernel8.txt` is reference boot config, but `build_image.sh` writes boot settings into the extracted Alpine boot partition.
- `SKIP_KERNEL_BUILD` must remain `1`; custom Raspberry Pi kernel builds are not part of the current Alpine diskless image flow.
- For dev Pis, use `sync_pi.sh <host> [remote-dir]`; override with `DRONEOS_PI_USER`, `DRONEOS_PI_PORT`, `DRONEOS_PI_DIR`, and `DRONEOS_RSYNC_DELETE`.
- `pi_runner.sh` supports `list`, `console`, `loopback`, `wait`, and `exec`. Automatic discovery sorts `/dev/serial/by-id` before `/dev/ttyUSB*` and `/dev/ttyACM*`, resolves and deduplicates aliases by device target, and keeps the stable by-id name as the canonical candidate. One canonical candidate is selected automatically; distinct devices require `--serial` or `DRONEOS_SERIAL_DEVICE`.
- Interactive `pi_runner.sh console` places terminal stdin in raw mode, transparently forwards terminal replies such as ANSI cursor-position reports, restores the workstation terminal on every exit path, and keeps `Ctrl-C` as the local exit command. Piped and other non-TTY input remains unchanged.
- `pi_runner.sh loopback` writes a marker and waits for it to echo back; use it with the adapter TX and RX pins temporarily shorted to prove host-side serial input/output before debugging Pi wiring.
- `pi_runner.sh wait` defaults to 115200 baud and sends carriage returns every two seconds to rediscover a getty prompt; override with `DRONEOS_SERIAL_BAUD`, `DRONEOS_SERIAL_POKE_INTERVAL`, and `DRONEOS_SERIAL_TIMEOUT`.
- `pi_runner.sh exec` logs in through the serial getty with `DRONEOS_PI_USER` and `DRONEOS_PI_PASSWORD`, then runs a shell command. It proves serial input/output and is the automation hook for agent-driven Pi checks when SSH is unavailable.

## Verification

Run focused checks for the files you touch. Useful commands:

- `gofmt -w <go files>`
- `go test ./...`
- `go test ./cmd/dev/pi_runner`
- `go build ./cmd/base ./cmd/drone ./cmd/dev/pi_runner`
- `bash -n build.sh setup.sh sync_pi.sh pi_runner.sh build_image.sh`
- `shellcheck build.sh setup.sh sync_pi.sh pi_runner.sh build_image.sh` if `shellcheck` is installed
- `git diff --check`

Local Go tests cover WiFi transport behavior and the UART runner's transient-EOF handling, raw-console cursor-position forwarding and interrupt behavior, mode-specific flag rejection, short-write reporting, and canonical by-id alias selection. `TestSerialCandidatesPreferStableByIDAlias` verifies that sorted by-id aliases resolving to one device collapse to the first stable alias and that distinct devices require explicit selection. They do not validate configured sensor/motor reflection signatures, real GPIO, LoRa hardware, OpenRC behavior, Alpine boot media, or controller hardware.

## Contribution Guidelines

- Keep changes small and tied to the runtime layer that owns the behavior.
- Prefer adding drivers/controls in new package directories, then registering them in the exact entrypoint maps that call them.
- Agents MUST update `README.md` in the same change, without waiting for a separate request or confirmation, whenever user-facing commands, setup, configuration/environment behavior, runtime contracts, architecture, supported hardware or status, dependencies, build/deploy behavior, safety caveats, or verification workflows change. Internal-only refactors MUST trigger a README accuracy check but MUST NOT cause churn when its user-facing claims remain correct.
- Avoid hiding durable behavior in ad hoc shell flags when it belongs in `configs/config.yaml` or a documented image-builder environment variable.
- Preserve existing generated/build outputs in `build/` as ignored artifacts; do not commit binaries or SD-card images.
- Do not rewrite unrelated IDE or editor metadata while doing code work.
