#!/usr/bin/env bash
# Approves a driver whose documents you have checked (there is no admin
# console yet).
#
#   bin/approve-driver.sh taxi  <phone>                 identity, documents and car verified
#   bin/approve-driver.sh truck <phone> [--trucks]      onboarding complete; --trucks also
#                                                       activates the driver's trucks
#
# It signs the request as admin-backoffice-service with the secret in that
# service's *_SERVICE_SECRETS setting and calls the service on the private
# network, so it only works on the server. Suspended accounts and drivers who
# have not finished their profile are refused.
source "$(dirname "$0")/lib.sh"

kind="${1:-}"; phone="${2:-}"; trucks="${3:-}"
case "$kind" in
  taxi)  env_file="$KARRYGO_ROOT/taxi-service/taxi.env"; var=TAXI_SERVICE_SECRETS
         container=karrygo-taxi-service; port=8102; path=/api/v1/taxi/internal/admin/providers/approve ;;
  truck) env_file="$KARRYGO_ROOT/hauling-service/hauling.env"; var=HAULING_SERVICE_SECRETS
         container=karrygo-hauling-service; port=8104; path=/api/v1/hauling/internal/admin/providers/approve ;;
  *) die "usage: $0 taxi|truck <phone> [--trucks]" ;;
esac
[ -n "$phone" ] || die "usage: $0 taxi|truck <phone> [--trucks]"

ip="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}' "$container" | awk '{print $1}')"
[ -n "$ip" ] || die "$container is not running"

ENV_FILE="$env_file" VAR="$var" URL="http://$ip:$port$path" URI="$path" PHONE="$phone" TRUCKS="$trucks" python3 - <<'PY'
import hashlib, hmac, json, os, secrets, sys, time, urllib.request, urllib.error
secret = None
for line in open(os.environ["ENV_FILE"]):
    if line.startswith(os.environ["VAR"] + "="):
        for part in line.split("=", 1)[1].strip().split(","):
            name, _, value = part.partition("=")
            if name.strip() == "admin-backoffice-service":
                secret = value.strip()
if not secret:
    sys.exit(f"no admin-backoffice-service entry in {os.environ['VAR']}; add one and run bin/apply-env.sh")
body = {"phone": os.environ["PHONE"], "note": "approved with bin/approve-driver.sh"}
if os.environ["TRUCKS"] == "--trucks":
    body["activate_trucks"] = True
raw = json.dumps(body).encode()
ts, nonce = str(int(time.time())), secrets.token_hex(16)
canonical = "\n".join(["2", "admin-backoffice-service", "POST", os.environ["URI"], ts, nonce, hashlib.sha256(raw).hexdigest()])
sig = hmac.new(secret.encode(), canonical.encode(), hashlib.sha256).hexdigest()
req = urllib.request.Request(os.environ["URL"], data=raw, method="POST", headers={
    "Content-Type": "application/json", "X-Service-Name": "admin-backoffice-service", "X-Service-Timestamp": ts,
    "X-Service-Nonce": nonce, "X-Service-Signature-Version": "2", "X-Service-Signature": sig})
try:
    with urllib.request.urlopen(req, timeout=15) as r:
        d = json.load(r)["data"]
        print("approved:", json.dumps(d, indent=1))
except urllib.error.HTTPError as e:
    err = json.load(e).get("error") or {}
    sys.exit(f"refused ({e.code}): {err.get('message')}")
PY
