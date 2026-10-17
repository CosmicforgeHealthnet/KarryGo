#!/usr/bin/env bash
# Rolls Vendor staging onto an image that is already on the server.
#
#   bash deploy-manual.sh karrygo/vendor-service:<commit>
#
# In order: backs up the Vendor database, points .env at the new image, runs
# the migration job, starts the API and waits until it is healthy, then starts
# the Worker. If the API never becomes healthy it puts the previous image back.
# Run it on the VPS from /opt/karrygo/vendor-service.
set -euo pipefail

NEW_IMAGE="${1:?usage: deploy-manual.sh karrygo/vendor-service:<commit>}"
cd /opt/karrygo/vendor-service
log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }

docker image inspect "$NEW_IMAGE" >/dev/null 2>&1 || { echo "image $NEW_IMAGE is not on this server" >&2; exit 1; }

TS="$(date +%Y%m%d-%H%M%S)"
BACKUP="backups/vendor-before-${NEW_IMAGE##*:}-$TS.dump"
log "backing up the Vendor database to $BACKUP"
docker exec karrygo-vendor-staging-postgres sh -c 'pg_dump -U "$POSTGRES_USER" -Fc "$POSTGRES_DB"' >"$BACKUP"
[ -s "$BACKUP" ] || { echo "backup is empty; stopping" >&2; exit 1; }
log "backup is $(stat -c %s "$BACKUP") bytes"

PREV_IMAGE="$(sed -n 's/^VENDOR_IMAGE=//p' .env)"
log "previous image: $PREV_IMAGE"
cp -p .env ".env.before-$TS"

use_image() {
  sed -i "s#^VENDOR_IMAGE=.*#VENDOR_IMAGE=$1#" .env
  set -a; . ./.env; set +a
}
# network.override.yml puts Vendor on the shared karrygo-net, where its
# PostgreSQL, Redis and the other services live. Without it Compose creates a
# private network and the migration cannot find the database.
[ -f network.override.yml ] || { echo "network.override.yml is missing; Vendor would start cut off from its database" >&2; exit 1; }
compose() { docker compose -f docker-compose.staging.yml -f network.override.yml "$@"; }
# A run without the override leaves an empty private network behind; drop it.
docker network rm karrygo-vendor-staging_default >/dev/null 2>&1 || true

use_image "$NEW_IMAGE"
log "running database migrations"
compose --profile operations run --rm vendor-migrate

log "starting the API on $NEW_IMAGE"
compose up -d vendor-api
for i in $(seq 1 30); do
  [ "$(docker inspect -f '{{.State.Health.Status}}' karrygo-vendor-api-staging)" = healthy ] && break
  if [ "$i" -eq 30 ]; then
    log "API is NOT healthy; putting $PREV_IMAGE back"
    docker logs --tail 30 karrygo-vendor-api-staging 2>&1 | sed 's/^/    /'
    use_image "$PREV_IMAGE"
    compose up -d vendor-api vendor-worker
    exit 1
  fi
  sleep 5
done
log "API is healthy"

compose up -d vendor-worker
sleep 5
[ "$(docker inspect -f '{{.State.Running}}' karrygo-vendor-worker-staging)" = true ] || { log "Worker is not running; check: docker logs karrygo-vendor-worker-staging"; exit 1; }
log "Worker is running"
log "done: Vendor is on $NEW_IMAGE (rollback: bash deploy-manual.sh $PREV_IMAGE)"
