#!/usr/bin/env bash
# Reports whether the Fleet Partner backend trusts a browser origin for sign-up.
#
#   scripts/fleet/check-auth-origin.sh [origin] [app-url]
#
# Defaults: origin https://fleet-karryam.vercel.app, app-url = origin.
#
# It posts a deliberately INVALID body to /api/auth/start, so no OTP is ever
# created or emailed:
#   422 validation_failed -> origin is trusted (the request got past the check)
#   403 untrusted_origin  -> origin is NOT in FLEET_PARTNER_AUTH_WEB_ORIGINS
set -euo pipefail

origin="${1:-https://fleet-karryam.vercel.app}"
app="${2:-$origin}"

body="$(mktemp)"
trap 'rm -f "$body"' EXIT
status="$(curl -sS -m 20 -o "$body" -w '%{http_code}' -X POST "$app/api/auth/start" \
  -H 'Content-Type: application/json' -H "Origin: $origin" \
  -d '{"email":"not-an-email","phone":"0"}')"
code="$(grep -o '"code":"[a-z_]*"' "$body" | head -1 | cut -d'"' -f4 || true)"

case "$status:$code" in
  422:validation_failed)
    echo "TRUSTED   $origin (via $app): backend accepted the origin; no OTP sent"
    ;;
  403:untrusted_origin)
    echo "REJECTED  $origin (via $app): add it to FLEET_PARTNER_AUTH_WEB_ORIGINS" >&2
    exit 1
    ;;
  *)
    echo "UNEXPECTED $origin (via $app): HTTP $status code=${code:-none}" >&2
    cat "$body" >&2; echo >&2
    exit 2
    ;;
esac
