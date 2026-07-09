# Partner API — End-to-End Test Sequence

This document walks a developer through the **complete Partner API flow** on
`api-gateway`, from partner self-registration to signed webhook delivery, using
copy-pasteable `curl` commands. It ends with a production readiness checklist.

> Two separate auth systems — never mix them:
> - **Dashboard session JWT** (`Authorization: Bearer <jwt>`) — register, login,
>   logout, `/me`, and key management.
> - **Partner API key** (`X-API-Key: sk_test_...` / `sk_live_...`) — quotes,
>   deliveries, webhooks.
>
> Admin is a completely separate platform. Nothing admin-facing lives in api-gateway.

---

## 1. Prerequisites

- **api-gateway** running on `http://localhost:8080`.
- api-gateway **database migrated** (apply `services/api-gateway/migrations/001_partner_schema.sql`
  then `002_partner_auth.sql`).
- **Redis** running (for Pub/Sub + rate limiting).
- **dispatch service** running — only needed for the full delivery lifecycle
  (Steps 10–11). Auth/keys/quotes/webhook-registration work without it.
- **payment-wallet service** running (or a local stub) — delivery creation
  performs a wallet payment hold; without a reachable wallet, `POST /deliveries`
  returns `402 payment_failed`.
- A **webhook receiver URL** — e.g. a `https://webhook.site/<your-id>` URL — for
  the webhook test.
- **jq** installed (recommended, to extract tokens/keys from JSON).
- **redis-cli** installed (optional, to verify Redis Pub/Sub).

### Minimum api-gateway `.env`

See `services/api-gateway/.env.example` for the full list. At minimum:

```env
HTTP_ADDR=:8080
API_GATEWAY_DATABASE_URL=postgres://user:pass@localhost:5432/api_gateway_db
API_GATEWAY_REDIS_ADDR=localhost:6379
PARTNER_JWT_SECRET=replace-with-min-32-char-random-string
PARTNER_INTERNAL_SERVICE_KEY=replace-with-strong-random-key
PRICING_BASE_KOBO=50000
PRICING_STANDARD_PER_KM_KOBO=15000
PRICING_EXPRESS_PER_KM_KOBO=25000
PRICING_LARGE_MULTIPLIER=1.2
WEBHOOK_SIGNING_SECRET=replace-with-webhook-signing-secret
WEBHOOK_MAX_RETRIES=5
WEBHOOK_DELIVERY_TIMEOUT_SECONDS=30
# Required only if you exercise delivery creation (payment hold):
API_GATEWAY_WALLET_SERVICE_URL=http://localhost:8105
API_GATEWAY_WALLET_SERVICE_NAME=api-gateway
API_GATEWAY_WALLET_SERVICE_SECRET=must-match-wallet-PAYMENT_WALLET_SERVICE_SECRETS
```

---

## 2. Variables

```bash
BASE_URL="http://localhost:8080"
PARTNER_EMAIL="dev@test.com"
PARTNER_PASSWORD="password123"
WEBHOOK_URL="https://webhook.site/your-unique-id"   # must be HTTPS
```

Captured during the flow (with `jq`):

```bash
SESSION_JWT=""            # dashboard session token
API_KEY=""               # raw partner API key (shown once)
KEY_ID=""                # id of a created key (for revoke)
BOOKING_REF=""           # KGP-XXXXXXXX
BOOKING_ID=""            # partner_bookings.id (UUID)
WEBHOOK_ID=""            # registered webhook id
WEBHOOK_SIGNING_SECRET="" # shown once at webhook creation
```

---

## 3. Full curl sequence

### Step 1 — Register partner

```bash
REGISTER_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/partner/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d "{
    \"business_name\": \"Test Pharmacy\",
    \"business_email\": \"$PARTNER_EMAIL\",
    \"password\": \"$PARTNER_PASSWORD\"
  }")
echo "$REGISTER_RESPONSE" | jq .

SESSION_JWT=$(echo "$REGISTER_RESPONSE" | jq -r '.data.token')
API_KEY=$(echo "$REGISTER_RESPONSE" | jq -r '.data.api_key.key')
```

