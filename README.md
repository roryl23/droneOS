# droneOS

An optimized system for running drones.

Designed and tested with the Raspberry PI Zero 2 W

## usage

* Insert your SD card and run: `bash install.sh && bash build.sh`

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
