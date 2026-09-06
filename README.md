# droneOS

A Go prototype for coordinating a base station and Raspberry Pi drone over framed TCP and an experimental SX1262 radio link.

> **Status:** unstable bring-up software, not a flight-ready system. Autonomous flight, reliable sensor events, motor/ESC actuation, and unattended operation are not implemented. Do not use it for flight or safety-critical control.

## Current Architecture And Hardware Status

- **Base runtime** (`cmd/base/main.go`) optionally loads `.base.env`, then uses `configs/config.yaml` by default or a path from `DRONEOS_CONFIG_FILE` or `--config-file`. It listens on `0.0.0.0:<base.port>`, optionally queues Xbox 360 controller input, and optionally serves framed protocol requests over the configured radio link.
- **Drone runtime** (`cmd/drone/main.go`) optionally loads `.drone.env`, then uses the same default/overridable config path. It polls the base over WiFi, polls controller commands and sends device-state reports over WiFi, and can send a radio `ping` keepalive every five seconds. Device reports are collected every 10 seconds but sent only while WiFi is connected. Configured control algorithms can run on pinned `SCHED_FIFO`/`SCHED_RR` threads when the realtime environment is enabled. `AutoTransport` is not the entrypoint's automatic failover path.
- **Protocol** messages are JSON fields `Id`, `Cmd`, and `Data`, prefixed by a 4-byte big-endian payload length and limited to 64 KiB. TCP handles one request per connection; radio receives, dispatches, and replies in a loop. Current commands are `ping`, `device_state`, `debug_log`, `next_command`, and `controller_ack`.
- **SX1262** is the only registered radio driver and has a Linux build tag. It supports GPIO/UART and USB serial modes, but GPIO-mode radio configuration uses placeholder register values and USB mode depends on correct physical jumpers. It is not a verified generic LoRa implementation.
- **Hardware support is partial.** MPU-6050 has an I2C identity probe and GT-U7 checks only for a configured serial device. Other sensor/output detection is configuration/GPIO inspection. Sensor, motor, and control packages are mostly stubs or have reflection signature mismatches when enabled. Camera and battery packages are empty; PiSugar installation is rejected by the Alpine image builder.

## Target Hardware (Not A Support Matrix)

### Drone

- Raspberry Pi with GPIO; the image flow targets Alpine Raspberry Pi media.
- An SX1262-compatible radio in the supported GPIO/UART or USB serial arrangement.
- Intended experimental peripherals: MPU-6050, HC-SR04, GT-U7, Frienda IR sensor, motors, and ESCs. Their presence here does **not** mean they are operational.

### Base Station

- Linux host for the included, radio-enabled base runtime. `build.sh` emits Linux binaries, and the bundled SX1262 driver is Linux-only.
- Matching radio hardware when exercising the experimental radio path.
- Optional Xbox 360 controller input. It queues commands at the base; it does not provide working manual flight.

## Quick Start

### 1. Install Prerequisites

Go **1.23 or newer** is required (`go.mod` declares `go 1.23.0`). Verify the selected toolchain before building:

```bash
go version
```

`setup.sh` is for Alpine hosts only. It requires `apk` and, when run as a non-root user, `sudo`; it installs build, image, and development-Pi synchronization tools and adds the current user to available hardware groups. Start a new login session before relying on new group membership.

```bash
bash setup.sh
```

On Ubuntu, install equivalent tools plus Docker or Podman. Realtime-kernel builds require a container engine capable of running an `aarch64` Alpine container through binfmt/QEMU. Ensure `golang-go` provides Go 1.23 or newer; otherwise install a newer Go toolchain.

```bash
sudo apt install ca-certificates dosfstools parted util-linux golang-go wget openssl openssh-client rsync tar docker.io
```

### 2. Build And Run Binaries

```bash
# Cross-compile the drone for 64-bit Raspberry Pi Alpine.
bash build.sh drone arm64

# Cross-compile the base for a Linux amd64 host.
bash build.sh base amd64
```

To run both binaries locally, build both for the same Linux host architecture first:

```bash
bash build.sh base amd64
bash build.sh drone amd64
bash run.sh
```

`configs/config.yaml` currently sets `base.host` to `192.168.0.68`. Before `run.sh`, set it to an address reachable from the local drone process—`127.0.0.1` when both processes run on one host—or use a configuration appropriate for the network.