Expected (`201`):

```json
{
  "success": true,
  "data": {
    "account": { "id": "...", "business_email": "dev@test.com", "status": "active", "plan": "sandbox", "created_at": "..." },
    "token": "<session_jwt>",
    "expires_at": "...",
    "api_key": { "key": "sk_test_...", "prefix": "sk_test_....", "warning": "Save this key now. It will not be shown again." }
  }
}
```

- `data.api_key.key` starts with `sk_test_` and is shown **once**.
- account `status=active`, `plan=sandbox`. No `password_hash` / `key_hash` anywhere.
- **If the email already exists** → `409 email_taken`. Use a unique email
  (e.g. `dev+$(date +%s)@test.com`) or continue at Step 2 (login).

### Step 2 — Login

```bash
LOGIN_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/partner/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{
    \"email\": \"$PARTNER_EMAIL\",
    \"password\": \"$PARTNER_PASSWORD\"
  }")
echo "$LOGIN_RESPONSE" | jq .

SESSION_JWT=$(echo "$LOGIN_RESPONSE" | jq -r '.data.token')
```

Expected (`200`): `success:true`, a **fresh** `data.token`, and `data.account`.
Wrong password → `401 invalid_credentials`. Inactive account → `403 account_inactive`.

### Step 3 — View account (`/me`)

```bash
curl -s "$BASE_URL/api/v1/partner/v1/auth/me" \
  -H "Authorization: Bearer $SESSION_JWT" | jq .
```

Expected (`200`): `data.account` with `id/business_name/business_email/status/plan/created_at`.
`password_hash` must **not** appear. Missing token → `401 missing_token`.

### Step 4 — Generate another sandbox key (JSON)

The keys handler accepts JSON.

```bash
KEY_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/partner/v1/keys" \
  -H "Authorization: Bearer $SESSION_JWT" \
  -H "Content-Type: application/json" \
  -d '{ "environment": "sandbox", "label": "Test Key 2" }')
echo "$KEY_RESPONSE" | jq .

API_KEY=$(echo "$KEY_RESPONSE" | jq -r '.data.key')
KEY_ID=$(echo "$KEY_RESPONSE" | jq -r '.data.id')
```

Expected (`201`): `data.key` (starts with `sk_test_`, shown once), `data.prefix`,
`data.id`, `data.environment`, `data.warning`.

> A **live** key (`environment: "live"`) requires the partner plan to be `live`
> (see Step 12 note / internal activation). A sandbox partner requesting a live
> key gets `403 sandbox_only`.

### Step 5 — List keys

```bash
curl -s "$BASE_URL/api/v1/partner/v1/keys" \
  -H "Authorization: Bearer $SESSION_JWT" | jq .
```

Expected (`200`): `data` is an array of
`{id, prefix, environment, label, status, last_used_at, created_at}`.
**Only the prefix** is returned — never the raw key, never `key_hash`.

### Step 6 — Get a quote (API key auth, new nested contract)

```bash
curl -s -X POST "$BASE_URL/api/v1/partner/v1/quotes" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "pickup":  { "lat": 6.4474, "lng": 3.4343 },
    "dropoff": { "lat": 6.4969, "lng": 3.3481 },
    "tier": "express",
    "size": "small"
  }' | jq .
```

Expected (`200`):

```json
{
  "success": true,
  "data": {
    "distance_km": 5.87,
    "currency": "NGN",
    "amount_kobo": 196750,
    "tier": "express",
    "size": "small"
  }
}
```

- `amount_kobo` is kobo; `currency` is `NGN`.
- Distance uses `geo.HaversineKm`; fare uses `pricing.CalculateFare` with env
  config (`PRICING_*`) — no hardcoded values.

