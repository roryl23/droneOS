#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage:
  bash sync_pi.sh <pi-host> [remote-dir]

environment:
  DRONEOS_PI_HOST=192.168.1.42
  DRONEOS_PI_USER=admin
  DRONEOS_PI_PORT=22
  DRONEOS_PI_DIR=/home/admin/droneOS
  DRONEOS_RSYNC_DELETE=1
EOF
}

quote_sh() {
  local value=${1//\'/\'\\\'\'}
  printf "'%s'" "$value"
}

require_command() {
  local name=$1
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "required command not found: ${name}" >&2
    exit 1
  fi
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PI_HOST=${1:-${DRONEOS_PI_HOST:-}}
PI_USER=${DRONEOS_PI_USER:-admin}
PI_PORT=${DRONEOS_PI_PORT:-22}
PI_DIR=${2:-${DRONEOS_PI_DIR:-/home/${PI_USER}/droneOS}}
RSYNC_DELETE=${DRONEOS_RSYNC_DELETE:-1}

if [[ -z "$PI_HOST" ]]; then
  usage
  exit 1
fi
if ! [[ "$PI_PORT" =~ ^[0-9]+$ ]]; then
  echo "invalid DRONEOS_PI_PORT: ${PI_PORT}" >&2
  exit 1
fi

require_command ssh
require_command rsync

remote="${PI_USER}@${PI_HOST}"
ssh_args=(-p "$PI_PORT" -o StrictHostKeyChecking=accept-new)
rsync_ssh="ssh -p ${PI_PORT} -o StrictHostKeyChecking=accept-new"
rsync_args=(
  -az
  --human-readable
  --info=stats2
  --exclude .git/
  --exclude .idea/
  --exclude build/
  --exclude '*.bin'
)

if [[ "$RSYNC_DELETE" == "1" || "$RSYNC_DELETE" == "true" ]]; then
  rsync_args+=(--delete)
fi

ssh "${ssh_args[@]}" "$remote" "mkdir -p $(quote_sh "$PI_DIR")"
rsync "${rsync_args[@]}" -e "$rsync_ssh" "${PROJECT_DIR}/" "${remote}:${PI_DIR}/"

echo "synced ${PROJECT_DIR} to ${remote}:${PI_DIR}"
