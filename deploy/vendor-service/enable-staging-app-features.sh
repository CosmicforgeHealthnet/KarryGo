#!/usr/bin/env bash
set -euo pipefail
umask 077

directory=/opt/karrygo/vendor-service
file="$directory/vendor.env"
backup="$file.before-app-features-$(date +%Y%m%d-%H%M%S)"
cd "$directory"
test -f "$file"
test -f docker-compose.staging.yml
test -f network.override.yml
cp -p "$file" "$backup"

compose() {
  docker-compose -f docker-compose.staging.yml -f network.override.yml "$@"
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

python3 <<'PY'
import os
import re
from pathlib import Path

path = Path('/opt/karrygo/vendor-service/vendor.env')
text = path.read_text()
values = dict(line.split('=', 1) for line in text.splitlines()
              if line and not line.startswith('#') and '=' in line)
if values.get('APP_ENV') != 'staging':
    raise SystemExit('This helper applies only to the staging environment.')
required = ['VENDOR_NOTIFICATION_URL', 'VENDOR_NOTIFICATION_SECRET',
            'VENDOR_MEDIA_URL', 'VENDOR_MEDIA_SECRET']
missing = [key for key in required if not values.get(key, '').strip()]
if missing:
    raise SystemExit('Required integration settings missing: ' + ', '.join(missing))
source = Path('/opt/KarryGo/services/vendor-service/internal/config/config.go').read_text()
keys = set(re.findall(r'getEnvBool\("(VENDOR_CRM_[A-Z_]+)", (?:true|false)\)', source))
if 'VENDOR_CRM_ENABLED' not in keys:
    raise SystemExit('CRM configuration was not recognized; nothing changed.')
keys.update('''
VENDOR_PURCHASING_ENABLED
VENDOR_RECIPES_ENABLED
VENDOR_COST_ENABLED
VENDOR_TAXONOMY_ENABLED
VENDOR_TAXONOMY_WRITES_ENABLED
VENDOR_READINESS_ORCHESTRATION_ENABLED
COMMERCE_BULK_RFQ_ENABLED
COMMERCE_CHECKOUT_RECOVERY_ENABLED
COMMERCE_FULFILMENT_WORKERS_ENABLED
'''.split())
lines = [line for line in text.splitlines()
         if line.partition('=')[0] not in keys]
temporary = path.with_name(path.name + '.app-features.tmp')
fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
try:
    with os.fdopen(fd, 'w') as handle:
        handle.write('\n'.join(lines + [f'{key}=true' for key in sorted(keys)]) + '\n')
    os.replace(temporary, path)
finally:
    temporary.unlink(missing_ok=True)
print('Staging app feature flags prepared; existing credentials preserved.')
PY

changed=true
compose up -d --force-recreate vendor-api vendor-worker
for attempt in $(seq 1 25); do
  if curl -fsS --max-time 2 http://127.0.0.1:8112/ready >/dev/null; then
    sleep 3
    worker_state=$(docker inspect --format '{{.State.Running}} {{.State.Restarting}} {{.RestartCount}}' karrygo-vendor-worker-staging)
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
