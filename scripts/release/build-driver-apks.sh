#!/usr/bin/env bash
# Builds the public driver APKs from a clean export of a commit, so uncommitted
# work in the checkout can never ship. Usage:
#   scripts/release/build-driver-apks.sh [taxi truck dispatch]
# Overrides: DRIVER_APK_GATEWAY, DRIVER_APK_OUT_DIR, DRIVER_APK_COMMIT.
#
# Release builds are signed with this machine's Android debug key for now, so
# updates must be built here (or with a copy of ~/.android/debug.keystore).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GATEWAY="${DRIVER_APK_GATEWAY:-https://api-gateway.staging.karryam.com}"
GATEWAY_HOST="${GATEWAY#https://}"
WS_URL="wss://${GATEWAY_HOST}/api/v1/notifications/ws"
COMMIT="$(git -C "$ROOT_DIR" rev-parse --short "${DRIVER_APK_COMMIT:-HEAD}")"
OUT_DIR="${DRIVER_APK_OUT_DIR:-$HOME/.karrygo/driver-apks/$COMMIT}"
ANDROID_SDK="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
APKSIGNER="$(ls -d "$ANDROID_SDK"/build-tools/* | sort -V | tail -1)/apksigner"

[[ -x "$APKSIGNER" ]] || { echo "apksigner not found under $ANDROID_SDK" >&2; exit 1; }
[[ "$GATEWAY" == https://* ]] || { echo "DRIVER_APK_GATEWAY must use https" >&2; exit 1; }

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT
mkdir -p "$OUT_DIR"

build_app() {
  local name="$1" app needs_maps allow_dev_fallback defines
  case "$name" in
    taxi)
      app=taxi_provider needs_maps=1 allow_dev_fallback=0
      defines=(
        "TAXI_API_BASE_URL=$GATEWAY/api/v1/taxi"
        "PAYMENT_WALLET_API_BASE_URL=$GATEWAY/api/v1/payment-wallet"
        "SUPPORT_API_BASE_URL=$GATEWAY/api/v1/support-disputes"
        "REFERRAL_API_BASE_URL=$GATEWAY/api/v1/referrals"
      ) ;;
    truck)
      app=truck_provider needs_maps=1 allow_dev_fallback=0
      defines=(
        "HAULING_API_BASE_URL=$GATEWAY/api/v1/hauling"
        "NOTIFICATION_WS_URL=$WS_URL"
        "PAYMENT_WALLET_API_BASE_URL=$GATEWAY/api/v1/payment-wallet"
        "SUPPORT_API_BASE_URL=$GATEWAY/api/v1/support-disputes"
        "REFERRAL_API_BASE_URL=$GATEWAY/api/v1/referrals"
      ) ;;
    dispatch)
      # AppConfig keeps its LAN defaults as runtime fallbacks, so those strings
      # stay in the binary even though these settings always win.
      app=dispatch_provider needs_maps=0 allow_dev_fallback=1
      defines=(
        "DISPATCH_PROVIDER_API_BASE_URL=$GATEWAY"
        "DISPATCH_PROVIDER_WALLET_BASE_URL=$GATEWAY/api/v1/payment-wallet"
        "NOTIFICATION_WS_URL=$WS_URL"
        "SUPPORT_API_BASE_URL=$GATEWAY/api/v1/support-disputes"
      ) ;;
    *) echo "Unknown app: $name (use taxi, truck or dispatch)" >&2; exit 1 ;;
  esac

  local source_props="$ROOT_DIR/apps/$app/android/local.properties"
  if [[ "$needs_maps" == 1 ]] && ! grep -q '^GOOGLE_MAPS_API_KEY=.' "$source_props"; then
    echo "$app: GOOGLE_MAPS_API_KEY is missing from android/local.properties" >&2
    exit 1
  fi

  local export_dir="$WORK_DIR/$name"
  mkdir -p "$export_dir"
  git -C "$ROOT_DIR" archive "$COMMIT" "apps/$app" packages | tar -x -C "$export_dir"
  [[ -f "$source_props" ]] && cp "$source_props" "$export_dir/apps/$app/android/local.properties"

  echo "==> Building $name from $COMMIT"
  local flags=()
  local define
  for define in "${defines[@]}"; do flags+=("--dart-define=$define"); done
  (cd "$export_dir/apps/$app" && flutter pub get >/dev/null && flutter build apk --release "${flags[@]}")

  local built="$export_dir/apps/$app/build/app/outputs/flutter-apk/app-release.apk"
  local apk="$OUT_DIR/karryam-$name-driver.apk"
  cp "$built" "$apk"

  local certs
  certs="$("$APKSIGNER" verify --print-certs "$apk")" || { echo "$name: invalid signature" >&2; exit 1; }

  # The compiled Dart code holds the addresses the app will call.
  local code="$WORK_DIR/$name.libapp"
  unzip -p "$apk" lib/arm64-v8a/libapp.so > "$code"
  if ! LC_ALL=C grep -aq "$GATEWAY_HOST" "$code"; then
    echo "$name: gateway address not found in the build" >&2
    exit 1
  fi
  if [[ "$allow_dev_fallback" == 0 ]] && LC_ALL=C grep -aq '192\.168\.' "$code"; then
    echo "$name: a developer network address is still in the build" >&2
    exit 1
  fi

  (cd "$OUT_DIR" && shasum -a 256 "$(basename "$apk")" > "$(basename "$apk").sha256")
  echo "==> $name: $(du -h "$apk" | cut -f1)  $(grep 'SHA-1' <<<"$certs" | head -1)"
}

apps=("$@")
[[ ${#apps[@]} -gt 0 ]] || apps=(taxi truck dispatch)
for name in "${apps[@]}"; do build_app "$name"; done
echo "APKs in $OUT_DIR"
