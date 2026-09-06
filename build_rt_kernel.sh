#!/usr/bin/env bash
# Build a Raspberry Pi PREEMPT_RT kernel media bundle for Alpine diskless boot.
# The --container entry point is private and is invoked only by host mode.
set -euo pipefail
IFS=$'\n\t'

readonly PROGRAM_NAME=${0##*/}
REPOSITORY_DIR=$(dirname -- "$(realpath -- "$0")")
readonly REPOSITORY_DIR
readonly APORTS_REPOSITORY=https://gitlab.alpinelinux.org/alpine/aports.git
readonly PROVENANCE_FILE=droneos-rt-kernel-build.txt

ALPINE_RELEASE=
ALPINE_BRANCH=
TARGET_ARCH=aarch64
APORTS_REF=
OUTPUT=
CACHE_DIR=
OUTPUT_DIR=
OUTPUT_NAME=
FORCE=0
PRINT_CACHE_PATH=0
CONTAINER_IMAGE=
CONTAINER_ENGINE_NAME=
HOST_UID=
HOST_GID=
HOST_WORK_DIR=
RESUME_WORK=

cleanup_host_work() {
    local status=$?

    [[ -n "$HOST_WORK_DIR" ]] || return
    if ((status == 0)); then
        rm -rf -- "$HOST_WORK_DIR"
    else
        printf '%s: build failed; preserved work at %s\n' "$PROGRAM_NAME" "$HOST_WORK_DIR" >&2
    fi
}

die() {
    printf '%s: %s\n' "$PROGRAM_NAME" "$*" >&2
    exit 1
}

usage() {
    cat <<'USAGE'
Usage:
  build_rt_kernel.sh --alpine-release X.Y.Z --aports-ref REF
                     [--arch aarch64] [--output PATH | --cache-dir DIR]
                     [--print-cache-path] [--resume-work PATH] [--force]

Builds a gzip tar archive whose root can replace the kernel files on Alpine
Raspberry Pi diskless boot media. Only Alpine aarch64/Raspberry Pi is supported.

Options:
  --alpine-release X.Y.Z  Alpine release used for the matching X.Y container
  --arch aarch64          Target architecture (only aarch64 is supported)
  --aports-ref REF        aports branch, tag, or commit to build
  --output PATH           Destination for an exported/custom gzip tar archive
  --cache-dir DIR         Cache directory (default: RT_KERNEL_CACHE_DIR or
                          <repository>/build/rt-kernel)
  --print-cache-path      Print the automatic artifact path and exit
  --resume-work PATH      Resume packaging from a preserved failed build
  --force                 Rebuild even when the destination is a valid artifact
  -h, --help              Show this help
USAGE
}

require_value() {
    local option=$1
    local value=${2:-}

    [[ -n "$value" ]] || die "missing value for ${option}"
}

parse_host_arguments() {
    while (($# > 0)); do
        case "$1" in
            --alpine-release)
                require_value "$1" "${2:-}"
                ALPINE_RELEASE=$2
                shift 2
                ;;
            --arch)
                require_value "$1" "${2:-}"
                TARGET_ARCH=$2
                shift 2
                ;;
            --aports-ref)
                require_value "$1" "${2:-}"
                APORTS_REF=$2
                shift 2
                ;;
            --output)
                require_value "$1" "${2:-}"
                OUTPUT=$2
                shift 2
                ;;
            --cache-dir)
                require_value "$1" "${2:-}"
                CACHE_DIR=$2
                shift 2
                ;;
            --print-cache-path)
                PRINT_CACHE_PATH=1
                shift
                ;;
            --resume-work)
                require_value "$1" "${2:-}"
                RESUME_WORK=$2
                shift 2
                ;;
            --force)
                FORCE=1
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                die "unknown option: $1 (use --help for usage)"
                ;;
        esac
    done

    [[ -n "$ALPINE_RELEASE" ]] || die "--alpine-release is required"
    [[ -n "$APORTS_REF" ]] || die "--aports-ref is required"
    [[ -z "$OUTPUT" || -z "$CACHE_DIR" ]] || die "--output and --cache-dir cannot be used together"
    ((PRINT_CACHE_PATH == 0)) || [[ -z "$OUTPUT" ]] ||
        die "--print-cache-path requires automatic cache mode; omit --output"
    [[ "$TARGET_ARCH" == aarch64 ]] || die "unsupported arch: ${TARGET_ARCH}; only aarch64 Raspberry Pi kernels are supported"
    [[ "$ALPINE_RELEASE" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || \
        die "invalid --alpine-release '${ALPINE_RELEASE}'; expected X.Y.Z"
    [[ ! "$APORTS_REF" =~ [[:space:]] ]] || die "invalid --aports-ref: whitespace is not allowed"

    ALPINE_BRANCH="${ALPINE_RELEASE%.*}"
    CONTAINER_IMAGE=${RT_BUILD_CONTAINER_IMAGE:-"alpine:${ALPINE_BRANCH}"}
    if [[ -n "$OUTPUT" ]]; then
        OUTPUT=$(realpath -m -- "$OUTPUT")
    else
        local ref_digest

        CACHE_DIR=${CACHE_DIR:-"${RT_KERNEL_CACHE_DIR:-"${REPOSITORY_DIR}/build/rt-kernel"}"}
        CACHE_DIR=$(realpath -m -- "$CACHE_DIR")
        ref_digest=$(printf '%s' "$APORTS_REF" | sha256sum)
        ref_digest=${ref_digest%% *}
        ref_digest=${ref_digest,,}
        OUTPUT="${CACHE_DIR}/droneos-rt-${ALPINE_RELEASE}-${TARGET_ARCH}-${ref_digest:0:12}.tar.gz"
    fi
    OUTPUT_DIR=$(dirname -- "$OUTPUT")
    OUTPUT_NAME=$(basename -- "$OUTPUT")
    [[ "$OUTPUT_NAME" != . && "$OUTPUT_NAME" != / ]] || die "--output must name an archive file"
    if [[ -n "$RESUME_WORK" ]]; then
        RESUME_WORK=$(realpath -m -- "$RESUME_WORK")
        [[ -d "$RESUME_WORK" ]] || die "--resume-work is not a directory: ${RESUME_WORK}"
    fi
}


archive_has_entry() {
    local wanted=$1
    shift
    local entry

    for entry in "$@"; do
        [[ "${entry#./}" == "$wanted" ]] && return 0
    done
    return 1
}

validate_archive() {
    local archive=$1
    local config_content provenance_content config_suffix map_suffix entry relative
    local -a entries=() configs=() maps=() dtbs=() overlays=() provenance_entries=()

    tar -tzf "$archive" >/dev/null 2>&1 || die "invalid gzip tar archive: ${archive}"
    mapfile -t entries < <(tar -tzf "$archive")

    archive_has_entry boot/vmlinuz-rpi "${entries[@]}" || die "artifact gap: boot/vmlinuz-rpi is missing"
    archive_has_entry boot/initramfs-rpi "${entries[@]}" || die "artifact gap: boot/initramfs-rpi is missing"
    archive_has_entry boot/modloop-rpi "${entries[@]}" || die "artifact gap: boot/modloop-rpi is missing"
    archive_has_entry "$PROVENANCE_FILE" "${entries[@]}" || die "artifact gap: ${PROVENANCE_FILE} is missing"

    for entry in "${entries[@]}"; do
        relative=${entry#./}
        case "$relative" in
            boot/config-*) configs+=("$entry") ;;
            boot/System.map-*) maps+=("$entry") ;;
            bcm*.dtb) dtbs+=("$entry") ;;
            overlays/*.dtbo) overlays+=("$entry") ;;
            "$PROVENANCE_FILE") provenance_entries+=("$entry") ;;
        esac
    done

    ((${#configs[@]} == 1)) || die "artifact mismatch: expected exactly one boot/config-* file, found ${#configs[@]}"
    ((${#maps[@]} == 1)) || die "artifact mismatch: expected exactly one boot/System.map-* file, found ${#maps[@]}"
    ((${#dtbs[@]} > 0)) || die "artifact gap: no root-level Raspberry Pi DTB files were generated"
    ((${#overlays[@]} > 0)) || die "artifact gap: no overlays/*.dtbo files were generated"
    ((${#provenance_entries[@]} == 1)) || die "artifact mismatch: expected exactly one ${PROVENANCE_FILE}, found ${#provenance_entries[@]}"

    config_suffix=${configs[0]#./boot/config-}
    config_suffix=${config_suffix#boot/config-}
    map_suffix=${maps[0]#./boot/System.map-}
    map_suffix=${map_suffix#boot/System.map-}
    [[ "$config_suffix" == "$map_suffix" ]] || \
        die "artifact mismatch: config and System.map do not describe one kernel version set"

    if ! config_content=$(tar -xOzf "$archive" "${configs[0]}" 2>/dev/null); then
        die "artifact gap: cannot read ${configs[0]}"
    fi
    grep -Fxq 'CONFIG_PREEMPT_RT=y' <<<"$config_content" || \
        die "artifact config mismatch: CONFIG_PREEMPT_RT=y is absent"
    if ! provenance_content=$(tar -xOzf "$archive" "${provenance_entries[0]}" 2>/dev/null); then
        die "artifact gap: cannot read ${provenance_entries[0]}"
    fi
    grep -Fqx "alpine_requested_release=${ALPINE_RELEASE}" <<<"$provenance_content" ||
        die "artifact provenance mismatch: expected Alpine release ${ALPINE_RELEASE}"
    grep -Fqx "arch=${TARGET_ARCH}" <<<"$provenance_content" ||
        die "artifact provenance mismatch: expected architecture ${TARGET_ARCH}"
    grep -Fqx "aports_ref=${APORTS_REF}" <<<"$provenance_content" ||
        die "artifact provenance mismatch: expected aports ref ${APORTS_REF}"
}

select_container_engine() {
    if command -v docker >/dev/null 2>&1; then
        CONTAINER_ENGINE_NAME=docker
    elif command -v podman >/dev/null 2>&1; then
        CONTAINER_ENGINE_NAME=podman
    else
        die "no container engine found; install Docker or Podman"
    fi
}

verify_container_architecture() {
    local runtime_arch

    if ! runtime_arch=$("$CONTAINER_ENGINE_NAME" run --rm --platform linux/arm64 \
        "$CONTAINER_IMAGE" uname -m); then
        die "cannot run Alpine aarch64 containers with ${CONTAINER_ENGINE_NAME}; Docker/Podman must be running and Linux binfmt/QEMU support must be available"
    fi
    case "$runtime_arch" in
        aarch64|arm64) ;;
        *) die "Linux binfmt/QEMU is unavailable: ${CONTAINER_ENGINE_NAME} ran Alpine as '${runtime_arch}', not aarch64" ;;
    esac
}

run_host() {
    local script_path resume=0

    parse_host_arguments "$@"
    if ((PRINT_CACHE_PATH)); then
        printf '%s\n' "$OUTPUT"
        return
    fi
    mkdir -p -- "$OUTPUT_DIR"
    if [[ -e "$OUTPUT" ]]; then
        if (validate_archive "$OUTPUT"); then
            if ((FORCE == 0)); then
                printf '%s: reusing validated artifact %s\n' "$PROGRAM_NAME" "$OUTPUT"
                return 0
            fi
        elif ((FORCE == 0)); then
            die "existing output is not a valid RT kernel artifact; pass --force to replace it: ${OUTPUT}"
        fi
    fi

    [[ ! -d "$OUTPUT" ]] || die "--output names a directory: ${OUTPUT}"
    select_container_engine
    verify_container_architecture

    script_path=$(realpath -- "$0")
    if [[ -n "$RESUME_WORK" ]]; then
        HOST_WORK_DIR=$RESUME_WORK
        resume=1
    else
        HOST_WORK_DIR=$(mktemp -d "${OUTPUT_DIR}/.rt-kernel-build.XXXXXXXX")
        chmod 0777 -- "$HOST_WORK_DIR"
    fi
    HOST_UID=$(id -u)
    HOST_GID=$(id -g)
    trap cleanup_host_work EXIT

    printf '%s: building Alpine %s linux-rpi from aports %s with %s\n' \
        "$PROGRAM_NAME" "$ALPINE_RELEASE" "$APORTS_REF" "$CONTAINER_ENGINE_NAME"

    "$CONTAINER_ENGINE_NAME" run --rm --platform linux/arm64 \
        --mount "type=bind,src=${script_path},dst=/opt/build_rt_kernel.sh,readonly" \
        --mount "type=bind,src=${HOST_WORK_DIR},dst=/work" \
        --mount "type=bind,src=${OUTPUT_DIR},dst=/output" \
        -e "DRONEOS_RT_KERNEL_RELEASE=${ALPINE_RELEASE}" \
        -e "DRONEOS_RT_KERNEL_ARCH=${TARGET_ARCH}" \
        -e "DRONEOS_RT_KERNEL_APORTS_REF=${APORTS_REF}" \
        -e "DRONEOS_RT_KERNEL_OUTPUT_NAME=${OUTPUT_NAME}" \
        -e "DRONEOS_RT_KERNEL_ENGINE=${CONTAINER_ENGINE_NAME}" \
        -e "DRONEOS_RT_KERNEL_HOST_UID=${HOST_UID}" \
        -e "DRONEOS_RT_KERNEL_HOST_GID=${HOST_GID}" \
        -e "DRONEOS_RT_KERNEL_RESUME=${resume}" \
        "$CONTAINER_IMAGE" sh -ec 'apk add --no-cache bash && exec /usr/bin/env bash /opt/build_rt_kernel.sh --container'

    validate_archive "$OUTPUT"
    printf '%s: wrote validated RT kernel artifact %s\n' "$PROGRAM_NAME" "$OUTPUT"
}

set_kernel_config() {
    local config_file=$1
    local key=$2
    local value=$3

    sed -i \
        -e "/^${key}=/d" \
        -e "/^# ${key} is not set$/d" \
        "$config_file"
    printf '%s=%s\n' "$key" "$value" >>"$config_file"
}

require_kernel_config() {
    local config_file=$1
    local expected=$2

    grep -Fxq -- "$expected" "$config_file" || die "build/config mismatch: expected '${expected}' in ${config_file}"
}

require_kernel_config_disabled() {
    local config_file=$1
    local key=$2

    if grep -Eq "^${key}=(y|m)$" "$config_file"; then
        die "build/config mismatch: expected ${key} to be disabled in ${config_file}"
    fi
}

configure_rt_kernel() {
    local config_file=$1

    [[ -f "$config_file" ]] || die "aports config is missing: ${config_file}"
    set_kernel_config "$config_file" CONFIG_EXPERT y

    # Linux 6.12 and newer ships PREEMPT_RT; do not apply an external patch series.
    set_kernel_config "$config_file" CONFIG_PREEMPT_RT y
    set_kernel_config "$config_file" CONFIG_PREEMPT_DYNAMIC n
    set_kernel_config "$config_file" CONFIG_PREEMPT_VOLUNTARY n
    set_kernel_config "$config_file" CONFIG_PREEMPT_NONE n
    set_kernel_config "$config_file" CONFIG_HZ_1000 y
    set_kernel_config "$config_file" CONFIG_HZ_300 n
    set_kernel_config "$config_file" CONFIG_HZ_250 n
    set_kernel_config "$config_file" CONFIG_HZ_100 n
    set_kernel_config "$config_file" CONFIG_HZ 1000
    set_kernel_config "$config_file" CONFIG_HZ_PERIODIC y
    set_kernel_config "$config_file" CONFIG_NO_HZ n
    set_kernel_config "$config_file" CONFIG_NO_HZ_IDLE n
    set_kernel_config "$config_file" CONFIG_NO_HZ_FULL n
    set_kernel_config "$config_file" CONFIG_RT_GROUP_SCHED n
    set_kernel_config "$config_file" CONFIG_CPU_FREQ_DEFAULT_GOV_PERFORMANCE y
    set_kernel_config "$config_file" CONFIG_CPU_FREQ_DEFAULT_GOV_SCHEDUTIL n
    set_kernel_config "$config_file" CONFIG_CPU_FREQ_DEFAULT_GOV_ONDEMAND n
    set_kernel_config "$config_file" CONFIG_CPU_FREQ_DEFAULT_GOV_POWERSAVE n
    set_kernel_config "$config_file" CONFIG_CPU_FREQ_DEFAULT_GOV_CONSERVATIVE n
    set_kernel_config "$config_file" CONFIG_LOCKUP_DETECTOR n
    set_kernel_config "$config_file" CONFIG_PROVE_LOCKING n
}

require_supported_kernel_source() {
    local apkbuild=$1
    local pkgver major minor

    pkgver=$(sed -nE 's/^pkgver=([0-9]+\.[0-9]+\.[0-9]+)$/\1/p' "$apkbuild" | sed -n '1p')
    [[ -n "$pkgver" ]] || die "cannot determine linux-rpi pkgver from ${apkbuild}"
    IFS=. read -r major minor _ <<<"$pkgver"
    if ((major < 6 || (major == 6 && minor < 12))); then
        die "aports ref builds linux-rpi ${pkgver}; Linux 6.12 or newer is required because this builder intentionally does not apply an external PREEMPT_RT patch series"
    fi
}


validate_media_bundle() {
    local media_dir=$1
    local config_file map_file config_suffix map_suffix
    local -a configs=() maps=() dtbs=() overlays=()

    [[ -f "$media_dir/boot/vmlinuz-rpi" ]] || die "artifact gap: update-kernel did not generate boot/vmlinuz-rpi"
    [[ -f "$media_dir/boot/initramfs-rpi" ]] || die "artifact gap: update-kernel did not generate boot/initramfs-rpi"
    [[ -f "$media_dir/boot/modloop-rpi" ]] || die "artifact gap: update-kernel did not generate boot/modloop-rpi"

    shopt -s nullglob
    configs=("$media_dir"/boot/config-*)
    maps=("$media_dir"/boot/System.map-*)
    dtbs=("$media_dir"/bcm*.dtb)
    overlays=("$media_dir"/overlays/*.dtbo)
    shopt -u nullglob

    ((${#configs[@]} == 1)) || die "artifact mismatch: update-kernel generated ${#configs[@]} boot/config-* files, expected one"
    ((${#maps[@]} == 1)) || die "artifact mismatch: update-kernel generated ${#maps[@]} boot/System.map-* files, expected one"
    ((${#dtbs[@]} > 0)) || die "artifact gap: update-kernel generated no root-level Raspberry Pi DTBs"
    ((${#overlays[@]} > 0)) || die "artifact gap: update-kernel generated no Raspberry Pi overlay DTBOs"

    config_file=${configs[0]}
    map_file=${maps[0]}
    config_suffix=${config_file##*/config-}
    map_suffix=${map_file##*/System.map-}
    [[ "$config_suffix" == "$map_suffix" ]] || \
        die "artifact mismatch: config and System.map do not describe one kernel version set"

    require_kernel_config "$config_file" 'CONFIG_EXPERT=y'
    require_kernel_config "$config_file" 'CONFIG_PREEMPT_RT=y'
    require_kernel_config "$config_file" 'CONFIG_HZ_1000=y'
    require_kernel_config "$config_file" 'CONFIG_HZ=1000'
    require_kernel_config "$config_file" 'CONFIG_HZ_PERIODIC=y'
    require_kernel_config "$config_file" 'CONFIG_CPU_FREQ_DEFAULT_GOV_PERFORMANCE=y'
    require_kernel_config_disabled "$config_file" CONFIG_PREEMPT_DYNAMIC
    require_kernel_config_disabled "$config_file" CONFIG_PREEMPT_VOLUNTARY
    require_kernel_config_disabled "$config_file" CONFIG_PREEMPT_NONE
    require_kernel_config_disabled "$config_file" CONFIG_NO_HZ
    require_kernel_config_disabled "$config_file" CONFIG_NO_HZ_IDLE
    require_kernel_config_disabled "$config_file" CONFIG_NO_HZ_FULL
    require_kernel_config_disabled "$config_file" CONFIG_RT_GROUP_SCHED
    require_kernel_config_disabled "$config_file" CONFIG_CPU_FREQ_DEFAULT_GOV_SCHEDUTIL
    require_kernel_config_disabled "$config_file" CONFIG_CPU_FREQ_DEFAULT_GOV_ONDEMAND
    require_kernel_config_disabled "$config_file" CONFIG_CPU_FREQ_DEFAULT_GOV_POWERSAVE
    require_kernel_config_disabled "$config_file" CONFIG_LOCKUP_DETECTOR
    require_kernel_config_disabled "$config_file" CONFIG_PROVE_LOCKING
    require_kernel_config_disabled "$config_file" CONFIG_CPU_FREQ_DEFAULT_GOV_CONSERVATIVE
}


