# Customer credit policy (Section 14)

## Policy, not accounting

A credit profile is an **organization's own policy** for a customer. It is not a
ledger and it is not an accounts-receivable record.

Fleet:

- owns **no** receivables ledger
- calculates **no** debt
- ages **no** invoices
- derives **no** available credit from transactions
- posts **no** journal

A second, wrong set of books is worse than no books, and the accounting system
that owns the real ones is not this service. `TestCRMSchemaGuarantees` scans the
live schema for receivable- and invoice-shaped tables, so a future migration that
adds one fails the suite rather than passing review.

What a credit profile holds is: a limit (integer minor units, nullable), a
currency, payment terms in days, a hold status, a hold reason, a snapshot of the
customer's risk status, and an approval reference.

## Hold statuses

```
NO_HOLD · REVIEW_REQUIRED · ON_HOLD · RESTRICTED
```

`ON_HOLD` and `RESTRICTED` are **blocking**. `REVIEW_REQUIRED` deliberately is
not: it is a flag for a human to look, not a decision that has been taken, and
treating "someone should check this" as "refuse" would make the state useless as
an early warning.

## What a blocking hold prevents

| Operation | Blocked |
| --- | --- |
| Quotation submit and issue | yes |
| Staff-recorded quotation acceptance | yes |
| Contract submit and activation | yes |
| Contract resume from suspension | yes |
| Contract renewal | yes |
| Customer acceptance through a secure link they already hold | **no** |

The last row is deliberate. The offer was already made; revoking it after the
fact is a **supersession**, not a silent refusal at the moment the customer
clicks accept.

A hold does **not** delete quotations or contracts, cancel marketplace bookings,
reverse wallet transactions, deduct provider funds, create Section 15 orders or
calculate receivables.

A customer with **no credit profile at all** is not blocked. An organization
that has not set a credit policy has not decided to refuse the customer, and
inventing a refusal on their behalf would be Fleet making a commercial decision
it does not own.

## Risk and credit stay consistent

Escalating a customer's risk to `RESTRICTED` forces their credit profile to
`RESTRICTED` **in the same transaction**. Leaving the two to disagree would mean
a risk decision that never reached the gate that actually blocks activation.

A new credit profile for an already-restricted customer is **created restricted**.
Otherwise deleting and recreating the profile would launder the risk decision
away.

A restricted customer's credit hold **cannot be released** while the restriction
stands: that would use the credit surface to undo a risk decision that has its
own, separately permissioned, workflow.

## Approvals

Both a **limit change** and a **hold release** are routed through the WP9
approval framework. Extending credit is a finance decision with a real exposure
attached; releasing a hold is the change that lets new commitments through.

The framework — not this feature — decides at what value a second pair of eyes
is required, and it enforces quorum, eligible roles, self-approval policy,
separation of duties and expiry. There is no CRM-specific approval table.

An approval that is required and not granted yields
`credit_change_approval_required`.

## History

Every limit change and every hold change appends an immutable
`fleet_partner_customer_credit_history` row, enforced append-only by trigger,
recording the previous and new hold status, the previous and new limit, the
reason code, the actor and any approval reference.

## Data minimization

`CreditProfileView` carries the currency, payment terms, hold status, whether
the hold is blocking, and **whether a limit exists** — not the limit itself.
Whether a customer is on hold is operationally relevant to anyone who may read
CRM; how much credit the organization extends them is a finance figure behind
`crm.credit.read`.

`organization.customer_credit.*` events carry the hold status and a `blocking`
flag. The **credit limit is absent** from every event: it is the operator's
commercial position, not something other services need.

## Routes

| Method | Path | Permission |
| --- | --- | --- |
| GET | `/customers/{id}/credit-profile` | `crm.credit.read` |
| PUT | `/customers/{id}/credit-profile` | `crm.credit.manage` |
| POST | `/customers/{id}/credit-profile/hold` | `crm.credit.manage` |
| POST | `/customers/{id}/credit-profile/release` | `crm.credit.manage` |
| GET | `/customers/{id}/credit-profile/history` | `crm.credit.read` |

A hold or release requires a reason code, an `expectedVersion` and an
`Idempotency-Key`. The expected *hold status* is also in the WHERE clause, so a
hold and a release racing each other resolve to one winner rather than to a
customer whose hold was released by a request that never saw it applied.

## Section 15

The Section 15 order flow consults the CRM credit profile before accepting new
private work. An active hold fails closed through the normal order use case;
recurring and bulk generation reuse that same gate.
