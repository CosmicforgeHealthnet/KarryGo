# Payment & Wallet Service — Local Setup & Postman

This folder contains everything you need to run the payment-wallet service on your
machine and exercise its API from Postman.

- `payment-wallet.postman_collection.json` — one request per endpoint, grouped by auth mode.
- `payment-wallet.postman_environment.json` — local environment variables (URLs + token slots).
- `devtoken/` — a small Go helper that mints dev bearer tokens so the collection works standalone.

For the full API contract (request/response shapes, enums, who-calls-what), see the
service [`../README.md`](../README.md). This guide focuses on **standing it up locally**.

## Overview

- Service: `payment-wallet-service`, local HTTP port **8105**.
- API base path: `/api/v1/payment-wallet`.
- All money values are **kobo integers** (1 NGN = 100 kobo). Default currency `NGN`.
- Response envelope: `{ "success": true, "data": { } }`.

Four auth modes:

| Caller          | Auth                                                          | Endpoints              |
|-----------------|--------------------------------------------------------------|------------------------|
| Customer app    | Bearer token (`role=customer`, `service=customer`)           | `/wallets/*`, `/topups` |
| Provider apps   | Bearer token (`service` = `taxi` \| `dispatch` \| `hauling`) | `/provider/*`          |
| Backend services| HMAC service auth (`shared/go/serviceauth`)                  | `/internal/*`          |
| Paystack        | Webhook signature (`x-paystack-signature`, HMAC-SHA512)      | `/webhooks/paystack`   |

## Prerequisites

- Go (matching the version in [`../go.mod`](../go.mod)).
- PostgreSQL and Redis — either run them yourself, or use the bootstrap script below.

## Quick start (bootstrap script)

From the repo root:

```bash
bash scripts/payment-local-bootstrap.sh
```

This starts Postgres on port `5437`, Redis on port `6384`, creates the
`cosmicforge_logistics` role and `payment_wallet_service` database, and applies the
migrations.

Then start the service:

```bash
cd services/payment-wallet-service
go run ./cmd
```

The service reads `.env` automatically. Copy `.env.example` to `.env` first if you do
not have one (`.env` is git-ignored — never commit secrets). The committed
[`../.env.example`](../.env.example) is the source of truth for every variable and its
local default.

## Manual setup (no script)

1. Start a local Postgres and Redis (any ports — just match them in the env vars below).
2. Create the role and database:
   ```sql
   CREATE ROLE cosmicforge_logistics WITH LOGIN PASSWORD 'cosmicforge_logistics';
   CREATE DATABASE payment_wallet_service OWNER cosmicforge_logistics;
   ```
3. Set the environment and run with migrations enabled on first boot:
   ```bash
   export PAYMENT_WALLET_DATABASE_URL='postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5437/payment_wallet_service?sslmode=disable'
   export PAYMENT_WALLET_REDIS_ADDR='localhost:6384'
   export MIGRATION=true
   cd services/payment-wallet-service
   go run ./cmd
   ```

## Key environment variables

The defaults below match the bootstrap script and `infra/docker-compose.yml`. See
[`../.env.example`](../.env.example) for the complete list. **Do not commit real secrets.**

| Variable | Local default | Purpose |
|----------|---------------|---------|
| `HTTP_ADDR` | `:8105` | Listen address / port |
| `PAYMENT_WALLET_DATABASE_URL` | `postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5437/payment_wallet_service?sslmode=disable` | Postgres DSN |
| `PAYMENT_WALLET_REDIS_ADDR` | `localhost:6384` | Redis address |
| `PAYMENT_WALLET_CUSTOMER_ACCESS_TOKEN_SECRET` | `development-customer-access-token-secret` | Verifies customer bearer tokens |
| `PAYMENT_WALLET_PROVIDER_ACCESS_TOKEN_SECRETS` | `taxi=...,dispatch=...,hauling=...` | Per-service provider token secrets |
| `PAYMENT_WALLET_SERVICE_SECRETS` | `taxi-service=development-payment-wallet-service-secret,...` | HMAC secrets for `/internal/*` |
| `PAYMENT_WALLET_PAYSTACK_PUBLIC_KEY` / `PAYMENT_WALLET_PAYSTACK_SECRET_KEY` | _(test keys, required for Paystack flows)_ | Paystack API keys |
| `PAYMENT_WALLET_PUBLIC_CALLBACK_BASE_URL` | `http://localhost:8105` | Paystack redirect target for top-ups |
| `PAYMENT_WALLET_PLATFORM_FEE_BPS` | `1500` | Platform fee in basis points (15%) |
| `MIGRATION` | `false` | Set `true` to apply migrations on startup |