write_provenance() {
    local media_dir=$1
    local aports_commit=$2
    local kernel_release=$3

    cat >"${media_dir}/${PROVENANCE_FILE}" <<EOF
alpine_requested_release=${DRONEOS_RT_KERNEL_RELEASE}
alpine_container_release=$(cat /etc/alpine-release)
alpine_branch=${ALPINE_BRANCH}
aports_ref=${DRONEOS_RT_KERNEL_APORTS_REF}
aports_commit=${aports_commit}
arch=${DRONEOS_RT_KERNEL_ARCH}
kernel_release=${kernel_release}
rt_config=CONFIG_PREEMPT_RT=y
EOF
}

cleanup_container_work() {
    local status=$?

    if ((status == 0)); then
        find /work -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + 2>/dev/null || true
    else
        printf '%s: container build failed; preserving /work\n' "$PROGRAM_NAME" >&2
    fi
}

write_build_request() {
    local aports_commit=$1

    cat >/work/build-request <<EOF
alpine_release=${ALPINE_RELEASE}
arch=${TARGET_ARCH}
aports_ref=${APORTS_REF}
aports_commit=${aports_commit}
EOF
}

validate_build_request() {
    local aports_commit=$1
    local request=/work/build-request

    [[ -f "$request" ]] || die "resumed work has no build-request metadata"
    grep -Fqx "alpine_release=${ALPINE_RELEASE}" "$request" ||
        die "resumed work targets a different Alpine release"
    grep -Fqx "arch=${TARGET_ARCH}" "$request" ||
        die "resumed work targets a different architecture"
    grep -Fqx "aports_ref=${APORTS_REF}" "$request" ||
        die "resumed work targets a different aports ref"
    grep -Fqx "aports_commit=${aports_commit}" "$request" ||
        die "resumed work has a different aports commit"
}

