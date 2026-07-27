# Passwordless Authentication

```text
KarryGo corporate authentication has no user password.
Email and phone are required.
OTP is delivered to email only.
Managed-provider authentication remains in the owning provider service.
No temporary provider password is sent.
```

## What does not exist

There is no password signup, password login, password hash, password reset,
temporary password, generated password, "change password on first login" or
security question anywhere in corporate authentication. No API accepts
`password`, `passwordConfirmation`, `oldPassword`, `newPassword`,
`temporaryPassword` or `passwordHash`. No table has a password column.

Two architecture tests enforce this on every build
(`internal/architecture_auth_test.go`): `TestNoPasswordAuthentication` scans
all Go sources and migrations for password-credential tokens, and
`TestNoPasswordLoginRoute` fails if a password-style login route appears.

Infrastructure credentials are a different thing entirely and remain
permitted: database URLs, SMTP logins and service HMAC secrets live in secure
environment configuration and are deliberately not matched by those scanners.

## The flow

```text
User enters email AND phone
        |
POST /fleet-partner/v1/auth/start
        |
pair normalized (email lowercased, phone E.164)
        |
6-digit OTP generated with crypto/rand
        |
notification-service sends the OTP to the EMAIL address
        |
User enters the code
        |
POST /fleet-partner/v1/auth/verify
        |
session issued (access token + rotating refresh token)
```

Both fields are mandatory on every start. The phone is part of identity
matching and account security; the code itself is only ever emailed. **No SMS
OTP is sent in this release** — `TestNoSMSOTPDelivery` fails the build if an
SMS channel is requested from the auth, staff or provider features.

The same flow serves new owner registration, existing owner login, staff
login, staff invitation acceptance, organization selection and step-up.

## OTP security

| Property | Implementation |
| --- | --- |
| Generation | `crypto/rand` via `shared/go/auth.GenerateNumericOTP` |
| Length | 6 digits (configurable 6–10) |
| Storage | **Never raw.** `code_digest` = `HMAC-SHA256(pepper, "otp"‖challengeID‖phone‖code)` |
| Comparison | `hmac.Equal` — constant time |
| Expiry | 10 minutes (bounded 1m–1h by config validation) |
| Attempts | 5, then the challenge locks |
| Resend | 60s cooldown; a new code supersedes the previous one immediately |
| Binding | challenge ID + normalized phone + purpose |
| Single use | consumed atomically under `FOR UPDATE` |

A bare SHA-256 of a six-digit code is trivially reversible, so the digest is
**keyed** with `FLEET_PARTNER_OTP_HMAC_SECRET`. That pepper is mandatory in
every environment (startup fails without it), is never logged, never
returned, and never appears in a validation error.

Because an error return rolls its transaction back, the failed-attempt
counter is written **outside** the verification transaction — otherwise a
wrong code would never consume an attempt and the limit could never be
reached. The same reasoning applies to refresh-reuse containment.

## Delivery honesty

OTP delivery goes through `notification-service`. There is deliberately **no**
logging sender, console sender or in-memory sender selectable from
configuration — `cmd` only ever wires the real adapter. When delivery fails
or the transport is unconfigured, the API returns

```json
{"error":{"code":"otp_delivery_unavailable","message":"We could not send your verification code right now. Please try again shortly."}}
```

with HTTP 503, and the challenge records `delivery_status = UNAVAILABLE`. It
never returns 200 for an OTP that was not sent. Tests inject
`clients.FakeSender` directly; it cannot be reached from config.

## Enumeration safety

`POST /auth/start` returns the same shape, channel, expiry and cooldown
whether the account exists, does not exist, is suspended, or the email and
phone belong to two different identities. Conflicts and suspended attempts
are recorded as append-only security events (hashed contacts only, never raw
values), so operators can see them without the API disclosing anything.

## Identity pair rules

- Email comparison is case-insensitive (`email_normalized` is pre-lowered).
- Phone is stored E.164.
- Email and phone must resolve to the **same** identity. A half-match —
  a known email with a different phone, or vice versa — is
  `identity_pair_conflict`, never a silent takeover.
- `CONTACT_VERIFIED` means the emailed OTP was verified and the normalized
  phone was bound. It does not claim the phone was SMS-verified.

## Sessions and refresh tokens

Access tokens are short-lived HS256 tokens carrying subject, session ID, role
and service. Refresh tokens are opaque `crypto/rand` values; only their
HMAC hashes are stored.

Rotation retires the current session row (`REVOKED`, reason `rotated`) and
issues a successor in the **same family**. Keeping the spent hash is what
makes reuse detectable: replaying a spent token still finds a row, and
finding a non-`ACTIVE` row is the reuse signal. On reuse the entire family is
marked `COMPROMISED`, a critical security event is written, and the caller
gets `refresh_token_reused`.

Every refresh re-checks identity status, membership status and organization
status, so a suspension takes effect at the next rotation rather than
outliving it.

## Organization selection

After verification: no membership → `ONBOARDING_REQUIRED`; exactly one →
an organization-scoped session; several → `ORGANIZATION_SELECTION_REQUIRED`
plus the allowed organizations. `POST /auth/select-organization` accepts a
**membershipId** that must belong to the authenticated identity — an
arbitrary `organizationId` is never trusted.

## Step-up

Sensitive actions use the same emailed OTP, bound to the current identity,
session, organization and action class, with a short TTL. A grant cannot be
transferred to another session, organization or action category — each of
those four mismatches is covered by a test.

## Rate limiting

Bounded counters (Redis when configured, in-process otherwise) throttle
challenge creation, verification, resend, refresh, organization switching and
step-up, keyed by hashed email, hashed phone, IP and challenge. Redis is
never the source of truth: challenge status and attempt counts live in
PostgreSQL, and a Redis outage degrades to the durable per-challenge limit
rather than locking everyone out.
