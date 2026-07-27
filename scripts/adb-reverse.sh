#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# adb-reverse.sh — restore the adb reverse tunnels the mobile apps need on a
# physical Android phone.
#
# The apps default to http://127.0.0.1:<port>. On a physical device that is the
# phone's own loopback, so every API call fails ("cannot connect to server")
# unless an adb reverse tunnel forwards the phone's 127.0.0.1:<port> back to
# this machine's <port>.
#
# These tunnels are NOT persistent — they drop on device unplug/reconnect, an
# adb server restart, or (on wireless adb) a Wi-Fi/wireless-debug drop. Re-run
# this script to restore them.
#
# Usage:
#   bash scripts/adb-reverse.sh            # set up tunnels (auto-picks device)
#   bash scripts/adb-reverse.sh --list     # show active tunnels
#   bash scripts/adb-reverse.sh --clear    # remove all tunnels
#   DEVICE=<id> bash scripts/adb-reverse.sh   # target a specific device
#
# Ports (backend services all apps may talk to):
#   8101 customer-service        8105 payment-wallet-service
#   8103 dispatch-delivery       8106 notification-service (websocket)
#   8104 hauling-service         8107 support-dispute-service
#                                8109 media-file-service
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

PORTS=(8101 8103 8104 8105 8106 8107 8109)

if ! command -v adb >/dev/null 2>&1; then
  echo "error: adb not found on PATH" >&2
  exit 1
fi

# Pick a device: honor $DEVICE, else the single connected device.
device_arg=()
if [[ -n "${DEVICE:-}" ]]; then
  device_arg=(-s "$DEVICE")
else
  count=$(adb devices | awk 'NR>1 && $2=="device"' | wc -l | tr -d ' ')
  if [[ "$count" == "0" ]]; then
    echo "error: no Android device connected (adb devices is empty)" >&2
    exit 1
  elif [[ "$count" != "1" ]]; then
    echo "error: multiple devices connected — set DEVICE=<id>. Devices:" >&2
    adb devices | awk 'NR>1 && $2=="device"{print "  "$1}' >&2
    exit 1
  fi
fi

case "${1:-}" in
  --list)
    adb ${device_arg[@]+"${device_arg[@]}"} reverse --list
    exit 0
    ;;
  --clear)
    adb ${device_arg[@]+"${device_arg[@]}"} reverse --remove-all
    echo "cleared all reverse tunnels"
    exit 0
    ;;
esac

for p in "${PORTS[@]}"; do
  adb ${device_arg[@]+"${device_arg[@]}"} reverse "tcp:$p" "tcp:$p" >/dev/null
  echo "reversed tcp:$p"
done

echo "── active tunnels ──"
adb ${device_arg[@]+"${device_arg[@]}"} reverse --list
