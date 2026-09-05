#!/usr/bin/env bash
set -euo pipefail
PROJECT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
IMAGE_ENV_FILE="${PROJECT_DIR}/.image.env"
if [[ -e "$IMAGE_ENV_FILE" || -L "$IMAGE_ENV_FILE" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "$IMAGE_ENV_FILE"
  set +a
fi


usage() {
  cat >&2 <<'EOF'
usage:
  bash build_image.sh sd# kernel8 drone [hostname] [username] [userpassword] [ssid] [ssidpassword] [wifi_country]
  bash build_image.sh sd# kernel8 drone username userpassword ssid ssidpassword wifi_country

examples:
  bash build_image.sh sdb kernel8 drone droneos
  BUILD_MODE=dev bash build_image.sh /dev/sdb kernel8 drone droneos admin password MySSID MyPass123 US
  BUILD_MODE=dev bash build_image.sh /dev/sdb kernel8 drone admin password MySSID MyPass123 US

environment:
  BUILD_MODE=prod|dev        prod enables droneOS on boot (default: prod)
                             dev leaves droneOS disabled and enables WiFi/SSH
  ALPINE_BRANCH=latest-stable
  ALPINE_VERSION=3.20.3      exact version; set ALPINE_BRANCH=v3.20 for this release
  ALPINE_TARBALL=/path/file  optional local alpine-rpi tarball
  ALPINE_TARBALL_URL=https://...
  ALPINE_ARCH=aarch64        optional override: aarch64, armv7, or armhf
  APK_FETCH_CONTAINER_IMAGE=alpine:latest
                             container image used to fetch dev APKs when host apk is unavailable
  IMAGE_HOSTNAME=droneos     hostname when using the old 8-argument form
  DEV_USER_NAME=admin        dev SSH user
  DEV_USER_PASSWORD=...      dev SSH user password
  WIFI_SSID=droneos          dev WiFi SSID
  WIFI_PASSWORD=...          dev WPA passphrase, 8-63 characters
  WIFI_COUNTRY=US            regulatory country code
  DISABLE_WIFI=1             add Raspberry Pi disable-wifi overlay (default: prod=1, dev=0)
  DISABLE_BLUETOOTH=1        add Raspberry Pi disable-bt overlay (default: 1)
  ENABLE_UART_CONSOLE=1      add serial console (default: prod=0, dev=1)
  UART_CONSOLE_TTY=ttyAMA0   serial console device for GPIO14/GPIO15 UART
  UART_CONSOLE_EXTRA_TTYS=ttyS0
                             extra serial console TTYs (default: dev=ttyS0, prod empty)
  UART_CONSOLE_BAUD=115200   serial console baud rate
EOF
}

require_command() {
  local name=$1
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "required command not found: ${name}" >&2
    exit 1
  fi
}

partition_path() {
  local device=$1
  local number=$2
  if [[ "$device" =~ [0-9]$ ]]; then
    printf '%sp%s\n' "$device" "$number"
  else
    printf '%s%s\n' "$device" "$number"
  fi
}

normalize_device() {
  local value=$1
  if [[ "$value" == /dev/* ]]; then
    printf '%s\n' "$value"
  else
    printf '/dev/%s\n' "$value"
  fi
}

append_once() {
  local file=$1
  local line=$2
  if [[ ! -f "$file" ]] || ! grep -Fxq "$line" "$file"; then
    printf '%s\n' "$line" | "${SUDO[@]}" tee -a "$file" >/dev/null
  fi
}

append_cmdline_arg() {
  local file=$1
  local arg=$2
  local key=${arg%%=*}

  [[ -f "$file" ]] || return 0
  if ! grep -Eq "(^|[[:space:]])${key}=" "$file"; then
    "${SUDO[@]}" sed -i "1s|$| ${arg}|" "$file"
  fi
}

append_cmdline_token_once() {
  local file=$1
  local token=$2
  local content

  [[ -f "$file" ]] || return 0
  content=$(<"$file")
  if [[ " $content " != *" $token "* ]]; then
    "${SUDO[@]}" sed -i "1s|$| ${token}|" "$file"
  fi
}

remove_cmdline_token() {
  local file=$1
  local token=$2

  [[ -f "$file" ]] || return 0
  "${SUDO[@]}" sed -i -E "s/(^|[[:space:]])${token}([[:space:]]|$)/ /g; s/^[[:space:]]+//; s/[[:space:]]+$//; s/[[:space:]]+/ /g" "$file"
}

quote_sh() {
  local value=${1//\'/\'\\\'\'}
  printf "'%s'" "$value"
}

write_shell_var() {
  local file=$1
  local name=$2
  local value=$3
  printf '%s=%s\n' "$name" "$(quote_sh "$value")" >> "$file"
}

wpa_quote() {
  local value=$1
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  printf '%s' "$value"
}

find_host_ssh_key() {
  local key_path

  HOST_SSH_KEY=""
  for key_path in \
    "${HOME}/.ssh/id_ed25519.pub" \
    "${HOME}/.ssh/id_rsa.pub" \
    "${HOME}/.ssh/id_ecdsa.pub"; do
    if [[ -f "$key_path" ]]; then
      HOST_SSH_KEY=$(<"$key_path")
      return
    fi
  done

  echo "no SSH key found on host, generating ${HOME}/.ssh/id_ed25519..."
  mkdir -p "${HOME}/.ssh"
  ssh-keygen -t ed25519 -f "${HOME}/.ssh/id_ed25519" -N "" -C "$(whoami)@$(hostname)"
  HOST_SSH_KEY=$(<"${HOME}/.ssh/id_ed25519.pub")
}

resolve_alpine_tarball() {
  local release_dir image_file listing

  if [[ -n "${ALPINE_TARBALL:-}" ]]; then
    if [[ ! -f "$ALPINE_TARBALL" ]]; then
      echo "ALPINE_TARBALL does not exist: ${ALPINE_TARBALL}" >&2
      exit 1
    fi
    ALPINE_IMAGE_FILE=$(basename "$ALPINE_TARBALL")
    ALPINE_IMAGE_PATH=$ALPINE_TARBALL
    return
  fi

  if [[ -n "${ALPINE_TARBALL_URL:-}" ]]; then
    ALPINE_IMAGE_URL=$ALPINE_TARBALL_URL
    ALPINE_IMAGE_FILE=$(basename "$ALPINE_IMAGE_URL")
  elif [[ -n "${ALPINE_VERSION:-}" ]]; then
    ALPINE_IMAGE_FILE="alpine-rpi-${ALPINE_VERSION}-${ALPINE_ARCH}.tar.gz"
    ALPINE_IMAGE_URL="${ALPINE_MIRROR}/${ALPINE_BRANCH}/releases/${ALPINE_ARCH}/${ALPINE_IMAGE_FILE}"
  else
    release_dir="${ALPINE_MIRROR}/${ALPINE_BRANCH}/releases/${ALPINE_ARCH}"
    echo "resolving latest Alpine Raspberry Pi image from ${release_dir}..."
    listing=$(wget -qO- "${release_dir}/")
    image_file=$(printf '%s\n' "$listing" \
      | sed -nE "s/.*href=\"(alpine-rpi-[^\"]+-${ALPINE_ARCH}\.tar\.gz)\".*/\1/p" \
      | sort -V \
      | tail -n 1)
    if [[ -z "$image_file" ]]; then
      echo "could not find alpine-rpi tarball for ${ALPINE_ARCH}; set ALPINE_VERSION or ALPINE_TARBALL_URL" >&2
      exit 1
    fi
    ALPINE_IMAGE_FILE=$image_file
    ALPINE_IMAGE_URL="${release_dir}/${ALPINE_IMAGE_FILE}"
  fi

  mkdir -p "$ALPINE_CACHE_DIR"
  ALPINE_IMAGE_PATH="${ALPINE_CACHE_DIR}/${ALPINE_IMAGE_FILE}"
  if [[ ! -f "$ALPINE_IMAGE_PATH" ]]; then
    echo "downloading ${ALPINE_IMAGE_URL}..."
    wget -O "$ALPINE_IMAGE_PATH" "$ALPINE_IMAGE_URL"
  fi
}

fetch_dev_apks() {
  local main_repo="${ALPINE_MIRROR}/${ALPINE_BRANCH}/main"
  local community_repo="${ALPINE_MIRROR}/${ALPINE_BRANCH}/community"
  local packages=(openssl openssh rsync go iw wireless-regdb)
  local container_image="${APK_FETCH_CONTAINER_IMAGE:-alpine:latest}"
  local container_apk_script

  if [[ "$TYPE" == "base" ]]; then
    packages+=(hostapd dnsmasq)
  else
    packages+=(wpa_supplicant)
  fi

  # shellcheck disable=SC2016
  container_apk_script='
set -e
printf "%s\n%s\n" "$1" "$2" > /etc/apk/repositories
shift 2
arch=$1
shift
apk --allow-untrusted --arch "$arch" update
apk --allow-untrusted --arch "$arch" fetch --recursive --output /out "$@"
'

  mkdir -p "$DEV_APK_CACHE_DIR"
  rm -f "$DEV_APK_CACHE_DIR"/*.apk
  echo "fetching Alpine dev packages for ${ALPINE_ARCH}: ${packages[*]}..."
  if command -v apk >/dev/null 2>&1; then
    if apk fetch \
      --update-cache \
      --recursive \
      --arch "$ALPINE_ARCH" \
      --repository "$main_repo" \
      --repository "$community_repo" \
      --output "$DEV_APK_CACHE_DIR" \
      "${packages[@]}" && compgen -G "${DEV_APK_CACHE_DIR}/*.apk" >/dev/null; then
      return
    fi
    echo "host apk fetch failed; trying a container fallback if available..." >&2
  fi

  if command -v docker >/dev/null 2>&1; then
    if "${SUDO[@]}" docker run --rm \
      -v "${DEV_APK_CACHE_DIR}:/out" \
      "$container_image" \
      sh -c "$container_apk_script" \
      sh \
      "$main_repo" \
      "$community_repo" \
      "$ALPINE_ARCH" \
      "${packages[@]}" && compgen -G "${DEV_APK_CACHE_DIR}/*.apk" >/dev/null; then
      return
    fi
    echo "docker apk fetch failed; trying podman if available..." >&2
  fi

  if command -v podman >/dev/null 2>&1; then
    if podman run --rm \
      -v "${DEV_APK_CACHE_DIR}:/out" \
      "$container_image" \
      sh -c "$container_apk_script" \
      sh \
      "$main_repo" \
      "$community_repo" \
      "$ALPINE_ARCH" \
      "${packages[@]}" && compgen -G "${DEV_APK_CACHE_DIR}/*.apk" >/dev/null; then
      return
    fi
  fi

  echo "could not fetch Alpine dev APKs; install apk-tools or install a working docker/podman setup" >&2
  exit 1
}

unmount_existing_partitions() {
  local source mountpoint
  while read -r source mountpoint; do
    if [[ -n "${mountpoint:-}" ]]; then
      echo "unmounting ${source} from ${mountpoint}..."
      "${SUDO[@]}" umount "$source"
    fi
  done < <(lsblk -nrpo NAME,MOUNTPOINT "$DEVICE")
}

create_dev_access_overlay() {
  local staging=$1
  local dev_env="$staging/etc/droneos/dev.env"
  local dev_password_hash
  local escaped_ssid escaped_password

  dev_password_hash=$(printf '%s\n' "$DEV_USER_PASSWORD" | openssl passwd -6 -stdin)
  escaped_ssid=$(wpa_quote "$WIFI_SSID")
  escaped_password=$(wpa_quote "$WIFI_PASSWORD")

  : > "$dev_env"
  write_shell_var "$dev_env" DRONEOS_ALPINE_ARCH "$ALPINE_ARCH"
  write_shell_var "$dev_env" DEV_USER_NAME "$DEV_USER_NAME"
  write_shell_var "$dev_env" DEV_USER_PASSWORD_HASH "$dev_password_hash"
  write_shell_var "$dev_env" WIFI_COUNTRY "$WIFI_COUNTRY"
  write_shell_var "$dev_env" DEV_WIFI_MODE "$([[ "$TYPE" == "base" ]] && printf ap || printf client)"
  write_shell_var "$dev_env" DEV_STATIC_IP "10.42.0.1"
  write_shell_var "$dev_env" DEV_PROJECT_DIR "$DEV_PROJECT_DIR"
  write_shell_var "$dev_env" ENABLE_UART_CONSOLE "$ENABLE_UART_CONSOLE"
  write_shell_var "$dev_env" UART_CONSOLE_TTY "$UART_CONSOLE_TTY"
  write_shell_var "$dev_env" UART_CONSOLE_EXTRA_TTYS "$UART_CONSOLE_EXTRA_TTYS"
  write_shell_var "$dev_env" UART_CONSOLE_BAUD "$UART_CONSOLE_BAUD"
  chmod 0600 "$dev_env"

  printf '%s\n' "$HOST_SSH_KEY" > "$staging/etc/droneos/authorized_keys"
  chmod 0600 "$staging/etc/droneos/authorized_keys"

  cat > "$staging/etc/modprobe.d/cfg80211.conf" <<EOF
options cfg80211 ieee80211_regdom=${WIFI_COUNTRY}
EOF

  cat > "$staging/etc/ssh/sshd_config" <<'EOF'
Port 22
HostKey /etc/ssh/ssh_host_rsa_key
HostKey /etc/ssh/ssh_host_ecdsa_key
HostKey /etc/ssh/ssh_host_ed25519_key
PasswordAuthentication yes
PubkeyAuthentication yes
PermitRootLogin no
ChallengeResponseAuthentication no
UsePAM no
Subsystem sftp /usr/lib/ssh/sftp-server
EOF
  cp "$staging/etc/ssh/sshd_config" "$staging/etc/droneos/sshd_config"

  if [[ "$TYPE" == "base" ]]; then
    cat > "$staging/etc/network/interfaces" <<'EOF'
auto lo
iface lo inet loopback

auto wlan0
iface wlan0 inet static
    address 10.42.0.1
    netmask 255.255.255.0
EOF
    cp "$staging/etc/network/interfaces" "$staging/etc/droneos/interfaces"

    cat > "$staging/etc/hostapd/hostapd.conf" <<EOF
interface=wlan0
driver=nl80211
ssid=${WIFI_SSID}
hw_mode=g
channel=6
wmm_enabled=1
auth_algs=1
wpa=2
wpa_passphrase=${WIFI_PASSWORD}
wpa_key_mgmt=WPA-PSK
rsn_pairwise=CCMP
country_code=${WIFI_COUNTRY}
EOF
    cp "$staging/etc/hostapd/hostapd.conf" "$staging/etc/droneos/hostapd.conf"

    cat > "$staging/etc/conf.d/hostapd" <<'EOF'
hostapd_args="/etc/hostapd/hostapd.conf"
EOF
    cp "$staging/etc/conf.d/hostapd" "$staging/etc/droneos/hostapd.conf.d"

    cat > "$staging/etc/dnsmasq.conf" <<'EOF'
interface=wlan0
bind-interfaces
dhcp-range=10.42.0.50,10.42.0.150,12h
domain-needed
bogus-priv
EOF
    cp "$staging/etc/dnsmasq.conf" "$staging/etc/droneos/dnsmasq.conf"
  else
    cat > "$staging/etc/network/interfaces" <<'EOF'
auto lo
iface lo inet loopback

auto wlan0
iface wlan0 inet dhcp
    pre-up ip link set wlan0 up || true
    pre-up wpa_supplicant -B -i wlan0 -c /etc/wpa_supplicant/wpa_supplicant.conf
    post-down killall wpa_supplicant || true
EOF
    cp "$staging/etc/network/interfaces" "$staging/etc/droneos/interfaces"

    cat > "$staging/etc/wpa_supplicant/wpa_supplicant.conf" <<EOF
ctrl_interface=/run/wpa_supplicant
update_config=0
country=${WIFI_COUNTRY}

network={
    ssid="${escaped_ssid}"
    psk="${escaped_password}"
}
EOF
    chmod 0600 "$staging/etc/wpa_supplicant/wpa_supplicant.conf"
    cp "$staging/etc/wpa_supplicant/wpa_supplicant.conf" "$staging/etc/droneos/wpa_supplicant.conf"
    chmod 0600 "$staging/etc/droneos/wpa_supplicant.conf"
  fi

  cat > "$staging/etc/init.d/droneos-dev-setup" <<'EOF'
#!/sbin/openrc-run

name="droneOS development access"
description="Install local dev packages and start WiFi/SSH"

depend() {
    need localmount
    after modules
}

install_dev_packages() {
    local dir
    for dir in \
        /media/*/droneos-apks/"${DRONEOS_ALPINE_ARCH}" \
        /mnt/*/droneos-apks/"${DRONEOS_ALPINE_ARCH}" \
        /droneos-apks/"${DRONEOS_ALPINE_ARCH}"; do
        [ -d "$dir" ] || continue
        set -- "$dir"/*.apk
        [ -e "$1" ] || continue
        apk add --no-network --allow-untrusted --force-non-repository --upgrade "$@" ||
            apk add --allow-untrusted --force-non-repository --upgrade "$@"
        return $?
    done
    eerror "could not find droneOS dev APK cache"
    return 1
}

ensure_group() {
    local group=$1
    grep -q "^${group}:" /etc/group || addgroup -S "$group" >/dev/null 2>&1 || true
}

restore_dev_configs() {
    mkdir -p /etc/conf.d /etc/hostapd /etc/network /etc/ssh /etc/wpa_supplicant
    cp /etc/droneos/interfaces /etc/network/interfaces
    cp /etc/droneos/sshd_config /etc/ssh/sshd_config

    if [ "$DEV_WIFI_MODE" = "ap" ]; then
        cp /etc/droneos/hostapd.conf /etc/hostapd/hostapd.conf
        cp /etc/droneos/hostapd.conf.d /etc/conf.d/hostapd
        cp /etc/droneos/dnsmasq.conf /etc/dnsmasq.conf
    else
        cp /etc/droneos/wpa_supplicant.conf /etc/wpa_supplicant/wpa_supplicant.conf
        chmod 600 /etc/wpa_supplicant/wpa_supplicant.conf
    fi
}

configure_dev_user() {
    local group
    if ! id "$DEV_USER_NAME" >/dev/null 2>&1; then
        adduser -D -h "/home/${DEV_USER_NAME}" -s /bin/ash "$DEV_USER_NAME"
    fi

    if [ -n "$DEV_USER_PASSWORD_HASH" ]; then
        printf '%s:%s\n' "$DEV_USER_NAME" "$DEV_USER_PASSWORD_HASH" | chpasswd -e
    fi

    for group in wheel dialout plugdev gpio i2c spi video input netdev; do
        ensure_group "$group"
        addgroup "$DEV_USER_NAME" "$group" >/dev/null 2>&1 || true
    done

    mkdir -p "/home/${DEV_USER_NAME}/.ssh"
    if [ -s /etc/droneos/authorized_keys ]; then
        cp /etc/droneos/authorized_keys "/home/${DEV_USER_NAME}/.ssh/authorized_keys"
    fi
    chmod 700 "/home/${DEV_USER_NAME}/.ssh"
    chmod 600 "/home/${DEV_USER_NAME}/.ssh/authorized_keys" 2>/dev/null || true
    chown -R "${DEV_USER_NAME}:${DEV_USER_NAME}" "/home/${DEV_USER_NAME}/.ssh"
    mkdir -p "$DEV_PROJECT_DIR"
    chown -R "${DEV_USER_NAME}:${DEV_USER_NAME}" "$DEV_PROJECT_DIR"
}

start_dev_network() {
    iw reg set "$WIFI_COUNTRY" >/dev/null 2>&1 || true
    rc-service networking restart || true

    if [ "$DEV_WIFI_MODE" = "ap" ]; then
        rc-service dnsmasq restart || true
        rc-service hostapd restart || true
    fi
}

start_dev_ssh() {
    ssh-keygen -A >/dev/null 2>&1 || true
    rc-service sshd restart || /usr/sbin/sshd || true
}

print_dev_ip() {
    local ip
    if [ "$DEV_WIFI_MODE" = "ap" ]; then
        ip=$DEV_STATIC_IP
    else
        ip=$(ip -4 addr show wlan0 2>/dev/null | awk '/inet / { sub(/\/.*/, "", $2); print $2; exit }')
    fi
    echo "droneOS dev SSH: ${DEV_USER_NAME}@${ip:-unknown}" | tee /dev/tty1 >/dev/null 2>&1 || true
}

configure_uart_getty() {
    local tty="$1"
    local baud="$2"

    [ -n "$tty" ] || return 1
    [ -c "/dev/${tty}" ] || return 1

    if ! grep -q "^${tty}::respawn:" /etc/inittab 2>/dev/null; then
        printf '%s::respawn:/sbin/getty -L %s %s vt100\n' "$tty" "$baud" "$tty" >> /etc/inittab
        kill -HUP 1 >/dev/null 2>&1 || true
    fi
    printf 'droneOS UART console ready on %s at %s baud\n' "$tty" "$baud" >"/dev/${tty}" 2>/dev/null || true
    return 0
}

configure_uart_console() {
    local baud="${UART_CONSOLE_BAUD:-115200}"
    local tty

    for tty in ${UART_CONSOLE_TTY:-ttyAMA0} ${UART_CONSOLE_EXTRA_TTYS:-}; do
        configure_uart_getty "$tty" "$baud" || true
    done
    return 0
}

start() {
    . /etc/droneos/dev.env
    ebegin "Configuring droneOS development access"
    if [ "${ENABLE_UART_CONSOLE:-0}" -eq 1 ]; then
        configure_uart_console
    fi
    configure_dev_user || return 1
    install_dev_packages || return 1
    restore_dev_configs || return 1
    start_dev_network
    start_dev_ssh
    print_dev_ip
    eend 0
}
EOF
  chmod 0755 "$staging/etc/init.d/droneos-dev-setup"
  ln -s /etc/init.d/droneos-dev-setup "$staging/etc/runlevels/default/droneos-dev-setup"
}

create_openrc_overlay() {
  local staging=$1
  local overlay_file=$2
  local binary_name="${TYPE}.bin"

  rm -rf "$staging"
  mkdir -p \
    "$staging/etc/conf.d" \
    "$staging/etc/droneos" \
    "$staging/etc/hostapd" \
    "$staging/etc/init.d" \
    "$staging/etc/modprobe.d" \
    "$staging/etc/network" \
    "$staging/etc/runlevels/default" \
    "$staging/etc/ssh" \
    "$staging/etc/wpa_supplicant" \
    "$staging/opt/droneOS"

  printf '%s\n' "$HOSTNAME" > "$staging/etc/hostname"
  printf 'i2c-dev\nspidev\n' > "$staging/etc/modules"
  if [[ "$DISABLE_WIFI" -eq 1 ]]; then
    printf 'blacklist brcmfmac\nblacklist brcmutil\n' > "$staging/etc/modprobe.d/droneos-no-wifi.conf"
  fi

  if [[ "$ENABLE_DRONEOS_SERVICE" -eq 1 ]]; then
    install -m 0755 "$BINARY_PATH" "$staging/opt/droneOS/${binary_name}"
    install -m 0644 "$PROJECT_DIR/configs/config.yaml" "$staging/opt/droneOS/config.yaml"
    if [[ -e "${PROJECT_DIR}/.${TYPE}.env" || -L "${PROJECT_DIR}/.${TYPE}.env" ]]; then
      install -m 0600 "${PROJECT_DIR}/.${TYPE}.env" "$staging/opt/droneOS/.${TYPE}.env"
    fi
    SERVICE_DIRECTORY="/opt/droneOS"
    SERVICE_COMMAND="/opt/droneOS/${binary_name}"
    SERVICE_ARGS="--config-file /opt/droneOS/config.yaml"
  else
    SERVICE_DIRECTORY="$DEV_PROJECT_DIR"
    SERVICE_COMMAND="${DEV_PROJECT_DIR}/build/droneOS/${binary_name}"
    SERVICE_ARGS="--config-file ${DEV_PROJECT_DIR}/configs/config.yaml"
  fi

  if [[ "$ENABLE_DEV_ACCESS" -eq 1 ]]; then
    create_dev_access_overlay "$staging"
  fi

  cat > "$staging/etc/init.d/droneOS" <<EOF
#!/sbin/openrc-run

name="droneOS"
description="droneOS ${TYPE} runtime"

directory="${SERVICE_DIRECTORY}"
command="${SERVICE_COMMAND}"
command_args="${SERVICE_ARGS}"
command_background="yes"
pidfile="/run/\${RC_SVCNAME}.pid"
retry="TERM/10/KILL/5"
output_log="/var/log/droneOS.log"
error_log="/var/log/droneOS.err"

depend() {
    need localmount
    after modules
}

start_pre() {
    checkpath -d -m 0755 /run
    checkpath -d -m 0755 /var/log
}
EOF
  chmod 0755 "$staging/etc/init.d/droneOS"

  if [[ "$ENABLE_DRONEOS_SERVICE" -eq 1 ]]; then
    ln -s /etc/init.d/droneOS "$staging/etc/runlevels/default/droneOS"
  fi

  mkdir -p "$(dirname "$overlay_file")"
  tar --numeric-owner --owner=0 --group=0 -czf "$overlay_file" -C "$staging" .
}

write_boot_config() {
  local config_txt="${SD_CARD_BOOT_DIR}/config.txt"
  local usercfg_txt="${SD_CARD_BOOT_DIR}/usercfg.txt"
  local cmdline_txt="${SD_CARD_BOOT_DIR}/cmdline.txt"
  local config_target=$config_txt

  if [[ ! -f "$config_target" ]]; then
    config_target=$usercfg_txt
    "${SUDO[@]}" touch "$config_target"
  fi

  append_once "$config_target" ""
  append_once "$config_target" "# droneOS hardware configuration"
  append_once "$config_target" "enable_uart=1"
  append_once "$config_target" "dtparam=i2c_arm=on"
  append_once "$config_target" "dtparam=spi=on"
  append_once "$config_target" "max_usb_current=1"
  append_once "$config_target" "usb_max_current_enable=1"
  append_once "$config_target" "dtoverlay=dwc2,dr_mode=host"

  if [[ "$DISABLE_WIFI" -eq 1 ]]; then
    append_once "$config_target" "dtoverlay=disable-wifi"
  fi
  if [[ "$DISABLE_BLUETOOTH" -eq 1 ]]; then
    append_once "$config_target" "dtoverlay=disable-bt"
  fi

  append_cmdline_arg "$cmdline_txt" "alpine_dev=LABEL=ALPINE"
  if [[ "$ENABLE_UART_CONSOLE" -eq 1 ]]; then
    remove_cmdline_token "$cmdline_txt" "quiet"
    append_cmdline_token_once "$cmdline_txt" "console=${UART_CONSOLE_TTY},${UART_CONSOLE_BAUD}"
    for tty in $UART_CONSOLE_EXTRA_TTYS; do
      append_cmdline_token_once "$cmdline_txt" "console=${tty},${UART_CONSOLE_BAUD}"
    done
  fi
}

cleanup_on_exit() {
  local status=$?
  if mountpoint -q "$SD_CARD_BOOT_DIR" 2>/dev/null; then
    echo "cleanup: syncing and unmounting ${BOOT_DEVICE}..."
    "${SUDO[@]}" sync
    "${SUDO[@]}" umount "$SD_CARD_BOOT_DIR" 2>/dev/null || true
  fi
  if [[ $status -ne 0 ]]; then
    echo "build_image.sh failed with exit ${status}" >&2
  fi
}

SD_CARD=${1:-}
KERNEL=${2:-kernel8}
TYPE=${3:-drone}
BUILD_MODE=${BUILD_MODE:-prod}

DEFAULT_HOSTNAME=${IMAGE_HOSTNAME:-droneos}
DEFAULT_DEV_USER_NAME=${DEV_USER_NAME:-admin}
DEFAULT_DEV_USER_PASSWORD=${DEV_USER_PASSWORD:-adminpassword}
DEFAULT_WIFI_SSID=${WIFI_SSID:-droneos}
DEFAULT_WIFI_PASSWORD=${WIFI_PASSWORD:-X0YhW2Wy2bmtKXkT2ST61v2SdBk4FGgE}
DEFAULT_WIFI_COUNTRY=${WIFI_COUNTRY:-US}

if [[ -z "$SD_CARD" ]]; then
  usage
  exit 1
fi

if [[ $# -ge 9 ]]; then
  HOSTNAME=${4:-$DEFAULT_HOSTNAME}
  DEV_USER_NAME=${5:-$DEFAULT_DEV_USER_NAME}
  DEV_USER_PASSWORD=${6:-$DEFAULT_DEV_USER_PASSWORD}
  WIFI_SSID=${7:-$DEFAULT_WIFI_SSID}
  WIFI_PASSWORD=${8:-$DEFAULT_WIFI_PASSWORD}
  WIFI_COUNTRY=${9:-$DEFAULT_WIFI_COUNTRY}
  if [[ $# -gt 9 ]]; then
    echo "warning: ignoring arguments after wifi_country" >&2
  fi
elif [[ $# -eq 8 ]]; then
  HOSTNAME=$DEFAULT_HOSTNAME
  DEV_USER_NAME=${4:-$DEFAULT_DEV_USER_NAME}
  DEV_USER_PASSWORD=${5:-$DEFAULT_DEV_USER_PASSWORD}
  WIFI_SSID=${6:-$DEFAULT_WIFI_SSID}
  WIFI_PASSWORD=${7:-$DEFAULT_WIFI_PASSWORD}
  WIFI_COUNTRY=${8:-$DEFAULT_WIFI_COUNTRY}
else
  HOSTNAME=${4:-$DEFAULT_HOSTNAME}
  DEV_USER_NAME=${5:-$DEFAULT_DEV_USER_NAME}
  DEV_USER_PASSWORD=${6:-$DEFAULT_DEV_USER_PASSWORD}
  WIFI_SSID=${7:-$DEFAULT_WIFI_SSID}
  WIFI_PASSWORD=${8:-$DEFAULT_WIFI_PASSWORD}
  WIFI_COUNTRY=${9:-$DEFAULT_WIFI_COUNTRY}
fi
WIFI_COUNTRY=${WIFI_COUNTRY^^}

DEV_PROJECT_DIR=${DEV_PROJECT_DIR:-"/home/${DEV_USER_NAME}/droneOS"}

if ! [[ "$HOSTNAME" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
  echo "invalid hostname: ${HOSTNAME}" >&2
  exit 1
fi

case "$TYPE" in
  base|drone) ;;
  *)
    echo "unsupported image type: ${TYPE} (use base or drone)" >&2
    exit 1
    ;;
esac

case "$KERNEL" in
  kernel8)
    DEFAULT_ALPINE_ARCH=aarch64
    ;;
  kernel7|kernel7l)
    DEFAULT_ALPINE_ARCH=armv7
    ;;
  kernel)
    DEFAULT_ALPINE_ARCH=armhf
    ;;
  *)
    echo "unsupported kernel variant: ${KERNEL} (use kernel, kernel7, kernel7l, or kernel8)" >&2
    exit 1
    ;;
esac

if [[ "$(id -u)" -eq 0 ]]; then
  SUDO=()
else
  SUDO=(sudo)
  require_command sudo
fi

for cmd in basename cp find grep install lsblk mkdir mount mountpoint sed sort tail tar umount wget; do
  require_command "$cmd"
done
for cmd in mkfs.vfat partprobe sfdisk sync; do
  require_command "$cmd"
done

BUILD_DIR=${BUILD_DIR:-"${PROJECT_DIR}/build"}
ALPINE_MIRROR=${ALPINE_MIRROR:-https://dl-cdn.alpinelinux.org/alpine}
ALPINE_BRANCH=${ALPINE_BRANCH:-latest-stable}
ALPINE_ARCH=${ALPINE_ARCH:-$DEFAULT_ALPINE_ARCH}
ALPINE_CACHE_DIR=${ALPINE_CACHE_DIR:-"${BUILD_DIR}/alpine"}
if [[ -z "${DISABLE_WIFI+x}" ]]; then
  if [[ "$BUILD_MODE" == "dev" ]]; then
    DISABLE_WIFI=0
  else
    DISABLE_WIFI=1
  fi
fi
DISABLE_BLUETOOTH=${DISABLE_BLUETOOTH:-1}
if [[ -z "${ENABLE_UART_CONSOLE+x}" ]]; then
  if [[ "$BUILD_MODE" == "dev" ]]; then
    ENABLE_UART_CONSOLE=1
  else
    ENABLE_UART_CONSOLE=0
  fi
fi
UART_CONSOLE_TTY=${UART_CONSOLE_TTY:-ttyAMA0}
if [[ -z "${UART_CONSOLE_EXTRA_TTYS+x}" ]]; then
  if [[ "$BUILD_MODE" == "dev" && "$ENABLE_UART_CONSOLE" -eq 1 ]]; then
    UART_CONSOLE_EXTRA_TTYS=ttyS0
  else
    UART_CONSOLE_EXTRA_TTYS=""
  fi
fi
UART_CONSOLE_BAUD=${UART_CONSOLE_BAUD:-115200}
SKIP_KERNEL_BUILD=${SKIP_KERNEL_BUILD:-1}
INSTALL_PISUGAR=${INSTALL_PISUGAR:-0}

case "$ALPINE_ARCH" in
  aarch64)
    BUILD_ARCH=arm64
    BUILD_GOARM=""
    ;;
  armv7)
    BUILD_ARCH=armv7
    BUILD_GOARM=${GOARM:-7}
    ;;
  armhf)
    BUILD_ARCH=arm
    BUILD_GOARM=${GOARM:-6}
    ;;
  *)
    echo "unsupported ALPINE_ARCH: ${ALPINE_ARCH} (use aarch64, armv7, or armhf)" >&2
    exit 1
    ;;