Express should exceed standard for the same coordinates:

```bash
STANDARD=$(curl -s -X POST "$BASE_URL/api/v1/partner/v1/quotes" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{"pickup":{"lat":6.4474,"lng":3.4343},"dropoff":{"lat":6.4969,"lng":3.3481},"tier":"standard","size":"small"}' | jq '.data.amount_kobo')
EXPRESS=$(curl -s -X POST "$BASE_URL/api/v1/partner/v1/quotes" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d '{"pickup":{"lat":6.4474,"lng":3.4343},"dropoff":{"lat":6.4969,"lng":3.3481},"tier":"express","size":"small"}' | jq '.data.amount_kobo')
echo "standard=$STANDARD express=$EXPRESS"   # express > standard
```

### Step 7 — Create a delivery (API key auth)

> Requires a reachable payment-wallet (payment hold). Without it → `402 payment_failed`.

```bash
DELIVERY_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/partner/v1/deliveries" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "pickup_address": "15 Awolowo Road",
    "pickup_lat": 6.4474,
    "pickup_lng": 3.4343,
    "dest_address": "32 Bode Thomas",
    "dest_lat": 6.4969,
    "dest_lng": 3.3481,
    "receiver_name": "Chidi Obi",
    "receiver_phone": "+2348011223344",
    "package_type": "medical",
    "package_size": "small",
    "is_fragile": true,
    "service_tier": "express",
    "payment_method": "wallet",
    "customer_id": "test-customer-uuid",
    "idempotency_key": "test-001"
  }')
echo "$DELIVERY_RESPONSE" | jq .

BOOKING_REF=$(echo "$DELIVERY_RESPONSE" | jq -r '.data.booking_ref')
BOOKING_ID=$(echo "$DELIVERY_RESPONSE" | jq -r '.data.id')
```

Expected (`201`):

```json
{
  "success": true,
  "data": {
    "id": "<uuid>",
    "booking_ref": "KGP-XXXXXXXX",
    "status": "dispatching",
    "service_tier": "express",
    "fare_amount": { "kobo": 196750, "display": "₦1,967.50" },
    "pickup_address": "15 Awolowo Road",
    "dest_address": "32 Bode Thomas",
    "receiver_name": "Chidi Obi",
    "external_ref": null,
    "created_at": "..."
  }
}
```

- `booking_ref` is `KGP-` + 8 chars.
- On success the booking's `dispatch_booking_id` (a server-generated UUID) is
  stored and used as the `source_reference` for the wallet hold and as the
  `booking_id` in the published dispatch event. It is **not** returned in the
  response; read it from the DB if needed:
  ```bash
  # psql ... -c "SELECT dispatch_booking_id FROM partner_bookings WHERE booking_ref='$BOOKING_REF';"
  ```
- **Idempotency**: repeating the same `idempotency_key` with the **same** body
  returns the cached `201` (no duplicate booking); a **different** body with the
  same key returns `409 idempotency_conflict`.

### Step 8 — Confirm the Redis event was published

Before Step 7, in another terminal subscribe:

```bash
redis-cli SUBSCRIBE "booking.dispatch.created"
# with host/port/db/password if configured:
# redis-cli -h localhost -p 6379 -n 0 SUBSCRIBE "booking.dispatch.created"
```

Then run Step 7. The subscriber receives a `booking.dispatch.created` message
whose payload includes `booking_id` (= the dispatch booking id), `customer_id`,
`pickup_lat/lng`, `dropoff_lat/lng`, `pickup_address`, `dropoff_address`,
`fare_amount`, `receiver_name/phone`, `package_type/size`, `is_fragile`,
`service_tier`, `service_type: "dispatch"`, `payment_method`, `currency: "NGN"`.

### Step 9 — Register a webhook (API key auth)

