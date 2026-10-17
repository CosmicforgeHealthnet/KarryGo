#!/usr/bin/env bash
# Makes a service pick up changes to its settings (env) file.
#
#   apply-env.sh <service-key>
#
# Why this exists: "docker restart" does NOT re-read a settings file. Docker
# copies the values into the container when it is created, so the container has
# to be recreated. This recreates only the program container(s) on the version
# that is already deployed, then waits until it is healthy again.
source "$(dirname "$0")/lib.sh"

[ $# -eq 1 ] || die "usage: apply-env.sh <service-key>"
load_service "$1"
check_service_files
[ -f "$SVC_RELEASE" ] || die "$SVC_KEY has no release.env yet; deploy it first."

targets=(app)
grep -qE '^  worker:' "$SVC_COMPOSE" && targets+=(worker)

log "recreating ${targets[*]} of $SVC_KEY with the updated settings ($(current_image))"
svc_compose up -d --no-deps --force-recreate "${targets[@]}"

for c in $(app_containers); do
  log "waiting for $c to report healthy"
  if ! wait_healthy "$c" 120; then
    docker logs --tail 40 "$c" 2>&1 | redact | sed 's/^/    /' >&2 || true
    die "$c is not healthy with the new settings. Check the value you changed in $SVC_ENV_PATH, then run this again."
  fi
done
log "$SVC_KEY is healthy with the updated settings"
