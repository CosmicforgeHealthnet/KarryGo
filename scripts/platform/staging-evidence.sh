#!/usr/bin/env bash
# Read-only staging evidence for the KarryGo platform services.
#
# Run on the staging server as the deploy account, from an unpacked copy of
# deploy/platform (so bin/lib.sh and services.tsv are available):
#
#   bash staging-evidence.sh <platform-dir> [service ...]
#
# It changes nothing and never prints a secret:
#   - settings files are never opened; only their owner and mode are read
#     (stat), which is what the deploy preflight checks;
#   - database queries are read-only SELECTs on migration bookkeeping tables;
#   - container inspection prints image ids, revision labels, state and health.
set -uo pipefail
PLATFORM="$1"; shift
cd "$PLATFORM" || exit 1
# shellcheck disable=SC1091
source bin/lib.sh
# lib.sh enables errexit for deploy scripts. Evidence must report every
# service even when one probe fails, so failures are tolerated per step.
set +e

services=("$@")
[ "${#services[@]}" -gt 0 ] || mapfile -t services < <(all_services)

echo "EVIDENCE host=$(hostname) at=$(date -u +%Y-%m-%dT%H:%M:%SZ) account=$(id -un)"

for key in "${services[@]}"; do
  ( load_service "$key" ) >/dev/null 2>&1 || { echo "SERVICE $key unknown"; continue; }
  load_service "$key"
  echo "== SERVICE $key"

  # Settings file: owner and mode only (never contents).
  if [ -e "$SVC_ENV_PATH" ]; then
    owner_mode="$(stat -c '%U:%G %a' "$SVC_ENV_PATH" 2>/dev/null || echo unreadable)"
    readable=no; [ -r "$SVC_ENV_PATH" ] && readable=yes
    echo "SETTINGS file=$SVC_ENV_PATH owner_mode=$owner_mode readable_by_deploy=$readable"
  else
    echo "SETTINGS file=$SVC_ENV_PATH missing"
  fi

  # Running containers for this service: everything under its karrygo-<home>
  # prefix (api, worker, db, redis, helpers).
  prefix="karrygo-${SVC_HOME%-service}"
  for c in $(docker ps -a --format '{{.Names}}' | grep -E "^${prefix}(-|$)|^${SVC_CONTAINER}$" | sort -u); do
    img="$(docker inspect -f '{{.Image}}' "$c" 2>/dev/null)"
    rev="$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$img" 2>/dev/null)"
    state="$(docker inspect -f '{{.State.Status}}' "$c" 2>/dev/null)"
    health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}-{{end}}' "$c" 2>/dev/null)"
    started="$(docker inspect -f '{{.State.StartedAt}}' "$c" 2>/dev/null)"
    echo "CONTAINER name=$c state=$state health=$health revision=${rev:-unknown} started=$started"
    # Databases, caches and stopped containers have no service trust or
    # access log to report.
    case "$c" in *-db|*-redis|*-redis-*) continue ;; esac
    [ "$state" = running ] || continue
    # Who this container trusts: the caller NAMES in each *_SERVICE_SECRETS
    # list (the part before "=", never the secret) and every *_CALLERS
    # allowlist (service identity names only). Split inside the container so
    # no secret value crosses the docker exec boundary.
    docker exec "$c" sh -c '
      printenv | grep -E "^[A-Z0-9_]+_(SERVICE_SECRETS|CALLERS)=" | while IFS= read -r kv; do
        name="${kv%%=*}"; value="${kv#*=}"
        case "$name" in
          *_SERVICE_SECRETS) names=$(printf "%s" "$value" | tr "," "\n" | sed "s/=.*//" | tr -d " " | grep -v "^$" | sort | tr "\n" "," | sed "s/,$//") ;;
          *) names=$(printf "%s" "$value" | tr -d " ") ;;
        esac
        echo "TRUST setting=$name names=${names:-none}"
      done' 2>/dev/null | sed "s/^/TRUST container=$c /; s/TRUST container=\([^ ]*\) TRUST /TRUST container=\1 /"
    # Signed internal calls this container RECEIVED in the last two hours, by
    # method, route template and HTTP status (the structured access log never
    # carries path parameters, query strings or identifiers). 401/403 here is
    # a refused service signature or caller.
    docker logs --since 2h "$c" 2>&1 | perl -ne '
      next unless /"event":"http_request"/;
      my ($m) = /"method":"([A-Z]+)"/; my ($r) = /"route":"([^"]*)"/; my ($s) = /"status":(\d+)/;
      next unless defined $r && $r =~ m{/internal/};
      print "$m $r $s\n"' 2>/dev/null | sort | uniq -c | sort -rn | head -20 |
      while read -r n m r s; do echo "INTERNAL_HTTP container=$c method=$m route=$r status=$s count=$n"; done
  done
  # Media only: private-object encryption at rest (FLEET-7h.c). Reports the
  # backend, whether keys are configured (yes/no, never the value) and, for
  # the filesystem backend, how many objects start with the encryption
  # header magic (first 4 bytes only; object contents are never read).
  if [ "$key" = media-file-service ] && [ "$(docker inspect -f '{{.State.Running}}' "$SVC_CONTAINER" 2>/dev/null)" = true ]; then
    docker exec "$SVC_CONTAINER" sh -c '
      keys=no; [ -n "${MEDIA_FILE_PRIVATE_ENCRYPTION_KEYS:-}" ] && keys=yes
      if [ -n "${MEDIA_FILE_FIREBASE_BUCKET:-}" ]; then
        echo "MEDIA_AT_REST backend=bucket keys_configured=$keys"
      else
        d="${MEDIA_FILE_PRIVATE_STORAGE_DIR:-/tmp/karrygo-media-private}"
        total=0; sealed=0
        for f in $(find "$d" -type f 2>/dev/null); do
          total=$((total+1))
          [ "$(head -c 4 "$f" | od -An -tx1 | tr -d " \n")" = "894b474d" ] && sealed=$((sealed+1))
        done
        echo "MEDIA_AT_REST backend=filesystem keys_configured=$keys objects=$total encrypted=$sealed legacy_plaintext=$((total-sealed))"
      fi' 2>/dev/null
  fi
  release="$(current_image 2>/dev/null || true)"
  echo "RELEASE image=${release:-none}"

  # Migration bookkeeping in the service's own database container.
  db="karrygo-${SVC_HOME%-service}-db"
  if [ "$(docker inspect -f '{{.State.Running}}' "$db" 2>/dev/null)" = true ]; then
    # One read-only block: for every table whose name contains "migration",
    # report its row count and the greatest value of its first version-like
    # column. SQL travels on stdin so the shell never expands it.
    docker exec -i "$db" sh -c 'psql -X -q -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -v ON_ERROR_STOP=0 -f - 2>&1' <<'SQL' | sed -n 's/^.*NOTICE:  //p'
