# Partner browser authentication

## Actual authority and scope

Current partner-web -> `/api/v1/partner/v1/auth/*` -> Gateway `partneraccount`
usecases -> Gateway `partner_accounts` / `partner_sessions`.

This is the existing merchant shipping/API-key/billing dashboard domain. It is
not a proxy for Fleet corporate organizations. Fleet's corporate passwordless
identities and session families remain Fleet-owned. No Fleet bridge, duplicate
session store, cross-service database access, or new identity system was added.
The user's permitted Gateway-authority exception is used. A future consolidation
would require an explicit account/organization migration, not a proxy rewrite.

## Browser contract

All auth POSTs require an exact configured trusted Origin. All auth responses
use Cache-Control no-store / Pragma no-cache. Browser requests use
`credentials: "include"`. There is no browser-readable refresh credential and
no JSON refresh fallback.

- Password login, registration, signup OTP verify and login OTP verify set the
  HttpOnly refresh cookie. JSON retains `token` (access JWT), `expires_at`, account
  and existing non-refresh metadata. API-key creation semantics are unchanged.
- POST `/api/v1/partner/v1/auth/refresh` has no JSON credential; cookie required.
  It checks active account/session/expiry, atomically replaces both hashes in the
  existing row, and issues a fresh cookie plus short-lived access JWT. Old refresh
  and access credentials no longer authenticate. A concurrent refresh has one
  winner; losers receive 401. No additional token-family persistence is created.
- POST `/api/v1/partner/v1/auth/logout` revokes the current cookie's session,
  expires the cookie with matching scope/flags, and succeeds for absent cookies
  or an already-revoked session. Invalid/unknown credentials receive 401, not a
  false revocation confirmation. It no longer requires a valid access token. Database failure does not
  falsely report revocation; the frontend reports unconfirmed logout.
- Normal dashboard calls retain Authorization Bearer authentication.

## Configuration and deployment

| Variable | Default / restriction |
| --- | --- |
| PARTNER_ACCESS_TTL_MINUTES | 15, service caps at 15 minutes; replaces PARTNER_JWT_TTL_HOURS |
| PARTNER_REFRESH_TTL_HOURS | 720; rolling server-side expiry and cookie TTL |
| PARTNER_AUTH_COOKIE_NAME | karrygo_partner_refresh |
| PARTNER_AUTH_COOKIE_SAMESITE | lax; strict/none supported |
| PARTNER_AUTH_COOKIE_SECURE | true outside development/test; insecure production forbidden |
| PARTNER_AUTH_WEB_ORIGINS | Exact HTTPS origins required in production; development/test defaults localhost:3000 |
| PARTNER_JWT_SECRET | Existing authority secret; at least 32 bytes in production |

HttpOnly is always true. Domain is never set. Path is fixed to
`/api/v1/partner/v1/auth`. Deletion uses Max-Age=0 and a past Expires (Go's
negative MaxAge emits deletion). Origin comparison is exact; missing, null or
untrusted Origin returns 403. OPTIONS allows trusted origins, credentials and
Authorization; never wildcard ACAO. Only Partner auth bypasses the legacy global
CORS middleware; unrelated Customer/provider/API-key routes keep their policy.

Same-site HTTPS hosting is preferred with Lax. A genuinely cross-site web host
requires explicitly configured None+Secure and is still subject to browser
third-party-cookie restrictions. Configure the real deployed origin; no hostname
was guessed. Local HTTP requires APP_ENV=development/test. Secure settings do not
replace infrastructure TLS. Redact Cookie/Set-Cookie at proxies/APM as well.

Apply new migration `010_partner_refresh_cookie.sql` before the new Gateway.
It adds nullable hashed refresh credentials to the existing session table and a
unique partial index/check; prior migrations are untouched. Existing sessions
have no refresh credential and must sign in again to establish the cookie.
Old access JWTs remain valid only until their original expiry/revocation; no bulk
revocation or secret rotation is performed by deployment. Rollback removes only
the new column/index/check, but reverting browser security is not recommended.

Gateway SHA-256 stores only the digest of 256-bit opaque refresh secrets. The
cookie is `session-uuid.random-secret.authentication-proof`. The proof is a
domain-separated HMAC-SHA256 using the existing Partner signing authority. It
authenticates the stable session/secret correlation for revocation; an identifier
alone cannot revoke a session. A retired authentic credential can revoke its
session, but cannot refresh it. No plaintext secret or additional history store
is persisted. This correction requires no migration beyond the existing 010.

Rotation is one conditional PostgreSQL UPDATE matching stable ID, old secret
digest, account, expiry and non-revocation. Logout updates that same ID regardless
of the current digest. Both updates serialize on the same row: refresh-first is
then revoked; logout-first prevents rotation. Repeated logout preserves the
original revocation timestamp; a missing row is not treated as confirmed logout.
The bearer row has the refresh expiry; JWT exp still independently
enforces the short access lifetime. Gateway rejects stale hashes rather than
implementing Fleet's distinct token-family compromise policy.

Cookies issued by the earlier uncommitted 43-character implementation are not
accepted by this corrected format; affected browsers must sign in again. There
is no unsigned or JSON fallback. Changing the existing signing key also
invalidates these authentication proofs; coordinate deployment as for access JWTs.

## Frontend and compatibility

partner-web auth store is memory-only. Root client bootstrap removes the old
`karrygo-partner-auth` storage entry without reading it and restores using refresh.
No credentials are passed through server rendering. Concurrent refreshes within
one runtime are single-flight; API requests retry once; delayed old-token 401s
reuse the new token. Logout waits for pending refresh before server revocation.
Separate browser tabs can still race rotation and require reauthentication (fail
closed); no cross-tab credential broadcast or persistence is introduced.

Affected callers: partner-web auth-service, api-client, store, root bootstrap,
sidebar/settings logout; Gateway legacy registration/password and OTP HTTP
clients; Fleet corporate verify/refresh/organization-selection/logout clients and
tests. No verified Fleet native refresh consumer was found. Native Customer,
Taxi, Dispatch and Truck auth contracts are unchanged. Provider membership
read-back uses the separately documented corrected Fleet internal path.

Fleet browser APIs independently use `karrygo_fleet_refresh`, Path
`/fleet-partner/v1/auth`, and their existing locked HMAC-hash family rotation.
No refresh token passes from Fleet through Gateway JSON because this browser
flow has no Fleet call at all. Do not describe tests as a Gateway->Fleet flow.

## Verification

Backend HTTP tests exercise real handlers/usecases with in-memory repositories;
frontend Node tests execute actual TS client/store code with simulated transport.
Fleet DB tests use only FLEET_PARTNER_TEST_DATABASE_URL. Gateway's new isolated
repository race/migration test requires explicitly provisioned
API_GATEWAY_TEST_DATABASE_URL (never production API_GATEWAY_DATABASE_URL).
Unset test DB variables mean BLOCKED_BY_TEST_INFRASTRUCTURE, not PostgreSQL PASS.
Production HTTPS browser/CORS/cookie delivery, live OTP and multi-instance DB
rotation must be verified in the configured deployment before rollout.
