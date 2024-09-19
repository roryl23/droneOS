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
