# Customer Service

Owns customer profiles, saved locations, preferences, customer auth entry, and
customer-facing request history views. Taxi, dispatch delivery, and hauling
bookings remain owned by their respective operational services.

## Customer Auth

Customer auth is phone OTP first:

1. `POST /api/v1/customer/auth/start` validates the phone number, rate-limits
   OTP requests, stores a hashed OTP challenge in the customer Redis instance,
   and sends or logs the OTP through the notification adapter.
2. `POST /api/v1/customer/auth/verify` verifies the OTP, creates the customer
   record if needed, creates a customer refresh session, and returns the access
   token, refresh token, and customer profile.
3. `POST /api/v1/customer/auth/refresh` rotates the refresh session and returns
   a new access token.
4. `POST /api/v1/customer/auth/logout` revokes the current refresh session.
5. `GET /api/v1/customer/me` returns the authenticated customer profile.

This service owns its own customer auth data. It does not use a standalone
identity service and it does not share auth/profile persistence with taxi,
dispatch delivery, or hauling.

## Feature Structure

Customer-service is organized feature-first under:

```text
internal/features/
```

Current feature folders:

| Folder | Purpose |
|---|---|
| `auth/http` | Auth route registration, request DTOs, handlers, and response envelope helpers |
| `auth/usecases` | Start OTP, verify OTP, refresh token, logout, and authenticated customer workflows |
| `auth/models` | OTP challenge, refresh token, and refresh-session data structures |
| `auth/repositories` | Redis OTP challenge storage and Postgres refresh-session persistence |
| `auth/clients` | OTP delivery clients, including the local logging sender |
| `profile/models` | Customer profile data structures |
| `profile/repositories` | Postgres customer profile persistence |

Shared service setup stays outside features in `internal/config`,
`internal/database`, and `migrations`.

Reusable phone-number normalization lives in `shared/go/phonenumber` so other
services can use the same Nigerian phone-number format.

## Local Infrastructure

In local Docker Compose, customer-service uses:

- `customer-postgres` on host port `5433`.
- `customer-redis` on host port `6380`.

For a pure local setup on macOS, this repo includes a helper script that starts
Redis and Postgres, creates the customer role and database if needed, and
applies the customer migrations:

```bash
./scripts/customer-local-bootstrap.sh
```

If you want to do it manually, the same flow is:

```bash
mkdir -p /tmp/redis-6380
redis-server --port 6380 --dir /tmp/redis-6380 --daemonize yes

mkdir -p /tmp/postgres-customer
initdb -D /tmp/postgres-customer
pg_ctl -D /tmp/postgres-customer -o "-p 5433" start

psql -p 5433 -d template1
```

Inside `psql`, create the role and database:

```sql
CREATE USER cosmicforge_logistics WITH PASSWORD 'cosmicforge_logistics';
CREATE DATABASE customer_service OWNER cosmicforge_logistics;
```

Then apply the customer migrations:

```bash
psql "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5433/customer_service?sslmode=disable" \
  -f services/customer-service/migrations/001_customer_auth.sql \
  -f services/customer-service/migrations/002_customer_email_auth.sql
```

Important environment variables:

| Variable | Purpose |
|---|---|
| `MIGRATION` | When `true`, applies customer-service SQL migrations on local startup |
| `CUSTOMER_DATABASE_URL` | Customer-owned Postgres connection string |
| `CUSTOMER_REDIS_ADDR` | Customer-owned Redis address or Redis namespace endpoint |
| `CUSTOMER_ACCESS_TOKEN_SECRET` | HMAC secret for customer access tokens |
| `CUSTOMER_REFRESH_TOKEN_SECRET` | HMAC secret for customer refresh token hashes |
| `CUSTOMER_OTP_SECRET` | HMAC secret for OTP hashes |
| `CUSTOMER_DEBUG_OTP` | Returns OTP in responses for local development only |

## Database

The customer auth schema is in:

```text
services/customer-service/migrations/001_customer_auth.sql
```

It creates:

- `customers`
- `customer_sessions`
- `customer_auth_events`

