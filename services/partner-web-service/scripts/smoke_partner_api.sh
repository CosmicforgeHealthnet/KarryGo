#!/usr/bin/env bash
#
# smoke_partner_api.sh - smoke test for the api-gateway Partner OTP onboarding flow.
#
# Covers: signup/start -> manual/dev OTP verify -> /me -> profile onboarding
#         (business info, logo URL, profile photo URL) -> password login fails
#         for OTP-only account -> create/list/revoke key -> quote -> logout.
#
# Env / flags:
#   BASE_URL            api-gateway base URL (default http://localhost:8080)
#   OTP_CODE            verification code; if empty, the script prompts
#   BUSINESS_LOGO_URL   logo URL to store on the profile
#   PROFILE_PHOTO_URL   profile photo URL to store on the profile
#
# Requires: curl, jq

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="dev+$(date +%s)@test.com"
OTP_CODE="${OTP_CODE:-}"
BUSINESS_LOGO_URL="${BUSINESS_LOGO_URL:-https://storage.example.test/partner/logo.jpg}"
PROFILE_PHOTO_URL="${PROFILE_PHOTO_URL:-https://storage.example.test/partner/profile.jpg}"

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

echo "== Partner OTP onboarding smoke test =="
echo "base_url=$BASE_URL email=$EMAIL"

# 1. Start OTP signup. The API must not return the OTP or hash.
call POST /api/v1/partner/v1/auth/signup/start \
	"{\"business_email\":\"$EMAIL\"}"
assert_code 200
echo "$RESP_BODY" | grep -q 'otp_code' && fail "signup/start leaked otp_code"
echo "$RESP_BODY" | grep -q 'otp_code_hash' && fail "signup/start leaked otp_code_hash"
pass "signup/start -> verification email requested"

if [[ -z "$OTP_CODE" ]]; then
	if [[ -t 0 ]]; then
		read -r -p "Enter OTP sent to $EMAIL: " OTP_CODE
	else
		fail "OTP_CODE is required in non-interactive mode"
	fi
fi
[[ "$OTP_CODE" =~ ^[0-9]{6}$ ]] || fail "OTP_CODE must be 6 digits"

# 2. Verify OTP and receive dashboard session JWT. No API key is created here.
call POST /api/v1/partner/v1/auth/signup/verify \
	"{\"business_email\":\"$EMAIL\",\"otp_code\":\"$OTP_CODE\"}"
assert_code 201
SESSION_JWT="$(jqr '.data.token')"
[[ "$SESSION_JWT" != "null" && -n "$SESSION_JWT" ]] || fail "no session token from OTP verify"
[[ "$(jqr '.data.account.business_email')" == "$EMAIL" ]] || fail "verify returned wrong email"
[[ "$(jqr '.data.account.status')" == "active" ]] || fail "OTP account not active"
[[ "$(jqr '.data.account.plan')" == "sandbox" ]] || fail "OTP account not sandbox"
echo "$RESP_BODY" | grep -q "password_hash" && fail "verify leaked password_hash"
echo "$RESP_BODY" | grep -q "api_key" && fail "OTP verify unexpectedly returned api_key"
pass "signup/verify -> active sandbox account + session JWT"

# 3. /me works with the session token.
call GET /api/v1/partner/v1/auth/me "" "Authorization: Bearer $SESSION_JWT"
assert_code 200
[[ "$(jqr '.data.account.business_email')" == "$EMAIL" ]] || fail "/me wrong account"
echo "$RESP_BODY" | grep -q "password_hash" && fail "/me leaked password_hash"
pass "/me -> account (no password_hash)"

# 4. Business info onboarding patch.
call PATCH /api/v1/partner/v1/profile \
	'{"business_name":"PharmaCare Nigeria Ltd","business_phone":"+2348033445566","state":"Lagos","city":"Ikeja"}' \
	"Authorization: Bearer $SESSION_JWT"
assert_code 200
[[ "$(jqr '.data.account.business_name')" == "PharmaCare Nigeria Ltd" ]] || fail "business_name not updated"
[[ "$(jqr '.data.account.business_phone')" == "+2348033445566" ]] || fail "business_phone not updated"
[[ "$(jqr '.data.account.state')" == "Lagos" ]] || fail "state not updated"
[[ "$(jqr '.data.account.city')" == "Ikeja" ]] || fail "city not updated"
pass "PATCH /profile -> business info"

