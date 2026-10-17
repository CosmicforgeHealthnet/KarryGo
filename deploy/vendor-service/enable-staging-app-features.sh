#!/usr/bin/env bash
# Enables the complete Vendor-app feature set on a Vendor Service host.
#
# Usage:  sudo bash enable-staging-app-features.sh [staging|production]
#
# The argument (default: staging) must equal APP_ENV in vendor.env, so the
# helper can never be pointed at the wrong environment by accident.
#
# Safety: backs up vendor.env, refuses to change anything when a required
# integration credential is missing, recreates the API and worker, waits for
# /ready and a cleanly running worker, and restores the backup (and restarts
# again) on any failure.
#
# The flag set mirrors appRequiredFlags in
# services/vendor-service/internal/config/app_feature_set_test.go, which proves
# it passes Config.Validate(). Keep the two in sync.
set -euo pipefail
umask 077

target_env="${1:-staging}"
case "$target_env" in
  staging|production) ;;
  *) echo "Usage: $0 [staging|production]" >&2; exit 2 ;;
esac

directory="${VENDOR_DEPLOY_DIR:-/opt/karrygo/vendor-service}"
file="$directory/vendor.env"
compose_file="${VENDOR_COMPOSE_FILE:-docker-compose.$target_env.yml}"
backup="$file.before-app-features-$(date +%Y%m%d-%H%M%S)"
cd "$directory"
test -f "$file"
test -f "$compose_file"
test -f network.override.yml
cp -p "$file" "$backup"

compose() {
  docker-compose -f "$compose_file" -f network.override.yml "$@"
}

changed=false
complete=false
recover() {
  if [ "$changed" = true ] && [ "$complete" = false ]; then
    cp -p "$backup" "$file"
    if compose up -d --force-recreate vendor-api vendor-worker; then
      echo 'Feature update failed; previous configuration restored.' >&2
    else
      echo "Restart failed. Previous environment restored from $backup; inspect the containers." >&2
    fi
  fi
}
trap recover EXIT

VENDOR_ENV_FILE="$file" TARGET_ENV="$target_env" python3 <<'PY'
import os
from pathlib import Path

path = Path(os.environ['VENDOR_ENV_FILE'])
target = os.environ['TARGET_ENV']
text = path.read_text()
values = dict(line.split('=', 1) for line in text.splitlines()
              if line and not line.startswith('#') and '=' in line)
if values.get('APP_ENV', '').strip() != target:
    raise SystemExit(f'APP_ENV in {path} is {values.get("APP_ENV")!r}, not {target!r}; nothing changed.')
if target == 'production' and values.get('VENDOR_AUTH_OTP_DEBUG', '').strip().lower() in ('1', 'true'):
    raise SystemExit('VENDOR_AUTH_OTP_DEBUG must be false in production; nothing changed.')

def present(*keys):
    return all(values.get(k, '').strip() for k in keys)

required = ['VENDOR_NOTIFICATION_URL', 'VENDOR_NOTIFICATION_SECRET',
            'VENDOR_MEDIA_URL', 'VENDOR_MEDIA_SECRET', 'VENDOR_MEDIA_PUBLIC_URL',
            'VENDOR_WALLET_URL', 'VENDOR_WALLET_SECRET',
            'VENDOR_VERIFICATION_URL', 'VENDOR_VERIFICATION_SECRET']
missing = [k for k in required if not values.get(k, '').strip()]
if missing:
    raise SystemExit('Required integration settings missing: ' + ', '.join(missing))
callers = [p.strip().replace(':', '=').split('=')[0].strip()
           for p in values.get('VENDOR_SERVICE_SECRETS', '').split(',')]
if 'verification-compliance-service' not in callers:
    raise SystemExit('VENDOR_SERVICE_SECRETS must allow verification-compliance-service; nothing changed.')

authorities = {
    'DISPATCH_COMMERCE_INTEGRATION_ENABLED': ('VENDOR_DISPATCH_URL', 'VENDOR_DISPATCH_SECRET', 'VENDOR_DISPATCH_HANDOVER_RELAY_ENABLED'),
    'HAULING_COMMERCE_INTEGRATION_ENABLED': ('VENDOR_HAULING_URL', 'VENDOR_HAULING_SECRET', 'VENDOR_HAULING_HANDOVER_RELAY_ENABLED'),
    'TAXI_GOODS_DELIVERY_ENABLED': ('VENDOR_TAXI_URL', 'VENDOR_TAXI_SECRET', 'VENDOR_TAXI_HANDOVER_RELAY_ENABLED'),
}
enabled_authorities = [f for f, (u, s, _) in authorities.items() if present(u, s)]
if not enabled_authorities:
    raise SystemExit('At least one logistics authority (Dispatch, Hauling or Taxi URL + secret) is required; nothing changed.')

