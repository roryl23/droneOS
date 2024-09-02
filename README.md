# droneOS

an optimized system for running drones on the Raspberry PI platform.

## usage

* `bash install.sh && bash build.sh`

## notes

* This won't work quite correctly until the real time patch is merged to the mainline Linux kernel:
  * https://wiki.linuxfoundation.org/realtime/start
  * Currently we're manually patching the kernel

## resources

* https://www.raspberrypi.com/documentation/computers/linux_kernel.html
  * https://cdn.kernel.org/pub/linux/kernel/projects/rt/6.6/
* https://github.com/xboot/libonnx
