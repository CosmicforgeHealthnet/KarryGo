#!/usr/bin/env bash
# Puts a service back on an earlier version.
#
#   rollback.sh <service-key>            go back to the version before the current one
#   rollback.sh <service-key> <image>    go to a specific version
#
# Examples:
#   rollback.sh payment-wallet-service
#   rollback.sh payment-wallet-service ghcr.io/cosmicforgehealthnet/payment-wallet-service:3f9c1e2...
#
# It uses deploy-service.sh, so it gets the same health check and safety net.
source "$(dirname "$0")/lib.sh"

[ $# -ge 1 ] && [ $# -le 2 ] || die "usage: rollback.sh <service-key> [image]"
load_service "$1"

[ "$SVC_KEY" != driver-dispatch-delivery-service ] || die "Dispatch rollback BLOCKED: old consumers ignore the uncertainty fence; retain evidence and use an approved forward recovery"

if [ $# -eq 2 ]; then
  target="$2"
else
  [ -f "$SVC_RELEASE_PREV" ] || die "no previous version is recorded for $SVC_KEY. Pass an image explicitly (see 'docker images' or GitHub Actions run history)."
  target="$(sed -n 's/^IMAGE=//p' "$SVC_RELEASE_PREV" | head -1)"
fi

log "rolling $SVC_KEY back from $(current_image || echo unknown) to $target"
exec "$(dirname "$0")/deploy-service.sh" "$SVC_KEY" "$target"
