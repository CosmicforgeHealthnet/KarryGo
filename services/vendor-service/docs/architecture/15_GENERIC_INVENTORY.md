# 15 — Generic Inventory (Section 14)

Section 14 makes `internal/features/inventory` the Vendor Service's stock
authority. This document records the authority model, the invariants that were
certified against real PostgreSQL, and the boundaries Section 14 deliberately
does not cross.

## Authority model

```
                         VENDOR SERVICE
                              |
                  +-----------+-----------+
                  |                       |
                 SHOP                  INVENTORY
                  |                       |
          product / variant             authority
          listing / measure               |
                  |             +---------+---------+
                  +------------>| balances          |
                                | lots              |
                                | movements         |
                                | reservations      |
                                | transfers         |
                                | stocktakes        |
                                | quarantine        |
                                | reorder / alerts  |
                                | expiry            |
                                +---------+---------+
                                          |
                                   future consumers
                                          |
                              +-----------+-----------+
                              |                       |
                         SECTION 15              SECTION 16
                         Purchasing          Ingredients/Recipes
```

**Shop owns** products, variants, listings, sell units and catalogue
measurement configuration, plus the Shop-specific catalogue semantics that
Section 12 certified.

**Inventory owns** generic inventory item identity, stock basis, balances,
lots, movements, reservations, stocktakes, transfers, quarantine, reorder
rules, alerts, inventory revision and expiry-related state.

### The dependency runs one way

`internal/features/shop` imports `internal/features/inventory`. Inventory never
imports Shop. This is not a convention: `internal/features/inventory/architecture_test.go`
and the service-wide boundary test fail the build if it is violated.

Consequently Inventory did not absorb Shop's catalogue ownership, and the
Section 12B Shop inventory API was neither migrated nor retired. Section 14C is
a bridge — `UpsertShopStockBasisCompatibilityTx` synchronises a certified Shop
stock basis into generic inventory item identity, preserving the Shop stock
basis ID exactly — not a replacement.

## Stock identity

A generic inventory item is identified by the tuple

```
(item_type, source_ref, stock_measure_ref, stock_measure_owner_key, stock_basis_mode)
```

within a business and branch. `item_type` is currently activated for
`SHOP_VARIANT`; `INGREDIENT`, `PACKAGING` and `PRODUCTION_COMPONENT` exist
structurally for Sections 15 and 16 and carry no application logic yet.

This tuple is why a branch transfer refuses source and destination items that
do not match on it. Two items can name the same physical product and still
disagree about what one unit of quantity means — `SAME_UNIT` versus
`EXACT_CONVERSION` — and moving a quantity across that boundary would silently
change the amount of stock in the world.

`SHOP_VARIANT` rows are additionally validated by a database trigger: the
variant must exist, be active and belong to the same business, and the variant
must be actively listed in that branch. Direct persistence cannot link another
business's variant.

## Certified invariants

Each of these is enforced in the database or the service and proved by a
real-PostgreSQL test, not by inspection.

| Invariant | Where it lives |
|---|---|
| Movement ledger is append-only | `inventory_movements_immutable_trigger`, unconditional on UPDATE and DELETE |
| Physical stock changes only through an authorized mutation | `inventory_authorized_mutation_required()` on balances and lots |
| Available quantity never goes negative | `inventory_balance_availability`, `GREATEST(..., 0)` |
| Quarantined and expired stock is not sellable | availability view and sellability resolution |
| Reservations are all-or-nothing | `allocation_policy = 'ALL_OR_NOTHING'` check plus transactional allocation |
| Reservation idempotency | unique `(business_id, source_type, source_ref)` plus a 64-character request fingerprint |
| A stale stocktake cannot overwrite newer truth | expected-quantity snapshot compared at post time |
| Transfers preserve stock identity | `transferIdentitiesMatch` before any line is created |
| Tenant and branch isolation | every query is predicated on `business_id`, and `BranchScope.Covers` gates branch access |

The movement immutability trigger is deliberately unconditional. The balance
and lot guards ask whether the caller holds the authorized-mutation session
flag; the movement guard does not, because rewriting history is never an
authorized mutation.

## PostgreSQL is the only stock truth