run_container() {
    local aports_dir package_dir config_file repository_dir kernel_apk
    local aports_commit kernel_release archive_tmp container_release
    local -a kernel_apks=() public_keys=() generated_configs=() repository_kernel_apks=()
    [[ ${DRONEOS_RT_KERNEL_RELEASE:-} =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
        die "private container mode requires a valid DRONEOS_RT_KERNEL_RELEASE"
    [[ ${DRONEOS_RT_KERNEL_ARCH:-} == aarch64 ]] || die "private container mode only supports aarch64"
    [[ -n ${DRONEOS_RT_KERNEL_APORTS_REF:-} ]] || die "private container mode requires DRONEOS_RT_KERNEL_APORTS_REF"
    ALPINE_RELEASE=$DRONEOS_RT_KERNEL_RELEASE
    ALPINE_BRANCH=${ALPINE_RELEASE%.*}
    TARGET_ARCH=$DRONEOS_RT_KERNEL_ARCH
    APORTS_REF=$DRONEOS_RT_KERNEL_APORTS_REF
    [[ -n ${DRONEOS_RT_KERNEL_OUTPUT_NAME:-} ]] || die "private container mode requires DRONEOS_RT_KERNEL_OUTPUT_NAME"
    [[ -d /work && -d /output ]] || die "private container mode requires mounted /work and /output directories"
    [[ $(uname -m) == aarch64 ]] || die "container architecture mismatch: expected aarch64, got $(uname -m)"
    container_release=$(cat /etc/alpine-release)
    [[ "${container_release%.*}" == "$ALPINE_BRANCH" ]] ||
        die "container release ${container_release} does not match requested Alpine series ${ALPINE_BRANCH}"
    trap cleanup_container_work EXIT

    # This remains a private, disposable build environment: it never touches boot media.
    apk add --no-cache alpine-sdk alpine-conf bash git initramfs-generator linux-firmware-brcm squashfs-tools
    # abuild-apk resolves build dependencies from the local APK index cache.
    apk update

    aports_dir=/work/aports
    package_dir=${aports_dir}/main/linux-rpi
    config_file=${package_dir}/common-changes.config
    export ABUILD_USERDIR=/work/abuild

    if [[ ${DRONEOS_RT_KERNEL_RESUME:-0} == 1 ]]; then
        [[ -d "$aports_dir/.git" ]] || die "resumed work has no aports checkout"
        aports_commit=$(git -C "$aports_dir" rev-parse HEAD)
        validate_build_request "$aports_commit"
        require_supported_kernel_source "${package_dir}/APKBUILD"
        require_kernel_config "$config_file" 'CONFIG_PREEMPT_RT=y'
    else
        git init "$aports_dir"
        git -C "$aports_dir" remote add origin "$APORTS_REPOSITORY"
        git -C "$aports_dir" sparse-checkout init --cone
        git -C "$aports_dir" sparse-checkout set main/linux-rpi
        git -C "$aports_dir" fetch --depth 1 origin "$DRONEOS_RT_KERNEL_APORTS_REF"
        git -C "$aports_dir" checkout --detach FETCH_HEAD
        aports_commit=$(git -C "$aports_dir" rev-parse HEAD)
        require_supported_kernel_source "${package_dir}/APKBUILD"
        configure_rt_kernel "$config_file"
        write_build_request "$aports_commit"

        SUDO='' PACKAGER='droneOS RT Kernel <rt-kernel@droneos.local>' abuild-keygen -a -i -n
        (
            cd "$package_dir"
            CBUILD=aarch64 abuild -F -P /work/packages checksum
            CBUILD=aarch64 abuild -F -P /work/packages -r
        )
    fi

    shopt -s nullglob
    kernel_apks=(/work/packages/main/aarch64/linux-rpi-[0-9]*.apk)
    public_keys=("$ABUILD_USERDIR"/*.rsa.pub)
    shopt -u nullglob
    ((${#kernel_apks[@]} == 1)) || \
        die "build artifact mismatch: expected one rebuilt linux-rpi APK, found ${#kernel_apks[@]}"
    kernel_apk=${kernel_apks[0]}
    ((${#public_keys[@]} == 1)) ||
        die "build key mismatch: expected one abuild public key, found ${#public_keys[@]}"
    install -m 0644 "${public_keys[0]}" "/etc/apk/keys/${public_keys[0]##*/}"

    repository_dir=/work/update-repository/aarch64
    rm -rf /work/update-repository
    mkdir -p "$repository_dir"
    apk fetch --recursive --arch "$TARGET_ARCH" --output "$repository_dir" \
        alpine-base initramfs-generator linux-firmware linux-firmware-brcm wireless-regdb
    cp "$kernel_apk" "$repository_dir/"
    shopt -s nullglob
    repository_kernel_apks=("$repository_dir"/linux-rpi-[0-9]*.apk)
    shopt -u nullglob
    ((${#repository_kernel_apks[@]} == 1)) ||
        die "isolated update repository contains ${#repository_kernel_apks[@]} linux-rpi packages, expected only the rebuilt package"
    (
        cd "$repository_dir"
        apk index --rewrite-arch "$TARGET_ARCH" -o APKINDEX.tar.gz ./*.apk
        abuild-sign -k "${public_keys[0]%.pub}" APKINDEX.tar.gz
    )
    printf '/work/update-repository\n' >/work/repositories

    rm -rf /work/update-kernel-tmp /work/media
    mkdir -p /work/update-kernel-tmp /work/media
    TMPDIR=/work/update-kernel-tmp update-kernel \
        -a aarch64 \
        -f rpi \
        -p linux-firmware \
        -p wireless-regdb \
        --repositories-file /work/repositories \
        -M /work/media
    validate_media_bundle /work/media
    shopt -s nullglob
    generated_configs=(/work/media/boot/config-*)
    shopt -u nullglob
    generated_configs=("${generated_configs[@]##*/}")
    ((${#generated_configs[@]} == 1)) ||
        die "artifact mismatch: cannot identify generated kernel release"
    kernel_release=${generated_configs[0]#config-}
    write_provenance /work/media "$aports_commit" "$kernel_release"

    archive_tmp=$(mktemp "/work/.${DRONEOS_RT_KERNEL_OUTPUT_NAME}.tmp.XXXXXXXX")
    tar -C /work/media -czf "$archive_tmp" .
    validate_archive "$archive_tmp"
    if [[ ${DRONEOS_RT_KERNEL_ENGINE:-} == docker ]]; then
        chown "${DRONEOS_RT_KERNEL_HOST_UID}:${DRONEOS_RT_KERNEL_HOST_GID}" "$archive_tmp"
    fi
    mv -f -- "$archive_tmp" "/output/${DRONEOS_RT_KERNEL_OUTPUT_NAME}"
}

if [[ ${1:-} == --container ]]; then
    shift
    (($# == 0)) || die "private container mode does not accept command-line arguments"
    run_container
else
    run_host "$@"
fi
