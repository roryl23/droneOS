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
# plugin builds
go build -buildmode=plugin -o build/droneOS/ca_droneos_so ./internal/plugin/droneos && \
# TODO: this can be done automatically for all directories that require plugin builds
go build -buildmode=plugin -o build/droneOS/sensor_frienda_obstacle_431S_so ./internal/input/sensor/plugin/frienda_obstacle_431S && \
go build -buildmode=plugin -o build/droneOS/sensor_GT_U7_so ./internal/input/sensor/plugin/GT_U7 && \
go build -buildmode=plugin -o build/droneOS/sensor_HC_SR04_so ./internal/input/sensor/plugin/HC_SR04 && \
go build -buildmode=plugin -o build/droneOS/sensor_MPU_6050_so ./internal/input/sensor/plugin/MPU_6050 && \
# application
go build -o build/droneOS/droneOS.bin ./cmd/droneOS && \
chmod +x build/droneOS/droneOS.bin
