#!/usr/bin/env bash
# usage examples:
#   bash pi_runner.sh
#   DRONEOS_PI_HOST=192.168.0.108 DRONEOS_PI_USER=pi bash pi_runner.sh ./configs/config.yaml --pi-dir /home/pi/droneOS

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CONFIG_FILE="${PROJECT_DIR}/configs/config.yaml"
EXTRA_ARGS=()
if [[ $# -gt 0 ]]; then
  if [[ "${1:-}" != -* ]]; then
    CONFIG_FILE="$1"
    shift
  fi
  EXTRA_ARGS=("$@")
fi

ARCH="${DRONEOS_PI_ARCH:-arm64}"
GOARM="${DRONEOS_PI_GOARM:-""}"
CC="${DRONEOS_PI_CC:-""}"
PI_HOST="${DRONEOS_PI_HOST:-raspberrypi.local}"
PI_USER="${DRONEOS_PI_USER:-pi}"
PI_PORT="${DRONEOS_PI_PORT:-22}"
PI_DIR="${DRONEOS_PI_DIR:-/home/pi/droneOS}"
PI_BIN="${DRONEOS_PI_BIN:-drone.bin}"
OUTPUT="${DRONEOS_PI_OUT:-${PROJECT_DIR}/build/droneOS/drone.pi}"
GO_CMD="${DRONEOS_GO_CMD:-go}"

ARGS=(
  --config-file "$CONFIG_FILE"
  --arch "$ARCH"
  --pi-host "$PI_HOST"
  --pi-user "$PI_USER"
  --pi-port "$PI_PORT"
  --pi-dir "$PI_DIR"
  --pi-bin-name "$PI_BIN"
  --output "$OUTPUT"
  --go-cmd "$GO_CMD"
)

if [[ -n "$GOARM" ]]; then
  ARGS+=(--goarm "$GOARM")
fi
if [[ -n "$CC" ]]; then
  ARGS+=(--cc "$CC")
fi

cd "$PROJECT_DIR"
exec "$GO_CMD" run ./cmd/dev/pi_runner/main.go "${ARGS[@]}" "${EXTRA_ARGS[@]}"
