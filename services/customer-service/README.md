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
