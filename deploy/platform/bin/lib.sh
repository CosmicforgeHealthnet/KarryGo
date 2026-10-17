#!/usr/bin/env bash
# Shared helpers for the deploy/platform/bin scripts. Not run directly.
#
# Layout on the server:
#   /opt/karrygo/platform/            this folder, copied from the repo by CI
#   /opt/karrygo/<home>/<env_file>    a service's private settings (mode 600)
#   /opt/karrygo/<home>/db.env        its database/Redis passwords (new services)
#   /opt/karrygo/<home>/release.env   IMAGE=<the image that is deployed now>
#   /opt/karrygo/<home>/release.previous.env   the one before (for rollback)

set -euo pipefail

KARRYGO_ROOT="${KARRYGO_ROOT:-/opt/karrygo}"
PLATFORM_DIR="${PLATFORM_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
REGISTRY="$PLATFORM_DIR/services.tsv"

log()  { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*"; }
warn() { printf '[%s] WARNING: %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }
die()  { printf '[%s] ERROR: %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; exit 1; }

# redact: blacks out passwords before anything is printed. Container logs are
# shown when a deploy fails, and GitHub Actions logs of a public repo are
# public, so a service that logs its own database address (some do, password
# included) must not leak it there.
redact() {
  sed -E \
    -e 's#(://[^:/@[:space:]]+:)[^@[:space:]]+@#\1***@#g' \
    -e 's#((PASSWORD|SECRET|TOKEN|_KEY|PEPPER)[A-Za-z_]*[=:][[:space:]]*"?)[^"[:space:],]+#\1***#gI'
}

# The modern "docker compose" plugin, or the standalone v2 binary as a fallback.
compose_bin() {
  if docker compose version >/dev/null 2>&1; then
    echo "docker compose"
  elif command -v docker-compose >/dev/null 2>&1; then
    echo "docker-compose"
  else
    die "neither 'docker compose' nor 'docker-compose' is installed (run server-setup.sh)"
  fi
}

# Lists every service key in the register.
all_services() {
  grep -vE '^\s*(#|$)' "$REGISTRY" | cut -d'|' -f1 | tr -d ' '
}

# load_service <key>: sets SVC_* variables from the register row.
load_service() {
  local key="$1" row
  # Compare a trimmed copy of the first column: editing $1 itself would make
  # awk rebuild the line with spaces in place of the "|" separators.
  row="$(grep -vE '^\s*(#|$)' "$REGISTRY" | awk -F'|' -v k="$key" '{ first = $1; gsub(/ /, "", first) } first == k { print; exit }')"
  [ -n "$row" ] || die "unknown service '$key' (see $REGISTRY)"
  IFS='|' read -r SVC_KEY SVC_DOCKERFILE SVC_HOME SVC_CONTAINER SVC_PORT SVC_ENV_FILE SVC_MIGRATE SVC_DB_URL_VAR <<<"$(echo "$row" | tr -d ' ')"
  SVC_DIR="$KARRYGO_ROOT/$SVC_HOME"
  SVC_ENV_PATH="$SVC_DIR/$SVC_ENV_FILE"
  SVC_COMPOSE="$PLATFORM_DIR/compose/$SVC_KEY.yml"
  SVC_RELEASE="$SVC_DIR/release.env"
  SVC_RELEASE_PREV="$SVC_DIR/release.previous.env"
  [ -f "$SVC_COMPOSE" ] || die "missing compose file $SVC_COMPOSE"
}

# svc_compose <args...>: runs compose for the loaded service with its release file.
svc_compose() {
  local release="${RELEASE_OVERRIDE:-$SVC_RELEASE}"
  # shellcheck disable=SC2046
  $(compose_bin) --project-directory "$PLATFORM_DIR/compose" \
    -f "$SVC_COMPOSE" --env-file "$release" "$@"
}

# check_private_file <path>: the file must exist and be readable by its owner only.
check_private_file() {
  local file="$1" mode
  [ -f "$file" ] || die "$file is missing. Create it from deploy/platform/env/ (see the guide, section G)."
  mode="$(stat -c '%a' "$file")"
  [ "$mode" = "600" ] || die "$file must have mode 600 (owner only); it is $mode. Fix: chmod 600 $file"
}

# check_service_files: every settings file the loaded service's compose file uses.
check_service_files() {
  check_private_file "$SVC_ENV_PATH"
  if grep -q "/opt/karrygo/$SVC_HOME/db.env" "$SVC_COMPOSE"; then
    check_private_file "$SVC_DIR/db.env"
  fi
}

# The program containers (compose services "app" and "worker") of the loaded
# service. Databases, Redis and the virus scanner are not replaced on a deploy,
# so a deploy only waits on these.
app_containers() {
  awk '
    /^services:/ { in_services = 1; next }
    /^[^ #]/     { in_services = 0 }
    in_services && /^  [a-z][a-z0-9_-]*:/ { svc = $1; sub(":", "", svc) }
    in_services && /^    container_name:/ && (svc == "app" || svc == "worker") { print $2 }
  ' "$SVC_COMPOSE"
}

# wait_healthy <container> [timeout_seconds]: waits for Docker to report "healthy".
wait_healthy() {
  local container="$1" timeout="${2:-120}" waited=0 status
  while :; do
    status="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container" 2>/dev/null || echo missing)"
    case "$status" in
      healthy|running) return 0 ;;  # "running" = a container with no health check (e.g. a worker)
      unhealthy|exited|dead|missing) [ "$waited" -ge 20 ] && return 1 ;;
    esac
    [ "$waited" -ge "$timeout" ] && return 1
    sleep 5
    waited=$((waited + 5))
  done
}

# current_image: the image recorded as deployed for the loaded service.
current_image() {
  [ -f "$SVC_RELEASE" ] && sed -n 's/^IMAGE=//p' "$SVC_RELEASE" | head -1
}
