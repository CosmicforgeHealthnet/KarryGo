#!/usr/bin/env bash
set -euo pipefail

# Runs every *-local-bootstrap.sh in this directory, in order.
# Each child script provisions its own Postgres/Redis on its own ports and
# data dirs, so they do not collide. Children are idempotent and safe to re-run.
#
# Usage:
#   bash scripts/bootstrap/bootstrap-all.sh            # run all
#   bash scripts/bootstrap/bootstrap-all.sh customer hauling   # run a subset
#   KEEP_GOING=1 bash scripts/bootstrap/bootstrap-all.sh       # don't stop on first failure

BOOTSTRAP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Run order. Infra services first, then the customer service.
SERVICES=(
  media
  notification
  payment
  hauling
  customer
)

KEEP_GOING="${KEEP_GOING:-0}"

log() {
  printf '\033[1;35m[bootstrap-all]\033[0m %s\n' "$*"
}

err() {
  printf '\033[1;31m[bootstrap-all]\033[0m %s\n' "$*" >&2
}

# Allow running a subset: `bootstrap-all.sh customer hauling`
if [ "$#" -gt 0 ]; then
  SERVICES=("$@")
fi

declare -a SUCCEEDED=()
declare -a FAILED=()

for service in "${SERVICES[@]}"; do
  script="$BOOTSTRAP_DIR/${service}-local-bootstrap.sh"

  if [ ! -f "$script" ]; then
    err "no bootstrap script for '${service}' (expected ${script})"
    FAILED+=("$service")
    [ "$KEEP_GOING" = "1" ] && continue
    err "aborting (set KEEP_GOING=1 to continue past failures)"
    exit 1
  fi

  log "=== ${service} ==="
  if bash "$script"; then
    SUCCEEDED+=("$service")
  else
    err "${service} bootstrap failed"
    FAILED+=("$service")
    if [ "$KEEP_GOING" != "1" ]; then
      err "aborting (set KEEP_GOING=1 to continue past failures)"
      exit 1
    fi
  fi
done

echo
log "summary:"
[ "${#SUCCEEDED[@]}" -gt 0 ] && log "  ok:     ${SUCCEEDED[*]}"
[ "${#FAILED[@]}" -gt 0 ] && err "  failed: ${FAILED[*]}"

if [ "${#FAILED[@]}" -gt 0 ]; then
  exit 1
fi

log "all bootstraps complete. start the Go services from their service directories, e.g.:"
log "  cd services/driver-hauling-service && go run ./cmd"