`build.sh` writes `build/droneOS/<type>.bin`, defaults to `CGO_ENABLED=0`, and always targets Linux. `run.sh` requires both prebuilt binaries and starts both with `configs/config.yaml`.

### 3. Create Alpine SD-Card Media

> **Destructive:** `build_image.sh` wipes the selected **whole block device**, repartitions it, and reformats its first partition. Verify the removable SD card with `lsblk`, unmount all of its partitions, back up needed data, and never select a system disk.

```bash
lsblk -o NAME,SIZE,MODEL,MOUNTPOINTS
# After confirming that /dev/sdX is the removable target:
bash build_image.sh /dev/sdX kernel8 drone droneos
```

Parameters:

- `/dev/sdX` is the selected whole SD-card device (the script also accepts `sdX`).
- `kernel8` selects the 64-bit Raspberry Pi Alpine image.
- `drone` selects `cmd/drone/main.go`; use `base` for `cmd/base/main.go`.
- `droneos` is the image hostname. The builder writes the diskless Alpine overlay as `droneos.apkovl.tar.gz` at the boot-partition root and keeps its filename aligned with the hostname by convention.

The builder downloads or reuses an Alpine Raspberry Pi tarball, creates FAT boot media, and writes the overlay plus boot settings. Drone images build and install an Alpine-native `CONFIG_PREEMPT_RT=y` kernel by default; base images retain Alpine's stock kernel by default. Production images cross-compile and embed the selected runtime, `configs/config.yaml`, and an optional selected role file.

> **Known Alpine overlay limitation:** `create_openrc_overlay` does not create `etc/.default_boot_services`. Standard boot services and hostname initialization may therefore not run, and console prompts can show `(none)`. This is an unfixed limitation; treat generated media as bring-up artifacts rather than dependable boot environments.

## Image Modes And Safety

### Production (Default)

```bash
bash build_image.sh /dev/sdX kernel8 drone droneos
```

Before this command, ensure `.image.env` is absent or sets `BUILD_MODE=prod`: file assignments override inherited and inline environment values when the builder sources it.

### Development

Keep development credentials in the ignored local `.image.env`, not on the command line:

```bash
cp .image.env.example .image.env
# Edit .image.env: set BUILD_MODE=dev and unique DEV_USER_PASSWORD,
# WIFI_SSID, WIFI_PASSWORD, and WIFI_COUNTRY values.
bash build_image.sh /dev/sdX kernel8 drone
```

`.image.env` is Bash syntax and is sourced before image defaults. Assignments in that file override inherited and inline environment values, so set the intended `BUILD_MODE` and other image overrides there—or remove conflicting assignments—before invoking the builder. Positional hostname/credential/WiFi arguments are applied afterward and retain precedence. The legacy eight-argument development form remains accepted, but avoid it for secrets: shell history and process listings can retain command-line passwords. Never use the builder's predictable development defaults. `.image.env` and the generated overlay contain credentials; keep them private and use development SSH/WiFi only on trusted networks. Generated SSH configuration permits password and public-key authentication but disables root login. The builder uses an existing host public key or generates `~/.ssh/id_ed25519` if none is found.

Development mode skips compiling and embedding the application, leaves the `droneOS` service disabled, and expects source synchronization and a local build on the Pi. A drone image is a WiFi client using `wpa_supplicant`; a base image creates a `wlan0` access point at `10.42.0.1` on channel 6 and serves `10.42.0.50`–`10.42.0.150` through `dnsmasq`.

For each development image, the builder deletes stale APK files from `build/dev-apks/<branch>/<arch>/<type>` and refetches the package closure: Go/SSH/rsync/network packages, plus `hostapd` and `dnsmasq` for base images or `wpa_supplicant` for drone images. It tries host `apk`, then Docker, then Podman; set `APK_FETCH_CONTAINER_IMAGE` for the container fallback. A failed fetch aborts the image build. The cache is copied to `droneos-apks/<arch>` on the boot media; `droneos-dev-setup` tries an offline install from mounted `/media`/`/mnt` paths (or `/droneos-apks`) before a network install. If neither works, development setup cannot complete.

### Realtime Kernel And Runtime

