# Audit — Authentication, Onboarding, Staff IAM and Managed Providers (Sections 8–10)

Performed before any Section 8–10 code was written.

Date: 2026-07-25
Branch: `staging` · Commit: `7320aaaeb3bb7427b1ffb15c003bc6389d549c1f`
Starting `git status --short`: `M go.work`, `?? services/fleet-partner-service/`
(the Sections 3–7 foundation, uncommitted).

---

## 1. Confirmed facts

### 1.1 Existing fleet-partner-service structure (Sections 3–7)

Migrations `000001_platform_foundation` and `000002_organizations_foundation`
are applied, providing: `fleet_partner_outbox`, `_inbox`, `_jobs`,
`_idempotency_keys`, `_audit_records`, `_file_references`, `_organizations`,
`_branches`, `_departments`, `_organization_settings`,
`_organization_status_history`, plus `fleet_partner_schema_migrations`.
Section 8–10 migrations therefore start at **`000003`** and are purely
additive; no applied migration is rewritten.

Existing repositories: `OrganizationRepository` (create, get, `UpdateStatus`
with compare-and-swap + append-only history), `BranchRepository`,
`DepartmentRepository`, `SettingsRepository`. Existing platform packages:
`ids`, `money`, `respond`, `cursor`, `filters`, `concurrency`, `idempotency`,
`audit`, `tenant`, `sourceref`, `cache`; plus `events` (outbox/inbox),
`jobs` (leases), `observability`, `platformclients`.

### 1.2 `shared/go/auth` already provides the approved OTP primitives

This is the most important finding — **the platform OTP standard the
specification asks for already exists and is generic**:

| Helper | Behaviour |
| --- | --- |
| `GenerateNumericOTP(length)` | `crypto/rand`, rejects length > 10 |
| `HashOTP(secret, challengeID, phone, otp)` | `HMAC-SHA256(pepper, "otp"‖challengeID‖phone‖otp)` — a **keyed digest**, exactly the construction §5.2 requires instead of a bare SHA-256 |
| `VerifyOTP(...)` | `hmac.Equal` → constant time |
| `GenerateOpaqueToken(n)` | `crypto/rand` base64url — used for refresh and invitation tokens |
| `HashRefreshToken(secret, token)` | `HMAC-SHA256(secret, "refresh"‖token)` |

**Decision:** reuse these rather than re-implementing locally. They are
stable, generic, domain-free and already used by `customer-service` and
`driver-hauling-service`. Fleet-partner-specific concerns (challenge
persistence, purposes, session families, organization binding) stay local.

`shared/go/auth` also has `TokenSigner` (HS256, claims
`{sub, role, service, session_id, typ, iat, exp}`) and `BearerMiddleware`,
already wired into the corporate route class in Sections 3–7.

### 1.3 Notification-service contract exists and is usable

`shared/go/notifications.Client.Send(ctx, Request)` posts a signed request to
`/api/v1/notifications/send` with `idempotency_key`, `source_service`,
`event_type`, `Recipient{Type,ID,Email,Phone}`, `channels`, `template_key`,
`locale`, `title`, `body`, `data`, `template_data`, `priority`. `ChannelEmail`
= `"email"` exists. Recipient types are currently `customer` and `provider`
only — corporate recipients are a **new** recipient type this work introduces
in its own request payload; the client itself needs no change.

**OTP delivery therefore has a real transport.** Delivery failures are
surfaced (the client returns the envelope error), so a genuine
`503 otp_delivery_unavailable` is implementable without faking success.

### 1.4 Provider services are ALREADY passwordless — no change required

| Service | Auth feature | Routes |
| --- | --- | --- |
| `driver-dispatch-delivery-service` | `features/auth` | `/api/v1/auth/signup/start`, `/login/start`, `/start`, `/verify`, `/refresh`, `/logout`, `/phone-change/*` |
| `driver-taxi-service` | `features/auth` | equivalent |
| `driver-hauling-service` | `features/provider_auth` | equivalent, already uses `HashOTP` |

