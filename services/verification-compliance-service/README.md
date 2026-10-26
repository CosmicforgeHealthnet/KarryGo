# Verification & Compliance Service

## Section 7 contract correction

Vendor verification uses exactly `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`; plain `PENDING` is rejected as a Vendor case status. Outbox `delivery_status=PENDING` is an unrelated worker state. Signed Vendor callbacks target the direct-service root route `POST /internal/vendor/v1/verification-events`, not the accidental `/api/v1/vendor/internal/vendor/v1/verification-events` composition.

Owns formal verification decisions, source versions, decision history, and
compliance review workflows. Vendor Service consumes projections; it cannot
approve or set a verification result.

## Vendor verification cases (Section 7)

The additive Vendor contract is mounted at
`/api/v1/verification-compliance/internal/vendor-cases` and requires HMAC v2,
an allowlisted service identity, and a database-claimed single-use nonce.

| Method | Path | Caller | Purpose |
| --- | --- | --- | --- |
| `POST` | `/cases` | `vendor-service` | Idempotently create a case in `PENDING_REVIEW` |
| `GET` | `/cases/by-client-reference/:clientReference` | Vendor/Admin | Authoritative unknown-outcome read-back |
| `GET` | `/cases/:caseRef` | Vendor/Admin | Read by opaque authority reference |
| `POST` | `/cases/:caseRef/decisions` | `admin-backoffice-service` only | Version-guarded formal decision |

