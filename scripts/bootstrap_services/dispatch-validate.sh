#!/usr/bin/env bash
set -euo pipefail

# Runtime integration validation for driver-dispatch-delivery-service.
#
# Builds the service with the CORRECT target (./cmd, not . — the service root has
# no Go files, only cmd/ holds package main), starts it, waits for /health, then
# runs a no-token route-protection check. Cleans up the service on exit.
#
# Prereq: dispatch Postgres (5435) + Redis (6382) must be up. If not, run:
#   bash scripts/bootstrap_services/dispatch-local-bootstrap.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVICE_DIR="$ROOT_DIR/services/driver-dispatch-delivery-service"

PORT="${PORT:-8103}"
PG_PORT="${PG_PORT:-5435}"
REDIS_PORT="${REDIS_PORT:-6382}"
HEALTH_URL="http://localhost:${PORT}/health"
PROTECTED_URL="http://localhost:${PORT}/api/v1/provider/vehicle"
HEALTH_RETRIES="${HEALTH_RETRIES:-30}"

log() {
  printf '\033[1;36m[dispatch-validate]\033[0m %s\n' "$*"
}

fail() {
  printf '\033[1;31m[dispatch-validate] ERROR:\033[0m %s\n' "$*" >&2
  exit 1
}

# --- cleanup -----------------------------------------------------------------
SERVICE_PID=""
TMPBIN=""
cleanup() {
  if [[ -n "$SERVICE_PID" ]] && kill -0 "$SERVICE_PID" 2>/dev/null; then
    log "stopping service (pid ${SERVICE_PID})"
    kill "$SERVICE_PID" 2>/dev/null || true
    wait "$SERVICE_PID" 2>/dev/null || true
  fi
  [[ -n "$TMPBIN" && -f "$TMPBIN" ]] && rm -f "$TMPBIN"
}
trap cleanup EXIT

echo "=================================================="
echo "DISPATCH RUNTIME INTEGRATION VALIDATION"
echo "=================================================="

# --- Step 0: dependency precheck ---------------------------------------------
log "checking Postgres (${PG_PORT}) and Redis (${REDIS_PORT}) are listening"
if ! lsof -nP -iTCP:"$PG_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  fail "Postgres not listening on ${PG_PORT}. Run: bash scripts/bootstrap_services/dispatch-local-bootstrap.sh"
fi
if ! lsof -nP -iTCP:"$REDIS_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  fail "Redis not listening on ${REDIS_PORT}. Run: bash scripts/bootstrap_services/dispatch-local-bootstrap.sh"
fi
log "dependencies up ✅"

# --- Step 1: build the correct target ----------------------------------------
TMPBIN="$(mktemp -t dispatch-svc.XXXXXX)"
log "building ./cmd -> ${TMPBIN}"
BUILD_LOG="$(mktemp -t dispatch-build.XXXXXX)"
if ! (cd "$SERVICE_DIR" && go build -o "$TMPBIN" ./cmd/) 2>"$BUILD_LOG"; then
  echo "----- go build output -----" >&2
  cat "$BUILD_LOG" >&2
  rm -f "$BUILD_LOG"
  fail "build failed"
fi
rm -f "$BUILD_LOG"
log "build succeeded ✅"

# --- Step 2: start service (from service dir so .env auto-loads) -------------
log "starting service on :${PORT}"
(cd "$SERVICE_DIR" && exec "$TMPBIN") &
SERVICE_PID=$!

# --- Step 3: health gate -----------------------------------------------------
log "waiting for ${HEALTH_URL} (up to ${HEALTH_RETRIES}s)"
ready=0
for _ in $(seq 1 "$HEALTH_RETRIES"); do
  if ! kill -0 "$SERVICE_PID" 2>/dev/null; then
    fail "service process exited before becoming ready (check its logs above)"
  fi
  code="$(curl -s -o /dev/null -w '%{http_code}' "$HEALTH_URL" 2>/dev/null || true)"
  if [[ "$code" == "200" ]]; then
    ready=1
    break
  fi
  sleep 1
done
[[ "$ready" == "1" ]] || fail "service did not become ready on ${HEALTH_URL}"
log "service ready ✅"

# --- Step 4: route protection check (no token -> 401) ------------------------
echo
echo "--- Step A: Route Protection Check (No Token) ---"
code="$(curl -s -o /dev/null -w '%{http_code}' "$PROTECTED_URL" 2>/dev/null || true)"
if [[ "$code" == "401" ]]; then
  printf '\033[1;32m✅ %s returned 401 (protected)\033[0m\n' "$PROTECTED_URL"
else
  fail "expected 401 from ${PROTECTED_URL}, got ${code}"
fi

echo
log "validation passed 🎉"
