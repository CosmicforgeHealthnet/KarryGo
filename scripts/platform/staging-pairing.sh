#!/usr/bin/env bash
# Runs ON the staging server (piped over SSH by
# .github/workflows/platform-staging-pairing.yml).
#
# Connects staging services to each other without any secret leaving the
# server. The committed desired-state file may contain only:
#
#   PAIR <caller> <callee>              callee trusts caller with a shared
#                                       HMAC secret generated here
#   SET  <service> NAME=true|false      an on/off setting
#   URL  <service> NAME=http://karrygo-<container>:<port>
#                                       an internal service address
#   CALLERS <service> NAME_CALLERS=a,b  a list of known service identities
#   SET/URL/CALLERS may end with requires=<service>[,<service>]: the line is
#                                       dropped when any required service is BLOCKED
#
# A pair is left alone when both sides already agree; otherwise a new secret
# is generated and written to both settings files. Secret values are never
# printed. Every touched settings file is backed up first; if any touched
# service is unhealthy afterwards, every file is restored and re-applied.
#
#   staging-pairing.sh <base64 of desired-state file>
set -euo pipefail
umask 077
apply=/opt/karrygo/platform/bin/apply-env.sh
desired="$(printf '%s' "${1:-}" | base64 -d)"

python3 - "$desired" <<'PY' > /tmp/pairing-plan.$$
import json, re, sys
# Service -> settings file. Extend as services are paired.
FILES = {
    "fleet-partner-service": "/opt/karrygo/fleet-partner-service/fleet.service.env",
    "verification-compliance-service": "/opt/karrygo/verification-service/verification.env",
    "payment-wallet-service": "/opt/karrygo/payment-wallet-service/wallet.env",
    "driver-hauling-service": "/opt/karrygo/hauling-service/hauling.env",
    "driver-taxi-service": "/opt/karrygo/taxi-service/taxi.env",
}
# Service identities that may appear in a CALLERS list.
KNOWN_CALLERS = {"fleet-partner-service", "verification-compliance-service", "payment-wallet-service",
                 "driver-hauling-service", "driver-taxi-service", "driver-dispatch-delivery-service"}
# (caller, callee) -> where each side keeps the shared secret.
#   caller_var: the caller's single secret setting
#   callee_list: the callee's "name=secret,..." list the caller is added to
PAIRS = {
    ("fleet-partner-service", "verification-compliance-service"): {
        "caller_var": "FLEET_PARTNER_VERIFICATION_COMPLIANCE_SERVICE_SECRET",
        "callee_list": "VERIFICATION_COMPLIANCE_SERVICE_SECRETS"},
    ("verification-compliance-service", "fleet-partner-service"): {
        "caller_var": "VERIFICATION_COMPLIANCE_FLEET_EVENTS_SECRET",
        "callee_list": "FLEET_PARTNER_SERVICE_SECRETS"},
    # Fleet -> Wallet internal finance API (company wallet, sub-wallets).
    ("fleet-partner-service", "payment-wallet-service"): {
        "caller_var": "FLEET_PARTNER_PAYMENT_WALLET_SERVICE_SECRET",
        "callee_list": "PAYMENT_WALLET_SERVICE_SECRETS"},
    # Fleet -> Hauling / Taxi internal provider, execution and work APIs.
    ("fleet-partner-service", "driver-hauling-service"): {
        "caller_var": "FLEET_PARTNER_HAULING_SERVICE_SECRET",
        "callee_list": "HAULING_SERVICE_SECRETS"},
    ("fleet-partner-service", "driver-taxi-service"): {
        "caller_var": "FLEET_PARTNER_TAXI_SERVICE_SECRET",
        "callee_list": "TAXI_SERVICE_SECRETS"},
    # Hauling / Taxi -> Fleet marketplace and trip events.
    ("driver-hauling-service", "fleet-partner-service"): {
        "caller_var": "HAULING_FLEET_EVENT_SECRET",
        "callee_list": "FLEET_PARTNER_SERVICE_SECRETS"},
    ("driver-taxi-service", "fleet-partner-service"): {
        "caller_var": "FLEET_PARTNER_SERVICE_SECRET",
        "callee_list": "FLEET_PARTNER_SERVICE_SECRETS"},
    # Wallet -> Fleet finance events (wallet/payment projections).
    ("payment-wallet-service", "fleet-partner-service"): {
        "caller_var": "PAYMENT_WALLET_FINANCE_OUTBOX_SECRET",
        "callee_list": "FLEET_PARTNER_SERVICE_SECRETS"},
}
plan = {"pairs": [], "sets": [], "urls": []}
for raw in sys.argv[1].splitlines():
    line = raw.strip()
    if not line or line.startswith("#"):
        continue
    parts = line.split()
    requires = []
    if parts[0] in ("SET", "URL", "CALLERS") and len(parts) == 4 and re.fullmatch(r"requires=[a-z-]+(,[a-z-]+)*", parts[3]):
        requires = parts[3].split("=", 1)[1].split(",")
        if not all(r in FILES for r in requires):
            sys.exit(f"Refusing unknown requires= service on: {parts[0]} {parts[1]}")
        parts = parts[:3]
    if parts[0] == "PAIR" and len(parts) == 3 and (parts[1], parts[2]) in PAIRS:
        plan["pairs"].append({"caller": parts[1], "callee": parts[2], **PAIRS[(parts[1], parts[2])]})
    elif parts[0] == "SET" and len(parts) == 3 and parts[1] in FILES and re.fullmatch(r"[A-Z][A-Z0-9_]{2,80}=(true|false)", parts[2]):
        plan["sets"].append({"service": parts[1], "line": parts[2], "requires": requires})
    elif parts[0] == "CALLERS" and len(parts) == 3 and parts[1] in FILES and re.fullmatch(r"[A-Z][A-Z0-9_]{2,80}_CALLERS=[a-z-]+(,[a-z-]+)*", parts[2]) \
            and all(name in KNOWN_CALLERS for name in parts[2].split("=", 1)[1].split(",")):
        plan["sets"].append({"service": parts[1], "line": parts[2], "requires": requires})
    elif parts[0] == "URL" and len(parts) == 3 and parts[1] in FILES and re.fullmatch(r"[A-Z][A-Z0-9_]{2,80}_URL=http://karrygo-[a-z0-9-]+:[0-9]{2,5}", parts[2]):
        plan["urls"].append({"service": parts[1], "line": parts[2], "requires": requires})
    else:
        sys.exit(f"Refusing unsupported line: {parts[0]} {parts[1] if len(parts) > 1 else ''}")
