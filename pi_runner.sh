#!/usr/bin/env bash
set -euo pipefail

cat >&2 <<'EOF'
pi_runner.sh is not yet ported to Alpine/OpenRC.

Development images configure WiFi and SSH access. Build a dev SD card, then sync source with sync_pi.sh:

  BUILD_MODE=dev bash build_image.sh sdb kernel8 drone droneos admin password MySSID MyPass US
  bash sync_pi.sh 192.168.1.42
EOF
exit 1
