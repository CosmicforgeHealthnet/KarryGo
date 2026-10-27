#!/usr/bin/env bash
# Loads the staging test data (deploy/platform/seeds/staging/*.sql): test
# customers, truck and taxi drivers with trips, wallets and notifications, and
# pricing's Lagos geography and staging operators.
#
# Staging only: it refuses to run unless the services' settings say
# APP_ENV=staging. Every file is idempotent (fixed ids, insert-if-missing) and
# runs in one transaction, so re-running is harmless and a failure changes
# nothing in that database.
#
# Usage (as root or deploy on the server):  bin/seed-staging.sh
source "$(dirname "$0")/lib.sh"

SEEDS="$PLATFORM_DIR/seeds/staging"

# seed file -> database container
declare -a PLAN=(
  "customer.sql:karrygo-customer-db"
  "hauling.sql:karrygo-hauling-db"
  "taxi.sql:karrygo-taxi-db"
  "payment-wallet.sql:karrygo-payment-wallet-db"
  "notification.sql:karrygo-notification-db"
  "pricing.sql:karrygo-pricing-db"
)

for env_file in "$KARRYGO_ROOT/taxi-service/taxi.env" "$KARRYGO_ROOT/hauling-service/hauling.env" "$KARRYGO_ROOT/customer-service/customer.env"; do
  env_name="$(sed -n 's/^APP_ENV=//p' "$env_file" 2>/dev/null | head -1)"
  [ "$env_name" = "staging" ] || die "$env_file has APP_ENV=${env_name:-unset}; test data is for staging only"
done

for step in "${PLAN[@]}"; do
  file="${step%%:*}"; db="${step#*:}"
  [ -f "$SEEDS/$file" ] || die "missing $SEEDS/$file"
  user="$(docker exec "$db" printenv POSTGRES_USER)"; name="$(docker exec "$db" printenv POSTGRES_DB)"
  log "seeding $db from $file"
  docker exec -i "$db" psql -U "$user" -d "$name" -q -v ON_ERROR_STOP=1 --single-transaction < "$SEEDS/$file" >/dev/null \
    || die "seeding $db failed; nothing was changed in that database"
done
log "staging test data is in place"