For `kernel8 drone` images, `ENABLE_REALTIME_KERNEL` defaults to `1`. `build_rt_kernel.sh` runs the matching Alpine `aarch64` userspace through Docker or Podman, rebuilds Alpine's `linux-rpi` package with `CONFIG_PREEMPT_RT=y`, and uses Alpine's `update-kernel` to generate one matching kernel, initramfs, modloop, module, firmware, and device-tree set. Without `--output`, it hashes the aports ref into the shared `build/rt-kernel` cache filename, so standalone and image builds address the same validated bundle. Set `RT_KERNEL_FORCE_REBUILD=1` for an image build, or pass `--force` directly to the kernel builder, to rebuild it.

Pin `RT_APORTS_REF` to an Alpine aports commit for reproducible production media. Its default is the stable branch matching the selected Alpine tarball, such as `3.24-stable`. The image builder rejects bundles missing the kernel, initramfs, modloop, Pi device trees, overlays, provenance, or a versioned configuration containing `CONFIG_PREEMPT_RT=y`. Use `--output` or `RT_KERNEL_BUNDLE` only for deliberately exported external bundles.

```bash
# Build the automatically named shared-cache bundle without writing an SD card.
bash build_rt_kernel.sh \
  --alpine-release 3.24.1 \
  --arch aarch64 \
  --aports-ref 3.24-stable

# Resume packaging after a post-build failure without recompiling the kernel.
bash build_rt_kernel.sh \
  --alpine-release 3.24.1 \
  --arch aarch64 \
  --aports-ref 3.24-stable \
  --resume-work build/rt-kernel/.rt-kernel-build.EXAMPLE

# Build media; repeated invocations reuse the validated shared-cache bundle.
bash build_image.sh /dev/sdX kernel8 drone droneos
```

The generated OpenRC service enables strict realtime scheduling for drone images that install the RT kernel. Startup verifies PREEMPT_RT and proves the requested scheduler/affinity settings before launching runtime loops. Each configured control algorithm then locks its goroutine to one OS thread and applies `SCHED_FIFO` priority 20 by default. Override `DRONEOS_RT_POLICY`, `DRONEOS_RT_PRIORITY`, or `DRONEOS_RT_CPU` in `.image.env`; `-1` leaves CPU affinity unchanged. `DRONEOS_RT_MLOCK=1` additionally requests process-wide `MCL_CURRENT|MCL_FUTURE` and should only be enabled with a sufficient memory-lock limit. Strict mode exits instead of launching controls when kernel verification, memory locking, or scheduler preflight fails.

Local/source-sync runs remain non-RT unless `.drone.env` sets `DRONEOS_RT_ENABLE=1`. `DRONEOS_RT_STRICT=0` permits startup when permissions or kernel support are unavailable and logs the setup failure; use that only for development. A PREEMPT_RT kernel and FIFO policy reduce scheduling latency but do not make blocking I/O, Go allocation/GC, drivers, or application logic hard real-time.

After boot, verify the installed kernel rather than trusting the bundle name:

```bash
uname -a
grep '^CONFIG_PREEMPT_RT=y$' /boot/config-*
```

The kernel banner must identify `PREEMPT_RT`, and exactly one installed kernel configuration must enable it. Measure worst-case latency under representative WiFi, radio, storage, and device load before relying on realtime behavior.

### UART Console

`ENABLE_UART_CONSOLE` defaults to `0` in production and `1` in development. When enabled, `UART_CONSOLE_TTY`, every space-separated `UART_CONSOLE_EXTRA_TTYS` value, and `UART_CONSOLE_BAUD` are validated.

Enabling UART adds kernel `console=<tty>,<baud>` output in either mode. Production has no droneOS-specific getty setup, so serial login availability is not guaranteed. Development's `droneos-dev-setup` explicitly adds gettys only for configured `/dev/<tty>` character devices; its defaults request `ttyAMA0` plus `ttyS0` at 115200 baud. A wrong or absent device, or the known overlay boot limitation above, can leave no login prompt. Set `UART_CONSOLE_EXTRA_TTYS=` only when another UART device must own that extra TTY.

Useful non-secret overrides:

```bash
ALPINE_BRANCH=v3.20 ALPINE_VERSION=3.20.3 bash build_image.sh /dev/sdX kernel8 drone droneos
ALPINE_TARBALL=/tmp/alpine-rpi.tar.gz bash build_image.sh /dev/sdX kernel8 drone droneos
ALPINE_ARCH=aarch64 bash build_image.sh /dev/sdX kernel8 drone droneos
APK_FETCH_CONTAINER_IMAGE=alpine:3.23 BUILD_MODE=dev bash build_image.sh /dev/sdX kernel8 drone
ENABLE_UART_CONSOLE=1 UART_CONSOLE_TTY=ttyAMA0 UART_CONSOLE_EXTRA_TTYS= UART_CONSOLE_BAUD=115200 \
  bash build_image.sh /dev/sdX kernel8 drone droneos
```

