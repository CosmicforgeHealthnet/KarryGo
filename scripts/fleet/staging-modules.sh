#!/usr/bin/env bash
# Runs ON the staging server (piped over SSH by .github/workflows/fleet-staging-modules.yml).
#
# Reports Fleet's capability settings without revealing secrets, and applies a
# committed list of capability changes. Only "NAME=true|false" lines and plain
# https URLs for a few named, non-secret URL settings are accepted, so this can
# never write a secret. The settings file is backed up first; if the service is
# not healthy with the new settings, the backup is restored and re-applied.
#
#   staging-modules.sh <base64 of desired-settings file>
set -euo pipefail
umask 077
env_file=/opt/karrygo/fleet-partner-service/fleet.service.env
apply=/opt/karrygo/platform/bin/apply-env.sh
[[ -f "$env_file" && ! -L "$env_file" ]] || { echo 'Fleet env file missing or unsafe'; exit 1; }
[[ "$(stat -c %a "$env_file")" == 600 && "$(stat -c %u "$env_file")" == "$(id -u)" ]] || { echo 'Fleet env file ownership or mode invalid'; exit 1; }

report() {
  echo "== Fleet settings ($1): names, booleans and URL presence only"
  python3 - "$env_file" <<'PY'
import re, sys
for line in open(sys.argv[1]).read().splitlines():
    if not line.strip() or line.lstrip().startswith('#') or '=' not in line:
        continue
    name, value = line.split('=', 1)
    name = name.strip()
    value = value.strip().strip('"').strip("'")
    if value.lower() in ('true', 'false', '1', '0', 'yes', 'no', 'on', 'off'):
        print(f"{name}={value.lower()}")
    elif name.endswith('_URL') or name.endswith('_BASE_URL'):
        print(f"{name}=<{'set' if value else 'empty'}>")
    else:
        print(f"{name}=<{'set' if value else 'empty'}>")
PY
}

desired="$(printf '%s' "${1:-}" | base64 -d)"
report "before"
if [[ -z "$(printf '%s' "$desired" | grep -vE '^\s*(#|$)' || true)" ]]; then
  echo "== No changes requested; inspection only."
  exit 0
fi

python3 - "$desired" <<'PY'
import re, sys
allowed_urls = {"FLEET_PARTNER_STAFF_ACCEPT_URL"}
for raw in sys.argv[1].splitlines():
    line = raw.strip()
    if not line or line.startswith('#'):
        continue
    m = re.fullmatch(r'([A-Z][A-Z0-9_]{2,80})=(true|false)', line)
    u = re.fullmatch(r'([A-Z][A-Z0-9_]{2,80})=(https://[A-Za-z0-9.-]+(?:/[A-Za-z0-9._~/-]*)?)', line)
    if m:
        continue
    if u and u.group(1) in allowed_urls:
        continue
    sys.exit(f"Refusing unsupported setting line: {line.split('=',1)[0]}")
PY

backup="$(mktemp "${env_file}.modules-backup.XXXXXXXX")"
cp -p -- "$env_file" "$backup"
changed=0
rollback() {
  result=$?
  trap - EXIT
  if [[ "$result" != 0 && "$changed" == 1 ]]; then
    echo '== Fleet was not healthy with the new settings; restoring the previous settings'
    cp -p -- "$backup" "$env_file"
    "$apply" fleet-partner-service || echo 'WARNING: rollback apply failed; operator intervention required' >&2
  fi
  exit "$result"
}
trap rollback EXIT
changed=1
python3 - "$env_file" "$desired" <<'PY'
import os, sys, tempfile, pathlib
path = pathlib.Path(sys.argv[1])
lines = path.read_text().splitlines(keepends=True)
wanted = {}
for raw in sys.argv[2].splitlines():
    raw = raw.strip()
    if raw and not raw.startswith('#'):
        k, v = raw.split('=', 1)
        wanted[k] = v
seen = set()
out = []
for line in lines:
    key = line.split('=', 1)[0].strip() if '=' in line and not line.lstrip().startswith('#') else None
    if key in wanted:
        if key in seen:
            continue  # drop duplicates of a managed key
        ending = line[len(line.rstrip('\r\n')):] or '\n'
        out.append(f"{key}={wanted[key]}{ending}")
        seen.add(key)
    else:
        out.append(line)
if out and not out[-1].endswith('\n'):
    out[-1] += '\n'
for k, v in wanted.items():
    if k not in seen:
        out.append(f"{k}={v}\n")
fd, tmp = tempfile.mkstemp(prefix='.fleet-modules-', dir=str(path.parent))
try:
    os.fchmod(fd, 0o600)
    with os.fdopen(fd, 'w') as f:
        f.write(''.join(out)); f.flush(); os.fsync(f.fileno())
    os.replace(tmp, path)
finally:
    if os.path.exists(tmp):
        os.unlink(tmp)
print(f"== Applied {len(wanted)} capability setting(s)")
PY
"$apply" fleet-partner-service
report "after"
echo '== Fleet healthy with the requested capability settings'