# 5. Store media-file-service returned URLs. api-gateway stores URLs only.
call PATCH /api/v1/partner/v1/profile \
	"{\"business_logo_url\":\"$BUSINESS_LOGO_URL\"}" \
	"Authorization: Bearer $SESSION_JWT"
assert_code 200
[[ "$(jqr '.data.account.business_logo_url')" == "$BUSINESS_LOGO_URL" ]] || fail "business_logo_url not updated"
pass "PATCH /profile -> business_logo_url"

call PATCH /api/v1/partner/v1/profile \
	"{\"profile_photo_url\":\"$PROFILE_PHOTO_URL\"}" \
	"Authorization: Bearer $SESSION_JWT"
assert_code 200
[[ "$(jqr '.data.account.profile_photo_url')" == "$PROFILE_PHOTO_URL" ]] || fail "profile_photo_url not updated"
pass "PATCH /profile -> profile_photo_url"

# 6. Confirm final onboarding profile state.
call GET /api/v1/partner/v1/auth/me "" "Authorization: Bearer $SESSION_JWT"
assert_code 200
[[ "$(jqr '.data.account.business_name')" == "PharmaCare Nigeria Ltd" ]] || fail "/me missing business_name"
[[ "$(jqr '.data.account.business_logo_url')" == "$BUSINESS_LOGO_URL" ]] || fail "/me missing business_logo_url"
[[ "$(jqr '.data.account.profile_photo_url')" == "$PROFILE_PHOTO_URL" ]] || fail "/me missing profile_photo_url"
pass "/me confirms onboarding profile fields"

# 7. OTP-only accounts have no password_hash, so password login fails generically.
call POST /api/v1/partner/v1/auth/login \
	"{\"email\":\"$EMAIL\",\"password\":\"password123\"}"
assert_code 401
[[ "$(jqr '.error.code')" == "invalid_credentials" ]] || fail "expected invalid_credentials for OTP-only password login"
pass "password login for OTP-only account fails safely"

# 8. Create an API key from the dashboard after login.
call POST /api/v1/partner/v1/keys '{"environment":"sandbox","label":"Smoke Key"}' \
	"Authorization: Bearer $SESSION_JWT"
assert_code 201
KEY_ID="$(jqr '.data.id')"
API_KEY="$(jqr '.data.key')"
[[ "$API_KEY" == sk_test_* ]] || fail "key not sk_test_: $API_KEY"
pass "create key -> id=$KEY_ID"

# 9. List keys must not leak raw key or hashes.
call GET /api/v1/partner/v1/keys "" "Authorization: Bearer $SESSION_JWT"
assert_code 200
echo "$RESP_BODY" | grep -q "key_hash" && fail "list leaked key_hash"
echo "$RESP_BODY" | grep -q "$API_KEY" && fail "list leaked raw key"
pass "list keys -> prefix only"

# 10. Quote with API key.
call POST /api/v1/partner/v1/quotes \
	'{"pickup":{"lat":6.4474,"lng":3.4343},"dropoff":{"lat":6.4969,"lng":3.3481},"tier":"standard","size":"small"}' \
	"X-API-Key: $API_KEY"
assert_code 200
[[ "$(jqr '.data.currency')" == "NGN" ]] || fail "quote currency not NGN"
pass "quote -> NGN kobo"

# 11. Revoke key and verify it no longer works.
call DELETE "/api/v1/partner/v1/keys/$KEY_ID" "" "Authorization: Bearer $SESSION_JWT"
assert_code 200
pass "revoke key $KEY_ID"

call POST /api/v1/partner/v1/quotes \
	'{"pickup":{"lat":6.4474,"lng":3.4343},"dropoff":{"lat":6.4969,"lng":3.3481},"tier":"standard","size":"small"}' \
	"X-API-Key: $API_KEY"
[[ "$RESP_CODE" != "200" ]] || fail "revoked key still returned 200"
pass "revoked key rejected (HTTP $RESP_CODE, code=$(jqr '.error.code'))"

# 12. Logout and verify session revoked.
call POST /api/v1/partner/v1/auth/logout "" "Authorization: Bearer $SESSION_JWT"
assert_code 200
pass "logout"

call GET /api/v1/partner/v1/auth/me "" "Authorization: Bearer $SESSION_JWT"
assert_code 401
[[ "$(jqr '.error.code')" == "session_revoked" ]] || fail "expected session_revoked, got $(jqr '.error.code')"
pass "session revoked after logout"

green "== ALL SMOKE CHECKS PASSED =="