Neither `internal/features/inventory` nor `internal/features/shop` references
Redis in any file. Redis serves caching and job coordination elsewhere in this
service, but no balance, lot, movement, reservation, stocktake, transfer,
quarantine or reorder decision reads it. Redis being down degrades unrelated
optimisation; it cannot make Inventory wrong.

## Concurrency

The reservation and operations paths take row locks in a deterministic order
and settle competing outcomes exactly once. The certified race matrix covers
reserve/reserve, commit/commit, commit/release, commit/expiry, release/expiry,
receipt/reservation, quarantine/reservation, adjustment/reservation,
stocktake/movement, transfer-dispatch/reservation and duplicate transfer
receipt. These races execute against real PostgreSQL; before Section 14I they
were pending a test database and had never actually run.

## Workers

Inventory workers use the existing `internal/jobs` ticker architecture and the
`vendor_job_controls` durable lease introduced by `000066`: a named job has one
active owner, heartbeats while it works, recovers when a lease expires, retries
within a bound, dead-letters past that bound, and can be replayed. Execution is
at-least-once; business effect is exactly-once, which is why duplicate expiry
or reconciliation passes cannot double-release stock.

Workers: reservation expiry, lot expiry, inventory alert generation, balance
reconciliation, backorder reconciliation, plus the retained Shop compatibility
and bulk-operation jobs.

The Vendor outbox is not a job queue and is not repurposed as one.

## Internal authority

`/internal/vendor/v1/inventory/...` carries stock-mutation authority on behalf
of another service. It requires HMAC v2, an explicit caller allowlist that
fails closed when empty, and a durably consumed single-use nonce. The Vendor
tenant is derived from the authenticated caller and the branch from the path; a
request body can never assert either. `internal/section14contract` fails if any
internal route is ever mounted under the public vendor base.

## Food seam

`MANUAL` Food availability is unaffected by Inventory. `INVENTORY_LINKED` fails
closed whenever authoritative recipe information is unavailable, reporting
`NOT_CONFIGURED`, `NOT_SUPPORTED` or `UNKNOWN`. There is no HYBRID mode.
`RecipeAvailabilityResolver` is a seam for Section 16, which owns recipe
authority; Section 14 does not fabricate availability from a missing resolver.

## Scale and query plans

Certified at 21,000 generic inventory items, 21,000 balances, 63,000 movements,
6,996 lots, 3,000 reservations, 3,000 reorder rules and 1,800 alerts across 4
businesses and 12 branches. All eleven hot paths — branch balances, item
availability, item branch lookup, reservation-by-source, FEFO lot allocation,
movement history by branch and by item, lot expiry sweep, reservation expiry
sweep, reorder scan and open alert scan — resolve through index scans in under
a millisecond. No index was added during Section 14I, because no plan justified
one.

## Boundaries Section 14 does not cross

- **Section 15** owns suppliers, supplier catalogues, purchase orders,
  procurement workflow and goods receiving as a business process. A reorder
  rule raises a restocking signal and holds an opaque `supplier_ref`; it never
  creates a purchase order.
- **Section 16** owns ingredients, recipes, bills of materials, yields, food
  costing and production execution.
- **Section 17 and later** own commerce orders, checkout, cart, payment,
  wallet and logistics.

Section 14 exposes the availability and reservation primitives those sections
will consume, and stops there.

## Observability

Inventory carries no log statements of its own. That is deliberate rather than
an omission: the operational evidence a diagnosis actually needs is durable and
queryable rather than scattered through a log stream.

- **What happened to stock**: `inventory_movements` is the audit trail, and it
  is immutable. Every row carries `business_id`, `branch_id`,
  `inventory_item_id`, `movement_type`, `source_type`, `source_ref`,
  `reason_code`, `actor_ref`, `occurred_at` and the revision it produced.
- **What is wrong right now**: `inventory_alerts`, including
  `BALANCE_MISMATCH` raised by the reconciliation worker and
  `TRANSFER_EXCEPTION` raised on transfer settlement problems.
- **What the workers are doing**: `vendor_job_controls` records lease owner,
  heartbeat, attempt count, dead-letter state and last error per named job.
  Worker failures additionally surface through `logging.Error("worker", ...)`
  with the bounded job name.

A useful consequence of having no log statements is that Inventory cannot leak
through them. No customer PII, payment detail, secret, HMAC material or raw
authorization header can reach a log line from this package, because there is
no line to reach.