DO $evidence$
DECLARE t record; col text; latest text; n bigint;
BEGIN
  FOR t IN SELECT table_schema, table_name FROM information_schema.tables
           WHERE table_name ILIKE '%migration%' AND table_schema NOT IN ('pg_catalog','information_schema') LOOP
    SELECT column_name INTO col FROM information_schema.columns
     WHERE table_schema = t.table_schema AND table_name = t.table_name
       AND column_name IN ('version','name','filename','version_id','migration')
     ORDER BY array_position(ARRAY['version','name','filename','version_id','migration'], column_name::text) LIMIT 1;
    EXECUTE format('SELECT count(*) FROM %I.%I', t.table_schema, t.table_name) INTO n;
    IF col IS NOT NULL THEN
      EXECUTE format('SELECT max(%I::text) FROM %I.%I', col, t.table_schema, t.table_name) INTO latest;
    ELSE latest := '-'; END IF;
    RAISE NOTICE 'MIGRATION_HEAD table=%.% latest=% applied=%', t.table_schema, t.table_name, latest, n;
  END LOOP;
END
$evidence$;
SQL
    # Integration state, aggregate counts only (no payload, id or name):
    #   - every cross-service outbox: pending / delivered / dead-lettered and
    #     the last delivery time, so signed delivery between services is
    #     evidenced by rows the receiver accepted;
    #   - Fleet: organizations, verification requests and wallet provisioning
    #     by status (the wallet-gate waiver removal condition);
    #   - Wallet: organization wallets by status.
    docker exec -i "$db" sh -c 'psql -X -q -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -v ON_ERROR_STOP=0 -f - 2>&1' <<'SQL' | sed -n 's/^.*NOTICE:  //p'
