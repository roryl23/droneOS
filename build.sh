#!/usr/bin/env bash

ARCH=${1:-"arm64"}

go mod tidy && \
# TODO: this can be done automatically for all directories in internal/plugin
env GOOS=linux GOARCH="$ARCH" GOARM=5 go build -buildmode=plugin -o plugin_droneos.so ./internal/plugin/droneos && \
env GOOS=linux GOARCH="$ARCH" GOARM=5 go build -o droneOS.bin ./cmd/droneOS && \
chmod +x droneOS.bin
