#!/usr/bin/env bash

# install build dependencies
sudo apt install \
  bc \
  bison \
  flex \
  libssl-dev \
  make \
  libc6-dev \
  libncurses5-dev \
  crossbuild-essential-arm64 \
  crossbuild-essential-armhf

# TODO: gotta be a better way to do this
# create container file for compiled binary
touch droneOS && chmod +x droneOS