DO $integration$
DECLARE t text; has_status boolean; has_delivered boolean; has_target boolean; r record; q text;
BEGIN
  FOREACH t IN ARRAY ARRAY['taxi_outbox','hauling_domain_outbox','finance_outbox','fleet_partner_outbox'] LOOP
    CONTINUE WHEN to_regclass('public.' || t) IS NULL;
    SELECT bool_or(column_name = 'status'), bool_or(column_name = 'delivered_at'), bool_or(column_name = 'target')
      INTO has_status, has_delivered, has_target
      FROM information_schema.columns WHERE table_schema = 'public' AND table_name = t;
    IF has_delivered THEN
      q := format('SELECT %s AS target, count(*) FILTER (WHERE delivered_at IS NULL AND dead_lettered_at IS NULL) AS pending,
                          count(*) FILTER (WHERE delivered_at IS NOT NULL) AS delivered,
                          count(*) FILTER (WHERE dead_lettered_at IS NOT NULL) AS dead, max(delivered_at)::text AS last
                     FROM %I GROUP BY 1 ORDER BY 1', CASE WHEN has_target THEN 'coalesce(target,''-'')' ELSE '''-''' END, t);
    ELSIF has_status THEN
      q := format('SELECT ''-'' AS target, count(*) FILTER (WHERE status = ''PENDING'') AS pending,
                          count(*) FILTER (WHERE status IN (''PUBLISHED'',''DELIVERED'',''SENT'')) AS delivered,
                          count(*) FILTER (WHERE status IN (''DEAD_LETTERED'',''DEAD_LETTER'',''FAILED'')) AS dead, ''-'' AS last
                     FROM %I', t);
    ELSE CONTINUE; END IF;
    FOR r IN EXECUTE q LOOP
      RAISE NOTICE 'OUTBOX table=% target=% pending=% delivered=% dead_lettered=% last_delivered=%', t, r.target, r.pending, r.delivered, r.dead, coalesce(r.last, '-');
    END LOOP;
  END LOOP;
  IF to_regclass('public.fleet_partner_organizations') IS NOT NULL THEN
    FOR r IN SELECT status, count(*) AS n FROM fleet_partner_organizations GROUP BY 1 ORDER BY 1 LOOP
      RAISE NOTICE 'FLEET_ORGANIZATIONS status=% count=%', r.status, r.n;
    END LOOP;
    FOR r IN SELECT status, count(*) AS n FROM fleet_partner_organization_verification_requests GROUP BY 1 ORDER BY 1 LOOP
      RAISE NOTICE 'FLEET_VERIFICATION_REQUESTS status=% count=%', r.status, r.n;
    END LOOP;
    FOR r IN SELECT status, count(*) AS n, count(*) FILTER (WHERE wallet_account_id IS NOT NULL) AS with_account
               FROM fleet_partner_organization_wallet_provisioning GROUP BY 1 ORDER BY 1 LOOP
      RAISE NOTICE 'FLEET_WALLET_PROVISIONING status=% count=% with_account=%', r.status, r.n, r.with_account;
    END LOOP;
    RAISE NOTICE 'FLEET_WALLET_PROVISIONING active_organizations_without_active_wallet=%',
      (SELECT count(*) FROM fleet_partner_organizations o
        LEFT JOIN fleet_partner_organization_wallet_provisioning w ON w.organization_id = o.id
        WHERE o.status = 'ACTIVE' AND coalesce(w.status, '') <> 'ACTIVE');
  END IF;
  IF to_regclass('public.finance_wallets') IS NOT NULL THEN
    FOR r IN SELECT status, currency, count(*) AS n FROM finance_wallets GROUP BY 1, 2 ORDER BY 1, 2 LOOP
      RAISE NOTICE 'WALLET_ORGANIZATION_WALLETS status=% currency=% count=%', r.status, r.currency, r.n;
    END LOOP;
    RAISE NOTICE 'WALLET_ORGANIZATION_WALLETS total=%', (SELECT count(*) FROM finance_wallets);
  END IF;
END
$integration$;
SQL
    # Dispatch only: the read-only inventory step 1 of the reviewed 070
    # transition needs (docs/deployment/staging-release-approval.md).
    # Aggregate counts only; no payload, key, provider or offer is printed.
    if [ "$key" = driver-dispatch-delivery-service ]; then
      docker exec -i "$db" sh -c 'psql -X -q -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -v ON_ERROR_STOP=0 -f - 2>&1' <<'SQL' | sed -n 's/^.*NOTICE:  //p'
DO $relay$
DECLARE r record; has_fence boolean;
BEGIN
  IF to_regclass('public.dispatch_fleet_relay_operations') IS NULL THEN
    RAISE NOTICE 'DISPATCH_RELAY table=absent'; RETURN;
  END IF;
  FOR r IN SELECT status, operation_type, count(*) AS n,
                  count(*) FILTER (WHERE lease_expires_at > now()) AS leased,
                  min(created_at) AS oldest
             FROM dispatch_fleet_relay_operations GROUP BY 1, 2 ORDER BY 1, 2 LOOP
    RAISE NOTICE 'DISPATCH_RELAY status=% type=% rows=% live_leases=% oldest=%', r.status, r.operation_type, r.n, r.leased, r.oldest;
  END LOOP;
  SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'dispatch_fleet_relay_operations' AND column_name = 'readback_required') INTO has_fence;
  RAISE NOTICE 'DISPATCH_RELAY fence_column_present=% would_be_fenced_by_070=%', has_fence,
    (SELECT count(*) FROM dispatch_fleet_relay_operations WHERE status IN ('UNKNOWN_OUTCOME','DELIVERING','DEAD_LETTER'));
  RAISE NOTICE 'DISPATCH_RELAY readback_audit_table=%', CASE WHEN to_regclass('public.dispatch_fleet_relay_readback_audit') IS NULL THEN 'absent' ELSE 'present' END;
END
$relay$;
DO $files$
BEGIN
  -- The files migrate.sh has recorded (names only), so the transition knows
  -- exactly which files it would apply before anyone approves it.
  IF to_regclass('public.karrygo_deploy_migrations') IS NOT NULL THEN
    RAISE NOTICE 'DISPATCH_MIGRATIONS recorded=%', (SELECT string_agg(filename || ':' || how, ' ' ORDER BY filename) FROM karrygo_deploy_migrations);
  END IF;
END
$files$;
SQL
      # Every process that could act as a Dispatch relay consumer: any
      # container (any state) running a Dispatch image, and its restart policy.
      for c in $(docker ps -a --format '{{.Names}}'); do
        img_name="$(docker inspect -f '{{.Config.Image}}' "$c" 2>/dev/null)"
        case "$img_name" in *driver-dispatch-delivery-service*|*dispatch-delivery*) ;; *) continue ;; esac
        echo "DISPATCH_CONSUMER name=$c state=$(docker inspect -f '{{.State.Status}}' "$c") restart=$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$c") image=$img_name"
      done
      systemctl list-units --all --no-legend 2>/dev/null | grep -iE 'karrygo|dispatch-delivery' | sed 's/^/DISPATCH_SUPERVISOR unit=/' || true
      crontab -l 2>/dev/null | grep -iE 'karrygo|dispatch-delivery' | sed 's/^/DISPATCH_SUPERVISOR cron=/' || true
    fi
  else
    echo "DATABASE container=$db not-running-or-shared"
  fi
done

echo "== DONE"
