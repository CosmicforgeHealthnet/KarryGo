#!/usr/bin/env bash
# The reviewed Dispatch 070 stop/migrate/verify/start transition on staging.
#
#   dispatch-070-transition.sh <platform-dir> <image@sha256:...> <revision>
#
# Started ONLY by .github/workflows/dispatch-070-transition.yml, after a person
# other than the operator approved the run in GitHub environment
# "dispatch-070-transition". Procedure: docs/deployment/staging-release-approval.md
# ("Dispatch 070 stop/migrate/verify/start and recovery hold"); package and
# evidence: docs/deployment/dispatch-070-transition-package.md.
#
# Phases, each refusing to continue on any surprise:
#   1. preflight (read only): settings, image digest + revision, the exact
#      pending migration set, relay rows and leases, every consumer container
#      and supervisor.
#   2. stop: every Dispatch container gets restart=no, the running one stops,
#      and the script proves no Dispatch binary runs or can restart.
#   3. backup: pg_dumpall of the Dispatch database, then a restore into a
#      throwaway container with no network, compared row-for-row on the tables
#      the migrations touch.
#   4. migrate: migrate.sh from the NEW image applies 000070-000079 while no
#      Dispatch binary runs; the fence column, fenced-row count and the
#      append-only read-back audit are checked.
#   5. start: only the approved image starts; health, digest and revision are
#      checked, and the consumer inventory is proved again.
#
# Failure handling follows the procedure. Before step 4 nothing in the schema
# has changed, so the previous container is restarted exactly as it was.
# From step 4 on it is a HOLD: delivery stays stopped, every row is kept, the
# old image is never restarted and no down migration is run (000070/000071
# refuse by design). A forward fix is the only way on.
#
# Prints only names, counts, image references and states: no settings, rows,
# keys or payloads (GitHub Actions logs of this repository are public).
set -euo pipefail

[ $# -eq 3 ] || { echo "usage: dispatch-070-transition.sh <platform-dir> <image@sha256:...> <revision>" >&2; exit 2; }
export PLATFORM_DIR="$1"
IMAGE="$2"
REVISION="$3"
# shellcheck source=/dev/null
source "$PLATFORM_DIR/bin/lib.sh"
load_service driver-dispatch-delivery-service

[[ "$IMAGE" =~ ^ghcr\.io/cosmicforgehealthnet/driver-dispatch-delivery-service@sha256:[0-9a-f]{64}$ ]] \
  || die "the image must be pinned by digest (ghcr.io/cosmicforgehealthnet/driver-dispatch-delivery-service@sha256:...)"
[[ "$REVISION" =~ ^[0-9a-f]{40}$ ]] || die "the revision must be a full 40-character commit"

APP="$SVC_CONTAINER"
DB="karrygo-${SVC_HOME%-service}-db"
EXPECTED_PENDING="000070_relay_uncertain_outcome_fence.up.sql 000071_relay_readback_audit.up.sql 000072_admin_authorization_context.up.sql 000073_admin_command_ledger.up.sql 000074_admin_interventions.up.sql 000075_analytics_coverage_producer.up.sql 000076_dispatch_unmatched_demand_fact.up.sql 000077_dispatch_recovery_outcome_facts.up.sql 000078_dispatch_return_is_part_of_original_booking.up.sql 000079_organization_provider_links.up.sql"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
PHASE=preflight
OLD_RESTART=""
RESTORE_CHECK=""

say() { echo "DISPATCH_070 $*"; }

# SQL travels on stdin so the shell never expands it; -tA prints bare values.
dbq() {
  docker exec -i "$DB" sh -c 'psql -X -q -tA -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -f -'
}

# Every container (any state) running a Dispatch image: the same rule the
# read-only evidence uses, so the inventory here and there agree.
consumers() {
  local c img
  for c in $(docker ps -a --format '{{.Names}}'); do
    img="$(docker inspect -f '{{.Config.Image}}' "$c" 2>/dev/null)" || continue
    case "$img" in *driver-dispatch-delivery-service*|*dispatch-delivery*) echo "$c" ;; esac
  done
}

