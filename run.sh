#!/usr/bin/env bash

bash build.sh && \
# run in emulation using QEMU
qemu-system-arm -m 512 -M raspi0 -serial stdio -kernel bin/droneos.elf
