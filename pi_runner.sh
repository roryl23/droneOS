#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage:
  bash pi_runner.sh list
  bash pi_runner.sh [flags] console
  bash pi_runner.sh [flags] loopback
  bash pi_runner.sh [flags] wait
  bash pi_runner.sh [flags] exec --command 'cd /home/admin/droneOS && go test ./...'
  bash pi_runner.sh [flags] exec 'uname -a'

important flags:
  --serial path|auto          select a serial device; auto is the default
  --baud n                    serial baud rate
  --timeout duration          wait, login, or command timeout
  --poke-interval duration    carriage-return interval in wait mode; 0 disables
  --wait-marker text          text required by wait mode; default: login:
  --verbose                   mirror login traffic during exec
  --command text              command for exec mode only
  --user name                 login user for exec
  --password value            login password for exec

environment:
  DRONEOS_SERIAL_DEVICE=/dev/serial/by-id/...
  DRONEOS_SERIAL_BAUD=115200
  DRONEOS_SERIAL_TIMEOUT=90s
  DRONEOS_SERIAL_POKE_INTERVAL=2s
  DRONEOS_SERIAL_WAIT_MARKER=login:
  DRONEOS_PI_USER=admin
  DRONEOS_PI_PASSWORD=...

Use `list` first when more than one USB serial adapter is attached.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_DIR"

exec go run ./cmd/dev/pi_runner "$@"
