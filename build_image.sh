#!/usr/bin/env bash
set -euo pipefail

# usage:
# bash build_image.sh sd# kernel8 drone username userpassword ssid ssidpassword wifi_country

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
WIFI_COUNTRY=${8:-"US"}

# local variables
THREADS=16
PROJECT_DIR=$PWD
BUILD_DIR=$PROJECT_DIR/build
RPI_LINUX_BRANCH=rpi-6.6.y
RPI_OS_DATE=${RPI_OS_DATE:-2024-07-04}
RPI_OS_SERIES=${RPI_OS_SERIES:-bookworm}
RPI_OS_FLAVOR_BASE=${RPI_OS_FLAVOR_BASE:-raspios}
RPI_OS_FLAVOR_DRONE=${RPI_OS_FLAVOR_DRONE:-raspios}
RPI_OS_CACHE_DIR=${RPI_OS_CACHE_DIR:-/tmp}
RT_PATCH_VERSION=6.6.78-rt51
RT_PATCH_BASE="${RT_PATCH_VERSION%-rt*}"
RT_PATCH_TARBALL="patches-${RT_PATCH_VERSION}.tar.gz"
RT_PATCH_URL="https://cdn.kernel.org/pub/linux/kernel/projects/rt/6.6/older/${RT_PATCH_TARBALL}"
RT_PATCH_DIR="${BUILD_DIR}/patches"
RT_PATCH_EXTRACT_MARKER="${RT_PATCH_DIR}/.rt_patch_version"
RT_PATCH_MARKER="${BUILD_DIR}/linux/.rt_patched_${RT_PATCH_VERSION}"
SD_CARD_BOOT_DEVICE="${SD_CARD}1"
SD_CARD_ROOT_DEVICE="${SD_CARD}2"
MOUNT_BASE=${MOUNT_BASE:-/tmp/droneos_mnt}
SD_CARD_BOOT_DIR="${MOUNT_BASE}/rpi_boot"
SD_CARD_ROOT_DIR="${MOUNT_BASE}/rpi_root"
INSTALL_DIR=${SD_CARD_ROOT_DIR}/opt/droneOS
if [[ $KERNEL == "kernel8" ]]; then
  ARM=aarch64
else
  ARM=arm
fi

cleanup_on_exit() {
  local status=$?
  if [[ $status -ne 0 ]]; then
    echo "cleanup: script failed (exit ${status}); leaving mounts in place."
    return
  fi
  echo "cleanup: syncing and unmounting /dev/${SD_CARD_BOOT_DEVICE} and /dev/${SD_CARD_ROOT_DEVICE}..."
  sudo sync
  sudo umount "/dev/${SD_CARD_BOOT_DEVICE}" 2>/dev/null || true
  sudo umount "/dev/${SD_CARD_ROOT_DEVICE}" 2>/dev/null || true
}

trap cleanup_on_exit EXIT

# get kernel source and configure
if ! [ -d "${BUILD_DIR}/linux/.git" ]; then
  if [ -d "${BUILD_DIR}/linux" ]; then
    echo "build/linux exists but is not a git repo; remove it to re-clone"
    exit 1
  fi
  echo "downloading Linux source..."
  mkdir -p "${BUILD_DIR}" && \
  git clone --depth=1 --branch "${RPI_LINUX_BRANCH}" https://github.com/raspberrypi/linux "${BUILD_DIR}/linux"
  cd "$PROJECT_DIR"
else
  CURRENT_LINUX_BRANCH=$(git -C "${BUILD_DIR}/linux" rev-parse --abbrev-ref HEAD)
  if [[ "${CURRENT_LINUX_BRANCH}" != "${RPI_LINUX_BRANCH}" ]]; then
    echo "linux source is on ${CURRENT_LINUX_BRANCH}; expected ${RPI_LINUX_BRANCH}. Remove build/linux or update RPI_LINUX_BRANCH."
    exit 1
  fi