```bash
WEBHOOK_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/partner/v1/webhooks" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{ \"url\": \"$WEBHOOK_URL\", \"events\": [\"delivery.accepted\", \"delivery.completed\"] }")
echo "$WEBHOOK_RESPONSE" | jq .

WEBHOOK_ID=$(echo "$WEBHOOK_RESPONSE" | jq -r '.data.id')
WEBHOOK_SIGNING_SECRET=$(echo "$WEBHOOK_RESPONSE" | jq -r '.data.signing_secret')
echo "signing secret (save now): $WEBHOOK_SIGNING_SECRET"
```

Expected (`201`): `data.id`, `data.url`, `data.events`, `data.status: "active"`,
and `data.signing_secret` (a `whsec_...` value shown **once**), plus a `note`.

- The URL must be **HTTPS**. An `http://` URL, `localhost`, or a private IP is
  rejected with `400 validation_error`. Try:
  ```bash
  curl -s -X POST "$BASE_URL/api/v1/partner/v1/webhooks" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
    -d '{"url":"http://example.com/hook","events":["delivery.completed"]}' | jq .   # -> 400 validation_error
  ```
- `GET /api/v1/partner/v1/webhooks` never returns `signing_secret`.
- You can send a real signed test event immediately:
  ```bash
  curl -s -X POST "$BASE_URL/api/v1/partner/v1/webhooks/$WEBHOOK_ID/test" -H "X-API-Key: $API_KEY" | jq .
  ```

### Step 10 — Drive the dispatch lifecycle (full E2E)

**Preferred (real):** accept/start/complete the trip via the dispatch service's
provider trip endpoints, e.g.:

```bash
DISPATCH_URL="http://localhost:8103"
curl -s -X PUT "$DISPATCH_URL/api/v1/provider/trips/$TRIP_ID/complete" \
  -H "Authorization: Bearer $PROVIDER_JWT" -H "Content-Type: application/json" -d '{}' | jq .
```

Expected chain:

1. dispatch publishes `trip.completed`.
2. api-gateway's dispatch subscriber receives it, looks up the partner booking
   by `dispatch_booking_id`, and sets `partner_bookings.status = completed`.
3. A partner webhook delivery is created and sent (with retry on failure).
4. Your webhook endpoint receives a signed `delivery.completed` POST.

**Local subscriber smoke (NOT a substitute for full E2E):** if dispatch is not
running, you can publish the raw event to Redis to exercise only the api-gateway
subscriber. This matches the live subscriber topic format (`booking_id` = the
dispatch booking id):

```bash
redis-cli PUBLISH "trip.completed" "{\"event\":\"trip.completed\",\"booking_id\":\"$DISPATCH_BOOKING_ID\",\"occurred_at\":\"2026-01-01T00:00:00Z\"}"
```

This updates the partner booking status and enqueues/sends the webhook, but does
**not** exercise the real rider/dispatch flow.

Dispatch topic → partner status → webhook event mapping:

| Dispatch topic              | Partner status | Webhook event         |
|-----------------------------|----------------|-----------------------|
| `request.accepted`          | `accepted`     | `delivery.accepted`   |
| `request.no_provider_found` | `no_provider`  | `delivery.no_provider`|
| `trip.started`              | `in_progress`  | `delivery.in_progress`|
| `trip.completed`            | `completed`    | `delivery.completed`  |
| `trip.cancelled`            | `cancelled`    | `delivery.cancelled`  |
| `trip.failed`               | `failed`       | `delivery.failed`     |

### Step 11 — Verify the webhook signature

Each webhook POST carries:

```
Content-Type: application/json
X-KarryGo-Event:     delivery.completed
X-KarryGo-Delivery:  <delivery_id>
X-KarryGo-Timestamp: <unix_seconds>
X-KarryGo-Signature: sha256=<hex>
```

The signature is `HMAC_SHA256(timestamp + "." + raw_body, signing_secret)`:

