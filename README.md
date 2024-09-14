# droneOS

A Go framework for remotely controlling a drone.

## Usage

* You'll need two RPis, one for the drone, one for the base station
  * The base station should have a screen, keyboard, and a joystick. 
    You'll also need an RPi that has a WiFi card.
  * To make your life easiest, get the newest RPi Zero, and 400 or 5
* Install build dependencies: `bash install.sh`
* For each RPi, insert your SD card and run `lsblk` to find which /dev/sd# file it is
  * NOTE: running `lsblk` is important! Choosing the wrong drive will wreck the data!
* Unmount the SD card if it's mounted: `sudo umount /dev/sd#1 && sudo umount /dev/sd#2`
* Build the image like this: `bash build_image.sh sd# kernel base username userpassword ssid ssidpassword`,
  changing the parameters for your case.
  * See the `build_image.sh` file for details on the parameter values

## Development

droneOS operates with a simple core loop that runs in this order:

* Obstacle avoidance
* Take action on vector input from base station
* Execute user defined plugin algorithms

The general development flow goes like this:

* A user defined algorithm is created here: `internal/plugin/user_defined_algorithm.go` that looks like this:

```go
package plugin

func main() {}
```

Your algorithm fundamentally needs to do this:
  * Utilize input interfaces in `internal/input` to determine what actions need to be taken.
  * Translate into actions that utilize output interfaces in `internal/output`, if necessary.
  * Complete this in less than the millisecond interval configured in `configs/config.yaml` under `pluginWaitInterval`.

By default, the main loop will prioritize obstacle avoidance, then process base station commands, over anything else;
this can be overridden if you're feeling adventurous by changing `overridePriority` in `configs/config.yaml`.

### Logging

droneOS logs in a very specific format to allow the base station to know the whole state of the system.
This is useful for debugging, where one could use the logs emitted by the base station to troubleshoot
using physics simulations, for example. An example of this is provided in `examples/simulation_debugging`.

### Raspberry PI GPIO

* 25 GPIO
* 8 ground
* 2 5V
* 2 3.3V
* 2 ID EEPROM

### Notes

* Currently, we're patching the kernel during build.
  Once the mainline kernel has the realtime patch, we can remove the kernel source patch and compilation:
  * https://wiki.linuxfoundation.org/realtime/start

### Resources

* Raspberry PI
  * https://www.raspberrypi.com/documentation/computers/linux_kernel.html
    * https://cdn.kernel.org/pub/linux/kernel/projects/rt/6.6/
  * https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#raspberry-pi-zero-2-w
  * https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#gpio-and-the-40-pin-header
* Go libraries
  * https://github.com/warthog618/go-gpiocdev
  * https://github.com/hybridgroup/gobot/tree/release/platforms/joystick
  * https://github.com/tinygo-org/drivers/tree/release/sx126x
  * https://github.com/thinkski/go-v4l2
