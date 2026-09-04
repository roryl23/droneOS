#!/usr/bin/env bash
set -euo pipefail
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v apk >/dev/null 2>&1; then
  echo "setup.sh targets Alpine Linux hosts. Run this on Alpine or install the equivalent packages manually." >&2
  exit 1
fi

if [[ "$(id -u)" -eq 0 ]]; then
  SUDO=()
else
  if ! command -v sudo >/dev/null 2>&1; then
    echo "setup.sh needs sudo when run as a non-root user" >&2
    exit 1
  fi
  SUDO=(sudo)
fi

# Host tools for static Go builds, Alpine Raspberry Pi media creation, and
# source synchronization to a development Pi.
"${SUDO[@]}" apk update
"${SUDO[@]}" apk add --no-cache \
  bash \
  ca-certificates \
  dosfstools \
  parted \
  go \
  openssh-client \
  openssh-keygen \
  openssl \
  rsync \
  tar \
  lsblk \
  sfdisk \
  util-linux-misc \
  wget

mkdir -p "${PROJECT_DIR}/build/droneOS"

for group in dialout plugdev input gpio i2c spi video; do
  if grep -q "^${group}:" /etc/group; then
    "${SUDO[@]}" addgroup "$USER" "$group" 2>/dev/null || true
  fi
done
