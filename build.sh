#!/usr/bin/env bash

ARCH=${1:-"arm64"}

go mod tidy -e && \
# TODO: this can be done automatically for all directories in internal/plugin
env \
CGO_ENABLED=1 \
GOOS=linux \
GOARCH="$ARCH" \
go build -buildmode=plugin -o plugin_droneos.so ./internal/plugin/droneos && \
env \
CGO_ENABLED=1 \
GOOS=linux \
GOARCH="$ARCH" \
go build -o droneOS.bin ./cmd/droneOS && \
chmod +x droneOS.bin
