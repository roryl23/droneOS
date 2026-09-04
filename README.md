# droneOS

A Go runtime for autonomous drone flight with pluggable drivers, radio transport, and control algorithms.

> Status: this project is actively under development and unstable. Use at your own risk.

## Hardware Requirements

### Drone

- Raspberry Pi Zero 2 W or another Raspberry Pi with GPIO
- LoRa radio module, such as an SX1262-based USB module
- Optional sensors:
  - MPU-6050 IMU
  - HC-SR04 ultrasonic distance sensor
  - GT-U7 GPS module
  - Frienda IR obstacle sensor
- Motors and ESCs for propulsion
- PiSugar3 power manager, optional for battery monitoring

### Base Station

- Linux/macOS/Windows PC
- LoRa radio module matching the drone frequency
- Xbox 360 controller, optional for manual piloting

## Quick Start

### 1. Install Build Dependencies

Run setup on an Alpine Linux host:

```bash
bash setup.sh
```

This installs the static-Go build, Alpine image, and development-Pi sync requirements through `apk`, including `parted` for `partprobe` plus `lsblk`, `sfdisk`, and `util-linux-misc` for the image builder.

On Ubuntu, install equivalent tools (including `dosfstools`, `parted`, `util-linux`, Go, OpenSSH client/key tools, OpenSSL, rsync, tar, and wget) plus either `apk-tools`, Docker, or Podman. Development images use Alpine `apk fetch` to preload WiFi/SSH packages onto the SD card; if host `apk` is not available, `build_image.sh` runs the fetch inside an Alpine container.

```bash
sudo apt install ca-certificates dosfstools parted util-linux golang-go wget openssl openssh-client rsync tar docker.io
```

### 2. Build an Application Binary

```bash
# Drone for Raspberry Pi 64-bit Alpine
bash build.sh drone arm64

# Base station for the current Linux amd64 host
bash build.sh base amd64
```

Build output is written to `build/droneOS/<type>.bin`. The default build is static (`CGO_ENABLED=0`) so the Raspberry Pi binary does not depend on Ubuntu or glibc runtime libraries.

### 3. Build an Alpine SD Card

Find the SD card device first. The image builder repartitions the target device.

```bash
lsblk
sudo umount /dev/sdb1 2>/dev/null || true
bash build_image.sh sdb kernel8 drone droneos
```

Parameters:

- `sdb` is the SD card device, with or without `/dev/`.
- `kernel8` selects the 64-bit Raspberry Pi Alpine image.
- `drone` selects `cmd/drone/main.go`; use `base` for `cmd/base/main.go`.
- `droneos` is the Alpine hostname and local backup overlay name.

The image builder downloads an Alpine Raspberry Pi tarball, formats the SD card as Alpine boot media, and packages droneOS configuration into an Alpine local backup overlay. Production images also cross-compile and embed the selected droneOS binary.

## Image Options

Production mode is the default and starts droneOS on boot. It disables WiFi with the Raspberry Pi `disable-wifi` overlay and supplies no network configuration or setup by default:

```bash
bash build_image.sh sdb kernel8 drone droneos
```

Set `DISABLE_WIFI=0` only to leave the WiFi hardware available; production still does not configure a network.

Development mode builds the same Alpine media but skips compiling or embedding the application. It leaves the `droneOS` OpenRC service disabled and configures WiFi plus SSH access:

```bash
BUILD_MODE=dev bash build_image.sh sdb kernel8 drone droneos admin password MySSID MyPass123 US
```

The development arguments are hostname, SSH username, SSH password, WiFi SSID, WiFi password, and WiFi country. The WiFi password must contain 8 to 63 characters because development images emit WPA passphrase configuration. Shell-quote values containing spaces or shell metacharacters so their argument positions are preserved.

The legacy 8-argument form is still accepted if you do not need a custom hostname:

```bash
BUILD_MODE=dev bash build_image.sh sdb kernel8 drone admin password MySSID MyPass123 US
```

For drone images, the Pi joins the configured WiFi network using `wpa_supplicant`. For base images, the Pi creates an access point at `10.42.0.1` using `hostapd` and `dnsmasq`. The image adds your host public SSH key to the dev user, generating `~/.ssh/id_ed25519` if no key exists, and installs `rsync` plus Go for source-based development. Development SSH passwords and WiFi access are intended only for trusted networks.

Sync the working tree to a development Pi over WiFi:

```bash
bash sync_pi.sh 192.168.1.42
```

By default this syncs to `/home/admin/droneOS`. Override the user, port, or destination with `DRONEOS_PI_USER`, `DRONEOS_PI_PORT`, and `DRONEOS_PI_DIR`.

Use the serial runner for UART console bring-up and command execution:

```bash
# List the canonical candidates selected from connected adapters.
bash pi_runner.sh list

# Open an interactive serial terminal (Ctrl-C exits).
bash pi_runner.sh --serial /dev/serial/by-id/<adapter> console

# With adapter TX and RX temporarily shorted, verify host-side serial I/O.
bash pi_runner.sh --serial /dev/serial/by-id/<adapter> loopback

# Send carriage returns until the Alpine login prompt is observed.
bash pi_runner.sh --serial /dev/serial/by-id/<adapter> wait

# Log in through the serial getty and run a command.
DRONEOS_PI_USER=admin DRONEOS_PI_PASSWORD=password \
  bash pi_runner.sh --serial /dev/serial/by-id/<adapter> exec 'uname -a'
```

