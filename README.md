# droneOS

A Go framework for remotely and automatically flying a drone.

NOTE: This project is unstable and currently under development

## Usage

### Hardware

* You'll need one RPi for the drone. 
  To make your life easiest, get the newest RPi Zero 2 W
* Realistically, you'll need a PC for the base station
  * Technically, you can run this wherever you can get the software/hardware to work,
    which is left as an exercise for the reader. I will say that cross-compiling SDL2
    is extremely annoying and doesn't work right now because I'm a scrub.
  * You'll need a radio communication module that plugs in via USB
  * Joystick supported by gobot

### Software

* Install build dependencies: `bash setup.sh`
* For each RPi, insert your SD card and run `lsblk` to find which `/dev/sd#` file it is:
```
sdf           8:80   1  29.7G  0 disk 
├─sdf1        8:81   1   512M  0 part /media/user/bootfs
└─sdf2        8:82   1  29.2G  0 part /media/user/rootfs
```
  * NOTE: running `lsblk` is important! Choosing the wrong drive will instantly wreck the filesystem!
* Unmount the SD card if it's mounted: `sudo umount /dev/sd#1 && sudo umount /dev/sd#2`
* Build the image like this: `bash build_image.sh sd# kernel8 drone username userpassword ssid ssidpassword`,
  changing the parameters for your case.
  * See the `build_image.sh` file for details on the parameter values

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
