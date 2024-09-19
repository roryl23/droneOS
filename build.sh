#!/usr/bin/env bash

ARCH=${1:-"arm64"}
if [[ $ARCH == "arm64" ]]; then
  CC=aarch64-linux-gnu-gcc
  ARM=8
else
  # TODO: gnueabihf?
  CC=arm-linux-gnueabi-gcc
  ARM=5
fi

go mod tidy -e && \
# TODO: this can be done automatically for all directories in internal/plugin
env \
CC="$CC" \
CGO_ENABLED=1 \
GOOS=linux \
GOARCH="$ARCH" \
GOARM="$ARM" \
go build -buildmode=plugin -o plugin_droneos.so ./internal/plugin/droneos && \
env \
CC="$CC" \
CGO_ENABLED=1 \
GOOS=linux \
GOARCH="$ARCH" \
GOARM="$ARM" \
go build -o droneOS.bin ./cmd/droneOS && \
chmod +x droneOS.bin
