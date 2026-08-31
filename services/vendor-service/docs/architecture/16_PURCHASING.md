# 16 — Suppliers, Purchasing and Goods Receiving (Section 15)

Section 15 gives the Vendor Service a purchasing record: who supplies what, at
what price, on what terms; what was ordered and who approved it; what actually
arrived and what was wrong with it; and what it really cost.

It does not give the service a second stock system, and it does not give it a
payment system. Both boundaries are load-bearing and are stated first.

## Two boundaries

**Generic Inventory (Section 14) is the only stock authority.** Purchasing
never writes a balance, a lot or a movement of its own. It hands the accepted
quantity to Inventory and Inventory decides what that means. A purchase order
is a promise about stock; only a posted goods receipt is stock.

**Purchasing records cost. It never moves money.** Receiving a delivery creates
no wallet transaction, no ledger entry, no payout and no payment intent.
`vendor_purchase_cost_history` is commercial evidence of what was bought at
what price — the thing an accountant reconciles against later — and is not
itself a financial settlement. Settlement is a separate authority that does not
exist yet.

## A. Supplier model

`vendor_suppliers` holds one supplier per business: a code unique within the
business, names, default lead time and status. Contacts and terms hang off it.
Suppliers are archived, never deleted, because purchase orders snapshot them.

## B. SupplierProduct model

`vendor_supplier_products` is the mapping that makes purchasing possible: it
says "this supplier's SKU, bought in this purchase unit, is this Inventory
item". It is branch-scoped, because the same supplier may serve two branches on
different terms.

## C. Exact purchase-unit conversion

A supplier sells cases; Inventory counts units. The mapping carries
`inventory_units_per_purchase_unit` and a conversion mode:

- `SAME_AS_INVENTORY` — one purchase unit is one inventory unit.
- `EXACT_INTEGER_MULTIPLE` — one purchase unit is exactly *n* inventory units.

There is no approximate mode, deliberately. An approximate multiplier would
make every received quantity slightly wrong, and stock that is slightly wrong
is worse than stock that is missing, because nobody notices.

## D. Inventory item mapping

The supplier product names an `inventory_item_id` in the same business and
branch. A composite foreign key enforces that; a supplier product cannot point
at another vendor's shelves.

## E. Purchase order lifecycle

```
DRAFT → SUBMITTED → APPROVED → ORDERED → PARTIALLY_RECEIVED → RECEIVED → CLOSED
          ↓             ↓
       REJECTED     CANCELLED
```

Orders snapshot the supplier and the approval policy at submission, so later
edits to either cannot rewrite what was approved.

## F. Approval policies

`vendor_purchase_approval_policies` and `..._rules` configure who may approve
what. A rule matches an amount band, optionally for one branch, and states:

- how many approvals are required,
- which permission an approver must hold,
- whether approval may come from the branch or must come from the business,
- whether step-up authentication is required.

## G. Threshold examples

A business might configure:

| Band | Required permission | Approvals |
|---|---|---|
| ₦0 – ₦100,000 | `vendor.purchasing.approve` | 1 |
| ₦100,000 – ₦1,000,000 | `vendor.purchasing.approve.business` | 1 |
| above ₦1,000,000 | `vendor.purchasing.approve.high_value` | 2 |

These are examples. The system reads the vendor's own configured bands; nothing
is hardcoded, and overlapping bands are rejected when the policy is saved.

## H. Branch-specific approvals

A rule with a branch wins over a business-wide rule for the same amount. This
lets a large branch approve locally while a small one escalates.

## I. Self-approval

`allow_self_approval` decides whether the person who submitted an order may
also approve it. With it off, the submitter's approval is refused even if they
hold the permission.

## J. Step-up approvals

A rule may require step-up. The approval consumes a fresh step-up grant; a
grant already spent cannot be reused.

## K. Historical snapshots

Supplier name, code, unit costs, conversion factors and the approval policy are
all snapshotted onto the order. Changing a supplier product tomorrow does not
change what was approved today.

## L. Goods receipt lifecycle

```
DRAFT → INSPECTED → POSTED → (REVERSED)
   ↓
CANCELLED
```

Only `POSTED` touches Inventory. A draft receipt is paperwork.

## M. Inspection outcomes

Each line records what physically happened:

```
delivered = accepted + damaged + rejected
short     = ordered − delivered   (when the supplier sent less)
```

Only **accepted** reaches Inventory.

## N–P. Multiple receipts, partial delivery, over-delivery

A purchase order may be received across several receipts. It becomes
`PARTIALLY_RECEIVED` while anything is outstanding and `RECEIVED` only when the
last unit arrives.

Over-delivery follows the branch's receiving policy: `DENY` refuses the receipt
atomically, `ALLOW_WITH_TOLERANCE` accepts up to a configured tolerance.
Accepted over-delivery never advances purchase-order progress beyond the
approved ordered quantity.

