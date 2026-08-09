# Quotations (Section 14)

## Pricing is manually supplied

There is **no rate card, no formula, no marketplace fare, no surge, no
commission calculation, no tax rate and no currency conversion** anywhere in the
quotation flow. Every number on a quotation was typed in by somebody who is
accountable for it.

An operator's commercial terms are theirs. A hidden calculation would silently
change what a customer was quoted, and the operator would find out when the
customer disputed the invoice.

The backend performs exactly one arithmetic operation: it sums the explicit line
amounts. `models.QuotationTotal` refuses a line whose currency differs from the
quotation's and refuses an overflow.

### Line item arithmetic

`lineAmountMinor` is optional on input. Where it is supplied it must equal the
exact decimal product of `unitAmountMinor` and `quantity`; a mismatch is
**refused rather than quietly corrected**, because a quotation whose lines do
not add up is a dispute and the caller needs to know which number is wrong.

Quantity is a **decimal string**, not a float. A quantity of 0.1 tonnes stays
0.1. `models.LineAmountFor` computes the product as an integer numerator over
the quantity's scale, so no intermediate value is fractional, and rounds half-up
at the final step only. The rounding convention is stated rather than inherited
from the float unit: 1.005 × 100 must round the same way on every platform and
in every test, and `float64` does not guarantee that.

## State machine

```
DRAFT             → AWAITING_APPROVAL, APPROVED, CANCELLED
AWAITING_APPROVAL → APPROVED, DRAFT (rejected for changes), CANCELLED
APPROVED          → ISSUED, CANCELLED
ISSUED            → VIEWED, ACCEPTED, REJECTED, EXPIRED, CANCELLED, SUPERSEDED
VIEWED            → ACCEPTED, REJECTED, EXPIRED, CANCELLED, SUPERSEDED
ACCEPTED · REJECTED · EXPIRED · CANCELLED · SUPERSEDED → (terminal)
```

An approval rejected *for changes* returns the quotation to `DRAFT` rather than
killing it: the commercial intent survives the review.

Only a `DRAFT` is editable, and that is enforced **in SQL** as well as in the
use case. An issued quotation is a promise already made; no edit path may reach
it, including one that raced past an in-memory status check.

## Approval

Submission routes the quotation through the existing **WP9 approval framework**.
There is no CRM-specific approval table, engine or decision record — a second
framework would be a second, weaker set of separation-of-duties rules.

The approval is bound to the quotation ID, the quotation's row version, the
**content fingerprint** and the framework's own policy version. The fingerprint
folds in every line item field in stable sequence order, so reordering,
repricing or adding a line all produce a different fingerprint. Any later edit
therefore invalidates the request rather than letting a reviewed cheap version
become an unreviewed expensive one.

At issue the approval is **re-checked against the exact payload that was
approved**. What was approved is what is issued.

Where the policy raises no request — because the value does not warrant one —
the quotation moves straight to `APPROVED`. It is never left `AWAITING_APPROVAL`
with nobody able to decide it.

## Issue

`POST /fleet-partner/v1/quotations/{quotationId}/issue` requires
`crm.quotations.issue`, an `Idempotency-Key` and an `expectedVersion`.

The order of operations is the point:

1. **Media is re-read before the transaction opens.** A projection written when
   the quotation was approved is a memory of what Media said then; a document
   revoked since must block the issue. An external call is deliberately not made
   inside an open transaction, which would hold a database connection hostage to
   somebody else's outage.
2. **The transaction** locks the quotation, re-checks the approval, re-checks the
   customer's status, risk and credit hold, writes the immutable issued version,
   mints the customer token digest, moves the status, and records history, audit
   and outbox atomically.
3. **Notification is not in that transaction.** A notification outage must not
   roll back a commitment already made to a customer. A durable intent survives
   and the worker keeps trying.

### Documents fail closed

Issue is refused when any *mandatory* quotation document is not confirmed both
`READY` and safe. `UNKNOWN` counts as not ready: a document whose safety could
not be confirmed is treated exactly like one confirmed unsafe, because the
alternative is sending a customer a file nobody has checked.

Where media-file-service is unconfigured, every projection is `UNKNOWN`, so a
quotation with a mandatory document can never be issued in that deployment. That
is the intended behaviour, and `TestCRMQuotationIssueFailsClosedOnUnreadyDocument`
asserts it.

Fleet stores an opaque Media object ID, a purpose and a readiness projection. It
stores **no bytes, no URL, no storage path, no upload token, no checksum and no
authoritative MIME type** — those belong to Media, and a second copy here would
be a second answer that can go stale exactly when a document's safety matters.

## Immutable issued versions

Every issue writes a `fleet_partner_quotation_versions` row carrying the customer
snapshot, the contact snapshot, the line items, the totals, the validity window,
the document references, the approval reference and a content fingerprint.

