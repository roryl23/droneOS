#!/usr/bin/env bash

ARCH=${1:-"arm64"}

go mod tidy && \
env GOOS=linux GOARCH="$ARCH" GOARM=5 go build -o droneOS.bin .
