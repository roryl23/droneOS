#!/usr/bin/env bash

# usage:
# bash build_image.sh sda kernel base username userpassword ssid ssidpassword

# input parameters
# lsblk will let you find the value for this parameter:
SD_CARD=${1:-""}
# [kernel, kernel7l, kernel8]
KERNEL=${2:-"kernel"}
# [base, drone]
TYPE=${3:-"base"}
# login user
USER_NAME=${4:-"admin"}
USER_PASSWORD=${5:-"adminpassword"}
# wifi credentials
SSID=${6:-"droneos"}
SSID_PASSWORD=${7:-"X0YhW2Wy2bmtKXkT2ST61v2SdBk4FGgE"}

# local variables
THREADS=8
PROJECT_DIR=$PWD
BUILD_DIR=$PROJECT_DIR/build
RPI_LINUX_BRANCH=rpi-6.6.y
SD_CARD_BOOT_DEVICE="${SD_CARD}1"
SD_CARD_ROOT_DEVICE="${SD_CARD}2"
SD_CARD_BOOT_DIR=$BUILD_DIR/linux/mnt/rpi_boot
SD_CARD_ROOT_DIR=$BUILD_DIR/linux/mnt/rpi_root
INSTALL_DIR=$SD_CARD_ROOT_DIR/opt/droneOS

# get kernel source and configure
if ! [ -d "build/linux" ]; then
  mkdir -p build && \
  cd "${BUILD_DIR}" && \
  git clone --depth=1 https://github.com/raspberrypi/linux && \
  git checkout $RPI_LINUX_BRANCH
  cd "$PROJECT_DIR"