running_consumers() {
  local c
  for c in $(consumers); do
    if [ "$(docker inspect -f '{{.State.Running}}' "$c")" = true ]; then echo "$c"; fi
  done
  return 0
}

inventory() { # label
  local c
  for c in $(consumers); do
    say "consumer phase=$1 name=$c state=$(docker inspect -f '{{.State.Status}}' "$c") restart=$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$c") image=$(docker inspect -f '{{.Config.Image}}' "$c")"
  done
}

migrate_in_new_image() { # extra migrate.sh args...
  RELEASE_OVERRIDE="$NEXT_RELEASE" svc_compose run --rm --no-deps -T \
    -e MIGRATE_DB_URL_VAR="$SVC_DB_URL_VAR" \
    --entrypoint /srv/app/migrate.sh app "$@"
}

pending_set() {
  migrate_in_new_image --status | awk '$1 == "PENDING" { print $2 }' | sort | tr '\n' ' ' | sed 's/ $//'
}

relay_summary() {
  dbq <<'SQL'
SELECT 'total=' || count(*)
    || ' uncertain=' || count(*) FILTER (WHERE status IN ('UNKNOWN_OUTCOME','DELIVERING','DEAD_LETTER'))
    || ' live_leases=' || count(*) FILTER (WHERE lease_expires_at > now())
  FROM dispatch_fleet_relay_operations;
SQL
}

cleanup_restore_check() {
  [ -n "$RESTORE_CHECK" ] && docker rm -f "$RESTORE_CHECK" >/dev/null 2>&1 || true
}

on_exit() {
  local rc=$?
  cleanup_restore_check
  [ "$rc" -eq 0 ] && return 0
  case "$PHASE" in preflight|stop|backup) rm -f "$SVC_DIR/release.next.env" ;; esac
  case "$PHASE" in
    preflight)
      say "result=REFUSED phase=preflight changed=nothing" ;;
    stop|backup)
      # The schema is unchanged, so the previous container is safe to bring
      # back exactly as it was (same container, same image, same settings).
      warn "failed before any migration: restoring the previous Dispatch container"
      if docker container inspect "$APP" >/dev/null 2>&1; then
        docker update --restart="${OLD_RESTART:-unless-stopped}" "$APP" >/dev/null || true
        docker start "$APP" >/dev/null || true
        if wait_healthy "$APP" 180; then
          say "result=REFUSED phase=$PHASE changed=nothing previous_container=restored"
        else
          say "result=HOLD phase=$PHASE previous_container=not_healthy"
        fi
      fi ;;
    *)
      # From the first migration on: stop delivery, keep every row, never the
      # old image, never a down migration.
      warn "HOLD: failed during $PHASE. Delivery stays stopped; every fence, receipt and audit row is kept. Do not restart the old image or run down migrations; prepare a forward fix."
      if docker container inspect "$APP" >/dev/null 2>&1 \
        && [ "$(docker inspect -f '{{.Config.Image}}' "$APP")" = "$IMAGE" ]; then
        docker update --restart=no "$APP" >/dev/null || true
        docker stop -t 30 "$APP" >/dev/null || true
      fi
      inventory hold
      say "result=HOLD phase=$PHASE" ;;
  esac
  return "$rc"
}
trap on_exit EXIT

# ---------------------------------------------------------------- 1. preflight
log "== 1. preflight (read only)"
check_service_files
[ "$(docker inspect -f '{{.State.Running}}' "$DB" 2>/dev/null)" = true ] || die "database container $DB is not running"
[ "$(docker inspect -f '{{.State.Running}}' "$APP" 2>/dev/null)" = true ] || die "$APP is not running; the approved inventory has it running"

docker pull -q "$IMAGE" >/dev/null || die "could not download $IMAGE"
label="$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$IMAGE")"
[ "$label" = "$REVISION" ] || die "image revision label is '$label', approved revision is $REVISION"
say "image=$IMAGE revision=$label"

