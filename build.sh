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

export CC="$CC" \
  CGO_ENABLED=1 \
  GOOS=linux \
  GOARCH="$ARCH" \
  GOARM="$ARM" \

mkdir -p build/droneOS && \
go mod tidy -e && \
go build -o build/droneOS/droneOS.bin ./cmd/droneOS && \
chmod +x build/droneOS/droneOS.bin