The five modes are `list`, `console`, `loopback`, `wait`, and `exec`. `list` shows stable, sorted `/dev/serial/by-id` names before `/dev/ttyUSB*` and `/dev/ttyACM*`; aliases resolving to the same device are collapsed so automatic selection uses the stable by-id name. Automatic mode selects one canonical candidate and requires `--serial` or `DRONEOS_SERIAL_DEVICE` when distinct devices remain. `loopback` requires the USB-UART adapter TX and RX pins to be temporarily shorted together; it proves host-side serial input/output before debugging Pi wiring. `wait` sends a carriage return every two seconds by default, so it can rediscover an Alpine `login:` prompt that printed before the listener attached. Dev images expose `ttyAMA0` and `ttyS0` serial consoles by default so Raspberry Pi UART alias differences do not require a manual SD-card patch.

Use `--baud` or `DRONEOS_SERIAL_BAUD` to set serial speed. `wait` also accepts `--poke-interval`, `--timeout`, and `--wait-marker` (or `DRONEOS_SERIAL_POKE_INTERVAL`, `DRONEOS_SERIAL_TIMEOUT`, and `DRONEOS_SERIAL_WAIT_MARKER`); `exec` accepts `--user`, `--password`, and `--verbose` (or `DRONEOS_PI_USER` and `DRONEOS_PI_PASSWORD`).

Useful image overrides:

```bash
ALPINE_BRANCH=v3.20 ALPINE_VERSION=3.20.3 bash build_image.sh sdb kernel8 drone droneos
ALPINE_TARBALL=/tmp/alpine-rpi.tar.gz bash build_image.sh sdb kernel8 drone droneos
ALPINE_ARCH=aarch64 bash build_image.sh sdb kernel8 drone droneos
APK_FETCH_CONTAINER_IMAGE=alpine:3.23 BUILD_MODE=dev bash build_image.sh sdb kernel8 drone droneos admin password MySSID MyPass123 US
ENABLE_UART_CONSOLE=1 UART_CONSOLE_TTY=ttyAMA0 UART_CONSOLE_EXTRA_TTYS= UART_CONSOLE_BAUD=115200 \
  bash build_image.sh sdb kernel8 drone droneos
```

`ENABLE_UART_CONSOLE` defaults to `0` in production and `1` in development. When it is enabled, `UART_CONSOLE_TTY`, every space-separated `UART_CONSOLE_EXTRA_TTYS` value, and `UART_CONSOLE_BAUD` are validated in either mode. Set `UART_CONSOLE_EXTRA_TTYS=` only when you need to suppress the dev-image `ttyS0` fallback, for example when another UART device must own that TTY during development.

The previous Debian package based PiSugar installer is not part of the Alpine image flow. I2C and SPI are enabled in the Raspberry Pi boot config so hardware drivers can access those buses directly.

## Development

### Local Runs

```bash
go run ./cmd/base/main.go --config-file ./configs/config.yaml
go run ./cmd/drone/main.go --config-file ./configs/config.yaml
```

Both the base and drone processes need to run to validate integration behavior locally.

### Configuration

```bash
cp configs/config.yaml configs/my_drone.yaml
```

Edit `configs/my_drone.yaml` for your hardware. The image builder currently copies `configs/config.yaml`; replace that file or update the script if you want to bake a different config into the SD card.

### Hardware Console

On an Alpine booted Pi with an attached console:

```bash
ssh admin@<pi-ip>
rc-service droneOS status
rc-service droneOS restart
tail -f /var/log/droneOS.log
tail -f /var/log/droneOS.err
```

## Project Layout

- Base station entrypoint: `cmd/base/main.go`
- Drone entrypoint: `cmd/drone/main.go`
- Shared config: `configs/config.yaml`
- Protocol framing and transports: `internal/protocol/*`
- Radio driver: `internal/drivers/radio/SX1262`
- Sensor drivers: `internal/drivers/sensor/*`
- Motor drivers: `internal/drivers/motor/*`
- Control algorithms: `internal/drone/control/*`

## Driver Patterns

- Driver `Main` functions are invoked through `utils.CallFunctionByName` with a `context.Context` as the first argument.
- Radio drivers should implement `protocol.RadioLink` with `Send([]byte)` and `Receive() ([]byte, error)`, then call `protocol.ServeRadio`.
- New drivers and controls should live in new package directories and be registered in the maps in `cmd/drone/main.go`.

## Logging

Use zerolog (`github.com/rs/zerolog/log`):

- `Info` and `Error` for human output
- `Debug` for machine-readable logs

Logs emitted by the OpenRC service are written to `/var/log/droneOS.log` and `/var/log/droneOS.err` on the running Alpine system.

## Notes

- `build.sh` no longer runs `go mod tidy`; dependency changes should be explicit.
- `pi_runner.sh` and `cmd/dev/pi_runner` drive the Alpine serial console. Higher-level Alpine/OpenRC remote deployment is still not ported.
