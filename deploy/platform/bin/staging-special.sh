#!/usr/bin/env bash
# Routine staging-only updates. Do not use this path for a first cutover,
# privileged enrollment, irreversible migration or unverified rollback.
set -euo pipefail
umask 077
[ "$#" -eq 3 ] || exit 2
svc="$1"; image="$2"; mode="$3"
[[ "$svc" =~ ^[a-z0-9-]+$ && "$image" =~ ^ghcr\.io/cosmicforgehealthnet/$svc:[0-9a-f]{40}$ ]] || exit 2
source "$(dirname "$0")/lib.sh"

vendor_compose() {
  VENDOR_IMAGE="$1" VENDOR_ENV_FILE=/opt/karrygo/vendor-service/vendor.env \
    docker compose -f "$PLATFORM_DIR/vendor.yml" -f /opt/karrygo/vendor-service/network.override.yml "${@:2}"
}
running() { [ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null)" = true ]; }
require_staging() {
  # Never treat absent environment identity as permission to mutate.
  python3 - "$1" <<'PY'
import sys
from pathlib import Path
values = [line.split("=", 1)[1].strip().strip("\"'") for line in Path(sys.argv[1]).read_text().splitlines()
          if line.startswith("APP_ENV=")]
if values != ["staging"]:
    raise SystemExit("explicit APP_ENV=staging required")
PY
}
verify_image() {
  local c="$1" ref="$2"
  [ "$(docker inspect -f '{{.Image}}' "$c")" = "$(docker image inspect -f '{{.Id}}' "$ref")" ] || die "runtime image mismatch: $c"
  wait_healthy "$c" 120 || die "runtime not healthy: $c"
}
migration_manifest() {
  # Compare exact SQL contents in isolated, non-networked, read-only containers.
  docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
    --entrypoint sh "$1" -ec 'cd "$1"; find . -type f -name "*.sql" | sort | while IFS= read -r f; do sha256sum "$f"; done' sh "$2"
}
backup_db() {
  local db="$1" out
  running "$db" || die "required database container is not running: $db"
  mkdir -p "$KARRYGO_ROOT/backups/predeploy"
  out="$KARRYGO_ROOT/backups/predeploy/$svc-$(date -u +%Y%m%dT%H%M%S)-$$.sql.gz"
  docker exec "$db" sh -c 'pg_dumpall -U "$POSTGRES_USER"' | gzip >"$out"
  test -s "$out" && gzip -t "$out"
  log "verified compressed backup for $svc"
}
preflight() {
  case "$svc" in
    vendor-service)
      check_private_file /opt/karrygo/vendor-service/vendor.env
      require_staging /opt/karrygo/vendor-service/vendor.env
      [ -r /opt/karrygo/vendor-service/network.override.yml ] || die "Vendor network enrollment missing"
      for c in karrygo-vendor-api-staging karrygo-vendor-worker-staging; do
        running "$c" || die "Vendor existing runtime missing: $c"
        [ "$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$c")" = karrygo-vendor-staging ] || die "unexpected Vendor Compose project"
      done
      running karrygo-vendor-staging-postgres || die "Vendor backup database missing"
      ;;
    *)
      load_service "$svc"
      check_service_files
      python3 "$PLATFORM_DIR/bin/check-admin-disabled.py" "$SVC_ENV_PATH"
      case "$svc" in
        driver-dispatch-delivery-service)
          running "$SVC_CONTAINER" || die "Dispatch post-070 runtime missing"
          running karrygo-dispatch-delivery-db || die "Dispatch database missing"
          ;;
        api-gateway|partner-web-service)
          # Never start duplicate legacy billing/webhook consumers or cut
          # the public gateway over without a verified existing split.
          for f in /opt/karrygo/api-gateway/public-gateway.env /opt/karrygo/partner-web-service/partner.env; do
            check_private_file "$f"
            require_staging "$f"
          done
          running karrygo-api-gateway && running karrygo-partner-web-service || die "Gateway/Partner cutover not complete"
          docker exec karrygo-api-gateway wget -qO- http://127.0.0.1:8080/api/v1/gateway/meta |
            python3 -c 'import json,sys; d=json.load(sys.stdin); assert d.get("partner_upstream_configured") is True'
          ;;
        analytics-service)
          require_staging "$SVC_ENV_PATH"
          check_private_file "$SVC_DIR/redis.env"
          docker network inspect cf-staging-karryam-owner-readonly >/dev/null
          [ -d /etc/cosmicforge/staging/karryam-analytics/runtime ] || die "Analytics source-reader credential enrollment missing"
          ;;
        admin-backoffice-service)
          # Existing operator enrollment remains the trust anchor. No new
          # signing keys, role grants, public listeners or VPN changes.
          check_private_file "$SVC_DIR/admin.migrate.env"
          check_private_file "$SVC_DIR/bridge.certification"
          running "$SVC_CONTAINER" || die "Admin first private enrollment not complete"
          for f in server.crt server.key hq-client-ca.pem trust.json; do
            [ -f "$SVC_DIR/secrets/$f" ] || die "Admin enrollment material missing: $f"
          done
          ;;
      esac
      ;;
  esac
}
preflight
[ "$mode" = preflight ] && exit 0
[ "$mode" = deploy ] || exit 2
docker pull -q "$image" >/dev/null
case "$svc" in
  vendor-service)
    old="$(docker inspect -f '{{.Config.Image}}' karrygo-vendor-api-staging)"
    [ "$(docker inspect -f '{{.Image}}' karrygo-vendor-api-staging)" = "$(docker inspect -f '{{.Image}}' karrygo-vendor-worker-staging)" ] || die "Vendor API/worker revisions differ"
    docker image inspect "$old" >/dev/null
    before="$(migration_manifest "$old" /srv/vendor-service/migrations)"
    after="$(migration_manifest "$image" /srv/vendor-service/migrations)"
    [ -n "$before" ] && [ "$before" = "$after" ] || die "Vendor schema changed; requires compatible migration/forward recovery procedure"
    backup_db karrygo-vendor-staging-postgres
    vendor_compose "$image" config --quiet
    vendor_compose "$image" stop vendor-worker
    # The normal owner migrator is idempotent and verifies the migration tip.
    # If it fails, leave the worker stopped; do not guess schema compatibility.
    vendor_compose "$image" --profile operations run --rm -T vendor-migrate
    if vendor_compose "$image" up -d --no-deps vendor-api && wait_healthy karrygo-vendor-api-staging 180 &&
       vendor_compose "$image" up -d --no-deps vendor-worker &&
       running karrygo-vendor-worker-staging; then
      verify_image karrygo-vendor-api-staging "$image"
      verify_image karrygo-vendor-worker-staging "$image"
      [ -z "$(docker port karrygo-vendor-worker-staging)" ] || die "Vendor worker unexpectedly published"
      printf 'IMAGE=%s\n' "$image" > /opt/karrygo/vendor-service/release.env
      log "Vendor runtime verified"
    else
      vendor_compose "$old" up -d --no-deps vendor-api
      wait_healthy karrygo-vendor-api-staging 180 && vendor_compose "$old" up -d --no-deps vendor-worker
      die "Vendor update failed; previous compatible image restored"
    fi
    ;;
  driver-dispatch-delivery-service)
    old="$(docker inspect -f '{{.Config.Image}}' "$SVC_CONTAINER")"
    # A post-fence routine replacement is allowed only with an unchanged
    # migration manifest and an existing complete 070/071 schema fence.
    before="$(migration_manifest "$old" /srv/app/migrations)"
    after="$(migration_manifest "$image" /srv/app/migrations)"
    [ -n "$before" ] && [ "$before" = "$after" ] || die "Dispatch schema changed; forward migration certification required"
    db=karrygo-dispatch-delivery-db
    proof="$(docker exec -i "$db" sh -c 'psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}"' <<'SQL'
