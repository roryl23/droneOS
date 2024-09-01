#!/usr/bin/env bash

# install source libraries
cd lib && \
bash install.sh && \
cd ..

# compile
# boot
arm-none-eabi-gcc -mcpu=arm1176jzf-s -fpic -ffreestanding -c src/boot.S -o bin/boot.o
# kernel
arm-none-eabi-gcc -mcpu=arm1176jzf-s -fpic -ffreestanding -std=gnu99 -c src/kernel.c -o bin/kernel.o -O2 -Wall #-Wextra
# link
arm-none-eabi-gcc -T src/linker.ld -o bin/droneos.elf -ffreestanding -O2 -nostdlib bin/boot.o bin/kernel.o -lgcc
arm-none-eabi-objcopy bin/droneos.elf -O binary kernel7.img
