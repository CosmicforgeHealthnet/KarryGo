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