I2C and SPI are enabled in the Raspberry Pi boot configuration. The former Debian PiSugar installer is not part of this Alpine flow.

## Development, Configuration, And Checks

### Runtime Environment Files

The optional `.image.env`, `.base.env`, and `.drone.env` files are ignored by Git because they may contain secrets. Start from the tracked non-secret examples:

```bash
cp .image.env.example .image.env
cp .base.env.example .base.env
cp .drone.env.example .drone.env
```

`build_image.sh` loads project-root `.image.env` before resolving image defaults; see `.image.env.example` and `bash build_image.sh --help` for supported image settings. The base and drone runtimes load `.base.env` and `.drone.env`, respectively, from their working directory before flag parsing. `DRONEOS_CONFIG_FILE` supplies the default for `--config-file`, but an explicit `--config-file` wins. Existing process environment values take precedence over the role file. A missing role file is optional; a present malformed file stops startup. Drone also accepts `DRONEOS_DISABLE_GC` and the validated `DRONEOS_RT_ENABLE`, `DRONEOS_RT_STRICT`, `DRONEOS_RT_POLICY`, `DRONEOS_RT_PRIORITY`, `DRONEOS_RT_CPU`, and `DRONEOS_RT_MLOCK` settings described above.

Production images copy the selected optional role file to `/opt/droneOS` with restrictive permissions. Development sync intentionally transfers `.drone.env` when present, but excludes `.image.env` and `.base.env`; copy the correct example on the target when another role needs local settings.

### Configuration And Local Runs

Both processes default to `configs/config.yaml`:

```bash
go run ./cmd/base/main.go --config-file ./configs/config.yaml
go run ./cmd/drone/main.go --config-file ./configs/config.yaml
```

To use an edited configuration, pass that file to **both** processes:

```bash
cp configs/config.yaml configs/my_drone.yaml
go run ./cmd/base/main.go --config-file ./configs/my_drone.yaml
go run ./cmd/drone/main.go --config-file ./configs/my_drone.yaml
```

`base.host` and `base.port` identify the base used by the drone's WiFi paths. `base` and `drone` each select a radio; `drone.radio.alwaysUse`, not the unused `drone.alwaysUseRadio` field, controls whether the drone keeps pinging by radio while WiFi is up. Devices carry named pins and free-form settings such as I2C address/bus or serial path. GPIO layouts support Raspberry Pi 40-pin aliases and BCM, physical, or chip addressing. Enabling sensors, outputs, or control priorities is experimental and can expose the current reflection signature mismatches.

Only production images embed the repository's `configs/config.yaml`; development images use the synchronized source configuration. To bake a custom production file, replace that source file before building the image or change the builder; `--config-file` affects local/runtime invocation only.

### Software Checks

```bash
go test ./...
go test ./cmd/dev/pi_runner
go build ./cmd/base ./cmd/drone ./cmd/dev/pi_runner
bash -n build.sh build_rt_kernel.sh setup.sh sync_pi.sh pi_runner.sh build_image.sh
```

These checks cover protocol WiFi behavior, realtime runtime configuration/setup sequencing, and serial-runner behavior. They do not measure realtime latency or validate real GPIO, SX1262/radio hardware, configured sensor/motor/control plugins, controller hardware, OpenRC boot, or Alpine SD-card media. Validate a kernel bundle separately and boot it on the target Pi before treating the image path as proven.

## Sync And Serial Bring-Up

Synchronize a working tree to a development Pi over SSH:

```bash
bash sync_pi.sh 'pi-host.example'
```

Pass an optional remote-directory path as the second argument.

The default destination is `/home/admin/droneOS`; override the user, port, and destination with `DRONEOS_PI_USER`, `DRONEOS_PI_PORT`, and `DRONEOS_PI_DIR`. By default `DRONEOS_RSYNC_DELETE=1`, so rsync uses `--delete` and removes remote project files absent locally; set it to `0` or `false` to preserve them. SSH uses `StrictHostKeyChecking=accept-new`; verify a newly accepted host key through a trusted channel.

Use `pi_runner.sh` for host-side USB-UART discovery, console bring-up, and serial command execution:

Set the adapter path once, then run the desired mode:

```bash
SERIAL_DEVICE='/dev/serial/by-id/usb-uart-adapter'

# List canonical candidates before automatic selection.
bash pi_runner.sh list

# Open an interactive serial terminal (Ctrl-C exits locally).
bash pi_runner.sh --serial "$SERIAL_DEVICE" console

# With adapter TX and RX temporarily shorted, prove host-side serial I/O.
bash pi_runner.sh --serial "$SERIAL_DEVICE" loopback

# Send carriage returns until the Alpine login prompt is observed.
bash pi_runner.sh --serial "$SERIAL_DEVICE" wait

# Avoid exposing a password in shell history while using serial exec.
read -r -s -p 'Pi password: ' DRONEOS_PI_PASSWORD; echo
# `admin` is the default; replace it when DEV_USER_NAME differs.
export DRONEOS_PI_USER=admin DRONEOS_PI_PASSWORD
bash pi_runner.sh --serial "$SERIAL_DEVICE" exec 'uname -a'
unset DRONEOS_PI_PASSWORD
```

The modes are `list`, `console`, `loopback`, `wait`, and `exec`. `list` prefers stable, sorted `/dev/serial/by-id` paths before `/dev/ttyUSB*` and `/dev/ttyACM*`; aliases for one device collapse to the stable path. Automatic mode selects one canonical candidate and requires `--serial` or `DRONEOS_SERIAL_DEVICE` when distinct devices remain. `loopback` requires a temporary TX/RX short and proves adapter I/O before Pi-wiring work. `wait` sends a carriage return every two seconds by default to rediscover a prompt that appeared before the listener attached.

Interactive `console` sets terminal stdin to raw mode: keystrokes and ANSI replies reach the Pi immediately without local echo or line buffering. It restores the workstation terminal on exit, keeps `Ctrl-C` as the local exit command, and leaves piped/non-TTY input unchanged. Use `--baud` or `DRONEOS_SERIAL_BAUD` for speed; `wait` accepts `--poke-interval`, `--timeout`, and `--wait-marker`; `exec` accepts `--user`, `--password`, and `--verbose`.

For a production image that has been given network access separately, or through an attached console, the enabled service writes logs to `/var/log/droneOS.log` and `/var/log/droneOS.err`:

```bash
rc-service droneOS status
rc-service droneOS restart
tail -f /var/log/droneOS.log
tail -f /var/log/droneOS.err
```

Development images provide SSH setup but leave that service disabled; build and run the synchronized source explicitly.

## Project Layout And Scripts

- `cmd/base/main.go`: base TCP server, controller queue, and optional radio server.
- `cmd/drone/main.go`: drone orchestration, WiFi loops, radio keepalive, device reporting, and reflection-based plugin dispatch.
- `cmd/dev/pi_runner`: serial-console helper used by `pi_runner.sh`.
- `configs/config.yaml`: shared runtime configuration embedded by the image builder.
- `internal/protocol/`: framing, transports, command handlers, and controller queue.
- `internal/drivers/`: GPIO, radio, sensor, and motor packages; many non-radio packages are partial.
- `internal/drone/control/`: placeholder control loops.
- `setup.sh`: Alpine host dependency installer.
- `build.sh`: static Linux base/drone binary builder.
- `build_rt_kernel.sh`: Alpine `linux-rpi` PREEMPT_RT package and validated boot-bundle builder.
- `run.sh`: local launcher for both prebuilt binaries.
- `build_image.sh`: destructive Alpine Raspberry Pi SD-card image builder.
- `sync_pi.sh`: SSH/rsync development-tree synchronization.
- `pi_runner.sh`: wrapper around the Go serial helper.

Radio drivers used by both runtimes must be registered in both entrypoint maps and implement `protocol.RadioLink`. Other plugins are reflection-dispatched from `cmd/drone/main.go`; validate their exact call signatures before enabling them.

## Logging

Use zerolog (`github.com/rs/zerolog/log`): `Info` and `Error` are human-facing, and `Debug` is structured diagnostic output. Base logging is configured from `base.logLevel`; the sample configuration disables all drone logging unless `drone.enableLogging` is set to `true`.

## Notes

- `build.sh` does not run `go mod tidy`; make dependency changes explicitly.
- Higher-level Alpine/OpenRC remote deployment is not ported; use source sync, the serial runner, or direct console access for bring-up.
