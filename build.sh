#!/usr/bin/env bash

TYPE=${1:-"drone"}
ARCH=${2:-"arm64"}
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
  GOARM="$ARM"
if [[ $TYPE == "base" ]]; then
  export CGO_CFLAGS="-DSDL_DISABLE_IMMINTRIN_H" \
    CGO_LDFLAGS="$(sdl2-config --libs)"
fi
go mod tidy -e && \
go build -o ${TYPE}.bin ./cmd/${TYPE}/main.go && \
chmod +x ${TYPE}.bin