esac

if [[ "$SKIP_KERNEL_BUILD" -ne 1 ]]; then
  echo "custom Raspberry Pi kernel builds are not part of the Alpine diskless image flow; use the Alpine rpi kernel or provide a custom tarball" >&2
  exit 1
fi

if [[ "$INSTALL_PISUGAR" -ne 0 ]]; then
  echo "INSTALL_PISUGAR used Debian packages and is not supported by the Alpine image builder" >&2
  exit 1
fi

if [[ "$BUILD_MODE" == "prod" ]]; then
  ENABLE_DRONEOS_SERVICE=1
  ENABLE_DEV_ACCESS=0
  echo "Production mode: droneOS OpenRC service enabled"
elif [[ "$BUILD_MODE" == "dev" ]]; then
  ENABLE_DRONEOS_SERVICE=0
  ENABLE_DEV_ACCESS=1
  echo "Development mode: droneOS OpenRC service disabled, WiFi/SSH enabled"
else
  echo "unsupported BUILD_MODE: ${BUILD_MODE} (use prod or dev)" >&2
  exit 1
fi

DEVICE=$(normalize_device "$SD_CARD")
BOOT_DEVICE=$(partition_path "$DEVICE" 1)
MOUNT_BASE=${MOUNT_BASE:-/tmp/droneos_mnt}
SD_CARD_BOOT_DIR="${MOUNT_BASE}/alpine_boot"
BINARY_PATH="${BUILD_DIR}/droneOS/${TYPE}.bin"
APKVOL_STAGING="${BUILD_DIR}/apkovl/${HOSTNAME}"
APKVOL_FILE="${BUILD_DIR}/apkovl/${HOSTNAME}.apkovl.tar.gz"
DEV_APK_CACHE_DIR="${BUILD_DIR}/dev-apks/${ALPINE_BRANCH}/${ALPINE_ARCH}/${TYPE}"