fi
# get real time kernel patch
if ! [ -d "build/patches" ]; then
  cd "${BUILD_DIR}"
  if ! [ -f "patches-6.6.48-rt40.tar.gz" ]; then
    wget https://cdn.kernel.org/pub/linux/kernel/projects/rt/6.6/patches-6.6.48-rt40.tar.gz
  fi
  tar -xvf patches-6.6.48-rt40.tar.gz && \
  # apply real time kernel patch
  cd linux && \
  git am -3 ../patches/*
  git am --skip
  cd "$PROJECT_DIR"
fi

# set up sd card
sudo parted /dev/"${SD_CARD}" --script <<EOF
mklabel msdos
y
mkpart primary fat32 1MiB 100%
set 1 lba on
EOF
sudo mkfs.ext4 -F /dev/"${SD_CARD_ROOT_DEVICE}"
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
  if [[ "$TYPE" == "base" ]]; then
    IMAGE_URL="https://downloads.raspberrypi.com/raspios_arm64/images/raspios_arm64-2024-07-04/2024-07-04-raspios-bookworm-arm64.img.xz"
    IMAGE_FILE_XZ="2024-07-04-raspios-bookworm-arm64.img.xz"
    IMAGE_FILE="2024-07-04-raspios-bookworm-arm64.img"
  elif [[ "$TYPE" == "drone" ]]; then
    IMAGE_URL="https://downloads.raspberrypi.com/raspios_lite_arm64/images/raspios_lite_arm64-2024-07-04/2024-07-04-raspios-bookworm-arm64-lite.img.xz"
    IMAGE_FILE_XZ="2024-07-04-raspios-bookworm-arm64-lite.img.xz"
    IMAGE_FILE="2024-07-04-raspios-bookworm-arm64-lite.img"
  fi
fi
# fetch, decompress, and write image
if ! [ -f "$IMAGE_FILE_XZ" ]; then
  wget "$IMAGE_URL"
fi
if ! [ -f "$IMAGE_FILE" ]; then
  xz --threads=${THREADS} --keep -v -d "$IMAGE_FILE_XZ"
fi
sudo dd bs=1M if="$IMAGE_FILE" of=/dev/"${SD_CARD}" status=progress

# filesystem and user configuration
cd "${BUILD_DIR}"/linux && \
mkdir -p "${SD_CARD_BOOT_DIR}" && \
mkdir -p "${SD_CARD_ROOT_DIR}" && \
sudo mount /dev/"${SD_CARD_BOOT_DEVICE}" "${SD_CARD_BOOT_DIR}" && \
sudo mount /dev/"${SD_CARD_ROOT_DEVICE}" "${SD_CARD_ROOT_DIR}" && \
# set up user to avoid booting into userconfig on first boot
PASSWORD_ENCRYPTED=$(echo "$USER_PASSWORD" | openssl passwd -6 -stdin)
echo "${USER_NAME}:${PASSWORD_ENCRYPTED}" | sudo tee "${SD_CARD_BOOT_DIR}"/userconf.txt && \
sudo mkdir -p "${SD_CARD_ROOT_DIR}"/home/"${USER_NAME}" && \
# set home directory permissions
sudo chown -Rv "${USER_NAME}":"${USER_NAME}" "$SD_CARD_ROOT_DIR"/home/"${USER_NAME}"

# set up wifi network
if [[ $TYPE == "base" ]]; then
  # copy wifi init script to filesystem
  echo "[Unit]
Description=Start droneOS wifi network
Requires=NetworkManager.service sys-subsystem-net-devices-wlan0.service
After=NetworkManager.service sys-subsystem-net-devices-wlan0.service

[Service]
ExecStart=/usr/bin/nmcli device wifi hotspot ssid $SSID password $SSID_PASSWORD
Type=oneshot
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target" | sudo tee "$SD_CARD_ROOT_DIR"/etc/systemd/system/droneOSNetwork.service
  # chroot to filesystem and enable wifi startup script
  if [[ $KERNEL == "kernel8" ]]; then
    ARM=aarch64
  else
    ARM=arm
  fi
  sudo cp /usr/bin/qemu-${ARM}-static "$SD_CARD_ROOT_DIR"/usr/bin/ && \
  sudo chroot "$SD_CARD_ROOT_DIR" /usr/bin/qemu-${ARM}-static /bin/bash -c 'systemctl enable droneOSNetwork.service'
elif [[ $TYPE == "drone" ]]; then
  sudo mkdir -p "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections && \
  echo "[connection]
id=${SSID}
uuid=
type=wifi
interface-name=wlan0
autoconnect=true

[wifi]
mode=infrastructure
ssid=${SSID}

[wifi-security]
auth-alg=open
key-mgmt=wpa-psk
psk=${SSID_PASSWORD}

[ipv4]
method=auto

[ipv6]
method=auto" | sudo tee "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections/"${SSID}".nmconnection && \
  sudo chmod -Rv 600 "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections/"${SSID}".nmconnection && \
  sudo chown -Rv root:root "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections/"${SSID}".nmconnection
fi
cd "$PROJECT_DIR"

# build kernel
if [ -d "build/linux" ]; then
  # add configs for kernel build
  cd "${BUILD_DIR}"/linux && \
  cp "$PROJECT_DIR"/configs/.config . && \
  sudo mkdir -p "${SD_CARD_BOOT_DIR}"/overlays/ && \
  sudo cp "$PROJECT_DIR"/configs/config-"${KERNEL}".txt "${SD_CARD_BOOT_DIR}"
  # compile and install kernel
  if [[ "$KERNEL" == "kernel" ]]; then
    make -j${THREADS} KERNEL="$KERNEL" ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- bcmrpi_defconfig && \
    make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- Image modules dtbs && \
    sudo env PATH="$PATH" make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- INSTALL_MOD_PATH="${SD_CARD_ROOT_DIR}" modules_install && \
    sudo cp arch/arm/boot/dts/broadcom/*.dtb "${SD_CARD_BOOT_DIR}"/ && \
    sudo cp arch/arm/boot/dts/overlays/*.dtb* "${SD_CARD_BOOT_DIR}"/overlays/ && \
    sudo cp arch/arm/boot/dts/overlays/README "${SD_CARD_BOOT_DIR}"/overlays/
  elif [[ "$KERNEL" == "kernel7" ]]; then
    make -j${THREADS} KERNEL="$KERNEL" ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- bcm2711_defconfig && \
    make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- Image modules dtbs && \
    sudo env PATH="$PATH" make -j${THREADS} ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- INSTALL_MOD_PATH="${SD_CARD_ROOT_DIR}" modules_install && \
    sudo cp arch/arm/boot/dts/broadcom/*.dtb "${SD_CARD_BOOT_DIR}"/ && \
    sudo cp arch/arm/boot/dts/overlays/*.dtb* "${SD_CARD_BOOT_DIR}"/overlays/ && \
    sudo cp arch/arm/boot/dts/overlays/README "${SD_CARD_BOOT_DIR}"/overlays/
  elif [[ "$KERNEL" == "kernel8" ]]; then
    make -j${THREADS} KERNEL="$KERNEL" ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- bcm2711_defconfig && \
    make -j${THREADS} ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- Image modules dtbs && \
    sudo env PATH="$PATH" make -j${THREADS} ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- INSTALL_MOD_PATH="${SD_CARD_ROOT_DIR}" modules_install && \
    sudo cp arch/arm64/boot/Image "${SD_CARD_BOOT_DIR}"/"$KERNEL".img && \
    sudo cp arch/arm64/boot/dts/broadcom/*.dtb "${SD_CARD_BOOT_DIR}"/ && \
    sudo cp arch/arm64/boot/dts/overlays/*.dtb* "${SD_CARD_BOOT_DIR}"/overlays/ && \
    sudo cp arch/arm64/boot/dts/overlays/README "${SD_CARD_BOOT_DIR}"/overlays/
  fi
  cd "$PROJECT_DIR"
fi

## build droneOS
#bash build.sh && \
## install droneOS
#mkdir -p "$INSTALL_DIR" && \
#cp droneOS.bin "$INSTALL_DIR" && \
#cp configs/config.yaml "$INSTALL_DIR" && \
#sudo cp configs/droneOS.service "$SD_CARD_ROOT_DIR"/lib/systemd/system/

# cleanup
sudo umount /dev/"${SD_CARD_BOOT_DEVICE}"
sudo umount /dev/"${SD_CARD_ROOT_DEVICE}"