Dimensions are exactly `IDENTITY`, `BUSINESS`, `BANK`, `LOCATION`, `BRAND`,
and `FOOD_KITCHEN`. Statuses are exactly `NOT_STARTED`, `IN_PROGRESS`,
`PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, and `SUSPENDED`.
Vendor case and history tables are migration `000002_vendor_verification_cases`.

Decisions append `vendor.verification.status_changed` to the transactional
outbox. When `VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_URL` and
`VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_SECRET` are configured together, a
leased worker signs the callback as `verification-compliance-service`, retries
with bounded exponential backoff, dead-letters after eight attempts, and
supports operator replay in code. Vendor independently polls by client
reference until a terminal result, so either delivery path repairs the other.
The machine-readable contract is `docs/openapi/vendor-cases.openapi.json`.


## Secure Admin (Section 18)

Verification is the sole authority for a vendor case's formal status. Automation
assists; a named reviewer decides; HQ and Vendor observe.

**One state machine.** `internal/features/vendors/casestate.go` is the only
definition of legal moves, used by the signed decision route, the Secure Admin
command executor and the expiry sweep:
`NOT_STARTED→IN_PROGRESS|PENDING_REVIEW`, `PENDING_REVIEW→IN_PROGRESS|VERIFIED|REJECTED`,
`IN_PROGRESS→PENDING_REVIEW|VERIFIED|REJECTED`, `VERIFIED→SUSPENDED|EXPIRED`,
`REJECTED|EXPIRED|SUSPENDED→PENDING_REVIEW` only with an appeal reference.
`VERIFIED` needs a future `expires_at`; `REJECTED`/`SUSPENDED` need an upper-case
token `reason_code`. Every decision (`vendors.ApplyDecision`) locks the case,
fences on `source_version`, writes the append-only history naming the decider
(`decided_by` + `decided_by_kind` STAFF/SERVICE/SYSTEM), records the appeal on a
reopen and queues the existing signed Vendor callback (`verification_outbox`,
retry, dead letter, `Replay`), all in one transaction.

The decision route now returns `404 CASE_NOT_FOUND`, `409 ILLEGAL_TRANSITION`,
`409 VERSION_CONFLICT`, `409 IDEMPOTENCY_CONFLICT` (same key, different case or
body) and `400` validation codes. It accepts optional `appeal_ref` and
`decided_by_staff_ref`. `cmd/vendor-review approve` now requires an expiry date.

**Automated pre-verification** (`VERIFICATION_COMPLIANCE_AUTOMATION_ENABLED`,
default off). A leased, bounded worker evaluates each open case version once and
records immutable findings (`vendor_case_automation_findings`) plus an advisory
`automation_recommendation`. It never changes status or version.

| Check | Meaning |
| --- | --- |
| `DOCUMENT_REFS_PRESENT`, `DOCUMENT_REF_FORMAT` | shape only |
| `DUPLICATE_DOCUMENT_ACROSS_BUSINESSES` | same document cited for another business: dual-source mismatch, `FAIL`/`BLOCKING` |
| `MEDIA_OBJECT_STATE` | missing/revoked/inactive/expired Media object `FAIL`; Media not configured or unreachable `NOT_EVALUATED` |
| `EXTERNAL_AUTHORITY` | no registry/KYC provider is contracted; `DisabledAuthority` is the only implementation, so `SOURCE_UNVERIFIED` |
| `ADDRESS_CONSISTENCY` | always `NOT_EVALUATED`: Verification holds no address data for vendor cases |

Recommendation: any `FAIL` → `REVIEW_REQUIRED`; otherwise external unavailable →
`SOURCE_UNVERIFIED`. `AUTO_ELIGIBLE` would need an external `VERIFIED` and is
unreachable in this build. `VERIFICATION_COMPLIANCE_EXTERNAL_AUTHORITY` accepts
only empty or `disabled`; config refuses anything else.

**Expiry sweep** (`VERIFICATION_COMPLIANCE_CASE_EXPIRY_ENABLED`, default off):
`VERIFIED` cases past `expires_at` become `EXPIRED` (decider `system:expiry`,
reason `COMPLIANCE_LAPSE`) with the normal Vendor callback, so an expired
`FOOD_KITCHEN` compliance stops counting in Vendor.

**Owner boundary** (prefix `VERIFICATION_COMPLIANCE`, everything default off):
`_ADMIN_RESOURCE_READ_ENABLED` / `_ADMIN_CONTEXT_PUBLISH_ENABLED` serve
`/internal/verification/v1/admin/resources/{vendor-cases,vendor-case-findings,asset-capability-cases,organization-submissions}`
(document references, subject text, remediation text and identity data are never
projected; findings are codes only). `_ADMIN_COMMAND_WRITES_ENABLED` /
`_ADMIN_COMMAND_READBACK_ENABLED` serve `/internal/verification/v1/admin/commands`
for `START_REVIEW`, `REQUEST_REMEDIATION` (native grant `verification.case.review`)
and `APPROVE_CASE`, `REJECT_CASE`, `SUSPEND_CASE`, `REOPEN_CASE` (grant
`verification.case.decide`, independent approval required). Read kinds need the
native grants `verification.vendor_cases.read`, `verification.findings.read`,
`verification.asset_cases.read`, `verification.organization_submissions.read` in
`verification_admin_staff_grants`. Nothing is seeded: no grant, policy, key or
approval.

**Admin events.** A trigger appends `verification.vendor_case.changed.v1`
(entity `vendor_verification_case`, version = `source_version`, `business_ref` =
`business_id` only when it is an opaque token) to `admin_event_outbox` in the
same transaction as every case insert or exported-field update; the relay runs
when `VERIFICATION_COMPLIANCE_ADMIN_EVENTS_ENABLED=true` (with
`_ADMIN_EVENTS_BRIDGE_ORIGIN` and `_ADMIN_EVENTS_SECRET_FILE`). Automation-only
recommendation changes are emitted without a version bump (formal versions are
reserved for decisions so automation can never invalidate a reviewer's fenced
command); the Bridge picks them up at the next version or from the snapshot.
Backfill: `GET /internal/verification/v1/admin-events/snapshot?cursor=&limit=`
(≤200), HMAC v2 from `admin-backoffice-service` only, durable nonce, flag
`VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_ENABLED` and dedicated secret file
`VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE`.

Tests: real-PostgreSQL suites serialize on one advisory lock (`internal/testdb`),
so `go test ./...` is safe with `VERIFICATION_COMPLIANCE_TEST_DATABASE_URL` set.
