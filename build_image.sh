#!/usr/bin/env bash

# usage:
# bash build_image.sh sd# kernel8 drone username userpassword ssid ssidpassword

# input parameters
# lsblk will let you find the value for this parameter:
SD_CARD=${1:-""}
# [kernel, kernel7l, kernel8]
KERNEL=${2:-"kernel"}
# [base, drone]
TYPE=${3:-"drone"}
# login user
USER_NAME=${4:-"admin"}
USER_PASSWORD=${5:-"adminpassword"}
# wifi credentials
SSID=${6:-"droneos"}
SSID_PASSWORD=${7:-"X0YhW2Wy2bmtKXkT2ST61v2SdBk4FGgE"}

# local variables
THREADS=4
PROJECT_DIR=$PWD
BUILD_DIR=$PROJECT_DIR/build
RPI_LINUX_BRANCH=rpi-6.6.y
SD_CARD_BOOT_DEVICE="${SD_CARD}1"
SD_CARD_ROOT_DEVICE="${SD_CARD}2"
SD_CARD_BOOT_DIR=$BUILD_DIR/linux/mnt/rpi_boot
SD_CARD_ROOT_DIR=$BUILD_DIR/linux/mnt/rpi_root
INSTALL_DIR=${SD_CARD_ROOT_DIR}/opt/droneOS
if [[ $KERNEL == "kernel8" ]]; then
  ARM=aarch64
else
  ARM=arm
fi

# get kernel source and configure
if ! [ -d "build/linux" ]; then
  echo "downloading Linux source..."
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
    echo "downloading real-time kernel patch..."
    wget https://cdn.kernel.org/pub/linux/kernel/projects/rt/6.6/older/patches-6.6.48-rt40.tar.gz
  fi
  echo "extracting real-time kernel patch"
  tar -xf patches-6.6.48-rt40.tar.gz && \

  echo "applying real-time kernel patch..."
  cd linux && \
  git am -3 ../patches/*
  git am --skip
  cd "$PROJECT_DIR"
fi

# set up sd card
echo "setting up sd card..."
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
if ! [ -f "$IMAGE_FILE_XZ" ]; then
  echo "downloading image..."
  wget "$IMAGE_URL"
fi
if ! [ -f "$IMAGE_FILE" ]; then
  echo "decompressing image..."
  xz --threads=${THREADS} --keep -d "$IMAGE_FILE_XZ"
fi
echo "writing image to sd card..."
sudo dd bs=1M if="$IMAGE_FILE" of=/dev/"${SD_CARD}" status=progress

echo "filesystem and user configuration..."
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
#sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/chown -Rv "${USER_NAME}":"${USER_NAME}" "${SD_CARD_ROOT_DIR}"/home/"${USER_NAME}"
# enable ssh
sudo touch "${SD_CARD_BOOT_DIR}"/ssh

echo "setting up wifi network..."
if [[ $TYPE == "base" ]]; then
  UNIT_FILE=$(cat <<EOF
[Unit]
Description=Start droneOS wifi network
Requires=NetworkManager.service sys-subsystem-net-devices-wlan0.service
After=NetworkManager.service sys-subsystem-net-devices-wlan0.service

[Service]
ExecStart=/usr/bin/nmcli device wifi hotspot ssid ${SSID} password ${SSID_PASSWORD}
Type=oneshot
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF
)
  echo "$UNIT_FILE" | sudo tee "${SD_CARD_ROOT_DIR}"/etc/systemd/system/droneOSNetwork.service
  # chroot to filesystem and enable wifi startup script
  sudo cp /usr/bin/qemu-${ARM}-static "${SD_CARD_ROOT_DIR}"/usr/bin/ && \
  sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /bin/bash -c 'systemctl enable droneOSNetwork.service'
elif [[ $TYPE == "drone" ]]; then
  UUID=$(uuidgen)
  sudo mkdir -p "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections
NM_FILE=$(cat <<EOF
[connection]
id=droneOS
uuid=${UUID}
type=wifi
interface-name=wlan0
autoconnect=true
autoconnect-retries=0

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
method=auto
EOF
)
  echo "$NM_FILE" | sudo tee "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections/"${SSID}".nmconnection && \
  sudo chmod -Rv 600 "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections/"${SSID}".nmconnection && \
  sudo chown -Rv root:root "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections/"${SSID}".nmconnection
fi
cd "$PROJECT_DIR"

echo "building Linux kernel..."
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

echo "building droneOS binary..."
if [[ $ARM == "aarch64" ]]; then
  bash build.sh ${TYPE} arm64
elif [[ $ARM == "arm" ]]; then
  bash build.sh ${TYPE} arm
fi
# install droneOS binary and config
echo "installing droneOS binary and config..."
sudo mkdir -p "$INSTALL_DIR" && \
sudo cp ${TYPE}.bin "$INSTALL_DIR" && \
sudo cp configs/config.yaml "$INSTALL_DIR"

echo "setting up systemd unit file..."
if [[ $TYPE == "base" ]]; then
  UNIT_FILE=$(cat <<'EOF'
[Unit]
Description=Start droneOS application
Requires=droneOSNetwork.service
After=droneOSNetwork.service

[Service]
Type=notify
WorkingDirectory=/opt/droneOS/
ExecStart=/opt/droneOS/base.bin --config-file config.yaml
ExecReload=/bin/kill -s HUP $MAINPID
TimeoutStartSec=0
RestartSec=1
Restart=always

[Install]
WantedBy=multi-user.target
EOF
)
elif [[ $TYPE == "drone" ]]; then
  UNIT_FILE=$(cat <<'EOF'
[Unit]
Description=Start droneOS application
Requires=network-online.service
After=network-online.service

[Service]
Type=notify
WorkingDirectory=/opt/droneOS/
ExecStart=/opt/droneOS/drone.bin --config-file config.yaml
ExecReload=/bin/kill -s HUP $MAINPID
TimeoutStartSec=0
RestartSec=1
Restart=always

[Install]
WantedBy=multi-user.target
EOF
)
fi
echo "$UNIT_FILE" | sudo tee "${SD_CARD_ROOT_DIR}"/etc/systemd/system/droneOS.service && \
# chroot to filesystem and enable wifi startup script
sudo cp /usr/bin/qemu-${ARM}-static "${SD_CARD_ROOT_DIR}"/usr/bin/ && \
sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /bin/bash -c 'systemctl enable droneOS.service'

# cleanup
sudo umount /dev/"${SD_CARD_BOOT_DEVICE}"
sudo umount /dev/"${SD_CARD_ROOT_DEVICE}"
