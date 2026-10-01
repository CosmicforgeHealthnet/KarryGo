# Sections 40-42 Traceability

This record closes the security/privacy, idempotency/recovery, and data-model
audit against the repository at migration `000115`. It supplements the focused
Section 40 record. No public or internal HTTP route was added.

## Closure matrix

| Requirement | Before | After | Evidence |
|---|---|---|---|
| Object authorization and IDOR | PASS | PASS | established tenant/branch/customer scoped stores and Section 40 security suites |
| HMAC v2 and durable replay rejection | PASS | PASS | shared `serviceauth`; Vendor `servicenonce`; Section 40 HMAC tests |
| Secret-safe durable failures | PARTIAL | PASS | `events.RedactError`; `TestRedactErrorRemovesCredentials` |
| Field encryption boundary | PASS | PASS | no new searchable contact secret or foreign credential stored; authority-owned opaque refs remain refs |
| Customer/receiver minimization | PARTIAL | PASS | migration `000116`; `privacy.RetentionWorker`; operational snapshot reads |
| Legal/financial evidence preservation | PASS | PASS | canonical order snapshots remain unchanged; restricted evidence is `ON DELETE RESTRICT` |
| Legal holds | MISSING | PASS | `vendor_data_legal_holds`; retention integration test |
| Media, Wallet, Customer, provider authorities | PASS | PASS | references/projections only; no authority moved |
| Pharmacy exclusion/restricted goods | PASS | PASS | canonical restricted-goods policy and Section 40 tests |
| Semantic request fingerprint | PARTIAL | PASS | canonical JSON fingerprint unit test |
| Concurrent command claim | PARTIAL | PASS | conflict-safe insert and atomic expired reclaim; live PostgreSQL test |
| Tenant-isolated replay and restart replay | PASS | PASS | live PostgreSQL idempotency test |
| Inbox dedupe with business transaction | PARTIAL | PASS | `InboxRepository.ClaimTx`; rollback/replay integration test |
| Inbox payload identity integrity | PARTIAL | PASS | fingerprint mismatch rejection integration test |
| Transactional outbox, retries, leases, dead letter | PASS | PASS | existing `events.OutboxRepository` and worker suites |
| Stock/capacity/order/promotion/challenge concurrency | PASS | PASS | existing live PostgreSQL domain concurrency suites |
| Unknown remote outcome/read-back | PASS | PASS | existing canonical operation refs and authority read-back adapters |
| Retention kill switch | MISSING | PASS | `VENDOR_PRIVACY_RETENTION_ENABLED`; disabled means no new claims and no state loss |
| Additive migration and rollback policy | MISSING | PASS | `000116` up/down; down refuses after executed minimization |
| Advanced commerce persistence | PASS | PASS | existing optimization proposals, recurring baskets, RFQ revisions/deposits, corporate recurrence |

## Idempotency and event rules

`vendor_idempotency_keys` remains unique by scope, endpoint, and key. Migration
`000116` adds actor, operation, optional resource, completion, and update
metadata. JSON fingerprints bind normalized method, exact path, and canonical
content, so member order and whitespace do not change command identity. A
conflict-safe insert works both on a pool and inside a transaction. Expired
claims use one conditional `UPDATE ... WHERE expires_at <= now() RETURNING`, so
only one caller can reclaim a key.

Inbox claims use `INSERT ... ON CONFLICT DO NOTHING`, avoiding an aborted
PostgreSQL transaction on duplicates. `ClaimTx` accepts the consumer's
transaction and optional canonical payload digest. Rolling back the business
mutation therefore also rolls back the claim; reusing an event ID for a
different type or fingerprint is an integrity error.

Existing outbox mutation/append atomicity, `SKIP LOCKED` claims, bounded retry,
lease recovery, dead-letter retention, canonical remote operation references,
and read-back reconciliation are reused. Durable error text now removes bearer
credentials, OTP/passcodes, card PAN/CVV, passwords, API/HMAC/service/gateway
secrets and private keys before persistence.

## Retention and authority boundaries

The active `ORDER_RECEIVER_LOCATION` policy defaults to a versioned 90-day
support window and is data-driven in `vendor_privacy_retention_policies`.
Terminal transitions enqueue a durable job. Pre-migration terminal orders are
discovered in bounded batches by the worker, avoiding a blocking migration
backfill. Each job runs in a short `FOR UPDATE SKIP LOCKED` transaction and is
restartable/idempotent. Active parent-order, customer, or business legal holds
defer work. Failures use bounded exponential backoff and a durable
`DEAD_LETTERED` state.

On completion, exact canonical order-time evidence stays in
`commerce_parent_orders` and is copied to the restricted-evidence boundary.
Only `operational_*` snapshots are minimized. Normal order and handover reads
use the operational projection; commerce-change overlays are suppressed after
minimization. The worker adds append-only audit evidence. Disabling
`VENDOR_PRIVACY_RETENTION_ENABLED` stops claims without deleting or pretending
to complete pending work.

The down migration is safe before any minimization executes. After execution
it explicitly refuses rollback because removing the operational boundary would
re-expose retained canonical PII. Forward recovery is required in that case.

## Section 42 domain audit

The logical identity/IAM, business, verification projection, CRM, Food/Pastry,
Shop, inventory, purchasing, recipe/production, Commerce, fulfilment, delivery
policy, promotions, RFQ/group, and operational groups all have repository
equivalents in migrations `000001`-`000115`; names differ where later sections
split aggregates more precisely. Verification keeps authoritative refs,
versions, expiry and badge projections. Quotes keep fingerprint/version,
approved and maximum totals, fulfilment mode and funding snapshots. Shop and
orders retain variable-weight, quality/preference, substitution decision, and
ETA histories without taking reusable Customer preference authority.
Fulfilment supports FASTEST/BALANCED/SAVER and versioned candidate reasoning.
Analytics stores privacy-safe projections outside Vendor money truth.

Budget optimization proposals, recurring templates/generations, RFQ quote
revisions/deposit references, and corporate recurrence already exist. Order
Guarantee remains deliberately future-disabled; no speculative compensation
or Wallet logic was added.

## Explicit exclusions

- Pharmacy/medicine commerce is outside Vendor Service scope and remains
  rejected by the restricted-goods boundary.
- Payment Wallet remains money/ledger authority.
- Media File Service remains media-byte and signed-access authority.
- Customer Service remains reusable preference authority.
- Dispatch, Hauling, Taxi Goods, and VendorMission authorities retain physical
  execution and OTP/QR authority.
- Sections 33-35 were reused, Section 37 remains intentionally skipped, and no
  Section 43+, frontend, new route, commit, or push is part of this change.

## Certification executed

- Clean PostgreSQL replay of all 116 migrations, latest rollback, and reapply:
  PASS.
- Upgrade from the pre-`000116` schema with representative existing
  idempotency data plus critical constraint/index/trigger checks: PASS.
- Live PostgreSQL idempotency concurrency, tenant isolation, restart replay,
  expired reclaim, inbox rollback/replay/fingerprint integrity, and
  legal-hold/minimization/evidence/audit tests: PASS.
- Vendor isolated runner: all 66 test-bearing packages PASS. The first run
  exposed obsolete 30-second harness deadlines in two packages; after sizing
  those bounded integration deadlines for the 116-migration schema, both
  packages passed in fresh isolated clones.
- `go test ./...`, `go vet ./...`, `go build ./...`, 48 OpenAPI JSON parses,
  changed-file `gofmt`, and `git diff --check`: PASS.
