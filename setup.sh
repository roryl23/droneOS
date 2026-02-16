#!/usr/bin/env bash

TINYGO_VERSION="0.33.0"
SDL2_VERSION="2.0.8"

# install build dependencies
sudo snap install go
sudo apt install -y \
  bc \
  bison \
  flex \
  libssl-dev \
  make \
  libc6-dev \
  libncurses5-dev \
  crossbuild-essential-arm64 \
  crossbuild-essential-armhf \
  qemu-user-static \
  gcc-arm-linux-gnueabi

# create build directory
mkdir -p build/droneOS

cd build
# xpad
#sudo git clone https://github.com/paroj/xpad.git /usr/src/xpad-0.4
#sudo dkms install -m xpad -v 0.4
