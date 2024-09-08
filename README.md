# droneOS

An optimized system for remotely controlling a drone that runs on the Raspberry PI hardware platform.

## usage

* You'll need two RPis, one for the drone, one for the base station
  * The base station should have a screen, keyboard, and a joystick. 
    You'll also need an RPi that has a WiFi card.
* Install build dependencies: `bash install.sh`
* For each RPi, insert your SD card and run `lsblk` to find which /dev file it is
* Build the image like this: `bash build_image.sh sda kernel base username userpassword ssid ssidpassword`,
  changing the parameters for your case.
  * See the `build_image.sh` file for details on the parameter values

## Raspberry PI GPIO

* 25 GPIO
* 8 ground
* 2 5V
* 2 3.3V
* 2 ID EEPROM

## notes

* Currently, we're patching the kernel during build.
  Once the mainline kernel has the realtime patch,
  we can remove the kernel source patch and compilation.
  * https://wiki.linuxfoundation.org/realtime/start

## resources

* https://www.raspberrypi.com/documentation/computers/linux_kernel.html
  * https://cdn.kernel.org/pub/linux/kernel/projects/rt/6.6/
* https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#raspberry-pi-zero-2-w
* https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#gpio-and-the-40-pin-header
* https://github.com/xboot/libonnx
