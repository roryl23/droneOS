#!/usr/bin/env bash

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
  qemu-user-static && \

# create build directory
mkdir -p build/droneOS

cd build
# tinygo
if [[ ! -f "tinygo_0.33.0_amd64.deb" ]]; then
  wget https://github.com/tinygo-org/tinygo/releases/download/v0.33.0/tinygo_0.33.0_amd64.deb
fi
sudo dpkg -i tinygo_0.33.0_amd64.deb

# install joystick dependencies
brew install sdl2
