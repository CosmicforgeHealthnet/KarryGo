# Notification Service — Local Setup & Postman

This folder contains everything you need to run the notification service on your
machine and exercise its API from Postman.

- `notification.postman_collection.json` — one request per endpoint, grouped by auth mode.
- `notification.postman_environment.json` — local environment variables (URLs + token slots).
- `devtoken/` — a small Go helper that mints dev bearer tokens for the app-facing proxy folders.

For the full notification system overview (event catalog, who-sends-what, the app
proxy flow, FCM), see [`../../../docs/notifications.md`](../../../docs/notifications.md).
This guide focuses on **standing it up locally**.

## How it works (for beginners)

New to this? Read this section first — it explains the whole idea before the setup
steps. Skip it if you already know the architecture.

### The problem

When something happens — a driver accepts a booking, a payment succeeds — you want
to **tell the user**. There are several ways to reach them:

- a **push** notification (the banner on the phone, even when the app is closed),
- an **email**,
- a live update **while they're looking at the app** (websocket),
- a **bell-icon list** inside the app to scroll later (in_app).

You *could* make every service (hauling, payments, …) talk to Firebase, an email
server, etc. directly — but that's the same plumbing rebuilt everywhere. So there is
**one service whose only job is notifications.** Everyone else just says "notify this
person," and it handles the how.

### The post office analogy

```
┌─────────────────┐   "notify customer X:        ┌──────────────────────┐
│ hauling-service │    their cargo was delivered" │ notification-service │
│ payment-service │ ───────────────────────────►  │  (the post office)   │
│ customer-service│         (HMAC-signed)         └──────────┬───────────┘
└─────────────────┘                                          │
   the SENDERS                                  decides HOW to deliver:
                                          push / email / websocket / in_app
                                                           │
                                            ┌──────────────┴───────────────┐
                                            ▼                              ▼
                                    📱 phone push (FCM)          🔔 live + saved feed
                                                                  (the app shows it)
```

`notification-service` is a **post office**. Other services drop off a letter
("tell customer-1 their cargo arrived"); the post office figures out which mailboxes
that person has (phone, email, open app) and delivers to each.

### Dropping off a letter (the send side)

A sender calls one endpoint, `POST /send`, with a small JSON:

```json
{
  "idempotency_key": "driver-hauling-service:cargo.delivered:booking-123",
  "source_service": "driver-hauling-service",
  "event_type": "cargo.delivered",
  "recipient": { "type": "customer", "id": "customer-1" },
  "template_key": "cargo.delivered",
  "template_data": { "booking_id": "booking-123" }
}
```

Two ideas to understand:

- **`idempotency_key`** is a unique fingerprint for this exact event. If the same
  letter is dropped off twice (a network retry, a webhook firing twice), the post
  office recognises the fingerprint and delivers only once — no duplicate spam.
- **`template_key`** points at a message template stored in the database
  ("Cargo delivered" / "Your cargo has been delivered…"). The sender names the
  template and fills the blanks (`booking_id`) via `template_data`, instead of
  writing the text in every service. That's why setup includes `go run ./cmd/seed`
  — it loads those templates.

Sending is **fire-and-forget**: if notifying fails, the booking still succeeds. A
notification is never allowed to break the actual business action.

### Why services sign requests, but apps don't

Senders are *backend services* trusted with a shared secret key. They **sign** each
request (HMAC) to prove "I really am hauling-service." That is the `Service (HMAC)`
Postman folder — the pre-request script does the signing math for you.

A **phone app cannot hold that secret** (anyone could decompile the app and steal
it). So apps never call notification-service directly. Instead they ask *their own*
service — which they already log into — to make the signed call for them:

```
📱 customer app ──(its normal login token)──► customer-service ──(HMAC)──► notification-service
                                              "the proxy / middleman"
```

That middleman is the **proxy** — the `/customer/notifications` and
`/provider/notifications` endpoints. The app uses the same login token it already has.

### How the app receives notifications

1. **The feed (catch-up):** `GET /customer/notifications` returns the recent list —
   the bell-icon screen. Works any time.
2. **Live (websocket):** a websocket is a phone line that *stays open*. Instead of the
   app asking "anything new?" repeatedly, the server pushes the moment something
   happens. Flow: the app asks the proxy for a short-lived **realtime token**
   (15-minute pass) → opens `ws://…/ws?token=…` → now every delivery for that person
   is pushed down the open line and the app updates instantly. (In the provider app
   this is the fast lane for incoming booking requests; a slower poll stays as a
   backup if the line drops.)

### The whole journey, one line each

1. Driver marks cargo delivered → hauling-service updates the booking status.
2. Hauling drops a letter at notification-service: "cargo.delivered → customer-1"
   (signed, fire-and-forget).
3. The post office saves it, looks up the template, and delivers on each channel the
   customer has.
4. App open? The websocket pushes it → seen **instantly** + saved to the feed.
5. App closed? FCM push wakes the phone (once Firebase is configured).
6. Later, the customer opens the bell icon → `GET /notifications` shows the list.

