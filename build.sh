#!/usr/bin/env bash
set -euo pipefail

TYPE=${1:-drone}
ARCH=${2:-arm64}

case "$TYPE" in
  base|drone) ;;
  *)
    echo "unsupported build type: ${TYPE} (use base or drone)" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  arm64|aarch64)
    GOARCH=arm64
    GOARM_VALUE=""
    ;;
  arm|armhf|armv6)
    GOARCH=arm
    GOARM_VALUE=${GOARM:-6}
    ;;
  armv7)
    GOARCH=arm
    GOARM_VALUE=${GOARM:-7}
    ;;
  amd64|x86_64)
    GOARCH=amd64
    GOARM_VALUE=""
    ;;
  *)
    echo "unsupported architecture: ${ARCH} (use arm64, arm, armv7, or amd64)" >&2
    exit 1
    ;;
esac

OUTPUT=${OUTPUT:-build/droneOS/${TYPE}.bin}
GO_TAGS=${GO_TAGS:-osusergo,netgo}
GO_LDFLAGS=${GO_LDFLAGS:--s -w}

mkdir -p "$(dirname "$OUTPUT")"

export CGO_ENABLED="${CGO_ENABLED:-0}"
export GOOS=linux
export GOARCH

if [[ -n "$GOARM_VALUE" ]]; then
  export GOARM="$GOARM_VALUE"
else
  unset GOARM
fi

go build \
  -trimpath \
  -tags "$GO_TAGS" \
  -ldflags "$GO_LDFLAGS" \
  -o "$OUTPUT" \
  "./cmd/${TYPE}/main.go"

chmod +x "$OUTPUT"
echo "built ${OUTPUT}"