fi
# get real time kernel patch
if ! [ -f "${RT_PATCH_MARKER}" ]; then
  cd "${BUILD_DIR}"
  if ! [ -f "${RT_PATCH_TARBALL}" ]; then
    echo "downloading real-time kernel patch..."
    wget "${RT_PATCH_URL}"
  fi
  if [ -d "${RT_PATCH_DIR}" ]; then
    if [ ! -f "${RT_PATCH_EXTRACT_MARKER}" ] || [ "$(cat "${RT_PATCH_EXTRACT_MARKER}")" != "${RT_PATCH_VERSION}" ]; then
      rm -rf "${RT_PATCH_DIR}"
    fi
  fi
  if ! [ -d "${RT_PATCH_DIR}" ]; then
    echo "extracting real-time kernel patch"
    tar -xf "${RT_PATCH_TARBALL}"
    echo "${RT_PATCH_VERSION}" > "${RT_PATCH_EXTRACT_MARKER}"
  fi

  echo "applying real-time kernel patch..."
  cd linux
  KERNEL_VERSION=$(make -s kernelversion)
  if [[ "${KERNEL_VERSION}" != "${RT_PATCH_BASE}" ]]; then
    echo "kernel version ${KERNEL_VERSION} does not match RT patch base ${RT_PATCH_BASE}; update RT_PATCH_VERSION or RPI_LINUX_BRANCH"
    exit 1
  fi
  if ! git am "${RT_PATCH_DIR}"/*.patch; then
    git am --abort || true
    echo "real-time kernel patch failed to apply"
    exit 1
  fi
  touch "${RT_PATCH_MARKER}"
  cd "$PROJECT_DIR"
fi

# determine which image to get
if [[ "$KERNEL" == "kernel" || "$KERNEL" == "kernel7l" ]]; then
  if [[ "$TYPE" == "base" ]]; then
    IMAGE_ARCH_DIR="raspios_arm64"
    IMAGE_ARCH_SUFFIX="arm64"
    IMAGE_FLAVOR="${RPI_OS_FLAVOR_BASE}"
  elif [[ "$TYPE" == "drone" ]]; then
    IMAGE_ARCH_DIR="raspios_lite_armhf"
    IMAGE_ARCH_SUFFIX="armhf-lite"
    IMAGE_FLAVOR="${RPI_OS_FLAVOR_DRONE}"
  fi
elif [[ "$KERNEL" == "kernel8" ]]; then
  if [[ "$TYPE" == "base" ]]; then
    IMAGE_ARCH_DIR="raspios_arm64"
    IMAGE_ARCH_SUFFIX="arm64"
    IMAGE_FLAVOR="${RPI_OS_FLAVOR_BASE}"
  elif [[ "$TYPE" == "drone" ]]; then
    IMAGE_ARCH_DIR="raspios_lite_arm64"
    IMAGE_ARCH_SUFFIX="arm64-lite"
    IMAGE_FLAVOR="${RPI_OS_FLAVOR_DRONE}"
  fi
fi
IMAGE_FILE_XZ="${RPI_OS_DATE}-${IMAGE_FLAVOR}-${RPI_OS_SERIES}-${IMAGE_ARCH_SUFFIX}.img.xz"
IMAGE_FILE="${IMAGE_FILE_XZ%.xz}"
IMAGE_URL="https://downloads.raspberrypi.com/${IMAGE_ARCH_DIR}/images/${IMAGE_ARCH_DIR}-${RPI_OS_DATE}/${IMAGE_FILE_XZ}"

if ! [ -f "${RPI_OS_CACHE_DIR}/${IMAGE_FILE_XZ}" ]; then
  echo "downloading image..."
  wget -O "${RPI_OS_CACHE_DIR}/${IMAGE_FILE_XZ}" "${IMAGE_URL}"
fi
if ! [ -f "${RPI_OS_CACHE_DIR}/${IMAGE_FILE}" ]; then
  echo "decompressing image..."
  xz --threads=${THREADS} --keep -d "${RPI_OS_CACHE_DIR}/${IMAGE_FILE_XZ}"
fi
echo "writing image to sd card..."
sudo dd bs=1M if="${RPI_OS_CACHE_DIR}/${IMAGE_FILE}" of=/dev/"${SD_CARD}" status=progress conv=fsync
sudo partprobe /dev/"${SD_CARD}"
sudo udevadm settle
if [[ ! -b /dev/"${SD_CARD_BOOT_DEVICE}" || ! -b /dev/"${SD_CARD_ROOT_DEVICE}" ]]; then
  echo "partition devices not found after imaging: /dev/${SD_CARD_BOOT_DEVICE} /dev/${SD_CARD_ROOT_DEVICE}"
  exit 1
fi

echo "building droneOS binary..."
if [[ $ARM == "aarch64" ]]; then
  bash build.sh ${TYPE} arm64
elif [[ $ARM == "arm" ]]; then
  bash build.sh ${TYPE} arm
fi

echo "filesystem and user configuration..."
cd "${BUILD_DIR}"/linux && \
mkdir -p "${SD_CARD_BOOT_DIR}" && \
mkdir -p "${SD_CARD_ROOT_DIR}" && \
sudo mount /dev/"${SD_CARD_BOOT_DEVICE}" "${SD_CARD_BOOT_DIR}" && \
sudo mount /dev/"${SD_CARD_ROOT_DEVICE}" "${SD_CARD_ROOT_DIR}" && \
# create user in rootfs to avoid first-boot user setup
PASSWORD_ENCRYPTED=$(echo "$USER_PASSWORD" | openssl passwd -6 -stdin)
# preseed first-boot user configuration to avoid rename prompts
echo "${USER_NAME}:${PASSWORD_ENCRYPTED}" | sudo tee "${SD_CARD_BOOT_DIR}"/userconf.txt >/dev/null
sudo cp /usr/bin/qemu-${ARM}-static "${SD_CARD_ROOT_DIR}"/usr/bin/
if ! sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /usr/bin/id -u "${USER_NAME}" >/dev/null 2>&1; then
  if ! sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /usr/sbin/useradd -m -s /bin/bash -G sudo "${USER_NAME}"; then
    echo "warning: useradd failed; falling back to userconf.txt for first boot"
    echo "${USER_NAME}:${PASSWORD_ENCRYPTED}" | sudo tee "${SD_CARD_BOOT_DIR}"/userconf.txt >/dev/null
  fi
fi
if sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /usr/bin/id -u "${USER_NAME}" >/dev/null 2>&1; then
  printf '%s:%s\n' "${USER_NAME}" "${PASSWORD_ENCRYPTED}" | \
    sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /usr/sbin/chpasswd -e
  sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /usr/bin/chown -Rv "${USER_NAME}":"${USER_NAME}" /home/"${USER_NAME}"
else
  # If user creation failed, ensure userconf.txt exists for first boot.
  echo "${USER_NAME}:${PASSWORD_ENCRYPTED}" | sudo tee "${SD_CARD_BOOT_DIR}"/userconf.txt >/dev/null
fi
# disable first-boot user prompts if present
sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /bin/bash -c \
  'systemctl disable userconf-pi.service userconf.service 2>/dev/null || true'
# enable ssh
sudo touch "${SD_CARD_BOOT_DIR}"/ssh

echo "setting wifi country..."
sudo mkdir -p "${SD_CARD_ROOT_DIR}"/etc/modprobe.d
echo "REGDOMAIN=${WIFI_COUNTRY}" | sudo tee "${SD_CARD_ROOT_DIR}"/etc/default/crda
echo "options cfg80211 ieee80211_regdom=${WIFI_COUNTRY}" | sudo tee "${SD_CARD_ROOT_DIR}"/etc/modprobe.d/cfg80211.conf

echo "setting up wifi network..."
if [[ $TYPE == "base" ]]; then
  UNIT_FILE=$(cat <<EOF
[Unit]
Description=Start droneOS wifi network
Wants=NetworkManager.service
Requires=sys-subsystem-net-devices-wlan0.device
After=NetworkManager.service sys-subsystem-net-devices-wlan0.device

[Service]
Type=oneshot
ExecStartPre=/usr/bin/nmcli radio wifi on
ExecStartPre=/usr/bin/nmcli device set wlan0 managed yes
ExecStart=/usr/bin/nmcli device wifi hotspot ifname wlan0 ssid "${SSID}" password "${SSID_PASSWORD}"
RemainAfterExit=yes
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
)
  echo "$UNIT_FILE" | sudo tee "${SD_CARD_ROOT_DIR}"/etc/systemd/system/droneOSNetwork.service
  # chroot to filesystem and enable wifi startup script
  sudo cp /usr/bin/qemu-${ARM}-static "${SD_CARD_ROOT_DIR}"/usr/bin/ && \
  sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /bin/bash -c 'systemctl enable NetworkManager.service' && \
  sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /bin/bash -c 'systemctl enable droneOSNetwork.service' && \
  sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /bin/bash -c 'systemctl enable ssh'
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
autoconnect-priority=100

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
method=ignore
EOF
)
  echo "$NM_FILE" | sudo tee "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections/"${SSID}".nmconnection && \
  sudo chmod -Rv 600 "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections/"${SSID}".nmconnection && \
  sudo chown -Rv root:root "${SD_CARD_ROOT_DIR}"/etc/NetworkManager/system-connections/"${SSID}".nmconnection
  sudo cp /usr/bin/qemu-${ARM}-static "${SD_CARD_ROOT_DIR}"/usr/bin/ && \
  sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /bin/bash -c 'systemctl enable NetworkManager.service' && \
  sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /bin/bash -c 'systemctl enable ssh'
fi

echo "setting up IP print service..."
IP_UNIT_FILE=$(cat <<'EOF'
[Unit]
Description=Print droneOS IP on console
After=network-online.target NetworkManager.service
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/bin/bash -c 'ip=$(/usr/bin/nmcli -t -f IP4.ADDRESS dev show wlan0 | /usr/bin/head -n1 | /usr/bin/cut -d: -f2 | /usr/bin/cut -d/ -f1); echo "droneOS IP: ${ip:-unknown}" | /usr/bin/tee /dev/tty1'

[Install]
WantedBy=multi-user.target
EOF
)
echo "$IP_UNIT_FILE" | sudo tee "${SD_CARD_ROOT_DIR}"/etc/systemd/system/droneOSPrintIP.service
sudo cp /usr/bin/qemu-${ARM}-static "${SD_CARD_ROOT_DIR}"/usr/bin/ && \
sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /bin/bash -c 'systemctl enable droneOSPrintIP.service'

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
sudo cp /usr/bin/qemu-${ARM}-static "${SD_CARD_ROOT_DIR}"/usr/bin/
# TODO: this should be done in production mode, but not development
#sudo chroot "${SD_CARD_ROOT_DIR}" /usr/bin/qemu-${ARM}-static /bin/bash -c 'systemctl enable droneOS.service'

# cleanup handled by trap on successful exit
