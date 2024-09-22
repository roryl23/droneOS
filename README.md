# droneOS

A Go framework for remotely and automatically flying a drone.

## Usage

* You'll need two RPis, one for the drone, one for the base station
  * The base station should have a screen, keyboard, and a joystick. 
    You'll also need an RPi that has a WiFi card.
  * To make your life easiest, get the newest RPi Zero, and 400 or 5
* Install build dependencies: `bash setup.sh`
* For each RPi, insert your SD card and run `lsblk` to find which `/dev/sd#` file it is
  * NOTE: running `lsblk` is important! Choosing the wrong drive will wreck the data!
* Unmount the SD card if it's mounted: `sudo umount /dev/sd#1 && sudo umount /dev/sd#2`
* Build the image like this: `bash build_image.sh sd# kernel base username userpassword ssid ssidpassword`,
  changing the parameters for your case.
  * See the `build_image.sh` file for details on the parameter values

## Development

#### Directories

* `internal/base`: base station operation
* `internal/drone`: drone operation
* `internal/gpio`: Raspberry Pi GPIO pin interface
* `internal/input`: Input sensor interfaces
* `internal/output`: Output interfaces
* `internal/plugin`: Plugins compiled to shared libraries for user defined behavior
* `internal/protocol`: Communication protocols for base and drone

#### General development flow

* A plugin (user defined algorithm) is created here: `internal/plugin/user_defined_plugin.go`
  like the default plugin at `internal/plugins/droneos/main.go`
* Your plugin needs to satisfy the following interfaces:
  * Have a Main function with the following signature: `Main(s *config.Config)`

Your plugin fundamentally needs to do these things:
  * Utilize input interfaces in `internal/input` to determine what actions need to be taken.
  * Translate into actions that utilize output interfaces in `internal/output`, if necessary.
  * Complete this in less than the millisecond interval configured in `configs/config.yaml` under `pluginWaitInterval`.

By default, the default priority is configured with the provided `internal/plugin/droneos` as highest.
This can be overridden if you're feeling adventurous by adjusting plugin priority using `pluginPriority` in 
`configs/config.yaml`.
If you override, your plugin needs to handle everything since obstacle avoidance and base station input are implemented
in the default plugin.

### Logging

droneOS logs in a very specific format to allow the base station to know the whole state of the system.
This is useful for debugging, where one could use the logs emitted by the base station to troubleshoot.
This also allows remote debugging of the drone. 
Keep in mind that the debug logging only works when the drone is in WiFi range of the base station.

Log levels are important. Here are the differences:

* Info: application output for human debugging
* Debug: output for machine processing

Logs can be filtered with [jq](https://jqlang.github.io/jq/download): 

`./droneOS | jq '.[] | select(.level == "Debug")'`

#### Raspberry PI GPIO

* 25 GPIO
* 8 ground
* 2 5V
* 2 3.3V
* 2 ID EEPROM

#### Notes

* Currently, we're patching the kernel during build.
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
  * https://github.com/hybridgroup/gobot/tree/release/platforms/joystick
  * https://github.com/tinygo-org/drivers/tree/release/sx126x
  * https://github.com/thinkski/go-v4l2

#### Contributing

Feel free to fork the PR and add plugins for your project.