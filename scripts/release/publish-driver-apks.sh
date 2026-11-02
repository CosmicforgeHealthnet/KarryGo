#!/usr/bin/env bash
# Uploads driver APKs to the downloads server. Each file goes up under a temp
# name, is checked there, and is then moved into place, so a download never gets
# a half-written file. Usage:
#   scripts/release/publish-driver-apks.sh <apk-dir> [taxi truck dispatch]
# Overrides: DRIVER_APK_SSH (default root@173.249.30.55), DRIVER_APK_REMOTE_DIR.
set -euo pipefail

APK_DIR="${1:?Usage: publish-driver-apks.sh <apk-dir> [taxi truck dispatch]}"
shift
SSH_TARGET="${DRIVER_APK_SSH:-root@173.249.30.55}"
REMOTE_DIR="${DRIVER_APK_REMOTE_DIR:-/var/www/karryam-downloads}"

apps=("$@")
[[ ${#apps[@]} -gt 0 ]] || apps=(taxi truck dispatch)

for name in "${apps[@]}"; do
  apk="karryam-$name-driver.apk"
  (cd "$APK_DIR" && shasum -a 256 -c "$apk.sha256")
  scp "$APK_DIR/$apk" "$SSH_TARGET:$REMOTE_DIR/.$apk.tmp"
  scp "$APK_DIR/$apk.sha256" "$SSH_TARGET:$REMOTE_DIR/.$apk.sha256.tmp"
  expected="$(cut -d' ' -f1 "$APK_DIR/$apk.sha256")"
  ssh "$SSH_TARGET" "set -e; cd '$REMOTE_DIR'; \
    [ \"\$(sha256sum '.$apk.tmp' | cut -d' ' -f1)\" = '$expected' ]; \
    chmod 644 '.$apk.tmp' '.$apk.sha256.tmp'; \
    mv '.$apk.tmp' '$apk'; mv '.$apk.sha256.tmp' '$apk.sha256'"
  echo "Published $apk"
done
