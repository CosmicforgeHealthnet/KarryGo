# Security

## Authentication

Corporate authentication is passwordless — see
[passwordless-authentication.md](passwordless-authentication.md). No user
password exists in any form. Provider authentication stays in the owning
driver service ([provider-authentication-boundary.md](provider-authentication-boundary.md)).

## Secrets and what is never stored

| Secret | Storage |
| --- | --- |
| OTP code | keyed HMAC digest only (`code_digest`) |
| Refresh token | HMAC hash only (`refresh_token_hash`) |
| Staff / provider invitation token | HMAC hash only |
| Email / phone (for rate limits and security events) | keyed hash only |
| Access token | never stored |
| User password | does not exist |

Mandatory secrets — `FLEET_PARTNER_OTP_HMAC_SECRET`,
`FLEET_PARTNER_SESSION_HASH_SECRET`, `FLEET_PARTNER_CONTACT_HASH_SECRET`,
`FLEET_PARTNER_ACCESS_TOKEN_SECRET` — have **no defaults in any
environment**; startup fails without them. Validation errors never echo a
secret value (asserted by `TestValidationErrorNeverLeaksAuthSecrets`).

## Never logged, never published

Authorization headers, cookies, API secrets, refresh tokens, OTP codes,
invitation tokens, identity documents, bank details and raw request bodies
containing sensitive data are never logged. Domain events and notification
payloads carry masked or referenced values only — never an OTP, token,
credential, identity document or full financial account detail.

## Authorization

Deny-by-default. Authority exists only where an unexpired, unrevoked role
binding covers the target scope. Token claims never override repository
tenant filtering, and a path or body `organizationId` never widens scope —
a mismatched path organization returns `404`, not `403`, so cross-tenant
existence is not disclosed.

Privilege escalation is refused: an actor may only grant permissions they
themselves hold. System roles are immutable. Suspension revokes sessions in
the same transaction and bumps the role version, so access stops immediately.

## Enumeration and disclosure

`/auth/start` is indistinguishable between existing, non-existent, suspended
and conflicted accounts. Cross-tenant reads return `not_found`. Session
lookups for another identity return `not_found`. Sessions listings never
serialize refresh hashes.

## Rate limiting and lockout

Bounded counters throttle challenge creation, verification, resend, refresh,
organization switching and step-up. Durable per-challenge attempt limits live
in PostgreSQL, so a Redis outage degrades throttling rather than removing the
hard limit.

## Audit

Sensitive mutations record actor, organization, membership, action, resource,
redacted before/after summaries, reason, request ID, correlation ID, session
and step-up reference where applicable, and timestamp — in the same
transaction as the mutation. Audit records, security events, status history,
approval decisions and provider membership history are append-only, enforced
by database triggers.

## Build-enforced prohibitions

`internal/architecture_auth_test.go` fails the build on: password-credential
tokens in code or migrations, a password-style login route, raw OTP/token
columns, SMS delivery in corporate auth, local provider credential ownership,
a foreign key on `provider_id`, and local balance/ledger columns.
`internal/architecture_test.go` additionally forbids importing another
service's packages, Partner API packages, foreign database DSNs, float money
and pricing tokens.
