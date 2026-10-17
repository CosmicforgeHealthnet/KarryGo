#!/usr/bin/env bash
# Deploys one service to a given image, safely.
#
#   deploy-service.sh <service-key> <image>
#   e.g. deploy-service.sh customer-service ghcr.io/cosmicforgehealthnet/customer-service:3f9c1e2...
#
# In order, it:
#   1. checks the private settings files exist and are owner-only (mode 600)
#   2. downloads the image
#   3. starts the service's own database/Redis if it has them
#   4. prepares database tables (migrate=script services; others do it at startup)
#   5. starts the new version and waits until Docker reports it "healthy"
#   6. if it never gets healthy, puts the previous version back and fails
#
# The GitHub workflow calls this; you can also run it by hand on the server.
source "$(dirname "$0")/lib.sh"

[ $# -eq 2 ] || die "usage: deploy-service.sh <service-key> <image>"
load_service "$1"
NEW_IMAGE="$2"

case "$NEW_IMAGE" in
  *:latest|*:staging) die "refusing a reusable tag ($NEW_IMAGE). Deploy an immutable :<commit> tag so it can be rolled back." ;;
esac

log "== deploying $SVC_KEY -> $NEW_IMAGE"
check_service_files

log "downloading the image"
docker pull -q "$NEW_IMAGE" >/dev/null

# Write the new release to a side file first; release.env only changes once the
# new version is healthy, so a failed deploy leaves the record pointing at the
# version that is actually running.
NEW_RELEASE="$SVC_DIR/release.next.env"
printf 'IMAGE=%s\n' "$NEW_IMAGE" >"$NEW_RELEASE"
PREV_IMAGE="$(current_image || true)"

has_compose_service() { grep -qE "^  $1:" "$SVC_COMPOSE"; }

# 3. The service's own database and Redis (new services only).
deps=()
has_compose_service db && deps+=(db)
has_compose_service redis && deps+=(redis)
if [ "${#deps[@]}" -gt 0 ]; then
  log "making sure ${deps[*]} is up"
  RELEASE_OVERRIDE="$NEW_RELEASE" svc_compose up -d --wait "${deps[@]}"
fi

# Helpers that are neither program, database nor Redis (e.g. media's virus
# scanner). Started if missing, never recreated when already running; they can
# take minutes to become healthy, so the deploy does not wait for them.
helpers="$(awk '
  /^services:/ { in_services = 1; next }
  /^[^ #]/     { in_services = 0 }
  in_services && /^  [a-z][a-z0-9_-]*:/ { s = $1; sub(":", "", s); if (s != "app" && s != "worker" && s != "db" && s != "redis") print s }
' "$SVC_COMPOSE")"
if [ -n "$helpers" ]; then
  log "making sure $(echo $helpers) is running"
  # shellcheck disable=SC2086
  RELEASE_OVERRIDE="$NEW_RELEASE" svc_compose up -d --no-recreate $helpers
fi

# 4. Database tables.
if [ "$SVC_MIGRATE" = "script" ]; then
  log "preparing database tables (migrate.sh, setting $SVC_DB_URL_VAR)"
  RELEASE_OVERRIDE="$NEW_RELEASE" svc_compose run --rm --no-deps -T \
    -e MIGRATE_DB_URL_VAR="$SVC_DB_URL_VAR" \
    --entrypoint /srv/app/migrate.sh app
else
  log "database tables: the service prepares them itself at startup ($SVC_MIGRATE)"
fi

# 5. Start the new version of the program containers.
targets=(app)
has_compose_service worker && targets+=(worker)
log "starting ${targets[*]}"
RELEASE_OVERRIDE="$NEW_RELEASE" svc_compose up -d --no-deps "${targets[@]}"

healthy=true
for c in $(app_containers); do
  log "waiting for $c to report healthy (up to 2 minutes)"
  if ! wait_healthy "$c" 120; then
    healthy=false
    warn "$c did not become healthy. Its last log lines:"
    docker logs --tail 40 "$c" 2>&1 | redact | sed 's/^/    /' >&2 || true
    break
  fi
done

if $healthy; then
  if [ -n "$PREV_IMAGE" ] && [ "$PREV_IMAGE" != "$NEW_IMAGE" ]; then
    printf 'IMAGE=%s\n' "$PREV_IMAGE" >"$SVC_RELEASE_PREV"
  fi
  mv "$NEW_RELEASE" "$SVC_RELEASE"
  log "== $SVC_KEY is healthy on $NEW_IMAGE"
  exit 0
fi

# 6. Automatic rollback.
rm -f "$NEW_RELEASE"
if [ -n "$PREV_IMAGE" ] && [ "$PREV_IMAGE" != "$NEW_IMAGE" ]; then
  warn "putting the previous version back: $PREV_IMAGE"
  svc_compose up -d --no-deps "${targets[@]}"
  for c in $(app_containers); do
    wait_healthy "$c" 120 || warn "$c is not healthy on the previous version either. Look at: docker logs $c"
  done
  warn "note: database table changes from the failed version (if any) are NOT undone; they are designed to be additive."
else
  warn "no previous version is recorded, so there is nothing to roll back to."
fi
die "deploy of $SVC_KEY to $NEW_IMAGE failed (see the log lines above)"
