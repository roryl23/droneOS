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
# tinygo
if [[ ! -f "tinygo_${TINYGO_VERSION}_amd64.deb" ]]; then
  wget https://github.com/tinygo-org/tinygo/releases/download/v${TINYGO_VERSION}/tinygo_${TINYGO_VERSION}_amd64.deb
fi
sudo dpkg -i tinygo_${TINYGO_VERSION}_amd64.deb

# install controller dependencies
if [[ ! -f "SDL2-${SDL2_VERSION}.tar.gz" ]]; then
  wget https://www.libsdl.org/release/SDL2-${SDL2_VERSION}.tar.gz
fi
if [[ ! -d "SDL2-${SDL2_VERSION}" ]]; then
  tar -zxvf SDL2-${SDL2_VERSION}.tar.gz
fi
cd SDL2-${SDL2_VERSION}/ && \
./configure && make && sudo make install && \
cd ../..
