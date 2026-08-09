# Configuration

Configuration is loaded once at startup (`internal/config.Load`) and
validated; **missing mandatory settings abort startup** with a message that
never contains secret values. Env names follow the repository prefix
convention. Mapping from the specification's generic names:

| Spec name | Actual variable | Mandatory | Default |
| --- | --- | --- | --- |
| SERVICE_NAME | (constant `fleet-partner-service`) | — | — |
| SERVICE_ENV | `APP_ENV` | no | `development` |
| HTTP_PORT | `HTTP_ADDR` | no | `:8113` |
| WORKER_ENABLED | `FLEET_PARTNER_WORKER_ENABLED` | no | `true` (in-process) |
| DATABASE_URL | `FLEET_PARTNER_DATABASE_URL` | **yes** | — |
| REDIS_URL | `FLEET_PARTNER_REDIS_ADDR` / `_PASSWORD` / `_DB` | no (unless `FLEET_PARTNER_REDIS_REQUIRED=true`) | unset |
| LOG_LEVEL | `FLEET_PARTNER_LOG_LEVEL` | no | `info` |
| METRICS_ENABLED | `FLEET_PARTNER_METRICS_ENABLED` | no | `true` |
| METRICS_AUTH_TOKEN | `FLEET_PARTNER_METRICS_AUTH_TOKEN` | **yes when metrics enabled** | — |
| ACCESS_TOKEN_VERIFICATION | `FLEET_PARTNER_ACCESS_TOKEN_SECRET` | **yes** | — |
| INTERNAL_SERVICE_AUTH | `FLEET_PARTNER_SERVICE_SECRETS` (`name=secret,…`) | **yes in production** | unset |
| *_SERVICE_BASE_URL | `FLEET_PARTNER_<SERVICE>_BASE_URL` + `_SECRET` (11 services, see `.env.example`) | no — unset means client not configured | unset |
| OUTBOX_POLL_INTERVAL | `FLEET_PARTNER_OUTBOX_POLL_INTERVAL` | no | `2s` |
| JOB_POLL_INTERVAL | `FLEET_PARTNER_JOB_POLL_INTERVAL` | no | `2s` |
| JOB_LEASE_DURATION | `FLEET_PARTNER_JOB_LEASE_DURATION` | no | `60s` |
| JOB_HEARTBEAT_INTERVAL | `FLEET_PARTNER_JOB_HEARTBEAT_INTERVAL` | no | `15s` |
| DATABASE_MAX_OPEN_CONNECTIONS | `FLEET_PARTNER_DATABASE_MAX_OPEN_CONNECTIONS` | no | `10` |
| DATABASE_MAX_IDLE_CONNECTIONS | `FLEET_PARTNER_DATABASE_MAX_IDLE_CONNECTIONS` | no | `2` |
| DATABASE_CONNECTION_MAX_LIFETIME | `FLEET_PARTNER_DATABASE_CONNECTION_MAX_LIFETIME` | no | `1h` |
| HTTP_CLIENT_TIMEOUT | `FLEET_PARTNER_HTTP_CLIENT_TIMEOUT` | no | `10s` |
| SHUTDOWN_TIMEOUT | `FLEET_PARTNER_SHUTDOWN_TIMEOUT` | no | `15s` |
| — | `MIGRATION` (`true` applies migrations at startup) | no | `false` |
| — | `FLEET_PARTNER_MAX_REQUEST_BODY_BYTES` | no | `1048576` |

Durations use Go syntax (`2s`, `15m`, `1h`).

## Sections 8–10 authentication settings

| Spec name | Actual variable | Mandatory | Default |
| --- | --- | --- | --- |
| OTP_HMAC_SECRET | `FLEET_PARTNER_OTP_HMAC_SECRET` | **yes, every environment** | — |
| SESSION_HASH_SECRET | `FLEET_PARTNER_SESSION_HASH_SECRET` | **yes, every environment** | — |
| — | `FLEET_PARTNER_CONTACT_HASH_SECRET` | **yes, every environment** | — |
| OTP_LENGTH | `FLEET_PARTNER_OTP_LENGTH` | no | `6` (bounded 6–10) |
| OTP_EXPIRY | `FLEET_PARTNER_OTP_EXPIRY` | no | `10m` (bounded 1m–1h) |
| OTP_MAX_ATTEMPTS | `FLEET_PARTNER_OTP_MAX_ATTEMPTS` | no | `5` (bounded 1–10) |
| OTP_RESEND_COOLDOWN | `FLEET_PARTNER_OTP_RESEND_COOLDOWN` | no | `60s` |
| OTP_MAX_REQUESTS_PER_HOUR | `FLEET_PARTNER_OTP_MAX_REQUESTS_PER_HOUR` | no | `10` |
| ACCESS_TOKEN_TTL | `FLEET_PARTNER_ACCESS_TOKEN_TTL` | no | `15m` (max 1h) |
| REFRESH_TOKEN_TTL | `FLEET_PARTNER_REFRESH_TOKEN_TTL` | no | `720h` (must exceed access TTL) |
| STEP_UP_TTL | `FLEET_PARTNER_STEP_UP_TTL` | no | `5m` |
| STAFF_INVITATION_TTL | `FLEET_PARTNER_STAFF_INVITATION_TTL` | no | `168h` |
| PROVIDER_INVITATION_TTL | `FLEET_PARTNER_PROVIDER_INVITATION_TTL` | no | `336h` |
| — | `FLEET_PARTNER_DEFAULT_COUNTRY_CODE` | no | `234` |
| — | `FLEET_PARTNER_STAFF_ACCEPT_URL` / `_PROVIDER_ACCEPT_URL` | no | unset |
| — | `FLEET_PARTNER_AUTHORIZATION_CACHE_TTL` | no | `30s` |

