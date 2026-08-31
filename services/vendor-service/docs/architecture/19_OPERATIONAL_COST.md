# Section 16D - Operational cost, COGS and production variance

## What this is, and what it is deliberately not

Section 16D answers one operational question: what did it cost us to make this,
based on what we actually paid for the inputs.

It is **not** accounting. There is no ledger, no journal, no settlement, no
supplier payment, no tax evidence and no Wallet effect anywhere in the package.
An estimated COGS is not accounting truth, and a production variance is an
operational observation that never becomes an automatic financial adjustment,
a supplier adjustment, or a change to a selling price. Pricing is a separate
authority that does not exist on this platform yet.

The structural guard is a test: `TestFinancialBoundaryHasNoWalletOrLedgerSurface`
scans the package for wallet, ledger, journal, settlement and payment
vocabulary and fails if any appears.

## Authority boundaries

| Owner | What it owns | How Section 16D uses it |
|---|---|---|
| Section 15 | Purchase cost evidence (`vendor_purchase_cost_history`) | **Reads** through `PurchasingCostReader`. Never copied into a second history. |
| Section 16A | Ingredient and packaging master data, branch inventory mapping | A policy *references* this identity. Packaging is never modelled as an ingredient. |
| Section 16B | Recipes and recipe versions | Read only. Costing never mutates a recipe. |
| Section 16C | Production batches, frozen requirements, actual consumption | Read only. A batch is costed from its own frozen evidence. |
| Section 14 | Inventory | Untouched. No cost calculation moves stock. |

## The expected-waste rule

This is the single most likely defect in the section, so it is stated plainly.

Section 16B persists **NET** component quantities. The gross-up

```
gross = net * 10000 / (10000 - waste_bps)
```

is applied by Section 16C when it freezes a batch's planned requirements.
Therefore:

- **Recipe estimated COGS** reads NET quantities and applies the gross-up
  **exactly once**.
- **Production expected cost** reads frozen batch quantities that already
  include it and **must not** apply it again.

The two calculations sit on opposite sides of the same rule. Applying it twice
overstates every batch; applying it nowhere understates every recipe. Both
directions are pinned by `TestExpectedWasteAppliedOnceVersusTwice`, which
records the exact divergence (10000 versus 11111 on the reference case) so a
regression is unmistakable rather than merely plausible.

## Cost strategies

Three, all explicit and deterministic.

**`LATEST_ACCEPTED_PURCHASE_COST`** - the most recent accepted, unreversed
observation, ordered `observed_at DESC, id DESC`. The id tie-break is not
decoration: two receipts confirmed in one transaction share a `now()`
timestamp, and without a stable second key "latest" would vary between calls
and a snapshot would stop being reproducible.

**`WEIGHTED_AVERAGE_PURCHASE_COST`** - quantity-weighted over a **bounded**
window of the most recent accepted observations (policy-configurable, default
10). Weighting is by accepted inventory quantity, so a 100kg delivery moves the
average a hundred times more than a 1kg one. The bound exists so one cost read
can never become an unbounded scan of a vendor's purchasing history.

**`MANUAL_CONFIGURED_COST`** - a vendor-configured operational fallback. It is
consulted **only** when the active policy selects it; no other code path reads
it, so it cannot silently outrank real purchase evidence.

### Eligibility and REVERSAL

Eligibility is a property of Section 15's table rather than a filter reimplemented
here: an `OBSERVED` row is written only for a receipt line that was actually
accepted. Draft and cancelled purchase orders, unconfirmed deliveries, rejected
and damaged quantities and quotation-only data never produce a row at all.

A `REVERSAL` row carries the **same positive magnitudes** as the observation it
undoes - not a negative amount - and the table permits at most one of each per
receipt line. A reversal therefore **cancels** its observation rather than
subtracting from a running total, so netting is `NOT EXISTS`, not arithmetic.
Treating reversals as negative quantities would corrupt the weighted average;
ignoring them would leave reversed evidence in the cost basis.

## Completeness and currency

Absence of cost is never zero. When nothing resolves, the component records why
(`NO_PURCHASE_EVIDENCE`, `NO_POLICY`, `CURRENCY_MISMATCH`,
`NO_INVENTORY_MAPPING`, `NO_MANUAL_COST`, `UNIT_NOT_CONVERTIBLE`) and the
snapshot is `INCOMPLETE` or `UNKNOWN`. A genuine zero, if purchasing ever
recorded one, stays distinguishable from a missing one.

No FX exists on this platform, so none is invented. Evidence in a currency other
than the policy's is excluded and reported, never converted at a made-up rate.

## Exact money

Integer minor units with exact rational quantities, using `math/big`. No float.

Rounding is **half away from zero, applied exactly once, at the component
boundary**. Rounding intermediates would make a total depend on the order
components happened to be summed in, which would make a historical snapshot
irreproducible - the opposite of what this section exists to provide. Component
amounts are exact integers, so a snapshot total is their exact sum.

`math/big` rather than int64 is what makes the arithmetic overflow-safe: a large
quantity times a large minor-unit cost can exceed int64 in the intermediate
product even when the rounded result fits comfortably.

## Snapshots

`RECIPE_PREVIEW` is recalculable. `RECIPE_SNAPSHOT`, `PRODUCTION_EXPECTED` and
`PRODUCTION_ACTUAL` are immutable evidence: historical reporting must never
recalculate a past production run at today's purchase prices.

Immutability is enforced by database trigger as well as by the service, because
a service-only guarantee lasts exactly until the next handler is written by
someone who has not read this file.

Snapshots store **component-level evidence**, not just a total: the quantity
used, the unit cost applied, the strategy, the currency, the receipt the cost
came from, and the waste contribution. A figure from six months ago can be
re-derived line by line.

Two uniqueness constraints make retries safe: one `PRODUCTION_EXPECTED` and one
`PRODUCTION_ACTUAL` per batch ever, and one snapshot per command reference. A
conflicting insert returns the existing row rather than writing a competing
history, so a retried command and two concurrent finalizations both converge.

## IAM and cost privacy

| Permission | Held by |
|---|---|
| `vendor.cost.read` | Owner, GM, Finance, Branch Manager (branch-scoped), Analyst/Auditor, Procurement |
| `vendor.production.cost.read` | Owner, GM, Finance, Branch Manager (branch-scoped), Analyst/Auditor |
| `vendor.cost.policy.manage` (high risk) | Owner, GM, Finance |

`role_kitchen_pastry` holds **none** of them. Kitchen and pastry staff keep
every recipe and production permission Sections 16B/16C gave them and gain no
cost visibility - that separation is the point of this section's IAM, and it is
asserted directly rather than assumed.

Branch managers and analysts never hold policy management: a branch cannot
decide how the business values its inputs, and analysing cost is not changing
it.

## Workers

**None.** Section 16D adds no background worker. Every calculation is driven by
an explicit request, which is what keeps a cost figure attributable to a
command and an actor rather than appearing from a sweep.