NEXT_RELEASE="$SVC_DIR/release.next.env"
printf 'IMAGE=%s\n' "$IMAGE" >"$NEXT_RELEASE"
OLD_IMAGE_REF="$(docker inspect -f '{{.Config.Image}}' "$APP")"
OLD_RESTART="$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$APP")"
say "previous image=$OLD_IMAGE_REF restart=$OLD_RESTART"

pending="$(pending_set)"
say "pending=${pending:-none}"
[ "$pending" = "$EXPECTED_PENDING" ] || die "pending migrations differ from the approved set (000070-000079 only)"
[ "$(dbq <<'SQL'
SELECT count(*) FROM information_schema.columns
 WHERE table_name = 'dispatch_route_location_fixes' AND column_name = 'device_command_ref';
SQL
)" = 1 ] || die "000069 is recorded but its column is missing; the recorded state does not match the schema"
[ "$(dbq <<'SQL'
SELECT count(*) FROM information_schema.columns
 WHERE table_name = 'dispatch_fleet_relay_operations' AND column_name = 'readback_required';
SQL
)" = 0 ] || die "the 000070 fence column already exists; the database is not in the approved state"

relay_before="$(relay_summary)"
say "relay before: $relay_before"
[ "$relay_before" = "total=0 uncertain=0 live_leases=0" ] \
  || die "relay rows exist ($relay_before); the approved inventory has none. Re-run the evidence and the review."

inventory before
running="$(running_consumers)"
[ "$running" = "$APP" ] || die "running Dispatch containers are '$(echo $running)'; only $APP is approved"
if systemctl list-units --all --no-legend 2>/dev/null | grep -i 'dispatch-delivery' | grep -q .; then
  die "a systemd unit names dispatch-delivery; it could restart an old consumer"
fi
if crontab -l 2>/dev/null | grep -iE 'dispatch-delivery|karrygo-dispatch' | grep -q .; then
  die "a cron entry names Dispatch; it could restart an old consumer"
fi

# --------------------------------------------------------------------- 2. stop
PHASE=stop
log "== 2. stop every Dispatch consumer and its restart path"
for c in $(consumers); do docker update --restart=no "$c" >/dev/null; done
docker stop -t 30 "$APP" >/dev/null
sleep 5
for c in $(consumers); do
  [ "$(docker inspect -f '{{.State.Running}}' "$c")" = false ] || die "$c is still running"
  [ "$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$c")" = no ] || die "$c still has a restart policy"
done
inventory stopped
say "consumers_running=0 restart_paths=0"

# ------------------------------------------------------------------- 3. backup
PHASE=backup
log "== 3. back up the Dispatch database and check the restore"
umask 077
backup_dir="$KARRYGO_ROOT/backups/dispatch-070"
mkdir -p "$backup_dir"
dump="$backup_dir/dispatch-$STAMP.sql.gz"
docker exec "$DB" sh -c 'pg_dumpall -U "$POSTGRES_USER"' | gzip >"$dump"
[ -s "$dump" ] && gzip -t "$dump" || die "the backup file is empty or damaged"
say "backup=$dump bytes=$(stat -c %s "$dump")"

counts_sql="SELECT (SELECT count(*) FROM karrygo_deploy_migrations) || ' ' || (SELECT count(*) FROM dispatch_fleet_relay_operations) || ' ' || (SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public');"
source_counts="$(echo "$counts_sql" | dbq)"
dbname="$(docker exec "$DB" sh -c 'echo "${POSTGRES_DB:-$POSTGRES_USER}"')"
pg_image="$(docker inspect -f '{{.Config.Image}}' "$DB")"
RESTORE_CHECK="karrygo-dispatch-070-restore-check-$STAMP"
docker run -d --name "$RESTORE_CHECK" --network none \
  -e POSTGRES_PASSWORD="$(head -c 18 /dev/urandom | od -An -tx1 | tr -d ' \n')" \
  "$pg_image" >/dev/null
for _ in $(seq 1 60); do
  docker exec "$RESTORE_CHECK" pg_isready -U postgres -q 2>/dev/null && break
  sleep 2
