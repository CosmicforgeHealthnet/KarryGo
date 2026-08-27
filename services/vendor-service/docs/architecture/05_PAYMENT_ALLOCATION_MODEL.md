# 05 — Payment Allocation Model

## Authority

`payment-wallet-service` remains the platform's ONLY financial ledger authority. Vendor Service never maintains a competing ledger — `internal/features/finance_projection` is explicitly a read model only (see its `doc.go`). This is a restatement of an already-established, unambiguous platform rule (CLAUDE.md, "Payment/Wallet Rules"), not a new decision.

## Feasibility, confirmed against the actual schema

`payment-wallet-service`'s `payment_intents.source_service` (and `ledger_transactions.source_service`, `finance_journals.source_service`) is a free-text column with no CHECK constraint and no enum/FK (`migrations/001_wallet_ledger.sql:51-72`, `migrations/000004_finance_foundation.up.sql:72`). **Introducing `"vendor-service"` as a new source requires zero schema migration on `payment-wallet-service`.** The only real gate is authentication: `vendor-service` must be added to `PAYMENT_WALLET_SERVICE_SECRETS` (and, if using the finance-private API, `PAYMENT_WALLET_FINANCE_ALLOWED_CALLERS`) — both env-var config changes, tracked in [PLATFORM_SERVICE_CHANGE_REGISTER.md](../PLATFORM_SERVICE_CHANGE_REGISTER.md). This directly confirms the specification's design is implementable without redesigning `payment-wallet-service`.

## One checkout, many allocations

A customer makes ONE checkout payment. Vendor Service computes the allocation breakdown and records each allocation as its own attributable entry via `payment-wallet-service`'s existing intent/ledger primitives — never as a single lump transfer a vendor must self-apportion.

```text
Customer payment
    |
    +-- Vendor A merchandise allocation   (source_reference: vendor A's child order)
    +-- Vendor B merchandise allocation   (source_reference: vendor B's child order)
    +-- Delivery Mission A allocation     (source_reference: fulfilment unit A's mission)
    +-- Delivery Mission B allocation     (source_reference: fulfilment unit B's mission)
    +-- Platform allocation               (commission / commercial share)
    +-- Promotion/subsidy allocations     (where applicable — see below)
```

**Strict rule, restated as an invariant this model must never violate:** Vendor A receives only money belonging to Vendor A. Vendor B never receives Vendor A's funds. No vendor ever receives the gross customer checkout amount. Every allocation is independently attributable and reconcilable back to the checkout, the vendor order, and (for delivery) the mission that earned it.

## Platform delivery vs. vendor-owned delivery

**Platform delivery** (dispatch/taxi/hauling execution):

```text
Customer
  |
  +--> Vendor merchandise payable          (Vendor A's cut only)
  +--> Platform delivery/provider allocation (paid to the executing provider via the
  |                                           owning service's own settlement path —
  |                                           Vendor Service does not settle providers
  |                                           directly)
  +--> Platform commercial allocation
```

**Vendor-owned delivery** (`internal/features/delivery`):

```text
Customer
  |
  +--> Vendor merchandise payable
  +--> Vendor-owned delivery allocation     (paid to the SAME vendor, additively)
  +--> Platform commercial allocation
```

The distinguishing rule: platform delivery money never lands on the vendor's payable; vendor-owned delivery money does, because the vendor is the one who performed the delivery. `internal/features/fulfilment` (which decides platform vs. vendor-owned execution per fulfilment unit) and `internal/features/finance_projection` (which reads the resulting allocations back) must agree on which case applies per unit — this is a single boolean decision recorded on the fulfilment unit, not inferred separately by each reader.

## Multi-vendor checkout

For a checkout spanning multiple vendors, each vendor's own child order (`internal/features/orders`) carries its own allocation, computed independently. A rejection or substitution on one vendor's order (`internal/features/substitutions`) adjusts only that vendor's allocation and its associated delivery allocation — it must never touch another vendor's already-settled allocation.

## Delivery promotion funding

Free or discounted delivery must retain funding attribution through settlement, computed by `internal/features/promotions` and recorded as part of the allocation breakdown, never silently absorbed by whichever party happens to be easiest to deduct from:

- **Customer-funded**: customer pays the full delivery allocation as normal.
- **Vendor-funded**: the vendor's own promotion configuration reduces the vendor's merchandise payable by the subsidized amount; the delivery allocation itself is still paid in full to whoever executed it (platform provider or the vendor, per the case above).
- **Platform-funded**: the platform's own promotion absorbs the delivery allocation (or part of it) from its own commercial allocation; it is never silently deducted from the vendor's payable.
- **Combined**: customer pays part, vendor subsidizes part, platform subsidizes part — each part is its own attributable allocation line, summing to the full delivery cost.

The actual delivery cost and the identity of who ultimately funded each part of it must remain traceable end to end (checkout → allocation → settlement), so a reconciliation report can always answer "who actually paid for this delivery" without re-deriving it from business rules after the fact.

## What this phase does not do

No allocation is computed, no payment intent is created, and no checkout exists yet. This document fixes the model every later commerce-checkout and fulfilment-settlement section must implement against, and confirms the model is buildable on `payment-wallet-service` as it exists today without requiring that service to be redesigned.

## Implemented settlement and refund boundary

Later Commerce implementation now realizes this model with one Wallet intent,
immutable VendorOrder allocations and allocation-local release/refund
operations. Vendor Service stores versioned policy evidence only. Wallet owns
all ledger movement and cumulative caps; Dispatch/Hauling own platform-provider
completion and identity; Inventory owns exact AUTO_REFUND stock release; and
Support owns ambiguous adjudication. Refund recovery reads frozen snapshots and
Wallet operation state, never current prices or caller-supplied amounts.