SELECT (EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='dispatch_fleet_relay_operations' AND column_name='readback_required')
AND to_regclass('public.dispatch_fleet_relay_readback_audit') IS NOT NULL
AND EXISTS (SELECT 1 FROM karrygo_deploy_migrations WHERE filename LIKE '000070_%')
AND EXISTS (SELECT 1 FROM karrygo_deploy_migrations WHERE filename LIKE '000079_%'))::text;
SQL
)"
    [ "$proof" = true ] || die "Dispatch 070 fence not proven"
    for c in $(docker ps -a --format '{{.Names}}'); do
      [ "$c" = "$SVC_CONTAINER" ] && continue
      ref="$(docker inspect -f '{{.Config.Image}}' "$c")"
      case "$ref" in *driver-dispatch-delivery-service*|*dispatch-delivery*)
        [ "$(docker inspect -f '{{.State.Running}} {{.HostConfig.RestartPolicy.Name}}' "$c")" = "false no" ] || die "another Dispatch consumer can run: $c";;
      esac
    done
    backup_db "$db"
    next=$(mktemp "$SVC_DIR/release.XXXXXX")
    trap 'rm -f "$next"' EXIT
    printf 'IMAGE=%s\n' "$image" >"$next"
    # No migrations, no generic rollback; only the verified post-fence old
    # image may be restored if the candidate fails readiness.
    if RELEASE_OVERRIDE="$next" svc_compose up -d --no-deps app && wait_healthy "$SVC_CONTAINER" 180; then
      verify_image "$SVC_CONTAINER" "$image"
      printf 'IMAGE=%s\n' "$old" >"$SVC_RELEASE_PREV"
      mv "$next" "$SVC_RELEASE"
      log "Dispatch post-fence image verified"
    else
      printf 'IMAGE=%s\n' "$old" >"$next"
      RELEASE_OVERRIDE="$next" svc_compose up -d --no-deps app
      wait_healthy "$SVC_CONTAINER" 180 || die "Dispatch compatible rollback failed"
      die "Dispatch update failed; previous post-fence image restored"
    fi
    ;;
  admin-backoffice-service)
    # Do not manufacture the operator's image certification or grant access.
    # Existing secure enrollment plus image-specific certification is still
    # needed until a live bridge acceptance automation is available.
    grep -Fxq "$image" "$SVC_DIR/bridge.certification" || die "Admin image certification not yet automated"
    backup_db karrygo-admin-db
    bash "$PLATFORM_DIR/bin/deploy-service.sh" "$svc" "$image"
    verify_image "$SVC_CONTAINER" "$image"
    ;;
  api-gateway|partner-web-service|analytics-service)
    case "$svc" in
      partner-web-service) backup_db karrygo-api-gateway-db ;;
      analytics-service) backup_db karrygo-analytics-db ;;
    esac
    bash "$PLATFORM_DIR/bin/deploy-service.sh" "$svc" "$image"
    verify_image "$SVC_CONTAINER" "$image"
    ;;
  *) die "unsupported special staging service" ;;
esac