## Q. Wrong items

A line flagged `wrong_item` never posts to the ordered item's Inventory. It
raises a `WRONG_ITEM` discrepancy: the supplier sent something else, and
guessing which item they meant would put unknown stock on a shelf.

## R. Quality issues

A line may be accepted physically but flagged for quality, raising a
`QUALITY_ISSUE` discrepancy without blocking the receipt.

## S. Immutable purchase cost history

`vendor_purchase_cost_history` is append-only. The database refuses `UPDATE`
and `DELETE`. A reversal appends reversal evidence rather than erasing the
original observation, because "we thought we paid this, then we didn't" is
itself a fact worth keeping.

## T. Inventory posting boundary

On confirmation, purchasing calls generic Inventory once per receipt with the
accepted quantities. Inventory creates its own receipt, lots and immutable
movements. **Every posted line creates a lot**, whatever the item's tracking
mode: Inventory allocates reservations and adjustments out of lots, so a
purchasing receipt that wrote only a balance row would leave stock that read as
available but could not be reserved, adjusted or sold.

## U. Exactly-once receiving

A unique index on `inventory_receipts (business_id, source_ref)` for
`PURCHASING:%` sources means one goods receipt can produce at most one
Inventory receipt. Concurrent confirmations of the same receipt post once; the
losers are refused. Cost evidence is likewise unique per receipt line.

## V. Reversal restrictions

A posted receipt can be reversed only while its stock is still attributable:
untouched, unreserved, unconsumed, not transferred, not quarantined, and the
purchase order not closed. Reversal appends inverse Inventory movements and
reversal cost evidence; it never deletes the originals.

## W. Discrepancies

`SHORT_DELIVERY`, `OVER_DELIVERY`, `DAMAGED`, `REJECTED`, `WRONG_ITEM`,
`QUALITY_ISSUE`, `COST_VARIANCE`, each moving through `OPEN` →
`APPROVAL_REQUIRED` → `APPROVED`/`REJECTED` → `RESOLVED`, or `VOIDED`.

### Cost variance

A supplier invoices ₦54,000 against an approved ₦50,000. The system:

- leaves the approved purchase order at ₦50,000,
- raises a `COST_VARIANCE` discrepancy recording expected, actual and variance,
- applies the configured approval requirement,
- records ₦54,000 as immutable purchase-cost evidence — what was actually paid,
- **creates no wallet transaction of any kind.**

Approving a cost variance approves the variance. It does not rewrite the order.

## X. Supplier performance

Derived from purchasing evidence at read time. There is no score.

The rule that shapes the whole design: **absence of evidence is never reported
as good performance.** A supplier nobody has received from does not have a 100%
on-time rate; it has no on-time rate. Every ratio carries its own denominator
and a `metric_status` of `OK` or `INSUFFICIENT_HISTORY`, and its value is null
in the second case. Rates need at least three observations before they are
reported at all, because a single delivery is an anecdote.

## Y. Workers

Two durable jobs on the existing coordinator — no second scheduler, queue,
lease or retry system:

- `purchasing-delivery-alerts` (5 min) raises expected-soon, expected-today,
  overdue and materially-overdue alerts, and resolves alerts whose order has
  since been received or cancelled.
- `purchasing-discrepancy-escalation` (15 min) raises and escalates unresolved
  discrepancies without ever mutating the discrepancy itself.

Both are idempotent through a unique `(business_id, dedupe_key)` on
`vendor_purchasing_alerts`: repeated and concurrent sweeps conflict rather than
duplicating.

Overdue applies only to genuinely outstanding delivery — `ORDERED` and
`PARTIALLY_RECEIVED`. A `DRAFT`, `REJECTED`, `CANCELLED`, `RECEIVED` or
`CLOSED` order is never overdue however old its expected date.

## Z. IAM and security

Eleven permissions, deny by default, authorized through the existing evaluator
and never by role name. Operational reads are separated from
`vendor.purchasing.read` because a dashboard aggregating a whole business is a
wider disclosure than reading one purchase order:

`read`, `manage`, `approve`, `approve.business`, `approve.high_value`,
`receive`, `discrepancies.manage`, `cost.read`, `cost_variance.approve`,
`performance.read`, `dashboard.read`.

Tenant comes from the authenticated session and branch from the caller's scope;
neither can be asserted by a request body. A branch filter is intersected with
the caller's scope, and asking for a branch the caller does not hold is refused
rather than silently narrowed.

## AA. Audit and outbox

High-value mutations write audit records and domain events through the existing
outbox in the same transaction as the mutation, so an event cannot survive a
rolled-back change.

## AB. No wallet mutation on stock receipt

Stated once more because it is the boundary most likely to erode: receiving
stock creates no wallet transaction, ledger entry, payout, payment intent or
transfer. A certification gate greps the purchasing source for wallet and
payment client references and fails if one appears.
