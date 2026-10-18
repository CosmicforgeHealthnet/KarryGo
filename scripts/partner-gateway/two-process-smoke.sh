#!/usr/bin/env bash
# Two real Go processes plus disposable PostgreSQL and Redis. NEVER target a live DB.
set -euo pipefail
: "$API_GATEWAY_TEST_DATABASE_URL"
: "$RUNNER_TEMP"
case "$API_GATEWAY_TEST_DATABASE_URL" in
  *"/partner_test?"*) ;;
  *) echo "refusing database other than disposable partner_test" >&2; exit 1 ;;
esac

work="$RUNNER_TEMP/partner-gateway-smoke"
mkdir -p "$work"
partner_pid=""
gateway_pid=""
cleanup() {
  [ -z "$gateway_pid" ] || kill "$gateway_pid" 2>/dev/null || true
  [ -z "$partner_pid" ] || kill "$partner_pid" 2>/dev/null || true
}
trap cleanup EXIT

echo "[smoke] applying existing partner migrations to disposable database"
MIGRATE_DB_URL_VAR=API_GATEWAY_TEST_DATABASE_URL \
  MIGRATIONS_DIR="$PWD/services/partner-web-service/migrations" \
  sh deploy/platform/bin/migrate.sh

go build -o "$work/partner-web" ./services/partner-web-service/cmd
go build -o "$work/api-gateway" ./services/api-gateway/cmd

echo "[smoke] starting private partner service"
(
  export APP_ENV=test HTTP_ADDR=127.0.0.1:18080
  export API_GATEWAY_DATABASE_URL="$API_GATEWAY_TEST_DATABASE_URL"
  export API_GATEWAY_REDIS_ADDR=127.0.0.1:6379
  export PARTNER_INTERNAL_SERVICE_KEY=test-only-strong-partner-callback-key
  export PARTNER_JWT_SECRET=test-only-partner-jwt-signature-key-000000000
  export PARTNER_AUTH_WEB_ORIGINS=http://localhost:3000
  exec "$work/partner-web"
) > "$work/partner.log" 2>&1 &
partner_pid=$!

for i in $(seq 1 30); do
  if curl -fsS --max-time 2 http://127.0.0.1:18080/ready >/dev/null; then break; fi
  sleep 1
done
if ! curl -fsS --max-time 3 http://127.0.0.1:18080/ready >/dev/null; then
  echo "[smoke] partner service is not ready"; tail -40 "$work/partner.log"; exit 1
fi

echo "[smoke] starting routing-only public gateway"
(
  export APP_ENV=test HTTP_ADDR=127.0.0.1:18081
  export GATEWAY_UPSTREAM_PARTNER_WEB_URL=http://127.0.0.1:18080
  export GATEWAY_ALLOWED_ORIGINS=http://localhost:3000
  exec "$work/api-gateway"
) > "$work/gateway.log" 2>&1 &
gateway_pid=$!
for i in $(seq 1 30); do
  if curl -fsS --max-time 2 http://127.0.0.1:18081/ready >/dev/null; then break; fi
  sleep 1
done
if ! curl -fsS --max-time 3 http://127.0.0.1:18081/ready >/dev/null; then
  echo "[smoke] gateway is not ready"; tail -40 "$work/gateway.log"; exit 1
fi

expect_status() {
  local expected="$1" url="$2" method="$3" code
  code="$(curl --silent --max-time 5 -o "$work/response.json" -w '%{http_code}' -X "$method" "$url")"
  if [ "$code" != "$expected" ]; then
    echo "expected HTTP $expected, got $code: $method $url" >&2
    cat "$work/response.json" >&2 || true
    exit 1
  fi
}
# Real DB-backed partner middleware exists on both direct and public paths.
expect_status 401 http://127.0.0.1:18080/api/v1/partner/v1/auth/me GET
expect_status 401 http://127.0.0.1:18081/api/v1/partner/v1/auth/me GET
# Internal callbacks are registered solely at the partner service.
expect_status 401 http://127.0.0.1:18080/api/v1/internal/callbacks/trip-started POST
expect_status 404 http://127.0.0.1:18081/api/v1/internal/callbacks/trip-started POST
expect_status 404 http://127.0.0.1:18081/internal/karryam-admin/v1/capabilities GET
expect_status 404 http://127.0.0.1:18081/api/v1/admin/users GET
expect_status 200 http://127.0.0.1:18081/health GET
expect_status 200 http://127.0.0.1:18081/api/v1/gateway/customer/overview GET
echo "[smoke] PASS: partner 401/401, callback 401 private + 404 public, Admin 404, gateway health + aggregate 200"
