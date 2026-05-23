#!/usr/bin/env bash
set -euo pipefail

if ! command -v apk >/dev/null 2>&1; then
  echo "setup.sh now targets Alpine Linux hosts. Run this on Alpine or install the equivalent packages manually." >&2
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
# optional Raspberry Pi kernel work.
"${SUDO[@]}" apk update
"${SUDO[@]}" apk add --no-cache \
  bash \
  bc \
  bison \
  build-base \
  coreutils \
  curl \
  dosfstools \
  e2fsprogs \
  flex \
  git \
  go \
  linux-headers \
  make \
  mtools \
  ncurses-dev \
  openssh-client \
  openssh-keygen \
  openssl \
  openssl-dev \
  perl \
  rsync \
  tar \
  util-linux \
  wget \
  xz

mkdir -p build/droneOS

for group in dialout plugdev input gpio i2c spi video; do
  if grep -q "^${group}:" /etc/group; then
    "${SUDO[@]}" addgroup "$USER" "$group" 2>/dev/null || true
  fi
done
