# droneOS

an optimized system for running drones on the Raspberry PI platform.

## usage

* Insert your SD card and run: `bash install.sh && bash build.sh`

## notes

* Currently, we're patching the kernel during build.
  Once the mainline kernel has the realtime patch,
  we can remove the kernel source patch and compilation.
  * https://wiki.linuxfoundation.org/realtime/start

## resources

* https://www.raspberrypi.com/documentation/computers/linux_kernel.html
  * https://cdn.kernel.org/pub/linux/kernel/projects/rt/6.6/
* https://github.com/xboot/libonnx