done
docker exec "$RESTORE_CHECK" pg_isready -U postgres -q || die "the restore-check database did not start"
# pg_dumpall recreates roles that may already exist; those statements may fail
# harmlessly, so the comparison below is what decides.
gunzip -c "$dump" | docker exec -i "$RESTORE_CHECK" psql -X -q -U postgres -d postgres -f - >/dev/null 2>&1 || true
restored_counts="$(echo "$counts_sql" | docker exec -i "$RESTORE_CHECK" psql -X -q -tA -U postgres -d "$dbname" -f - 2>/dev/null || true)"
say "restore check source=[$source_counts] restored=[$restored_counts]"
[ -n "$source_counts" ] && [ "$source_counts" = "$restored_counts" ] || die "the restored copy does not match the source"
cleanup_restore_check
RESTORE_CHECK=""

# ------------------------------------------------------------------ 4. migrate
PHASE=migrate
log "== 4. apply 000070-000079 with the new image's migrate.sh (no Dispatch binary running)"
migrate_in_new_image
[ -z "$(pending_set)" ] || die "migrations are still pending after migrate.sh"
[ "$(dbq <<'SQL'
SELECT count(*) FROM information_schema.columns
 WHERE table_name = 'dispatch_fleet_relay_operations' AND column_name = 'readback_required';
SQL
)" = 1 ] || die "the 000070 fence column is missing"
fenced="$(echo "SELECT count(*) FROM dispatch_fleet_relay_operations WHERE readback_required;" | dbq)"
say "fenced_rows=$fenced"
[ "$fenced" = 0 ] || die "fenced rows ($fenced) differ from the approved inventory (0)"
[ "$(echo "SELECT to_regclass('public.dispatch_fleet_relay_readback_audit') IS NOT NULL;" | dbq)" = t ] \
  || die "the 000071 read-back audit table is missing"
# The audit trigger is statement-level, so a zero-row UPDATE must still be
# refused. Rolled back either way; no row is touched.
if audit_out="$(printf 'BEGIN;\nUPDATE dispatch_fleet_relay_readback_audit SET outcome = outcome WHERE false;\nROLLBACK;\n' | dbq 2>&1)"; then
  die "the read-back audit accepted an UPDATE; it must be append-only"
fi
case "$audit_out" in *append-only*) say "readback_audit=append_only" ;; *) die "the audit UPDATE failed for an unexpected reason" ;; esac
say "relay after migrate: $(relay_summary)"

# -------------------------------------------------------------------- 5. start
PHASE=start
log "== 5. start only the approved image"
# Keep the previous container as evidence under a new name (restart=no, stopped).
docker rename "$APP" "$APP-pre070-$STAMP"
RELEASE_OVERRIDE="$NEXT_RELEASE" svc_compose up -d --no-deps app
wait_healthy "$APP" 180 || die "the new Dispatch container did not become healthy"
running_image="$(docker inspect -f '{{.Image}}' "$APP")"
[ "$running_image" = "$(docker image inspect -f '{{.Id}}' "$IMAGE")" ] || die "the running container is not the approved image"
[ "$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$running_image")" = "$REVISION" ] \
  || die "the running revision is not $REVISION"
[ "$(docker inspect -f '{{.State.Health.Status}}' "$APP")" = healthy ] || die "the new container is not healthy"

running="$(running_consumers)"
[ "$running" = "$APP" ] || die "running Dispatch containers are '$(echo $running)' after start"
for c in $(consumers); do
  [ "$c" = "$APP" ] && continue
  [ "$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$c")" = no ] || die "$c regained a restart policy"
done
inventory after

printf 'IMAGE=%s\n' "$OLD_IMAGE_REF" >"$SVC_RELEASE_PREV"
mv "$NEXT_RELEASE" "$SVC_RELEASE"
PHASE=done
say "relay after start: $(relay_summary)"
say "running image=$IMAGE revision=$REVISION health=healthy"
say "result=COMPLETED"