plan["files"] = FILES
print(json.dumps(plan))
PY
plan_file=/tmp/pairing-plan.$$
trap 'rm -f "$plan_file"' EXIT

touched="$(python3 - "$plan_file" <<'PY'
import json, sys
plan = json.load(open(sys.argv[1]))
services = set()
for p in plan["pairs"]:
    services.update([p["caller"], p["callee"]])
for item in plan["sets"] + plan["urls"]:
    services.add(item["service"])
print(" ".join(sorted(services)))
PY
)"
[[ -n "$touched" ]] || { echo '== Nothing requested'; exit 0; }

# A settings file that is missing, a link, not mode 600 or not owned by this
# account is never touched or loosened. Every request involving that service
# is dropped (both sides of a pair, so nothing is half-paired), the rest is
# applied, and the service is reported as BLOCKED so the workflow fails
# visibly with the exact owner action.
blocked=""
for service in $touched; do
  file="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["files"][sys.argv[2]])' "$plan_file" "$service")"
  reason=""
  if [[ ! -f "$file" || -L "$file" ]]; then
    reason="settings file missing or a link"
  elif [[ "$(stat -c %a "$file")" != 600 || "$(stat -c %u "$file")" != "$(id -u)" ]]; then
    reason="settings file is not mode 600 owned by $(id -un) (owner $(stat -c %U "$file"), mode $(stat -c %a "$file")). Owner action, as root: chown $(id -un):$(id -gn) $file && chmod 600 $file"
  fi
  if [[ -n "$reason" ]]; then
    echo "BLOCKED: $service: $reason"
    blocked="$blocked $service"
  fi
done
if [[ -n "$blocked" ]]; then
  python3 - "$plan_file" $blocked <<'PY'
import json, sys
path, blocked = sys.argv[1], set(sys.argv[2:])
plan = json.load(open(path))
plan["pairs"] = [p for p in plan["pairs"] if p["caller"] not in blocked and p["callee"] not in blocked]
# A setting on a healthy service that only makes sense with a blocked peer
# (marked requires=<peer>) is dropped too, so the healthy service is never
# configured for a connection that was not made.
keep = lambda x: x["service"] not in blocked and not (set(x.get("requires", [])) & blocked)
plan["sets"] = [x for x in plan["sets"] if keep(x)]
plan["urls"] = [x for x in plan["urls"] if keep(x)]
json.dump(plan, open(path, "w"))
PY
  touched="$(python3 - "$plan_file" <<'PY'