**There is no password setting** — corporate authentication is passwordless
and no user password exists in the platform.

The three hashing secrets have **no default in any environment**, including
development: a predictable OTP pepper would make a six-digit code guessable
offline from a leaked table. Startup fails without them, and validation
errors never echo a secret value.

The base URLs for `notification-service`,
`verification-compliance-service`, `payment-wallet-service`,
`media-file-service` and the three driver services use the client settings in
the table above. `notification-service` must be configured for OTP delivery
to work — if it is not, `/auth/start` returns `503
otp_delivery_unavailable` rather than pretending a code was sent.

## Section 55 organization integrations

All organization API-client and webhook capabilities default to false. Enabling API clients requires `FLEET_PARTNER_API_CLIENT_HASH_KEY` with at least 32 bytes. Enabling webhooks requires an exactly 32-byte `FLEET_PARTNER_WEBHOOK_SECRET_ENCRYPTION_KEY`; neither key has a default or may be logged.

| Variable | Default | Validation/effect |
| --- | --- | --- |
| `FLEET_PARTNER_ORGANIZATION_API_CLIENTS_ENABLED` | `false` | Enables scoped client management/authentication. |
| `FLEET_PARTNER_WEBHOOKS_ENABLED` | `false` | Enables subscription management/fan-out. |
| `FLEET_PARTNER_WEBHOOK_DELIVERY_WORKER_ENABLED` | `false` | Requires webhooks; registers the shared durable delivery handler/coordinator. |
| `FLEET_PARTNER_WEBHOOK_ALLOW_HTTP` | `false` | Development/test only; production rejects `true`. |
| `FLEET_PARTNER_WEBHOOK_PRIVATE_DESTINATION_ALLOWLIST` | unset | Optional comma-separated enterprise CIDRs; metadata, loopback, link-local, and unspecified addresses remain prohibited. |
| `FLEET_PARTNER_WEBHOOK_DELIVERY_TIMEOUT` | `15s` | Positive and no more than one minute. |
| `FLEET_PARTNER_WEBHOOK_REPLAY_WINDOW` | `5m` | Between one and fifteen minutes. |
| `FLEET_PARTNER_WEBHOOK_ROTATION_OVERLAP_MAX` | `1h` | Bounded to 0–24 hours. |
| `FLEET_PARTNER_WEBHOOK_DISABLE_AFTER_FAILURES` | `5` | Bounded to 1–100 terminal failures. |
| `FLEET_PARTNER_WEBHOOK_DELIVERY_MAX_ATTEMPTS` | `12` | Bounded to 1–50 attempts. |
| `FLEET_PARTNER_WEBHOOK_FANOUT_INTERVAL` / `_BATCH_SIZE` | `2s` / `50` | Positive interval; batch 1–500. |

See [section55-organization-api-clients-and-webhooks.md](section55-organization-api-clients-and-webhooks.md) for the authentication, signature, SSRF, retry, and recovery contract.

## Validation rules

- URLs of configured clients must be http(s) with a host.
- `HTTP_ADDR` must carry a valid port (1–65535).
- All intervals/timeouts must be positive.
- `JOB_LEASE_DURATION` must exceed `JOB_HEARTBEAT_INTERVAL`.
- `FLEET_PARTNER_REDIS_ADDR` is required when Redis is marked required.
- Secrets are never printed; there are **no permissive authentication
  fallbacks** — auth-critical settings have no defaults, in any environment.

## Database configuration rule

`FLEET_PARTNER_DATABASE_URL` is the **only** database connection string this
service accepts, and it must resolve to the `fleet-partner-service` database
alone. Configuring a second business database here — `CUSTOMER_DATABASE_URL`,
`WALLET_DATABASE_URL`, `DISPATCH_DATABASE_URL`, `TAXI_DATABASE_URL`,
`HAULING_DATABASE_URL`, `ADMIN_DATABASE_URL` or any equivalent — is
prohibited and fails the architecture test
`TestNoForeignServiceDSNsInConfig`. Data owned by another service is reached
through its API or events, never through a connection string.

## Test-only

`FLEET_PARTNER_TEST_DATABASE_URL` gates the PostgreSQL integration suite
(`internal/integration`). It must point at a **disposable** database
belonging only to this service — the suite applies real migrations,
truncates foundation tables between tests, and exercises rollback.

The suite refuses to run against anything that does not look like a test
database: the name must contain `test`, and names containing `prod`,
`production`, `staging` or `live` are rejected outright. Never point it at
another service's database, or at any database holding real data. See
[local-development.md](local-development.md) for how to create and dispose of
one.
