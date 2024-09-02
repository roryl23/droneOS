#!/usr/bin/env bash

# input parameters
SD_CARD=${1:-""}

# local variables
PROJECT_DIR=$PWD
RPI_LINUX_BRANCH=rpi-6.6.y

# get kernel source
if ! [ -d "build/linux" ]; then
  mkdir -p build && \
  cd build && \
  git clone --depth=1 https://github.com/raspberrypi/linux && \
  git checkout $RPI_LINUX_BRANCH && \
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
  cd build/linux && \
  KERNEL=kernel8 make ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- bcm2711_defconfig && \
  make -j6 ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- Image modules dtbs && \
  mkdir -p mnt/rpi_boot && \
  mkdir -p mnt/rpi_root && \
  sudo mount /dev/"${SD_CARD}" mnt/rpi_boot && \
  sudo mount /dev/"${SD_CARD}" mnt/rpi_root && \
  sudo env PATH=$PATH make -j6 ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- INSTALL_MOD_PATH=mnt/rpi_root modules_install && \
  sudo cp mnt/rpi_boot/"$KERNEL".img mnt/rpi_boot/"$KERNEL"-backup.img && \
  sudo cp arch/arm64/rpi_boot/Image mnt/rpi_boot/"$KERNEL".img && \
  sudo cp arch/arm64/rpi_boot/dts/broadcom/*.dtb mnt/rpi_boot/ && \
  sudo cp arch/arm64/rpi_boot/dts/overlays/*.dtb* mnt/rpi_boot/overlays/ && \
  sudo cp arch/arm64/rpi_boot/dts/overlays/README mnt/rpi_boot/overlays/ && \
  sudo umount mnt/rpi_boot && \
  sudo umount mnt/rpi_root && \
  cd $PROJECT_DIR
fi

# install source libraries
cd lib && \
bash install.sh && \
cd $PROJECT_DIR

# build droneOS
