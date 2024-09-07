#!/usr/bin/env bash

# usage: bash build.sh sda kernel username password base

# input parameters
# lsblk will let you find the value for this parameter:
SD_CARD=${1:-""}
# [kernel, kernel7l, kernel8]
KERNEL=${2:-"kernel"}
USER=${3:-"admin"}
PASSWORD=${4:-"adminpassword"}
# [base, drone]
TYPE=${5:-"base"}

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

# set up sd card
sudo parted /dev/"${SD_CARD}" --script <<EOF
mklabel msdos
y
mkpart primary fat32 1MiB 100%
set 1 lba on
EOF
sudo mkfs.ext4 -F /dev/"${SD_CARD_ROOT}"
# determine which image to get
if [[ "$KERNEL" == "kernel" || "$KERNEL" == "kernel7l" ]]; then
  if [[ "$TYPE" == "base" ]]; then
    IMAGE_URL="https://downloads.raspberrypi.com/raspios_arm64/images/raspios_arm64-2024-07-04/2024-07-04-raspios-bookworm-arm64.img.xz"
    IMAGE_FILE_XZ="2024-07-04-raspios-bookworm-arm64.img.xz"
    IMAGE_FILE="2024-07-04-raspios-bookworm-arm64.img"
  elif [[ "$TYPE" == "drone" ]]; then
    IMAGE_URL="https://downloads.raspberrypi.com/raspios_lite_armhf/images/raspios_lite_armhf-2024-07-04/2024-07-04-raspios-bookworm-armhf-lite.img.xz"
    IMAGE_FILE_XZ="2024-07-04-raspios-bookworm-armhf-lite.img.xz"
    IMAGE_FILE="2024-07-04-raspios-bookworm-armhf-lite.img"
  fi
elif [[ "$KERNEL" == "kernel8" ]]; then
  IMAGE_URL="https://downloads.raspberrypi.com/raspios_lite_arm64/images/raspios_lite_arm64-2024-07-04/2024-07-04-raspios-bookworm-arm64-lite.img.xz"
  IMAGE_FILE_XZ="2024-07-04-raspios-bookworm-arm64-lite.img.xz"
  IMAGE_FILE="2024-07-04-raspios-bookworm-arm64-lite.img"
fi
# fetch, decompress, and write image
if ! [ -f "$IMAGE_FILE_XZ" ]; then
  wget "$IMAGE_URL"
fi
if ! [ -f "$IMAGE_FILE" ]; then
  xz --threads=${THREADS} --keep -v -d "$IMAGE_FILE_XZ"
fi
sudo dd bs=1M if="$IMAGE_FILE" of=/dev/"${SD_CARD}" status=progress

# build kernel
if [ -d "build/linux" ]; then
  cd build/linux && \
  # configure
  cp "$PROJECT_DIR"/configs/.config . && \
  mkdir -p mnt/rpi_boot && \
  mkdir -p mnt/rpi_root && \
  sudo mount /dev/"${SD_CARD_BOOT}" mnt/rpi_boot && \
  sudo mount /dev/"${SD_CARD_ROOT}" mnt/rpi_root && \
  sudo mkdir -p mnt/rpi_boot/overlays/ && \
  sudo cp "$PROJECT_DIR"/configs/config-"${KERNEL}".txt mnt/rpi_boot
  # set up user to avoid booting into userconfig on first boot
  PASSWORD_ENCRYPTED=$(echo "$PASSWORD" | openssl passwd -6 -stdin)
  echo "${USER}:${PASSWORD_ENCRYPTED}" | tee mnt/rpi_boot/userconf.txt

  # compile and install kernel
  if [[ "$KERNEL" == "kernel" ]]; then
    make -j${THREADS} KERNEL="$KERNEL" ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- bcmrpi_defconfig && \
    make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- Image modules dtbs && \
    sudo env PATH="$PATH" make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- INSTALL_MOD_PATH=mnt/rpi_root modules_install && \
    sudo cp arch/arm/boot/dts/broadcom/*.dtb mnt/rpi_boot/ && \
    sudo cp arch/arm/boot/dts/overlays/*.dtb* mnt/rpi_boot/overlays/ && \
    sudo cp arch/arm/boot/dts/overlays/README mnt/rpi_boot/overlays/
  elif [[ "$KERNEL" == "kernel7" ]]; then
    make -j${THREADS} KERNEL="$KERNEL" ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- bcm2711_defconfig && \
    make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- Image modules dtbs && \
    sudo env PATH="$PATH" make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- INSTALL_MOD_PATH=mnt/rpi_root modules_install && \
    sudo cp arch/arm/boot/dts/broadcom/*.dtb mnt/rpi_boot/ && \
    sudo cp arch/arm/boot/dts/overlays/*.dtb* mnt/rpi_boot/overlays/ && \
    sudo cp arch/arm/boot/dts/overlays/README mnt/rpi_boot/overlays/
  elif [[ "$KERNEL" == "kernel8" ]]; then
    make -j${THREADS} KERNEL="$KERNEL" ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- bcm2711_defconfig && \
    make -j${THREADS} ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- Image modules dtbs && \
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
