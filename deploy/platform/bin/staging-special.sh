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
  [ "$(docker inspect -f '{{.Image}}' "$c")" = "$(docker image inspect -f '{{.Id}}' "$ref")" ] || { log "runtime image mismatch: $c"; return 1; }
  wait_healthy "$c" 120 || { log "runtime not healthy: $c"; return 1; }
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
# The VPS guide authorizes the deploy account to own KarryGo settings, mode
# 600. Never widen access, use Docker to bypass file permissions, or add sudo
# rules. The repair uses an existing exact-command sudo grant only.
vendor_settings() {
  local f=/opt/karrygo/vendor-service/vendor.env
  if [ -r "$f" ]; then
    check_private_file "$f"
    require_staging "$f"
    return
  fi
  [ -f "$f" ] && [ ! -L "$f" ] &&
    [ "$(readlink -f "$f")" = "$f" ] &&
    [ "$(stat -c '%u:%a:%h' "$f")" = "0:600:1" ] ||
    die "Vendor configuration is not a regular root-owned 0600 file"
  [ "$(id -un)" = deploy ] || die "Vendor permission repair requires the documented deploy identity"
  # Confirm the already-running service is staging without emitting its env.
  docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' karrygo-vendor-api-staging |
    grep -Fxq APP_ENV=staging || die "Vendor runtime is not explicitly staging"
  command -v sudo >/dev/null || die "Vendor ownership repair requires existing authorized sudo"
  sudo -n -l /usr/bin/chown --no-dereference "deploy:$(id -gn)" -- "$f" >/dev/null 2>&1 ||
    die "Vendor file is root-owned; existing account has no authorized noninteractive ownership repair"
  if [ "$mode" = deploy ]; then
    sudo -n /usr/bin/chown --no-dereference "deploy:$(id -gn)" -- "$f"
    check_private_file "$f"
    require_staging "$f"
    log "Vendor settings ownership repaired; mode 600 retained"
  else
    log "Vendor settings ownership can be repaired after CI authorization"
  fi
}
# First-time split: retain the stopped legacy container itself, never recreate
# it from the routing-only Compose file. All mutation happens after exact CI.
gateway_split_ready() {
  running karrygo-api-gateway && running karrygo-partner-web-service &&
    docker exec karrygo-api-gateway wget -qO- http://127.0.0.1:8080/api/v1/gateway/meta |
      python3 -c 'import json,sys; assert json.load(sys.stdin).get("partner_upstream_configured") is True'
}
gateway_cutover_preflight() {
  running karrygo-api-gateway || die "legacy Gateway is not running"
  ! docker container inspect karrygo-partner-web-service >/dev/null 2>&1 ||
    die "Partner container exists but split is not healthy; reconcile it first"
  ! docker container inspect karrygo-api-gateway-legacy-rollback >/dev/null 2>&1 ||
    die "previous Gateway cutover recovery container exists"
  running karrygo-api-gateway-db && running karrygo-api-gateway-redis ||
    die "legacy partner database/Redis unavailable"
  docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' karrygo-api-gateway |
    python3 -c 'import sys; d=dict(l.rstrip("\n").split("=",1) for l in sys.stdin if "=" in l); assert d.get("APP_ENV")=="staging" and d.get("API_GATEWAY_DATABASE_URL")'
  [ -w /opt/karrygo/api-gateway ] && [ -w /opt/karrygo/partner-web-service ] ||
    die "Gateway and Partner release directories must be writable"
  [ "$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' karrygo-api-gateway)" != karrygo-public-gateway ] ||
    die "legacy gateway already uses the new Compose project"
}
gateway_cutover() (
  set -euo pipefail
  gateway_cutover_preflight
  sha="${image##*:}"
  gateway="ghcr.io/cosmicforgehealthnet/api-gateway:$sha"
  partner="ghcr.io/cosmicforgehealthnet/partner-web-service:$sha"
  docker pull -q "$gateway" >/dev/null
  docker pull -q "$partner" >/dev/null
  for ref in "$gateway" "$partner"; do
    [ "$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$ref")" = "$sha" ] ||
      die "cutover candidate revision mismatch"
  done
  export IMAGE="$partner"
  pc=(docker compose -f "$PLATFORM_DIR/compose/partner-web-service.yml")
  gc=(docker compose -f "$PLATFORM_DIR/compose/api-gateway.yml")
  IMAGE="$partner" "${pc[@]}" config --quiet
  IMAGE="$gateway" "${gc[@]}" config --quiet
  # Existing migration ledger must already exist. Do not baseline or run
  # pending migrations while transferring worker ownership.
  db=karrygo-api-gateway-db
  ledger=$(docker exec "$db" sh -c 'psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -c "SELECT to_regclass('\''public.karrygo_deploy_migrations'\'') IS NOT NULL"')
  [ "$ledger" = t ] || die "legacy migration ledger not established"
  status=$(IMAGE="$partner" "${pc[@]}" run --rm --no-deps -T -e MIGRATE_DB_URL_VAR=API_GATEWAY_DATABASE_URL --entrypoint /srv/app/migrate.sh app --status </dev/null)
  printf '%s\n' "$status" | grep -Eq '^migrate.sh: [1-9][0-9]* applied, 0 pending$' ||
    die "Partner migration status must show zero pending migrations"
  backup_db "$db"
  old_id=$(docker inspect -f '{{.Id}}' karrygo-api-gateway)
  old_policy=$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' karrygo-api-gateway)
  if [ "$old_policy" = on-failure ]; then
    retries=$(docker inspect -f '{{.HostConfig.RestartPolicy.MaximumRetryCount}}' karrygo-api-gateway)
    [ "$retries" = 0 ] || old_policy="on-failure:$retries"
  fi
  moved=false
  committed=false
  rollback_cutover() {
    rc=$?
    trap - EXIT
    if ! $committed; then
      set +e
      # Stop new workers before bringing the original owner back. If stopping
      # fails, leave legacy stopped rather than run duplicate consumers.
      IMAGE="$partner" "${pc[@]}" stop app >/dev/null 2>&1
      if docker container inspect karrygo-partner-web-service >/dev/null 2>&1; then
        docker rm -f karrygo-partner-web-service >/dev/null || exit 1
      fi
      if $moved; then
        if docker container inspect karrygo-api-gateway >/dev/null 2>&1; then
          docker rm -f karrygo-api-gateway >/dev/null || exit 1
        fi
        docker rename "$old_id" karrygo-api-gateway || exit 1
      fi
      docker update --restart="$old_policy" "$old_id" >/dev/null || exit 1
      docker start "$old_id" >/dev/null || exit 1
      wait_healthy karrygo-api-gateway 180 || exit 1
      log "Gateway cutover failed; original legacy runtime restored"
    fi
    exit "$rc"
  }
  trap rollback_cutover EXIT
  docker update --restart=no "$old_id" >/dev/null
  docker stop -t 30 "$old_id" >/dev/null
  [ "$(docker inspect -f '{{.State.Running}}' "$old_id")" = false ]
  docker rename "$old_id" karrygo-api-gateway-legacy-rollback
  moved=true
  IMAGE="$partner" "${pc[@]}" up -d --no-deps app
  verify_image karrygo-partner-web-service "$partner"
  [ -z "$(docker port karrygo-partner-web-service)" ]
  IMAGE="$gateway" "${gc[@]}" up -d --no-deps app
  verify_image karrygo-api-gateway "$gateway"
  gateway_split_ready
  # Check route ownership and deny rules through the real staging listener,
  # without generating orders, invoices, SMS or OTPs.
  curl -fsS --max-time 10 http://127.0.0.1:4100/ready >/dev/null
  code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' \
    http://127.0.0.1:4100/api/v1/partner/v1/deliveries)
  [[ "$code" = 401 || "$code" = 403 ]]
  code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST \
    http://127.0.0.1:4100/api/v1/internal/callbacks/charge-result)
  [ "$code" = 404 ]
  curl -fsS --max-time 15 --resolve api-gateway.staging.karryam.com:443:127.0.0.1 \
    https://api-gateway.staging.karryam.com/health >/dev/null
  for key in api-gateway partner-web-service; do
    release="/opt/karrygo/$key/release.env"
    tmp=$(mktemp "/opt/karrygo/$key/release.XXXXXX")
    printf 'IMAGE=ghcr.io/cosmicforgehealthnet/%s:%s\n' "$key" "$sha" >"$tmp"
    mv "$tmp" "$release"
  done
  committed=true
  log "GATEWAY_PARTNER_CUTOVER_OK unique Partner worker owner and routing-only Gateway"
)
preflight() {
  case "$svc" in
    vendor-service)
      vendor_settings
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
          if ! gateway_split_ready; then
            gateway_cutover_preflight
          fi
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
          python3 "$PLATFORM_DIR/bin/admin-auto-deploy.py" "$image" preflight
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
    [ -n "$after" ] || die "Vendor candidate migrations missing"
    changed=false
    if [ "$before" != "$after" ]; then changed=true; fi
    vendor_compose "$image" config --quiet
    # Rehearse against an isolated snapshot before stopping the live processes.
    # The clone has no external network or published ports.
    backup_db karrygo-vendor-staging-postgres
    if $changed; then
      snapshot="$KARRYGO_ROOT/backups/predeploy/vendor-forward-$(date -u +%Y%m%dT%H%M%S)-$$.sql.gz"
      docker exec karrygo-vendor-staging-postgres sh -c 'pg_dumpall --clean --if-exists -U "$POSTGRES_USER"' | gzip >"$snapshot"
      gzip -t "$snapshot"
      python3 "$PLATFORM_DIR/bin/vendor-rehearsal.py" "$image" "$snapshot"
    fi
    vendor_compose "$image" stop vendor-worker vendor-api
    if $changed; then
      # Capture the final quiesced recovery point. Never replay down migrations
      # or restore the old binary after a potentially committed forward step.
      final="$KARRYGO_ROOT/backups/predeploy/vendor-quiesced-$(date -u +%Y%m%dT%H%M%S)-$$.sql.gz"
      if ! (docker exec karrygo-vendor-staging-postgres sh -c 'pg_dumpall --clean --if-exists -U "$POSTGRES_USER"' | gzip >"$final" && gzip -t "$final"); then
        vendor_compose "$old" up -d --no-deps vendor-api vendor-worker
        die "Vendor final backup failed; previous processes restarted"
      fi
    fi
    migration_log="$KARRYGO_ROOT/backups/predeploy/vendor-migrate-$(date -u +%Y%m%dT%H%M%S)-$$.log"
    if ! vendor_compose "$image" --profile operations run --rm --no-deps -T vendor-migrate >"$migration_log" 2>&1; then
      die "Vendor migration failed; consumers stopped for forward recovery; private migration log retained"
    fi
    if vendor_compose "$image" up -d --no-deps vendor-api &&
       wait_healthy karrygo-vendor-api-staging 180 &&
       vendor_compose "$image" up -d --no-deps vendor-worker &&
       running karrygo-vendor-worker-staging &&
       verify_image karrygo-vendor-api-staging "$image" &&
       verify_image karrygo-vendor-worker-staging "$image" &&
       [ -z "$(docker port karrygo-vendor-worker-staging)" ]; then
      printf 'IMAGE=%s\n' "$image" > /opt/karrygo/vendor-service/release.env
      log "VENDOR_RUNTIME_OK candidate migration and both processes verified"
    else
      if $changed; then
        vendor_compose "$image" stop vendor-worker vendor-api || true
        die "Vendor candidate failed after forward migration; recover forward from private backup, no unsafe binary downgrade"
      fi
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
    python3 "$PLATFORM_DIR/bin/admin-auto-deploy.py" "$image" deploy
    ;;
  api-gateway|partner-web-service|analytics-service)
    if [[ "$svc" = api-gateway || "$svc" = partner-web-service ]]; then
      if ! gateway_split_ready; then
        [ "$svc" = api-gateway ] || die "Gateway must complete the paired cutover first"
        gateway_cutover
        exit 0
      fi
      if [ "$(docker inspect -f '{{.Image}}' "$SVC_CONTAINER")" = "$(docker image inspect -f '{{.Id}}' "$image")" ]; then
        verify_image "$SVC_CONTAINER" "$image"
        log "Already running exact candidate image: $svc"
        exit 0
      fi
    fi
    case "$svc" in
      partner-web-service) backup_db karrygo-api-gateway-db ;;
      analytics-service) backup_db karrygo-analytics-db ;;
    esac
    bash "$PLATFORM_DIR/bin/deploy-service.sh" "$svc" "$image"
    verify_image "$SVC_CONTAINER" "$image"
    ;;
  *) die "unsupported special staging service" ;;
esac
