# 20 — Order Orchestration (Section 17E)

This document fixes the ParentOrder / VendorOrder model. It is the reference
every later commerce section (payment execution, mission execution,
substitutions, settlement) must implement against.

Section 17E creates orders. It **does not finish payment execution**, move
money, or execute a logistics mission. Those are later sections' work, and the
composition root deliberately wires no Wallet, payment-intent or mission
dependency (`cmd/section17e.go`).

## The shape

```text
ParentOrder                       one customer commerce purchase
    |
    +-- VendorOrder A             one financially and operationally isolated
    +-- VendorOrder B             vendor/branch allocation each
    +-- VendorOrder C
```

A ParentOrder is cut from exactly one approved, unexpired Section 17D quote.
A VendorOrder is cut from exactly one quote group (one vendor/branch
allocation of that quote).

## Source of truth

Every commercial figure on an order is copied from the frozen quote. Nothing in
Section 17E recomputes a historical price from the live catalogue — that is
what makes a receipt readable years later, and it is asserted directly by
`TestSection17EOrders_HistoricalSnapshotSurvivesCatalogueChange`.

The ParentOrder preserves immutable references and snapshots:

| Evidence | Source |
|---|---|
| customer | quote |
| receiver | cart, read under the exact cart version the quote binds (see below) |
| delivery address | quote address snapshot |
| quote / quote fingerprint / quote version | quote |
| currency, fulfilment mode | quote |
| customer-approved total, max authorized total | quote |
| product / delivery / fee / tax / discount / subsidy subtotals | quote |
| funding breakdown, price disclosure | quote |
| payment operation reference | **empty**; bound by a later section |

**The receiver is a deliberate exception worth naming.** Section 17D does not
fingerprint the cart receiver, so an order cannot read it from the quote. It is
read from `commerce_cart_receivers` joined on `commerce_carts.version =
quote.cart_version`. A receiver change bumps that version, and a cart in
checkout is not mutable at all, so this can never snapshot a contact the
customer did not approve. If the version has moved, no receiver is attached.

**Delivery authority comes from the quote's own frozen mission snapshot**
(`commerce_quote_missions.snapshot->>'authority'`), never from the live
fulfilment plan row. Delivery responsibility decides who gets paid, so it has
to be the evidence the customer approved.

## Multi-vendor financial isolation

One payment is not one undifferentiated vendor balance. The allocation engine
(`internal/features/orders/allocation.go`) is a pure function from the approved
quote to per-vendor allocations, and it refuses rather than guessing.

Two quantities are computed per VendorOrder and kept distinct:

- **`customer_payable_minor`** — how much of the customer's approved total this
  allocation accounts for (money *in*).
- **`vendor_payable_minor`** — what the vendor is owed for it (money *out*),
  which becomes the prepared settlement allocation on completion.

```text
customer_payable = product_subtotal
                 − all product-scope benefit on this allocation
                 + delivery_customer (after delivery-scope benefit)
                 − order-scope benefit allocated to this allocation

vendor_payable   = product_subtotal
                 − product-scope benefit this vendor funded
                 − order-scope benefit this vendor funded
                 − delivery benefit this vendor funded
                 + delivery_gross   (only when the vendor executed the delivery)
```

The structural guard: `Σ customer_payable + platform fee/tax` must equal the
customer-approved total exactly. If it does not, `ComputeAllocation` returns
`ErrAllocationImbalance` and **no order is created at all**.

### Attribution rules

| Benefit | Rule |
|---|---|
| Delivery funding per group | Already attributed by Section 17C. Carried through unchanged. |
| Vendor-funded discount, single-vendor quote | Attributed to that vendor. |
| Vendor-funded discount, multi-vendor quote | **Refused** (`ErrNotAttributable`). Section 17D's adjustment carries no vendor identity, and splitting it would make vendor B pay for vendor A's promotion. |
| Platform / other-approved / delivery-subsidy discount | Spread by largest remainder. It costs no vendor anything, so no vendor is harmed by the split. |
| Fee and tax | Parent-scope platform money. Never charged against a vendor. |

The refusal is the important one. It is a deliberate fail-closed choice: a
promotion that cannot name its funding vendor must be fixed upstream (Section
17D must carry vendor attribution on such adjustments) rather than silently
cross-subsidised here.

### The allocation register

`commerce_order_allocations` records every part of the approved total as its own
attributable row: allocation type, which VendorOrder it belongs to, who benefits
(`VENDOR` / `PLATFORM_PROVIDER` / `PLATFORM` / `CUSTOMER`), who funded it, and
the amount. "Who actually paid for this delivery" is answerable by reading rows,
never by re-deriving business rules afterwards.

Delivery money never lands on a vendor payable unless the vendor performed the
delivery — enforced both by `beneficiaryFor` in code and by
`commerce_vendor_orders_beneficiary_chk` in the schema.

## Stock isolation

Each VendorOrder carries its own Section 14 reservation reference and source
reference. Order creation re-verifies each group's reservation is still held; a
released or failed reservation refuses the **whole** creation rather than
producing an order nobody can honour. One VendorOrder failing therefore cannot
corrupt another vendor's stock, because nothing was written at all.

## Fulfilment isolation

Each VendorOrder runs its own lifecycle:

```text
CREATED → PREPARING → READY → HANDED_OVER → DELIVERED → COMPLETED
   └──────────────── any non-terminal ────────────────┘ → CANCELLED
```

`ApplyTransition` is pure and reads only the order's own state, so one vendor's
progress can never depend on, or disturb, another vendor's.