You can replay this whole flow yourself in Postman without any app — see
[End-to-end smoke test](#end-to-end-smoke-test) below.

## Overview

- Service: `notification-service`, local HTTP port **8106**.
- API base path: `/api/v1/notifications`.
- Channels: `push` (FCM), `email` (SMTP), `websocket` (realtime), `in_app` (durable feed).
- Response envelope: `{ "success": true, "data": { } }`.

Auth modes:

| Caller            | Auth                                                          | Endpoints |
|-------------------|--------------------------------------------------------------|-----------|
| Backend services  | HMAC service auth (`shared/go/serviceauth`)                  | `/send`, `/messages`, `/realtime/token`, `/devices` |
| Realtime client   | Realtime token (minted from `/realtime/token`)              | `/ws` (websocket upgrade) |
| Customer app      | Bearer token via **customer-service** proxy                 | `/api/v1/customer/notifications/*` |
| Provider app      | Bearer token via **driver-hauling-service** proxy           | `/api/v1/hauling/provider/notifications/*` |

Apps never hold the HMAC secret — they call their owning service's proxy, which
signs the downstream HMAC call to notification-service on their behalf.

## Prerequisites

- Go (matching the version in [`../go.mod`](../go.mod)).
- PostgreSQL and Redis — either run them yourself, or use the bootstrap script below.

## Quick start (bootstrap script)

From the repo root:

```bash
bash scripts/notification-local-bootstrap.sh
```

This starts Postgres on port `5438`, Redis on port `6385`, creates the
`cosmicforge_logistics` role and `notification_service` database, and applies the
migrations.

Then start the service:

```bash
cd services/notification-service
MIGRATION=true go run ./cmd
```

The service reads `.env` automatically. Copy `.env.example` to `.env` first if you do
not have one (`.env` is git-ignored — never commit secrets). The committed
[`../.env.example`](../.env.example) is the source of truth for every variable and its
local default.

### Seed notification templates (recommended)

`POST /send` requests that use a `template_key` resolve their title/body from the
`notification_templates` table. Seed the platform templates once:

```bash
cd services/notification-service
go run ./cmd/seed
```

Without templates, `template_key` sends fall back to whatever inline `title`/`body`
the request includes (the "inline title/body" Postman example needs no templates).

## Manual setup (no script)

1. Start a local Postgres and Redis (any ports — just match them in the env vars below).
2. Create the role and database:
   ```sql
   CREATE ROLE cosmicforge_logistics WITH LOGIN PASSWORD 'cosmicforge_logistics';
   CREATE DATABASE notification_service OWNER cosmicforge_logistics;
   ```
3. Set the environment and run with migrations enabled on first boot:
   ```bash
   export NOTIFICATION_DATABASE_URL='postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5438/notification_service?sslmode=disable'
   export NOTIFICATION_REDIS_ADDR='localhost:6385'
   export MIGRATION=true
   cd services/notification-service
   go run ./cmd
   ```

## Key environment variables

The defaults below match the bootstrap script and `infra/docker-compose.yml`. See
[`../.env.example`](../.env.example) for the complete list. **Do not commit real secrets.**

| Variable | Local default | Purpose |
|----------|---------------|---------|
| `HTTP_ADDR` | `:8106` | Listen address / port |
| `NOTIFICATION_DATABASE_URL` | `postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5438/notification_service?sslmode=disable` | Postgres DSN |
| `NOTIFICATION_REDIS_ADDR` | `localhost:6385` | Redis address (delivery streams) |
| `NOTIFICATION_SERVICE_SECRETS` | `customer-service=development-customer-notification-secret` | HMAC secrets per sending service (`name=secret,...`) |
| `NOTIFICATION_REALTIME_TOKEN_SECRET` | `development-notification-realtime-token-secret` | Signs/verifies websocket realtime tokens |
| `FIREBASE_PROJECT_ID` / `GOOGLE_APPLICATION_CREDENTIALS` | _(empty — logging push sender)_ | Enables real FCM push when set |
| `NOTIFICATION_SMTP_HOST` (+ `_PORT`/`_USERNAME`/`_PASSWORD`/`_FROM`) | _(empty — logging email sender)_ | Enables real SMTP email when set |
| `NOTIFICATION_WORKER_CONCURRENCY` | `5` | Delivery worker goroutines |
| `NOTIFICATION_MAX_ATTEMPTS` | `5` | Retry attempts before dead-letter |
| `MIGRATION` | `false` | Set `true` to apply migrations on startup |

When `NOTIFICATION_SERVICE_SECRETS` has multiple senders, add each one, e.g.:

```
NOTIFICATION_SERVICE_SECRETS=customer-service=development-customer-notification-secret,driver-hauling-service=development-hauling-notification-secret,payment-wallet-service=development-wallet-notification-secret
```

Each sender signs with the secret keyed by its own service name. The default `.env`
only ships the customer-service secret; add the hauling/wallet entries if you test
those senders.

## Verify it's up

```bash
curl http://localhost:8106/health
curl http://localhost:8106/api/v1/notifications/meta
```

## Using the Postman collection

1. In Postman: **Import** both files in this folder
   (`notification.postman_collection.json` and `notification.postman_environment.json`).
2. Select the **"Notification (local)"** environment (top-right).
3. The `Health` folder needs no auth and is the fastest way to confirm connectivity.

### Service (HMAC) folder — sending notifications

No tokens needed. The `Service (HMAC)` folder has a pre-request script that signs every
request automatically using `{{service_name}}` and `{{service_secret}}` (defaults:
`customer-service` / `development-customer-notification-secret`). It reproduces
`shared/go/serviceauth`: HMAC-SHA256 over `METHOD\nrequestURI\ntimestamp\nsha256hex(body)`,
setting the `X-Service-Name`, `X-Service-Timestamp`, and `X-Service-Signature` headers.

`{{service_name}}` / `{{service_secret}}` must match an entry in the service's
`NOTIFICATION_SERVICE_SECRETS`. To send as a different source service, change both
values to that service's name and secret.

- **POST /send (inline title/body)** works immediately — no templates needed.
- **POST /send (template key)** needs the templates seeded (`go run ./cmd/seed`).
- **GET /messages** lists what a recipient received — set `{{recipient_id}}` to the id
  you sent to.

### WebSocket (realtime)

Postman's REST request cannot open a `ws://` upgrade. To watch live pushes:

1. Run **Service (HMAC) > POST /realtime/token** — its test script saves the minted
   token to `{{realtime_token}}`.
2. Connect a real WebSocket client to
   `ws://localhost:8106/api/v1/notifications/ws?token=<realtime_token>`, e.g. with
   [`websocat`](https://github.com/vi/websocat):
   ```bash
   websocat "ws://localhost:8106/api/v1/notifications/ws?token=$REALTIME_TOKEN"
   ```
   or Postman's **New > WebSocket Request**.
3. Send a notification (Service (HMAC) > POST /send) to the same recipient and watch it
   arrive as `{ event_type, title, body, data }`.

### App proxy folders (bearer tokens)

The `Customer App Proxy` and `Provider App Proxy` folders hit the owning services
(customer-service `:8101`, driver-hauling-service `:8104`) with a bearer token. Those
services must be running for these folders to work. Mint dev tokens with the bundled
helper (run from the notification-service directory):

```bash
cd services/notification-service

# Customer token -> paste into {{customer_token}}
go run ./docs/devtoken -kind=customer

# Provider token -> paste into {{hauling_provider_token}}
go run ./docs/devtoken -kind=provider
```

The helper signs with the same documented dev secrets the proxy services verify
against (`CUSTOMER_ACCESS_TOKEN_SECRET` / `HAULING_PROVIDER_TOKEN_SECRET`), so the
tokens are accepted out of the box. Override with `-secret`, `-sub`, `-role`, or `-ttl`.
The token prints to stdout; a summary (kind/service/role/sub/expiry) goes to stderr.

> The recipient is always taken from the token subject, so a customer/provider can only
> read or act on their own notifications. To line up a proxy feed with a `/send` you made
> directly, mint the token with `-sub` matching the `recipient.id` you sent to
> (e.g. `go run ./docs/devtoken -kind=customer -sub=customer-1`).

## End-to-end smoke test

With notification-service running and templates seeded:

1. **Service (HMAC) > POST /send (inline title/body)** with `{{recipient_id}}` = `customer-1`.
   Expect `202` and a `message_id`.
2. **Service (HMAC) > GET /messages** (recipient `customer-1`) — your message appears.
3. (Optional) Start customer-service (`:8101`), mint a customer token with
   `-sub=customer-1`, set `{{customer_token}}`, then **Customer App Proxy > GET
   /customer/notifications** returns the same feed through the app path.

## Troubleshooting

- **`401` on a Service (HMAC) call** — `service_name`/`service_secret` mismatch with
  `NOTIFICATION_SERVICE_SECRETS`, or clock skew (the signed timestamp must be within
  5 minutes of the server). The request URI used for signing must exactly match the
  path the server sees, including the `/api/v1/notifications` prefix and any query string.
- **`WebSocket upgrade failed` on /ws** — you sent it over REST. Use a real WebSocket
  client (see above).
- **Realtime token rejected by /ws** — token expired (15-minute TTL); re-run
  `POST /realtime/token`. The `NOTIFICATION_REALTIME_TOKEN_SECRET` must be the same value
  that minted the token.
- **`template_key` send has empty title/body** — templates not seeded. Run
  `go run ./cmd/seed`, or send inline `title`/`body` instead.
- **Push/email do nothing** — by default the service uses logging senders (no FCM/SMTP
  config). Set `FIREBASE_PROJECT_ID` + `GOOGLE_APPLICATION_CREDENTIALS` for push, and
  the `NOTIFICATION_SMTP_*` vars for email. `websocket` and `in_app` work without either.
- **`403` on a proxy call** — bearer token secret mismatch, or the owning service
  (customer-service / hauling) is not running. Re-mint with `devtoken`.