The service expects these migrations to be applied to the customer-service
database before handling live auth requests.

## Postgres Access

To log in and inspect the database:

```bash
psql "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5433/customer_service?sslmode=disable"
```

Useful queries:

```sql
\dt
SELECT * FROM customers ORDER BY created_at DESC LIMIT 10;
SELECT * FROM customer_sessions ORDER BY created_at DESC LIMIT 10;
SELECT * FROM customer_auth_events ORDER BY created_at DESC LIMIT 10;
```

To leave `psql`, run:

```sql
\q
```

## Response Format

All auth endpoints use the shared Cosmicforge Logistics success and error envelopes. Error
responses come from `shared/go/httpx` and `shared/go/apperrors`, for example:

```json
{
  "success": false,
  "error": {
    "code": "validation_failed",
    "message": "Check your details.",
    "request_id": "req-id",
    "fields": [
      {
        "field": "phone",
        "message": "Enter a valid Nigerian phone number."
      }
    ]
  }
}
```

## V4 Section 16/18 customer integration (local)

Authenticated customers can request a scoped support-evidence upload via
`POST /api/v1/customer/media-upload-sessions`. Customer Service verifies the
Support complaint owner, purpose, MIME and size, then uses backend HMAC to mint
a short-lived, single-use Media capability. The phone receives only that
capability; profile photos use the customer bearer proxy and the mobile app no
longer embeds a Media service token. Media owns bytes and metadata. Configure
the Support URL, private/public Media URLs and Media HMAC secret on trusted
backend services before enabling this path in staging.

Journey Share create/revoke/expiry now write reference-only notification
intents atomically with share state in migration `011_journey_share_notifications.sql`.
The worker retries and dead-letters Notification outages; no share token enters
the outbox. PostgreSQL concurrency and outage/retry tests pass locally. The
consumer must read current Share state after following a route intent.

## Section 17 marketing consent authority

customer-service owns marketing consent (`migrations/020_marketing_consent.sql`):
`customer_marketing_consents` (one row per customer per channel
`in_app|push|email`, `OPTED_IN|WITHDRAWN`, `market` as declared by the app at
opt-in or NULL = unknown, `version`, `captured_at`, `withdrawn_at`,
`capture_source`) plus the append-only `customer_marketing_consent_history`.
Consent is never defaulted, inferred or backfilled; an unrecorded channel reads
`NOT_RECORDED`.

| Route | Auth | Notes |
|---|---|---|
| `GET /api/v1/customer/consents/marketing` | customer bearer | All three channels. |
| `PUT /api/v1/customer/consents/marketing` | customer bearer | `{channel, opted_in, market?}`. Idempotent (a repeat changes nothing). Withdrawal always allowed; opt-in needs `CUSTOMER_MARKETING_CONSENT_ENABLED=true` (else 503). |
| `GET /internal/customer/v1/marketing-audience?channel=&market=&cursor=&limit=` | HMAC v2, caller `notification-service` only, single-use nonce | Active customers opted into channel in market, id order, ≤500 per page, with `consent_version` and `as_of`. NULL market never matches. |
| `POST /internal/customer/v1/marketing-consent/check` | same | `{channel, customer_ids[≤500]}` → current eligibility per id with a reason; the email address is returned only for the email channel and only when eligible. |

The internal routes need `CUSTOMER_MARKETING_AUDIENCE_ENABLED=true` (else 503)
and a `notification-service=<secret>` entry in `CUSTOMER_SERVICE_SECRETS`
(the same value notification-service reads from
`NOTIFICATION_CUSTOMER_AUDIENCE_SECRET_FILE`).

Admin: the `customers` resource's `marketing_consent` dimension is `OPTED_IN`
when any channel is opted in, `WITHDRAWN` when consent was recorded but none is
opted in, and null when nothing was ever recorded. A fulfilled
`CONSENT_WITHDRAWAL` privacy request now also withdraws every opted-in
marketing channel (version advance + history row bound to the decision id) and
counts them in `consents_withdrawn`.