import json, sys
plan = json.load(open(sys.argv[1]))
services = set()
for p in plan["pairs"]:
    services.update([p["caller"], p["callee"]])
for item in plan["sets"] + plan["urls"]:
    services.add(item["service"])
print(" ".join(sorted(services)))
PY
)"
  [[ -n "$touched" ]] || { echo '== Nothing left to apply'; exit 0; }
fi

declare -A backups paths
for service in $touched; do
  file="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["files"][sys.argv[2]])' "$plan_file" "$service")"
  paths[$service]="$file"
  backups[$service]="$(mktemp "${file}.pairing-backup.XXXXXXXX")"
  cp -p -- "$file" "${backups[$service]}"
done

restore() {
  result=$?
  trap - EXIT
  rm -f "$plan_file"
  if [[ "$result" != 0 ]]; then
    echo '== A paired service was not healthy; restoring every touched settings file'
    for service in $touched; do
      cp -p -- "${backups[$service]}" "${paths[$service]}"
      "$apply" "$service" || echo "WARNING: rollback apply failed for $service" >&2
    done
  fi
  exit "$result"
}
trap restore EXIT

python3 - "$plan_file" <<'PY'
import json, os, secrets, sys, tempfile, pathlib
plan = json.load(open(sys.argv[1]))
files = plan["files"]

def read(service):
    path = pathlib.Path(files[service])
    lines = path.read_text().splitlines(keepends=True)
    values = {}
    for line in lines:
        if "=" in line and not line.lstrip().startswith("#"):
            k, v = line.split("=", 1)
            values[k.strip()] = v.strip().strip('"').strip("'")
    return path, lines, values

def write(service, updates):
    path, lines, _ = read(service)
    seen, out = set(), []
    for line in lines:
        key = line.split("=", 1)[0].strip() if "=" in line and not line.lstrip().startswith("#") else None
        if key in updates:
            if key in seen:
                continue
            out.append(f"{key}={updates[key]}\n")
            seen.add(key)
        else:
            out.append(line if line.endswith("\n") else line + "\n")
    for k, v in updates.items():
        if k not in seen:
            out.append(f"{k}={v}\n")
    fd, tmp = tempfile.mkstemp(prefix=".pairing-", dir=str(path.parent))
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, "w") as f:
            f.write("".join(out)); f.flush(); os.fsync(f.fileno())
        os.replace(tmp, path)
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)

def parse_list(value):
    entries = []
    for part in value.split(","):
        part = part.strip()
        if part and "=" in part:
            name, secret = part.split("=", 1)
            entries.append([name.strip(), secret.strip()])
    return entries

updates = {}
def stage(service, key, value):
    updates.setdefault(service, {})[key] = value

for pair in plan["pairs"]:
    caller, callee = pair["caller"], pair["callee"]
    _, _, caller_values = read(caller)
    _, _, callee_values = read(callee)
    caller_secret = updates.get(caller, {}).get(pair["caller_var"], caller_values.get(pair["caller_var"], ""))
    entries = parse_list(updates.get(callee, {}).get(pair["callee_list"], callee_values.get(pair["callee_list"], "")))
    trusted = [secret for name, secret in entries if name == caller]
    if caller_secret and trusted and trusted[0] == caller_secret:
        print(f"== {caller} -> {callee}: already paired")
        continue
    secret = secrets.token_hex(32)
    entries = [e for e in entries if e[0] != caller] + [[caller, secret]]
    stage(caller, pair["caller_var"], secret)
    stage(callee, pair["callee_list"], ",".join(f"{n}={s}" for n, s in entries))
    print(f"== {caller} -> {callee}: new shared secret generated (value not shown)")

for item in plan["sets"] + plan["urls"]:
    key, value = item["line"].split("=", 1)
    stage(item["service"], key, value)
    print(f"== {item['service']}: {key}={'<url>' if key.endswith('_URL') else value}")

for service, values in updates.items():
    write(service, values)
PY

for service in $touched; do
  echo "== Applying $service"
  "$apply" "$service"
done
echo '== Paired services healthy'