flags = '''
VENDOR_SERVICE_ENABLED
VENDOR_ONBOARDING_WRITES_ENABLED
VENDOR_CRM_ENABLED
VENDOR_CRM_WRITES_ENABLED
VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED
VENDOR_CRM_CONSENT_INTEGRATION_ENABLED
VENDOR_CRM_PRIVACY_INTEGRATION_ENABLED
VENDOR_CRM_SEGMENTATION_ENABLED
VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED
VENDOR_CRM_INTERACTIONS_ENABLED
VENDOR_CRM_TASKS_ENABLED
VENDOR_CRM_LOYALTY_ENABLED
VENDOR_CRM_CAMPAIGNS_ENABLED
VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED
VENDOR_CRM_CAMPAIGN_EXECUTION_ENABLED
VENDOR_CRM_CORPORATE_ENABLED
VENDOR_CRM_INTELLIGENCE_ENABLED
VENDOR_CRM_INTELLIGENCE_WORKER_ENABLED
VENDOR_CRM_EXPORTS_ENABLED
VENDOR_PRIVACY_RETENTION_ENABLED
VENDOR_INVENTORY_ENABLED
VENDOR_INVENTORY_WRITES_ENABLED
VENDOR_PURCHASING_ENABLED
VENDOR_RECIPES_ENABLED
VENDOR_COST_ENABLED
COMMERCE_FOOD_ENABLED
VENDOR_FOOD_CATALOGUE_WRITES_ENABLED
COMMERCE_SHOP_ENABLED
VENDOR_SHOP_CATALOGUE_WRITES_ENABLED
CUSTOMER_FOOD_ENABLED
CUSTOMER_SHOP_ENABLED
COMMERCE_MULTI_VENDOR_ENABLED
COMMERCE_VARIABLE_WEIGHT_ENABLED
COMMERCE_BULK_RFQ_ENABLED
VENDOR_OWN_DELIVERY_ENABLED
COMMERCE_CHECKOUT_ENABLED
COMMERCE_CHECKOUT_RECOVERY_ENABLED
COMMERCE_FULFILMENT_PLANNING_ENABLED
COMMERCE_FULFILMENT_WORKERS_ENABLED
COMMERCE_QUOTE_LOCK_ENABLED
COMMERCE_ORDERS_ENABLED
VENDOR_READINESS_ORCHESTRATION_ENABLED
VENDOR_JIT_PICKUP_ENABLED
VENDOR_AUTO_AUTHORITY_SELECTION_ENABLED
VENDOR_TAXONOMY_ENABLED
VENDOR_TAXONOMY_WRITES_ENABLED
VENDOR_SEARCH_ENABLED
PAYMENT_WALLET_COMMERCE_ENABLED
VENDOR_VERIFICATION_INTEGRATION_ENABLED
'''.split()
settings = {key: 'true' for key in flags}
for flag in enabled_authorities:
    settings[flag] = 'true'
    settings[authorities[flag][2]] = 'true'
for flag in set(authorities) - set(enabled_authorities):
    print(f'NOTE: {flag} left unchanged: its URL/secret are not configured.')
if present('VENDOR_DISPATCH_URL', 'VENDOR_DISPATCH_SECRET'):
    settings['VENDOR_OWNED_HANDOVER_RELAY_ENABLED'] = 'true'
# The kill switch makes every catalogue/onboarding write read-only, hides the
# readiness and orchestration routes and stops all workers: the app cannot
# work with it on.
settings['COMMERCE_GLOBAL_KILL_SWITCH'] = 'false'
# Internal allowlists Validate() requires once taxonomy/search are on. Kept
# when the operator already set them.
for key, default in (('VENDOR_TAXONOMY_INTERNAL_CALLERS', 'admin-backoffice-service'),
                     ('VENDOR_SEARCH_INTERNAL_CALLERS', 'customer-service')):
    if not values.get(key, '').strip():
        settings[key] = default

lines = [line for line in text.splitlines() if line.partition('=')[0] not in settings]
temporary = path.with_name(path.name + '.app-features.tmp')
fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
try:
    with os.fdopen(fd, 'w') as handle:
        handle.write('\n'.join(lines + [f'{k}={v}' for k, v in sorted(settings.items())]) + '\n')
    os.replace(temporary, path)
finally:
    temporary.unlink(missing_ok=True)
print(f'{target} app feature flags prepared; existing credentials preserved.')
PY

changed=true
compose up -d --force-recreate vendor-api vendor-worker
for attempt in $(seq 1 25); do
  if curl -fsS --max-time 2 http://127.0.0.1:8112/ready >/dev/null; then
    sleep 3
    worker_id=$(compose ps -q vendor-worker)
    worker_state=$(docker inspect --format '{{.State.Running}} {{.State.Restarting}} {{.RestartCount}}' "$worker_id")
    if [ "$worker_state" != 'true false 0' ]; then
      echo 'Vendor worker did not start cleanly; restoring the previous environment.' >&2
      exit 1
    fi
    complete=true
    echo "Vendor API is ready and worker is running. Previous environment: $backup"
    echo 'Authenticated app-flow verification is still required.'
    exit 0
  fi
  sleep 1
done
echo 'Vendor API did not become ready; restoring the previous environment.' >&2
exit 1
