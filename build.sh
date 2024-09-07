#!/usr/bin/env bash

# usage: bash build.sh sda kernel

# input parameters
# lsblk will let you find the value for this parameter:
SD_CARD=${1:-""}
KERNEL=${2:-"kernel"}

# local variables
THREADS=8
PROJECT_DIR=$PWD
RPI_LINUX_BRANCH=rpi-6.6.y
SD_CARD_BOOT="${SD_CARD}1"
SD_CARD_ROOT="${SD_CARD}2"

# get raspberry pi firmware
if ! [ -d "build/firmware" ]; then
  cd build && \
  git clone --depth=1 git@github.com:raspberrypi/firmware.git
  cd $PROJECT_DIR
fi

# get kernel source and configure
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

# set up sd card with raspberry pi image
sudo parted /dev/"${SD_CARD}" --script <<EOF
mklabel msdos
y
mkpart primary fat32 1MiB 100%
set 1 lba on
EOF
sudo mkfs.ext4 -F /dev/"${SD_CARD_ROOT}"
if [[ "$KERNEL" == "kernel" || "$KERNEL" == "kernel7l" ]]; then
  if ! [ -f "2024-07-04-raspios-bookworm-armhf-lite.img.xz" ]; then
    wget https://downloads.raspberrypi.com/raspios_lite_armhf/images/raspios_lite_armhf-2024-07-04/2024-07-04-raspios-bookworm-armhf-lite.img.xz
  fi
  if ! [ -f "2024-07-04-raspios-bookworm-armhf-lite.img" ]; then
    xz --threads=${THREADS} --keep -v -d 2024-07-04-raspios-bookworm-armhf-lite.img.xz
  fi
  sudo dd bs=1M if=2024-07-04-raspios-bookworm-armhf-lite.img of=/dev/"${SD_CARD}" status=progress
elif [[ "$KERNEL" == "kernel8" ]]; then
  if ! [ -f "2024-07-04-raspios-bookworm-arm64-lite.img.xz" ]; then
    wget https://downloads.raspberrypi.com/raspios_lite_arm64/images/raspios_lite_arm64-2024-07-04/2024-07-04-raspios-bookworm-arm64-lite.img.xz
  fi
  if ! [ -f "2024-07-04-raspios-bookworm-arm64-lite.img" ]; then
    xz --threads=${THREADS} --keep -v -d 2024-07-04-raspios-bookworm-arm64-lite.img.xz
  fi
  sudo dd bs=1M if=2024-07-04-raspios-bookworm-arm64-lite.img of=/dev/"${SD_CARD}" status=progress
fi

# build kernel
if [ -d "build/linux" ]; then
  cd build/linux && \
  # configure
  cp "$PROJECT_DIR"/config/.config . && \
  mkdir -p mnt/rpi_boot && \
  mkdir -p mnt/rpi_root && \
  sudo mount /dev/"${SD_CARD_BOOT}" mnt/rpi_boot && \
  sudo mount /dev/"${SD_CARD_ROOT}" mnt/rpi_root && \
  sudo mkdir -p mnt/rpi_boot/overlays/

  # compile and install kernel
  if [[ "$KERNEL" == "kernel" ]]; then
    make -j${THREADS} KERNEL="$KERNEL" ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- bcmrpi_defconfig && \
    make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- Image modules dtbs && \
    sudo cp "$PROJECT_DIR"/config/config-${KERNEL}.txt mnt/rpi_boot && \
    sudo env PATH="$PATH" make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- INSTALL_MOD_PATH=mnt/rpi_root modules_install && \
    sudo cp arch/arm/boot/dts/broadcom/*.dtb mnt/rpi_boot/ && \
    sudo cp arch/arm/boot/dts/overlays/*.dtb* mnt/rpi_boot/overlays/ && \
    sudo cp arch/arm/boot/dts/overlays/README mnt/rpi_boot/overlays/
  elif [[ "$KERNEL" == "kernel7" ]]; then
    make -j${THREADS} KERNEL="$KERNEL" ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- bcm2711_defconfig && \
    make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- Image modules dtbs && \
    sudo cp "$PROJECT_DIR"/config/config-"${KERNEL}".txt mnt/rpi_boot && \
    sudo env PATH="$PATH" make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- INSTALL_MOD_PATH=mnt/rpi_root modules_install && \
    sudo cp arch/arm/boot/dts/broadcom/*.dtb mnt/rpi_boot/ && \
    sudo cp arch/arm/boot/dts/overlays/*.dtb* mnt/rpi_boot/overlays/ && \
    sudo cp arch/arm/boot/dts/overlays/README mnt/rpi_boot/overlays/
  elif [[ "$KERNEL" == "kernel8" ]]; then
    make -j${THREADS} KERNEL="$KERNEL" ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- bcm2711_defconfig && \
    make -j${THREADS} ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- Image modules dtbs && \
    sudo cp "$PROJECT_DIR"/config/config-"${KERNEL}".txt mnt/rpi_boot && \
    sudo env PATH="$PATH" make -j${THREADS} ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- INSTALL_MOD_PATH=mnt/rpi_root modules_install && \
    sudo cp arch/arm64/boot/Image mnt/rpi_boot/"$KERNEL".img && \
    sudo cp arch/arm64/boot/dts/broadcom/*.dtb mnt/rpi_boot/ && \
    sudo cp arch/arm64/boot/dts/overlays/*.dtb* mnt/rpi_boot/overlays/ && \
    sudo cp arch/arm64/boot/dts/overlays/README mnt/rpi_boot/overlays/
  fi

  # install firmware
#  sudo cp $PROJECT_DIR/build/firmware/boot/start*.elf mnt/rpi_boot/ && \
#  sudo cp $PROJECT_DIR/build/firmware/boot/fixup*.dat mnt/rpi_boot/ && \
#  sudo cp $PROJECT_DIR/build/firmware/boot/bootcode.bin mnt/rpi_boot/bootcode.bin

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