## Verify it's up

```bash
curl http://localhost:8105/health
curl http://localhost:8105/api/v1/payment-wallet/meta
```

## Using the Postman collection

1. In Postman: **Import** both files in this folder
   (`payment-wallet.postman_collection.json` and `payment-wallet.postman_environment.json`).
2. Select the **"Payment & Wallet (local)"** environment (top-right).
3. Fill in the token slots (below), then send requests.

The `Health` folder needs no auth and is the fastest way to confirm connectivity.

### Customer & provider tokens

These bearer tokens are normally minted by customer-service / the provider apps, signed
with secrets that must match `PAYMENT_WALLET_CUSTOMER_ACCESS_TOKEN_SECRET` /
`PAYMENT_WALLET_PROVIDER_ACCESS_TOKEN_SECRETS`. For standalone local testing, mint them
with the bundled helper (run from the service directory):

```bash
cd services/payment-wallet-service

# Customer token -> paste into {{customer_token}}
go run ./docs/devtoken -kind=customer

# Provider token -> paste into {{hauling_provider_token}} (or taxi/dispatch)
go run ./docs/devtoken -kind=provider -service=hauling
```

The helper uses the same documented dev secrets the service verifies against, so the
tokens are accepted out of the box. Override with `-secret`, `-sub`, `-role`, or `-ttl`
if needed. The token is printed to stdout; a summary (kind/service/role/sub/expiry) goes
to stderr.

> The `Provider` folder's auth points at `{{hauling_provider_token}}` by default. To test
> a taxi or dispatch provider, fill the matching token variable and change the folder
> auth token to it.

### Internal (HMAC) endpoints

No tokens needed. The `Internal (HMAC)` folder has a pre-request script that signs every
request automatically using `{{service_name}}` and `{{service_secret}}` (defaults:
`taxi-service` / `development-payment-wallet-service-secret`). It reproduces
`shared/go/serviceauth`: HMAC-SHA256 over `METHOD\nrequestURI\ntimestamp\nsha256hex(body)`,
setting the `X-Service-Name`, `X-Service-Timestamp`, and `X-Service-Signature` headers.

### Paystack webhook

The `Webhook` request signs the raw body with HMAC-SHA512 using `{{paystack_secret_key}}`,
matching Paystack's `x-paystack-signature`. Set `paystack_secret_key` to the same value as
`PAYMENT_WALLET_PAYSTACK_SECRET_KEY` in your `.env`. The body must stay byte-for-byte
identical between signing and sending — do not put this route behind anything that
re-encodes the body.

## Paystack note

Top-ups (`POST /topups`) and withdrawals (`POST /provider/withdrawals`) call Paystack and
will fail without valid **test** keys in `.env`
(`PAYMENT_WALLET_PAYSTACK_PUBLIC_KEY` / `PAYMENT_WALLET_PAYSTACK_SECRET_KEY` from your
Paystack dashboard). The wallet-summary, transaction-list, and HMAC internal endpoints
work without Paystack.

## Troubleshooting

- **`403` on a customer or provider call** — token secret mismatch. The token must be
  signed with the same secret the service holds for that service key. Re-mint with the
  `devtoken` helper, or check `PAYMENT_WALLET_*_TOKEN_SECRET(S)`.
- **`401` on an internal call** — usually clock skew (signed timestamp must be within
  5 minutes of the server) or a mismatched canonical string. The request URI used for
  signing must exactly match the path the server sees, including the `/api/v1/payment-wallet`
  prefix and any query string.
- **Paystack errors / `5xx` on top-ups or withdrawals** — missing or invalid Paystack keys
  in `.env`.
- **Webhook rejected** — `paystack_secret_key` does not match `PAYMENT_WALLET_PAYSTACK_SECRET_KEY`,
  or the body was modified after signing.
