# Vendor Order Lifecycle — Final Remediation Certification

## Scope

This remediation continues the existing dirty tree and stops at Vendor Order
Lifecycle. Section 19 was not started. Historical migration `000084` was not
edited.

## Implemented

- Wallet-held payment now durably advances each child through `PAID` and
  `SENT_TO_VENDOR`, with row locking, monotonic retries, lifecycle evidence and
  transactional outbox publication.
- Platform logistics handover/pickup/delivery commands are forbidden to vendor
  staff; Vendor Own Delivery remains supported.
- Cancellation is pre-handover only and is guarded by the existing OCC version.
- Fulfilment activation, recovery queues and mission planning exclude rejected,
  cancelled, refund-pending/refunded and other terminal non-fulfillable orders.
- Shop substitutions resolve active same-business/branch catalogue listings;
  request-supplied price, identity, metadata, quantity and snapshot are not
  authoritative.
- Refund outcomes persist the Wallet-confirmed amount, support expected-version
  OCC, are idempotent on duplicate authority results, and re-derive parent
  refund states including `REFUND_PENDING`.
- Inventory partial finalisation is implemented in the Inventory authority with
  per-allocation committed/released evidence, lot/balance locking and retry
  safety (`000086_inventory_partial_finalization`).
- Fresh databases now accept the explicit `AUTHORITY` and `VENDOR` lifecycle
  event actors emitted by payment and vendor operations
  (`000087_vendor_order_event_actor_types`); historical migrations remain
  unchanged.
- Root HMAC-v2 Dispatch/Hauling ingress validates the canonical caller,
  single-use nonce, mission relationship, source version and event dedupe.
- Material order, picking, substitution, refund and logistics mutations record
  lifecycle evidence and outbox envelopes in their state transaction.
- Existing vendor idempotency middleware covers lifecycle commands and
  substitution proposals; customer command routes retain customer-scoped
  idempotency.
- Capability-named lifecycle tests cover parent-state derivation, refund action
  vocabulary and logistics caller allowlisting.
- OpenAPI and API route register include the root internal logistics ingress;
  production route contract is 769 routes.

## Test-infrastructure closure evidence

The five Section 17A tests previously fell through from
`VENDOR_SERVICE_TEST_DATABASE_URL` to `VENDOR_DATABASE_URL` and finally a
production-style default at `localhost:5445`. The certification helper now
requires the canonical test DSN, rejects non-local/system/unrecognised
databases, and skips cleanly when that DSN is intentionally absent. The
canonical `tools/vendor-test-runner` supplies a freshly migrated isolated
database per package; no fixed service listener is used.

- Five previously blocked Section 17A tests: **PASS 5/5** against real
  PostgreSQL (`localhost:55439`, canonical isolated test infrastructure).
- Full isolated Vendor regression: **PASS 54/54 packages**.
- `TestMigrationReplay`: **PASS**, apply → rollback latest → reapply through
  `000087_vendor_order_event_actor_types`.
- Lifecycle smoke gates (payment handoff, platform handover restriction,
  Vendor Own Delivery, cancellation guard, rejected fulfilment exclusion,
  authoritative substitutions and reapproval, refund OCC/derivation, partial
  Inventory finalisation, Dispatch/Hauling HMAC ingress and dedupe/order
  protection, multi-Vendor isolation, transactional outbox): **PASS** in the
  isolated regression.
- OpenAPI/live-router/API route-register parity: **PASS**; derived route count
  remains 769.
- `go vet ./services/vendor-service/...`: **PASS**.
- `go build ./services/vendor-service/...`: **PASS**.
- `gofmt` verification and `git diff --check`: **PASS**.

## Defects found and fixed

- Section 17A's PostgreSQL helper selected `localhost:5445` through an unsafe
  production-DSN fallback. It now consumes only the canonical test DSN and
  fails closed for non-test targets.
- Fresh migration replay rejected legitimate `AUTHORITY` and `VENDOR` order
  event actors. Additive migration `000087` expands only that constraint;
  historical migrations were not rewritten.
- A few Section 17E/17F integration expectations still encoded the pre-
  remediation platform-handover and post-payment transition flow. They now
  assert the enforced platform restriction and explicit `PREPARING` step.

## Result

**VENDOR ORDER LIFECYCLE RESULT: GO**

**VENDOR ORDER LIFECYCLE CERTIFIED COMPLETE**

Branch: `staging`  
HEAD: `f6fc70bb630d7735bbd05c14679ba4910fde85c7`  
Migration tip: `000087_vendor_order_event_actor_types`  
Route count: `769`