The table carries a `BEFORE UPDATE OR DELETE` trigger that refuses both. No
later edit, migration or repair script can restate what a customer was offered.
A correction produces a **new version or a superseding quotation**; it never
rewrites the original.

The customer snapshot deliberately records the legal name and **not** the tax or
registration reference. An immutable version is written once and never redacted,
so anything put in it is there forever, and a versioned copy of somebody's
identity documents is a liability nothing in this flow needs.

The contact snapshot records the contact ID and role, not the address. Delivery
resolves the address from the live contact record under that contact's *current*
consent, so a customer who withdraws consent is not still reachable through a
frozen snapshot.

## Acceptance

See [quotation-customer-access.md](quotation-customer-access.md) for the secure
link. Acceptance methods:

- `SECURE_CUSTOMER_TOKEN` — the customer clicked accept
- `AUTHORIZED_COMPANY_USER` — staff recorded it
- `APPROVED_EXTERNAL_INTEGRATION`
- `DOCUMENTED_OFFLINE_ACCEPTANCE` — requires an evidence reference or an actor
  reference; "the customer said yes on the phone" with nothing attached is an
  assertion, not evidence

Both the customer link and the staff record converge on **one** acceptance path,
so there is exactly one place that decides what an acceptance means and no second
implementation to drift.

Rules:

- The **exact issued version** must be named. An acceptance of "the current
  version" is an acceptance of something nobody has agreed is the same thing.
- Server time decides expiry. A client clock is the one thing a party with an
  interest in a late acceptance controls.
- A staff-recorded acceptance is blocked by a credit hold; a customer clicking a
  link they were already sent is not. The offer was already made, and revoking it
  after the fact is a supersession, not a silent refusal.
- Acceptance is immutable and idempotent. A replay returns the original; a retry
  with different content conflicts.
- The IP address and user agent are stored as **keyed digests only**, and are
  never returned by any API. An acceptance record is evidence that somebody
  accepted, not a log of where they were sitting.

## Conversion intent — not an order

Acceptance creates exactly one `fleet_partner_quotation_conversion_intents` row:

```
conversion_type = FUTURE_PRIVATE_ORDER
status          = PENDING_SECTION_15
```

It mints **no order ID, no shipment ID, no package ID, no assignment ID, no
reservation and no canonical trip**, and it moves no money — not as omissions,
but because none of those things exist yet, and inventing a placeholder for them
would be a lie that downstream consumers would act on.

The unique index on `accepted_version_id` guarantees exactly one intent per
accepted version under concurrency.

`organization.quotation.accepted.v1` carries the intent ID and its
`PENDING_SECTION_15` status. It never implies an order exists.

## Cancellation and supersession

Cancellation and supersession both preserve the original in full — its issued
version, its history and its documents stay exactly as they were. What changes
is that the customer's access tokens are **revoked**: a customer holding a link
to a withdrawn offer must not be able to accept it, and revoking the link is the
only thing that actually stops them.

## Expiry

`QUOTATION_EXPIRY_WORKER_ENABLED` (default false) drives the sweep. See
[crm-workers.md](crm-workers.md). Server time is the authority throughout; a
client timestamp never expires a quotation.

The worker never expires an `ACCEPTED`, `REJECTED`, `CANCELLED` or `SUPERSEDED`
quotation: the claim query excludes them and the transition re-checks the status
under a row lock, so an acceptance landing between the claim and the write wins.

## Routes

| Method | Path | Permission |
| --- | --- | --- |
| GET | `/quotations` | `crm.quotations.read` |
| POST | `/quotations` | `crm.quotations.manage` |
| GET | `/quotations/{id}` | `crm.quotations.read` |
| PATCH | `/quotations/{id}` | `crm.quotations.manage` |
| GET | `/quotations/{id}/history` | `crm.quotations.read` |
| GET | `/quotations/{id}/versions` | `crm.quotations.read` |
| GET | `/quotations/{id}/versions/{versionId}` | `crm.quotations.read` |
| GET | `/quotations/{id}/documents` | `crm.quotations.read` |
| POST | `/quotations/{id}/documents` | `crm.quotations.manage` |
| POST | `/quotations/{id}/submit` | `crm.quotations.submit` |
| POST | `/quotations/{id}/issue` | `crm.quotations.issue` |
| POST | `/quotations/{id}/accept` | `crm.quotations.accept` |
| POST | `/quotations/{id}/reject` | `crm.quotations.accept` |
| POST | `/quotations/{id}/cancel` | `crm.quotations.manage` |
| POST | `/quotations/{id}/supersede` | `crm.quotations.manage` |

`internalNotes` is absent from the standard quotation view and appears only in
`SensitiveQuotationView`, behind `crm.sensitive.read`. Somebody who may read a
quotation is not thereby entitled to the sales team's private commentary on the
customer.

The list view omits line items: a page of quotations is a summary, and shipping
every line of every offer widens the blast radius of one over-broad read.
