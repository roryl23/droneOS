#!/usr/bin/env bash

# install build dependencies
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
  qemu-user-static

# create build directory
mkdir -p build/droneOS

# install joystick dependencies
cd build
if [[ ! -f "SDL2-2.0.8.tar.gz" ]]; then
  wget https://www.libsdl.org/release/SDL2-2.0.8.tar.gz
fi
if [[ ! -d "SDL2-2.0.8" ]]; then
  tar -zxvf SDL2-2.0.8.tar.gz
fi
cd SDL2-2.0.8/ && \
./configure && make && sudo make install && \
cd ../..