if [[ ! -b "$DEVICE" ]]; then
  echo "block device not found: ${DEVICE}" >&2
  exit 1
fi

resolve_alpine_tarball

if [[ "$ENABLE_UART_CONSOLE" -eq 1 ]]; then
  if ! [[ "$UART_CONSOLE_TTY" =~ ^tty[A-Za-z0-9_]+$ ]]; then
    echo "invalid UART_CONSOLE_TTY: ${UART_CONSOLE_TTY}" >&2
    exit 1
  fi
  for tty in $UART_CONSOLE_EXTRA_TTYS; do
    if ! [[ "$tty" =~ ^tty[A-Za-z0-9_]+$ ]]; then
      echo "invalid UART_CONSOLE_EXTRA_TTYS entry: ${tty}" >&2
      exit 1
    fi
  done
  if ! [[ "$UART_CONSOLE_BAUD" =~ ^[0-9]+$ ]]; then
    echo "invalid UART_CONSOLE_BAUD: ${UART_CONSOLE_BAUD}" >&2
    exit 1
  fi
fi

if [[ "$ENABLE_DEV_ACCESS" -eq 1 ]]; then
  require_command openssl
  require_command ssh-keygen
  if ! [[ "$DEV_USER_NAME" =~ ^[a-z_][a-z0-9_-]*$ ]]; then
    echo "invalid dev username: ${DEV_USER_NAME}" >&2
    exit 1
  fi
  if ! [[ "$WIFI_COUNTRY" =~ ^[A-Z]{2}$ ]]; then
    echo "invalid WiFi country: ${WIFI_COUNTRY} (use a two-letter country code like US)" >&2
    exit 1
  fi
  if [[ "$WIFI_SSID" == *$'\r'* || "$WIFI_SSID" == *$'\n'* || "$WIFI_PASSWORD" == *$'\r'* || "$WIFI_PASSWORD" == *$'\n'* ]]; then
    echo "WiFi SSID and password must not contain carriage returns or newlines" >&2
    exit 1
  fi
  if (( ${#WIFI_PASSWORD} < 8 || ${#WIFI_PASSWORD} > 63 )); then
    echo "WiFi password must contain 8 to 63 characters" >&2
    exit 1
  fi
  find_host_ssh_key
  fetch_dev_apks
fi

if [[ "$ENABLE_DRONEOS_SERVICE" -eq 1 ]]; then
  echo "building ${TYPE} binary for Alpine ${ALPINE_ARCH}..."
  if [[ -n "$BUILD_GOARM" ]]; then
    GOARM="$BUILD_GOARM" OUTPUT="$BINARY_PATH" bash "$PROJECT_DIR/build.sh" "$TYPE" "$BUILD_ARCH"
  else
    OUTPUT="$BINARY_PATH" bash "$PROJECT_DIR/build.sh" "$TYPE" "$BUILD_ARCH"
  fi
else
  echo "skipping ${TYPE} binary build for development image"
fi

echo "creating Alpine OpenRC overlay..."
create_openrc_overlay "$APKVOL_STAGING" "$APKVOL_FILE"

trap cleanup_on_exit EXIT

echo "partitioning ${DEVICE} for Alpine Raspberry Pi boot media..."
unmount_existing_partitions
printf 'label: dos\nstart=2048, type=c, bootable\n' | "${SUDO[@]}" sfdisk --wipe always "$DEVICE"
"${SUDO[@]}" partprobe "$DEVICE" 2>/dev/null || true
for _ in {1..10}; do
  [[ -b "$BOOT_DEVICE" ]] && break
  sleep 1
done
if [[ ! -b "$BOOT_DEVICE" ]]; then
  echo "partition device not found after partitioning: ${BOOT_DEVICE}" >&2
  exit 1
fi

echo "formatting ${BOOT_DEVICE} as FAT32..."
"${SUDO[@]}" mkfs.vfat -F 32 -n ALPINE "$BOOT_DEVICE"

mkdir -p "$SD_CARD_BOOT_DIR"
"${SUDO[@]}" mount "$BOOT_DEVICE" "$SD_CARD_BOOT_DIR"

echo "extracting ${ALPINE_IMAGE_FILE}..."
"${SUDO[@]}" tar -xzf "$ALPINE_IMAGE_PATH" -C "$SD_CARD_BOOT_DIR"

if [[ "$ENABLE_DEV_ACCESS" -eq 1 ]]; then
  echo "installing local Alpine dev APK cache..."
  "${SUDO[@]}" mkdir -p "${SD_CARD_BOOT_DIR}/droneos-apks/${ALPINE_ARCH}"
  mapfile -t DEV_APK_FILES < <(find "$DEV_APK_CACHE_DIR" -maxdepth 1 -type f -name '*.apk' | sort)
  if [[ "${#DEV_APK_FILES[@]}" -eq 0 ]]; then
    echo "no Alpine dev APKs found in ${DEV_APK_CACHE_DIR}" >&2
    exit 1
  fi
  "${SUDO[@]}" cp "${DEV_APK_FILES[@]}" "${SD_CARD_BOOT_DIR}/droneos-apks/${ALPINE_ARCH}/"
fi

echo "installing droneOS Alpine overlay..."
"${SUDO[@]}" cp "$APKVOL_FILE" "${SD_CARD_BOOT_DIR}/${HOSTNAME}.apkovl.tar.gz"
write_boot_config

echo "Alpine image build complete for ${DEVICE}"
echo "Boot media: ${BOOT_DEVICE}"
echo "Service: $([[ "$ENABLE_DRONEOS_SERVICE" -eq 1 ]] && printf enabled || printf disabled)"
if [[ "$ENABLE_DEV_ACCESS" -eq 1 ]]; then
  echo "Dev SSH user: ${DEV_USER_NAME}"
fi

# cleanup handled by trap
