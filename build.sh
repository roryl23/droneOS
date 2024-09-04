#!/usr/bin/env bash

# input parameters
# lsblk will let you find the value for this parameter:
SD_CARD=${1:-""}

# local variables
PROJECT_DIR=$PWD
RPI_LINUX_BRANCH=rpi-6.6.y
SD_CARD_BOOT="${SD_CARD}1"
SD_CARD_ROOT="${SD_CARD}2"
KERNEL=kernel8

# get raspberry pi firmware
if ! [ -d "build/firmware" ]; then
  cd build && \
  git clone --depth=1 git@github.com:raspberrypi/firmware.git
  cd $PROJECT_DIR
fi

# get kernel source
if ! [ -d "build/linux" ]; then
  mkdir -p build && \
  cd build && \
  git clone --depth=1 https://github.com/raspberrypi/linux && \
  git checkout $RPI_LINUX_BRANCH
  cd $PROJECT_DIR
fi
# get real time kernel patch
if ! [ -d "build/patches" ]; then
  cd build
  if ! [ -f "patches-6.6.48-rt40.tar.gz" ]; then
    wget https://cdn.kernel.org/pub/linux/kernel/projects/rt/6.6/patches-6.6.48-rt40.tar.gz
  fi
  tar -xvf patches-6.6.48-rt40.tar.gz && \
  # apply real time kernel patch
  cd linux && \
  git am -3 ../patches/*
  git am --skip
  cd $PROJECT_DIR
fi

# build kernel
if [ -d "build/linux" ]; then
  # set up SD card
  sudo parted /dev/"${SD_CARD}" --script <<EOF
mklabel msdos
y
mkpart primary fat32 1MiB 65MiB
set 1 lba on
mkpart primary ext4 65MiB 100%
EOF
  sudo mkfs.vfat -F 32 /dev/"${SD_CARD_BOOT}" && \
  sudo mkfs.ext4 -F /dev/"${SD_CARD_ROOT}" && \
  # build kernel and install to sd card
  cd build/linux && \
  make KERNEL=kernel8 ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- bcm2711_defconfig
  make -j6 ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- Image modules dtbs
  mkdir -p mnt/rpi_boot
  mkdir -p mnt/rpi_root
  sudo mount /dev/"${SD_CARD_BOOT}" mnt/rpi_boot && \
  sudo mount /dev/"${SD_CARD_ROOT}" mnt/rpi_root && \
  sudo env PATH=$PATH make -j6 ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- INSTALL_MOD_PATH=mnt/rpi_root modules_install
  if [ -f "mnt/rpi_boot/$KERNEL.img" ]; then
    sudo cp mnt/rpi_boot/"$KERNEL".img mnt/rpi_boot/"$KERNEL"-backup.img
  fi
  sudo cp arch/arm64/boot/Image mnt/rpi_boot/"$KERNEL".img && \
  sudo cp arch/arm64/boot/dts/broadcom/*.dtb mnt/rpi_boot/ && \
  sudo mkdir -p mnt/rpi_boot/overlays/ && \
  sudo cp arch/arm64/boot/dts/overlays/*.dtb* mnt/rpi_boot/overlays/ && \
  sudo cp arch/arm64/boot/dts/overlays/README mnt/rpi_boot/overlays/ && \
  # copy raspberry pi firmware
  sudo cp $PROJECT_DIR/build/firmware/boot/start*.elf mnt/rpi_boot/ && \
  sudo cp $PROJECT_DIR/build/firmware/boot/fixup*.dat mnt/rpi_boot/ && \
  sudo cp $PROJECT_DIR/build/firmware/boot/bootcode.bin mnt/rpi_boot/bootcode.bin
  # cleanup
  sudo umount /dev/"${SD_CARD_BOOT}"
  sudo umount /dev/"${SD_CARD_ROOT}"
  cd $PROJECT_DIR
fi

# install source libraries
cd lib && \
bash install.sh && \
cd $PROJECT_DIR

# build droneOS
