# droneOS

A Go-based real-time operating system for autonomous drone flight with remote control capabilities.

> ⚠️ **Status:** This project is actively under development and unstable. Use at your own risk.

## Table of Contents

- [Features](#features)
- [Hardware Requirements](#hardware-requirements)
- [Quick Start](#quick-start)
- [Development](#development)
  - [Initial Setup](#initial-setup)
  - [Development Workflow](#development-workflow)
  - [Building SD Card Images](#building-sd-card-images)
  - [Testing on Hardware](#testing-on-hardware)
- [Architecture](#architecture)
- [Configuration](#configuration)
- [Contributing](#contributing)

## Features

- **Real-time flight control** with pluggable control algorithms
- **Dual communication** via WiFi (high-bandwidth) and LoRa radio (long-range fallback)
- **Modular sensor/motor drivers** with hot-swappable plugins
- **Remote debugging** with structured logging over WiFi
- **Xbox 360 controller support** for manual piloting
- **Obstacle avoidance** with multiple sensor types
- **Base station** for remote monitoring and control

## Hardware Requirements

### Drone
- **Raspberry Pi Zero 2 W** (recommended) or any RPi with GPIO
- **LoRa radio module** (USB, e.g., SX1262-based)
- **Sensors** (optional):
  - MPU-6050 IMU (gyroscope/accelerometer)
  - HC-SR04 ultrasonic distance sensor
  - GT-U7 GPS module
  - Frienda IR obstacle sensor
- **Motors/ESCs** for propulsion
- **PiSugar3** power manager (optional, for battery monitoring)

### Base Station
- **PC** (Linux/macOS/Windows)
- **LoRa radio module** (USB, matching drone's frequency)
- **Xbox 360 controller** (optional, for manual control)

## Quick Start

### 1. Install Dependencies

```bash
bash setup.sh
```

This installs Go, cross-compilation toolchains, and other build dependencies.

### 2. Build Development Image

```bash
# Find your SD card (DO NOT skip this - wrong device = data loss!)
lsblk

# Unmount if mounted
sudo umount /dev/sdb1 /dev/sdb2  # Replace sdb with your device

# Build image (development mode)
bash build_image.sh sdb kernel8 drone myuser mypassword MyWiFiSSID MyWiFiPassword US
```

**Parameters:**
- `sdb` - SD card device (from `lsblk`)
- `kernel8` - Kernel variant (kernel/kernel7l/kernel8 for different RPi models)
- `drone` - Image type (drone or base)
- `myuser` - Login username
- `mypassword` - Login password
- `MyWiFiSSID` - WiFi network name
- `MyWiFiPassword` - WiFi password
- `US` - WiFi country code

### 3. Boot and Test

Insert SD card into Raspberry Pi and power on. The drone will:
1. Connect to WiFi (prints IP on console)
2. Wait for commands from base station

**Note:** Development mode has the droneOS service **disabled** by default - use `pi_runner.sh` for testing.

## Development

### Initial Setup

```bash
# Clone repository
git clone https://github.com/roryl23/droneOS.git
cd droneOS

# Install dependencies
bash setup.sh

# Configure your drone
cp configs/config.yaml configs/my_drone.yaml
# Edit configs/my_drone.yaml with your hardware setup
```

### Development Workflow

#### Option 1: Live Development with pi_runner.sh (Recommended)

The fastest way to iterate - automatically builds, deploys, and restarts the drone:

```bash
# Set your Pi's IP address
export DRONEOS_PI_HOST=192.168.0.34  # Replace with your Pi's IP

# Run development server (starts base station + deploys to Pi)
bash pi_runner.sh ./configs/config.yaml

# Or use custom config
bash pi_runner.sh ./configs/my_drone.yaml

# The script will:
# 1. Start base station locally
# 2. Cross-compile drone binary for ARM64
# 3. SCP binary + config to Pi
# 4. Restart droneOS service on Pi
```

**Environment Variables:**
```bash
DRONEOS_PI_HOST=192.168.0.34     # Pi IP address
DRONEOS_PI_USER=root             # SSH user (default: root)
DRONEOS_PI_PORT=22               # SSH port
DRONEOS_PI_DIR=/opt/droneOS      # Install directory on Pi
DRONEOS_PI_ARCH=arm64            # Target architecture
```

#### Option 2: Manual Testing

```bash
# Build drone binary
GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc \
  go build -o drone.bin ./cmd/drone/main.go

# Copy to Pi
scp drone.bin root@192.168.0.34:/opt/droneOS/
scp configs/config.yaml root@192.168.0.34:/opt/droneOS/

# SSH to Pi and run manually
ssh root@192.168.0.34
cd /opt/droneOS
./drone.bin --config-file config.yaml
```

#### Option 3: Run Base Station Only

```bash
# For testing base station without drone
go run ./cmd/base/main.go --config-file ./configs/config.yaml
```

### Building SD Card Images

#### Development Image (Default)

For active development with frequent updates:

```bash
bash build_image.sh sdb kernel8 drone admin password MySSID MyPass US
```

- droneOS service **disabled** (doesn't start on boot)
- No OverlayFS (filesystem is writable)
- Use `pi_runner.sh` for deployment
- Logs written to disk

#### Production Image

For deployed drones with filesystem protection:

```bash
BUILD_MODE=prod bash build_image.sh sdb kernel8 drone admin password MySSID MyPass US
```

- droneOS service **enabled** (starts on boot)
- OverlayFS **enabled** (read-only root, changes in RAM)
- Volatile journald (logs in RAM only)
- Requires `sudo overlayroot-chroot` for persistent changes

#### Optional Features

```bash
# Install PiSugar power manager
INSTALL_PISUGAR=1 bash build_image.sh ...

# Combine options
BUILD_MODE=prod INSTALL_PISUGAR=1 bash build_image.sh ...
```

### Testing on Hardware

#### View Logs

```bash
# Live logs from running drone
ssh root@192.168.0.34 'sudo journalctl -u droneOS.service -f'

# Filter specific log levels
ssh root@192.168.0.34 'sudo journalctl -u droneOS.service -n 100' | jq 'select(.level == "error")'

# Debug logs (only sent when WiFi connected)
ssh root@192.168.0.34 'sudo journalctl -u droneOS.service -n 100' | jq 'select(.level == "debug")'
```

#### Common Issues

**Filesystem becomes read-only:**
- Caused by excessive I/O or power issues
- Fixed in recent commits with volatile journald and optimized polling
- Check power supply voltage (should be 5V ±0.25V)

**WiFi not connecting:**
```bash
ssh root@192.168.0.34
nmcli device wifi list
nmcli connection up ShowMeWhatYouGot  # Your SSID
```

**Service not starting:**
```bash
ssh root@192.168.0.34
sudo systemctl status droneOS.service
sudo journalctl -u droneOS.service -n 50
```

## Development

### Directories

* `internal/base`: base station operation
* `internal/drone`: drone operation
* `internal/gpio`: Raspberry Pi GPIO pin interface
* `internal/input`: Input sensor interfaces
* `internal/output`: Output interfaces
* `internal/control`: Control algorithms compiled to shared libraries
* `internal/protocol`: Communication protocol for base and drone

### General development flow

* A user defined control algorithm is created here: `internal/control/some_name/main.go`
* Your algorithm needs to satisfy the following interfaces:
  * Have a `Main` function with the following signature: 
    `Main(c *config.Config, priority int, eCh *chan sensor.Event, pq *output.Queue)`

Your algorithm fundamentally needs to do these things:
  * Utilize input interfaces in `internal/input` to determine what actions need to be taken.
  * Translate into actions that utilize output interfaces in `internal/output`, if necessary.

If you write more than one control algorithm, such as the default examples of `obstacle_avoidance` and `pilot`,
you'll need to define their priority using `controlAlgorithmPriority` in `configs/config.yaml`.

### Pi dev deploy (USB/SSH)

Use the GoLand run config `base + pi` or run this from the repo root:

```
DRONEOS_PI_HOST=192.168.7.2 go run ./cmd/dev/pi_runner/main.go --config-file ./configs/config.yaml
```

The tool starts the base station locally, cross-compiles the drone binary, copies the binary and config to the Pi,
then runs the drone binary over SSH. Override defaults with environment variables:
`DRONEOS_PI_HOST`, `DRONEOS_PI_USER`, `DRONEOS_PI_PORT`, `DRONEOS_PI_DIR`, `DRONEOS_PI_ARCH`, `DRONEOS_PI_GOARM`,
`DRONEOS_PI_CC`, `DRONEOS_PI_BIN`, `DRONEOS_PI_OUT`.

### Logging

droneOS logs in a very specific format to allow the base station to know the whole state of the system.
This is useful for debugging the drone offline.
Keep in mind that the debug logging only works when the drone is in WiFi range of the base station.,
in order to save on bandwidth constraints over radio.

Log levels are important, and divide two categories of emitted output:
* Human readable:
  * Error
  * Info
* Machine readable:
  * Debug

Logs can be filtered with [jq](https://jqlang.github.io/jq/download): 

`./droneOS.bin | jq '.[] | select(.level == "Debug")'`

### Raspberry PI GPIO


* 25 GPIO 
* 8 ground 
* 2 5V 
* 2 3.3V 
* 2 ID EEPROM


| Pin | Name   | BCM GPIO | Function                   |
|-----|--------|----------|----------------------------|
| 1   | 3.3V   |          | Power                      |
| 2   | 5V     |          | Power                      |
| 3   | GPIO2  | GPIO2    | SDA1, I²C Data             |
| 4   | 5V     |          | Power                      |
| 5   | GPIO3  | GPIO3    | SCL1, I²C Clock            |
| 6   | GND    |          | Ground                     |
| 7   | GPIO4  | GPIO4    | GPCLK0                     |
| 8   | GPIO14 | GPIO14   | UART0_TXD                  |
| 9   | GND    |          | Ground                     |
| 10  | GPIO15 | GPIO15   | UART0_RXD                  |
| 11  | GPIO17 | GPIO17   | GPIO_GEN0                  |
| 12  | GPIO18 | GPIO18   | PCM_CLK, PWM0              |
| 13  | GPIO27 | GPIO27   | GPIO_GEN2                  |
| 14  | GND    |          | Ground                     |
| 15  | GPIO22 | GPIO22   | GPIO_GEN3                  |
| 16  | GPIO23 | GPIO23   | GPIO_GEN4                  |
| 17  | 3.3V   |          | Power                      |
| 18  | GPIO24 | GPIO24   | GPIO_GEN5                  |
| 19  | GPIO10 | GPIO10   | SPI0_MOSI                  |
| 20  | GND    |          | Ground                     |
| 21  | GPIO9  | GPIO9    | SPI0_MISO                  |
| 22  | GPIO25 | GPIO25   | GPIO_GEN6                  |
| 23  | GPIO11 | GPIO11   | SPI0_SCLK                  |
| 24  | GPIO8  | GPIO8    | SPI0_CE0_N                 |
| 25  | GND    |          | Ground                     |
| 26  | GPIO7  | GPIO7    | SPI0_CE1_N                 |
| 27  | ID_SD  | GPIO0    | I²C ID EEPROM Data (ID_SD) |
| 28  | ID_SC  | GPIO1    | I²C ID EEPROM Clock (ID_SC)|
| 29  | GPIO5  | GPIO5    | GPIO_GEN1                  |
| 30  | GND    |          | Ground                     |
| 31  | GPIO6  | GPIO6    | GPIO_GEN2                  |
| 32  | GPIO12 | GPIO12   | PWM0                       |
| 33  | GPIO13 | GPIO13   | PWM1                       |
| 34  | GND    |          | Ground                     |
| 35  | GPIO19 | GPIO19   | PCM_FS, PWM1               |
| 36  | GPIO16 | GPIO16   | GPIO_GEN4                  |
| 37  | GPIO26 | GPIO26   | GPIO_GEN7                  |
| 38  | GPIO20 | GPIO20   | PCM_DIN                    |
| 39  | GND    |          | Ground                     |
| 40  | GPIO21 | GPIO21   | PCM_DOUT                   |

#### Other detail

* Power Pins: Pins 1 (3.3V), 2 (5V), 4 (5V), and 17 (3.3V) are power supply pins. 
* Ground Pins: Pins 6, 9, 14, 20, 25, 30, 34, and 39 are ground pins. 
* GPIO Pins: The GPIO (General Purpose Input/Output) pins can be programmed for various functions. 
* Special Function Pins: Some GPIO pins have special functions like I²C, SPI, UART, and PWM.

### Notes

* Currently, we're patching the kernel during compilation from source.
  Once the mainline kernel has the realtime patch, we can remove the kernel source patch and compilation:
  * https://wiki.linuxfoundation.org/realtime/start

#### Resources

* Raspberry PI
  * https://www.raspberrypi.com/documentation/computers/linux_kernel.html
    * https://cdn.kernel.org/pub/linux/kernel/projects/rt/6.6/
  * https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#raspberry-pi-zero-2-w
  * https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#gpio-and-the-40-pin-header
* Go libraries
  * https://github.com/warthog618/go-gpiocdev
  * https://gobot.io/documentation/drivers
  * https://github.com/tinygo-org/drivers
  * https://github.com/thinkski/go-v4l2

#### Contributing

Feel free to fork the PR and add plugins for your project.