Their start DTOs already carry **both** `phone_number` and `email`, and
verification is `identifier` + `otp_code`. A repository-wide grep for
password fields in provider auth returns **only SMTP infrastructure
credentials** (`clients/email_client.go` — `smtp.PlainAuth`), which §1.1 of
the specification explicitly permits and which the architecture test must not
flag.

**Correction to the task's expectation:** §1.3 anticipated that a provider
service might need "the smallest backward-compatible change" to support
passwordless email+phone OTP. **None is needed** — the standard is already
met. No provider service source file was modified by this work package.

### 1.5 Missing cross-service contracts (the real gap)

Grepping every authoritative service's route surface:

| Needed capability | Owning service | Exists today? |
| --- | --- | --- |
| Resolve provider by normalized email **and** phone | dispatch / taxi / hauling | **No** |
| Provision organization-sponsored provider identity | dispatch / taxi / hauling | **No** |
| Create / accept / decline organization association | dispatch / taxi / hauling | **No** |
| Read provider platform status; revoke org eligibility | dispatch / taxi / hauling | **No** |
| Ensure organization wallet | payment-wallet-service | **No** (only payment intents, refunds, provider earnings, bank accounts under `/internal`) |
| Ensure provider sub-wallet | payment-wallet-service | **No** |
| Submit organization verification / read status | verification-compliance-service | **No** — the service only has `identity_checks`, `license_checks`, `provider_verification`, `vehicle_documents`; there is no organization-level verification surface and no HTTP feature routes |
| File metadata / ownership / readiness | media-file-service | **Partial** — `/api/v1/media-files`, `/files/:id`, `/uploads` exist; the ownership+readiness assertion contract is not defined |

**Consequence, per the standing instruction "do not invent missing live
endpoints":** every one of these is implemented here as a **typed client
interface + documented contract + explicitly unconfigured adapter** that
returns `ErrNotConfigured`, plus durable reconciliation jobs. No endpoint
path is fabricated as if it were live, and no authoritative service's
database is read as a fallback.

### 1.6 Other confirmed facts

- **Request/correlation IDs**: `httpx.RequestID` + local `middleware.CorrelationID`, already propagating.
- **Service auth**: `shared/go/serviceauth` HMAC (`X-Service-Name`/`-Timestamp`/`-Signature`), already used on internal routes.
- **Existing owner/company identity**: **none anywhere** — corporate identity is entirely new.
- **Existing role/permission infrastructure**: **none anywhere** — new.
- **Event transport**: still no broker; the outbox `DisabledPublisher` remains, so Section 8–10 events accumulate durably rather than being lost.
- **Integration-test harness**: `internal/integration` gated on `FLEET_PARTNER_TEST_DATABASE_URL` with a test-name safety guard and per-test truncation; `scripts/ci.sh` provisions a disposable container and fails rather than skips.
- **`phonenumber.NormalizeNigerianPhoneNumber`** exists but is Nigeria-only; corporate identity needs general E.164 normalization, implemented locally (`internal/platform/contact`) and **not** promoted to `shared/go` since only this service needs it so far.

---

## 2. Decisions taken from this audit

1. Reuse `shared/go/auth` OTP/token primitives; keep challenge persistence,
   purposes and session families local.
2. Reuse `shared/go/notifications` as the real OTP transport; add a local
   `OTPSender` port with a **fake adapter for tests only**, and make an
   unconfigured notification client a hard `503`, never a silent success.
3. No provider-service code changes (§1.4).
4. All missing authoritative contracts become typed unconfigured adapters
   with reconciliation (§1.5), documented in `docs/service-contracts.md`.
5. Migrations continue from `000003`.

## 3. Gaps carried forward (deferred, documented)

- Live provider resolve/provision/association endpoints.
- Live organization wallet and provider sub-wallet provisioning endpoints.
- Live organization verification submission endpoint.
- Event-broker transport for the outbox.

Each has a typed contract, an unconfigured adapter, a reconciliation job and
a runbook, so enabling them later is configuration plus one adapter
implementation — no domain rewrite.