```bash
node - <<'NODE'
const crypto = require("crypto");
const signingSecret = process.env.WEBHOOK_SIGNING_SECRET;
const timestamp     = process.env.WEBHOOK_TIMESTAMP;
const body          = process.env.WEBHOOK_RAW_BODY;
const received      = process.env.WEBHOOK_SIGNATURE;
const expected = "sha256=" + crypto
  .createHmac("sha256", signingSecret)
  .update(`${timestamp}.${body}`)
  .digest("hex");
console.log({ expected, received, valid: expected === received });
NODE
```

Usage (copy the values from your webhook receiver, e.g. webhook.site):

```bash
export WEBHOOK_SIGNING_SECRET="whsec_..."
export WEBHOOK_TIMESTAMP="1720000000"
export WEBHOOK_SIGNATURE="sha256=..."
export WEBHOOK_RAW_BODY='{"event":"delivery.completed","delivery_id":"...","booking_ref":"KGP-...","status":"completed",...}'
# then run the node command above -> { valid: true }
```

> Use the **exact raw body bytes** the receiver got — re-serializing may change
> whitespace/key order and break the signature.

### Step 12 — Revoke a key

```bash
curl -s -X DELETE "$BASE_URL/api/v1/partner/v1/keys/$KEY_ID" \
  -H "Authorization: Bearer $SESSION_JWT" | jq .
```

Expected (`200`): `{ "success": true, "message": "Key revoked." }`. A key you
don't own → `404 not_found`.

Then the revoked key must fail on the next API call:

```bash
curl -i -X POST "$BASE_URL/api/v1/partner/v1/quotes" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"pickup":{"lat":6.4474,"lng":3.4343},"dropoff":{"lat":6.4969,"lng":3.3481},"tier":"standard","size":"small"}'
```

**Actual behavior:** `403` with code `api_key_revoked` (the API-key middleware
returns `403 api_key_revoked` for a revoked key — not `401 invalid_api_key`).
Must **not** return `200`.

### Step 13 — Logout (session revocation)

```bash
curl -s -X POST "$BASE_URL/api/v1/partner/v1/auth/logout" \
  -H "Authorization: Bearer $SESSION_JWT" | jq .
```

Expected (`200`): `{ "success": true, "message": "Logged out." }`.

Then the same session JWT must fail on protected dashboard routes:

```bash
curl -i "$BASE_URL/api/v1/partner/v1/auth/me" -H "Authorization: Bearer $SESSION_JWT"
```

Expected: `401` with code `session_revoked`.

---

## 4. Executable smoke script

A non-delivery smoke script is provided at
`services/api-gateway/scripts/smoke_partner_api.sh`. It runs register → login →
`/me` → create key → list keys → quote → (optional delivery) → (optional webhook)
→ revoke key → verify revoked → logout → verify session revoked, and exits
non-zero on any failure. See the script header for flags
(`RUN_DELIVERY_SMOKE`, `WEBHOOK_URL`, `BASE_URL`).

---

## 5. Production checklist

### Auth System
- [x] `password_hash` column added to `partner_accounts`
- [x] `partner_sessions` table created with `token_hash` index
- [x] `POST /auth/register` creates account + first sandbox key + session JWT
- [x] `POST /auth/login` verifies bcrypt + returns session JWT
- [x] `POST /auth/logout` revokes session by token hash
- [x] `RequirePartnerSession` verifies JWT and checks the session is not revoked
- [x] `RequireActiveAccount` enforces `partner_accounts.status` on API-key routes
- [x] `register`/`login` are public (no auth middleware)
- [ ] `PARTNER_JWT_SECRET` is set in the deployed environment
- [ ] `PARTNER_JWT_SECRET` is at least 32 random characters