**Delivery confirmation is asymmetric on purpose.** A vendor may confirm
delivery only when it executed the delivery itself. A platform-executed delivery
is confirmed by the logistics authority that performed it; Section 17E has no
mission execution, so it returns `403` rather than letting a vendor assert a
delivery it cannot observe. Wiring the authority-driven confirmation is Section
17F's work.

A customer may withdraw from a single VendorOrder up to `PREPARING`. Once the
goods have left the vendor, withdrawing is no longer a cancellation and belongs
to a returns/dispute flow that does not exist yet.

## Aggregate state

The ParentOrder's state is always **derived** from its children, never written
from a caller's opinion, and it is recomputed on every transition inside the
same transaction with the parent row locked.

| Children | ParentOrder |
|---|---|
| all COMPLETED | COMPLETED |
| all CANCELLED | CANCELLED |
| all terminal, mixed | **PARTIALLY_COMPLETED** |
| all CREATED | CREATED |
| anything else | IN_PROGRESS |

A mixed outcome is never hidden behind either extreme.

## Prepared, not executed

Completion prepares `settlement_allocated_minor` (= that vendor's payable).
Cancellation prepares `refund_allocated_minor` (= what the customer paid for
that allocation). Parent-scope fee and tax become refundable only when the whole
purchase is cancelled: while any allocation still stands, the customer is still
buying something.

These are records of what a later payment-capable section will have to move.
Section 17E moves nothing, creates no payment intent, and never writes
`payment_operation_ref`.

## Idempotency and atomicity

- **Exactly-once** is a database constraint: `commerce_parent_orders.quote_id`
  is `UNIQUE`. One approved quote can only ever produce one ParentOrder. A retry
  returns the existing order (HTTP 200 rather than 201) and duplicates no order,
  vendor order, line or allocation.
- **Atomicity** is one PostgreSQL transaction. If the third VendorOrder cannot
  be written, nothing is: there is never a ParentOrder falsely claiming full
  success. A per-quote advisory lock makes concurrent creations agree on one
  winner instead of racing to the constraint.
- **Transitions** carry a version guard (`If-Match`) and lock the parent, so a
  stale client cannot overwrite a change it never saw and concurrent sibling
  transitions serialize on the aggregate they both re-derive.

## Immutability

Item snapshots, allocation rows and lifecycle events reject `UPDATE` and
`DELETE` at the database, by trigger, so a future writer cannot rewrite what a
customer was charged or which vendor earned what. The two aggregate rows are
deliberately **not** frozen: their status, prepared allocations and version are
exactly what a transition changes.

## Domains

Food carts are single-branch in Section 17A, so a Food ParentOrder normally has
one VendorOrder — but it uses the same model, with no special case anywhere in
the code. Shop may have many, grouped by actual vendor/branch allocation
(`quote_group_ref`), never merely by vendor name.

## Security

- Customer identity comes only from the Customer Service bearer token; every
  customer query is scoped by `customer_id` in the `WHERE` clause.
- Vendor identity and branch scope come only from the Vendor staff session,
  resolved server-side through the Section 5 permission evaluator. A request
  body or path can never assert them.
- A branch-scoped actor reading outside its branches gets `404`, so a probe
  cannot map which branches hold orders. A branch **filter** outside scope gets
  `403`, because an empty page would still confirm the branch exists.
- Section 17E adds no permission and no role mapping: `vendor.orders.read` and
  `vendor.orders.manage` were both seeded in Section 5.

## Section 17F: payment, stock finalization and execution

Section 17E's prepared allocations become real through Section 17F.

**Payment is bound, not trusted.** A payment operation carries the exact quote
id, version, fingerprint, totals, currency, customer and allocation
fingerprint. A materially different input is a different binding and is refused
under the same operation, so a quote swap, an amount tamper or a re-allocation
cannot ride an existing payment.

**One leg per VendorOrder.** The Wallet contract settles and refunds an intent
as a whole, to a single provider, so a single parent intent could never refund
one vendor while settling another. The split is what makes the multi-vendor
refund isolation described above actually reachable.

**Order-first.** The order exists before any money is asked for, so every leg
names a real VendorOrder. The remaining window - money held for an order whose
legs did not all answer - is durable and reconciled by asking the money
authority, never by guessing.

**Stock timing is explicit:** HELD → CONFIRMED (payment held) → COMMITTED
(completion) or RELEASED (cancellation), recorded per allocation so it is never
inferred and never double-committed.

**Execution.** One logistics mission per Section 17C plan mission, created for
the fee the customer already paid; an authority that answers with a different
fee is refused. Vendor-owned delivery gets no platform mission. A CAR mission
may execute only through the false-by-default, distinct Taxi Goods authority;
it never uses passenger or direct-parcel booking APIs.

**Commercial changes.** A customer address or service-level change creates a
new Section 17D quote and an append-only chain from the original ParentOrder;
the old quote is never edited. The replacement quote must be explicitly
approved before a positive delta is held by Wallet. A cheaper fulfilment result
uses the customer-facing delivery commitment as its benefit basis and issues
one deterministic partial refund through Wallet. Terminal VendorOrders call
Wallet `CompleteJob` or `RequestRefund` only for their own payment leg, with
durable adjustment evidence and recovery after an unknown outcome.

## What Section 17E does not do

Section 17E itself does not execute payment or logistics; Section 17F performs
those effects through the external authorities described above. There is no
cart or checkout terminal transition (an order exists before payment, and
cancelling it must not strand a cart), and no admin operator surface.
