#!/usr/bin/env bash
# CI entry point for fleet-partner-service.
#
# The repository has no hosted CI platform yet; this script is the
# platform-agnostic job any future CI system (or a developer) invokes:
#
#   bash services/fleet-partner-service/scripts/ci.sh
#
# Behaviour required by the foundation spec:
#   * a dedicated disposable PostgreSQL database is mandatory;
#   * integration tests FAIL (never skip) when it is unavailable;
#   * no staging/production credentials, no live downstream calls,
#     no other service's database.
#
# Provide FLEET_PARTNER_TEST_DATABASE_URL yourself, or let the script start
# a throwaway postgres:16-alpine container (requires Docker) that is removed
# afterwards.
set -euo pipefail

cd "$(dirname "$0")/../../.."   # repo root
SERVICE=services/fleet-partner-service

STARTED_CONTAINER=""
BUILD_DIR="$(mktemp -d)"
cleanup() {
  if [ -n "$STARTED_CONTAINER" ]; then
    docker rm -f "$STARTED_CONTAINER" >/dev/null 2>&1 || true
  fi
  rm -rf "$BUILD_DIR"
}
trap cleanup EXIT

if [ -z "${FLEET_PARTNER_TEST_DATABASE_URL:-}" ]; then
  if ! docker version >/dev/null 2>&1; then
    echo "CI FAIL: FLEET_PARTNER_TEST_DATABASE_URL is not set and Docker is unavailable." >&2
    echo "Mandatory integration tests must run; refusing to skip." >&2
    exit 1
  fi
  PW="ci_$(date +%s)_$RANDOM"
  STARTED_CONTAINER="karrygo-fleet-partner-ci-db-$$"
  docker run -d --name "$STARTED_CONTAINER" \
    -e POSTGRES_USER=fleet_partner_ci -e POSTGRES_PASSWORD="$PW" \
    -e POSTGRES_DB=karrygo_fleet_partner_test \
    -p 127.0.0.1:0:5432 postgres:16-alpine >/dev/null
  PORT=$(docker port "$STARTED_CONTAINER" 5432/tcp | head -1 | sed 's/.*://')
  for i in $(seq 1 30); do
    docker exec "$STARTED_CONTAINER" pg_isready -U fleet_partner_ci -d karrygo_fleet_partner_test >/dev/null 2>&1 && break
    sleep 2
    [ "$i" = 30 ] && { echo "CI FAIL: test database did not become ready" >&2; exit 1; }
  done
  export FLEET_PARTNER_TEST_DATABASE_URL="postgres://fleet_partner_ci:${PW}@127.0.0.1:${PORT}/karrygo_fleet_partner_test?sslmode=disable"
fi

echo "== gofmt =="
UNFORMATTED=$(gofmt -l "$SERVICE")
if [ -n "$UNFORMATTED" ]; then
  echo "CI FAIL: unformatted files:" >&2
  echo "$UNFORMATTED" >&2
  exit 1
fi

echo "== go vet =="
go vet ./$SERVICE/...

echo "== go build =="
go build ./$SERVICE/...

echo "== binaries =="
go build -o "$BUILD_DIR/fleet-partner-api" ./$SERVICE/cmd
go build -o "$BUILD_DIR/fleet-partner-worker" ./$SERVICE/cmd/worker

echo "== unit + architecture + migration-content tests =="
go test -count=1 ./$SERVICE/...

echo "== mandatory PostgreSQL integration tests (no skips) =="
OUTPUT=$(go test -count=1 -v ./$SERVICE/internal/integration/... 2>&1) || { echo "$OUTPUT"; exit 1; }
echo "$OUTPUT" | grep -E "^--- (PASS|FAIL|SKIP)" || true
if echo "$OUTPUT" | grep -q -- "--- SKIP"; then
  echo "CI FAIL: mandatory integration tests were skipped." >&2
  exit 1
fi

echo "CI PASS"
