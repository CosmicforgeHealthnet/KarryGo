#!/usr/bin/env bash
#
# smoke_partner_api.sh — end-to-end smoke test for the api-gateway Partner API.
#
# Covers: register -> login -> /me -> create key -> list keys -> quote ->
#         (optional delivery) -> (optional webhook) -> revoke key ->
#         verify revoked key fails -> logout -> verify session revoked.
#
# Exits non-zero on the first failure.
#
# Env / flags:
#   BASE_URL             api-gateway base URL (default http://localhost:8080)
#   RUN_DELIVERY_SMOKE   "true" to also create a delivery (needs payment-wallet)
#   WEBHOOK_URL          if set (must be HTTPS), registers a webhook
#
# Requires: curl, jq

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="dev+$(date +%s)@test.com"
PASSWORD="password123"

RESP_BODY=""
RESP_CODE=""

green() { printf '\033[32m%s\033[0m\n' "$1"; }
red() { printf '\033[31m%s\033[0m\n' "$1"; }
pass() { green "PASS: $1"; }
fail() {
	red "FAIL: $1"
	exit 1
}

require_cmd() {
	command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

# call METHOD PATH DATA [HEADER...] -> sets RESP_CODE, RESP_BODY
call() {
	local method="$1" path="$2" data="$3"
	shift 3
	local args=(-s -w $'\n%{http_code}' -X "$method" "$BASE_URL$path")
	if [[ -n "$data" ]]; then
		args+=(-H "Content-Type: application/json" -d "$data")
	fi
	local h
	for h in "$@"; do args+=(-H "$h"); done
	local out
	out="$(curl "${args[@]}")"
	RESP_CODE="${out##*$'\n'}"
	RESP_BODY="${out%$'\n'*}"
}

assert_code() {
	[[ "$RESP_CODE" == "$1" ]] || fail "expected HTTP $1, got $RESP_CODE. body=$RESP_BODY"
}

jqr() { echo "$RESP_BODY" | jq -r "$1"; }

require_cmd curl
require_cmd jq

echo "== Partner API smoke test =="
echo "base_url=$BASE_URL email=$EMAIL"

# 1. Register
call POST /api/v1/partner/v1/auth/register \
	"{\"business_name\":\"Smoke Test\",\"business_email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
assert_code 201
SESSION_JWT="$(jqr '.data.token')"
API_KEY="$(jqr '.data.api_key.key')"
[[ "$SESSION_JWT" != "null" && -n "$SESSION_JWT" ]] || fail "no session token from register"
[[ "$API_KEY" == sk_test_* ]] || fail "first key not sk_test_: $API_KEY"
pass "register -> session JWT + sandbox key"

# 2. Login
call POST /api/v1/partner/v1/auth/login "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
assert_code 200
SESSION_JWT="$(jqr '.data.token')"
[[ "$SESSION_JWT" != "null" && -n "$SESSION_JWT" ]] || fail "no token from login"
pass "login -> fresh session JWT"

# 3. /me
call GET /api/v1/partner/v1/auth/me "" "Authorization: Bearer $SESSION_JWT"
assert_code 200
[[ "$(jqr '.data.account.business_email')" == "$EMAIL" ]] || fail "/me wrong account"
echo "$RESP_BODY" | grep -q "password_hash" && fail "/me leaked password_hash"
pass "/me -> account (no password_hash)"

# 4. Create a second sandbox key
call POST /api/v1/partner/v1/keys '{"environment":"sandbox","label":"Smoke Key 2"}' \
	"Authorization: Bearer $SESSION_JWT"
assert_code 201
KEY_ID="$(jqr '.data.id')"
API_KEY2="$(jqr '.data.key')"
[[ "$API_KEY2" == sk_test_* ]] || fail "second key not sk_test_"
pass "create key -> id=$KEY_ID"

# 5. List keys (prefix only, no raw key / hash)
call GET /api/v1/partner/v1/keys "" "Authorization: Bearer $SESSION_JWT"
assert_code 200
echo "$RESP_BODY" | grep -q "key_hash" && fail "list leaked key_hash"
echo "$RESP_BODY" | grep -q "$API_KEY2" && fail "list leaked raw key"
pass "list keys -> prefix only"

# 6. Quote (nested pickup/dropoff contract), express > standard
call POST /api/v1/partner/v1/quotes \
	'{"pickup":{"lat":6.4474,"lng":3.4343},"dropoff":{"lat":6.4969,"lng":3.3481},"tier":"standard","size":"small"}' \
	"X-API-Key: $API_KEY"
assert_code 200
[[ "$(jqr '.data.currency')" == "NGN" ]] || fail "quote currency not NGN"
STD="$(jqr '.data.amount_kobo')"
call POST /api/v1/partner/v1/quotes \
	'{"pickup":{"lat":6.4474,"lng":3.4343},"dropoff":{"lat":6.4969,"lng":3.3481},"tier":"express","size":"small"}' \
	"X-API-Key: $API_KEY"
assert_code 200
EXP="$(jqr '.data.amount_kobo')"
[[ "$EXP" -gt "$STD" ]] || fail "express ($EXP) not > standard ($STD)"
pass "quote -> NGN kobo; express ($EXP) > standard ($STD)"

# 7. Optional: create delivery (needs payment-wallet)
if [[ "${RUN_DELIVERY_SMOKE:-false}" == "true" ]]; then
	call POST /api/v1/partner/v1/deliveries \
		'{"pickup_address":"15 Awolowo Road","pickup_lat":6.4474,"pickup_lng":3.4343,"dest_address":"32 Bode Thomas","dest_lat":6.4969,"dest_lng":3.3481,"receiver_name":"Chidi Obi","receiver_phone":"+2348011223344","package_type":"medical","package_size":"small","is_fragile":true,"service_tier":"express","payment_method":"wallet","customer_id":"smoke-customer","idempotency_key":"smoke-'"$(date +%s)"'"}' \
		"X-API-Key: $API_KEY"
	assert_code 201
	REF="$(jqr '.data.booking_ref')"
	[[ "$REF" == KGP-* ]] || fail "delivery ref not KGP-: $REF"
	pass "delivery -> $REF (status $(jqr '.data.status'))"
else
	echo "SKIP: delivery smoke (set RUN_DELIVERY_SMOKE=true; needs payment-wallet)"
fi

# 8. Optional: register webhook (HTTPS URL required)
if [[ -n "${WEBHOOK_URL:-}" ]]; then
	call POST /api/v1/partner/v1/webhooks \
		"{\"url\":\"$WEBHOOK_URL\",\"events\":[\"delivery.accepted\",\"delivery.completed\"]}" \
		"X-API-Key: $API_KEY"
	assert_code 201
	[[ "$(jqr '.data.signing_secret')" == whsec_* ]] || fail "webhook signing_secret missing"
	pass "webhook registered -> signing_secret returned once"
else
	echo "SKIP: webhook registration (set WEBHOOK_URL=https://...)"
fi

# 9. Revoke the second key
call DELETE "/api/v1/partner/v1/keys/$KEY_ID" "" "Authorization: Bearer $SESSION_JWT"
assert_code 200
pass "revoke key $KEY_ID"

# 10. Revoked key must fail on the next API call (403 api_key_revoked)
call POST /api/v1/partner/v1/quotes \
	'{"pickup":{"lat":6.4474,"lng":3.4343},"dropoff":{"lat":6.4969,"lng":3.3481},"tier":"standard","size":"small"}' \
	"X-API-Key: $API_KEY2"
[[ "$RESP_CODE" != "200" ]] || fail "revoked key still returned 200"
pass "revoked key rejected (HTTP $RESP_CODE, code=$(jqr '.error.code'))"

# 11. Logout
call POST /api/v1/partner/v1/auth/logout "" "Authorization: Bearer $SESSION_JWT"
assert_code 200
pass "logout"

# 12. Session JWT must now fail on /me (401 session_revoked)
call GET /api/v1/partner/v1/auth/me "" "Authorization: Bearer $SESSION_JWT"
assert_code 401
[[ "$(jqr '.error.code')" == "session_revoked" ]] || fail "expected session_revoked, got $(jqr '.error.code')"
pass "session revoked after logout"

green "== ALL SMOKE CHECKS PASSED =="