### API Key Management
- [x] `POST /keys` issues `sk_test_` / `sk_live_` keys
- [x] live key requires partner `plan=live`
- [x] raw key shown exactly once
- [x] API key hash stored (bcrypt); raw key never recoverable
- [x] `GET /keys` returns prefix only
- [x] `DELETE /keys/:id` verifies ownership before revoking
- [x] `POST /internal/partner/accounts/:id/activate-live` upgrades plan
- [x] revoked key fails on the next API call (`403 api_key_revoked`)

### Pricing and Delivery
- [x] `HaversineKm` in `internal/pkg/geo/`
- [x] `CalculateFare` in `internal/pkg/pricing/`
- [x] `GenerateBookingRef` uses `crypto/rand` in `internal/pkg/ref/`
- [x] `POST /quotes` returns fare via the nested pickup/dropoff contract
- [x] `POST /quotes` uses env pricing config, no hardcoded values
- [x] `POST /deliveries` returns a `KGP-XXXXXXXX` booking ref
- [x] idempotency on `POST /deliveries` prevents duplicate bookings
- [x] `booking.dispatch.created` published to Redis after the payment hold

### Dispatch Subscribers and Webhooks
- [x] all 6 dispatch topics covered (`request.accepted`, `request.no_provider_found`, `trip.started`, `trip.completed`, `trip.cancelled`, `trip.failed`)
- [x] `partner_bookings.status` updates per dispatch event
- [x] webhook delivery created/enqueued after each status update
- [x] webhook POST includes `X-KarryGo-Signature`
- [x] webhook POST includes `X-KarryGo-Event`
- [x] webhook POST includes `X-KarryGo-Delivery`
- [x] webhook POST includes `X-KarryGo-Timestamp`
- [x] retry schedule `30s → 5m → 30m → 2h → failed_permanent`
- [x] already-delivered webhooks are skipped (idempotent)

### Security
- [x] passwords bcrypt (cost 12), never plaintext
- [x] API keys bcrypt-hashed, never plaintext
- [x] session tokens stored as SHA-256 hash, never plaintext
- [x] partner isolation on all partner queries
- [x] partner A cannot see/revoke partner B keys/webhooks/deliveries
- [x] webhook HTTPS-only; HTTP/localhost/private IPs rejected with `400`
- [x] rate limiting per partner per route with `X-RateLimit-*` headers
- [ ] `.env` is git-ignored and not committed (verify in deploy)
- [x] `signing_secret` shown once, stored AES-GCM encrypted at rest
- [x] internal routes require `X-Internal-Service-Key`

### Smoke Test
- [x] register returns session JWT + first API key
- [x] login returns a fresh session JWT
- [x] `/auth/me` works with the session JWT
- [x] `/keys` creates another sandbox key
- [x] `/keys` list returns prefix only
- [x] `/quotes` with Lagos coordinates returns fare in kobo
- [x] `/deliveries` returns a `KGP-` ref and `dispatching` status
- [x] Redis receives `booking.dispatch.created`
- [x] webhook registration returns `signing_secret`
- [ ] dispatch completion causes the receiver to get `delivery.completed` (needs full stack)
- [x] webhook signature verifies with `signing_secret`
- [x] revoked key fails on the next API call
- [x] logout causes `session_revoked` on the next dashboard call

### Partner API is production-ready when
- [ ] a partner can self-register and receive a sandbox API key in under 2 minutes
- [ ] the partner can log in to their dashboard
- [ ] the partner can manage keys
- [ ] the partner can register webhooks
- [ ] the partner can get a quote
- [ ] the partner can create a delivery
- [ ] creating a delivery publishes to dispatch
- [ ] a rider receives the broadcast
- [ ] dispatch completion updates the partner booking status
- [ ] the partner receives a signed webhook automatically
- [ ] the partner can verify the webhook signature
- [ ] admin is completely separate; nothing admin-facing lives in api-gateway

> The unchecked "production-ready when" items span the full multi-service runtime
> (dispatch rider broadcast, real webhook receipt, deployed secrets) and are
> validated during a staging/E2E run, not by api-gateway unit tests alone.
