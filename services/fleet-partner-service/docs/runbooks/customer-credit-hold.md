# Runbook: customer credit hold

## Symptom

A CRM operation is refused with `customer_credit_hold`, or somebody asks why a
customer cannot be quoted or contracted.

## What a hold blocks

| Operation | Blocked by `ON_HOLD` / `RESTRICTED` |
| --- | --- |
| Quotation submit and issue | yes |
| Staff-recorded quotation acceptance | yes |
| Contract submit and activation | yes |
| Contract resume from suspension | yes |
| Contract renewal | yes |
| Customer acceptance through a link they already hold | **no** |

The last row is deliberate. The offer was already made; withdrawing it after the
fact is a **supersession**, not a silent refusal at the moment the customer
clicks accept.

`REVIEW_REQUIRED` does **not** block. It is a flag for a human to look, not a
decision that has been taken.

## What a hold does not do

It does not delete quotations or contracts, cancel marketplace bookings, reverse
wallet transactions, deduct provider funds, create orders, or calculate
receivables. Fleet owns no receivables ledger and calculates no debt.

## Diagnosis

```
GET /fleet-partner/v1/customers/{customerId}/credit-profile
GET /fleet-partner/v1/customers/{customerId}/credit-profile/history
```

The history is append-only and records the previous and new hold status, the
previous and new limit, the reason code, the actor and any approval reference.

Also read the customer's risk status:

```
GET /fleet-partner/v1/customers/{customerId}
```

## Hold reason `customer_risk_restricted`

The customer's risk status is `RESTRICTED`, and that forced the credit profile
to `RESTRICTED` in the same transaction.

**The credit hold cannot be released while the restriction stands.** Releasing it
would use the credit surface to undo a risk decision that has its own,
separately permissioned, workflow — and the API refuses with
`customer_risk_restricted`.

The correct path is to de-escalate the risk status:

```
POST /fleet-partner/v1/customers/{customerId}/risk-status
```

with a reason code. De-escalating away from `RESTRICTED` routes through the WP9
approval framework where policy requires it, because it removes a block somebody
deliberately applied.

## Releasing an ordinary hold

```
POST /fleet-partner/v1/customers/{customerId}/credit-profile/release
```

Requires `crm.credit.manage`, a reason code, an `expectedVersion` and an
`Idempotency-Key`.

A release is routed through the approval framework. If it comes back
`credit_change_approval_required`, an approval is required and has not been
granted — find the request in the approvals surface and have it decided. Do not
attempt to change the row directly.

## Hold and release racing each other

They cannot both win. The UPDATE guards on both the expected row version **and**
the expected hold status, so the loser is refused rather than overwriting a hold
that was applied a moment earlier.

If a release appears to have "not taken", re-read the profile: the version will
show that something else moved it, and the history will show what.

## A customer with no credit profile

Not blocked. An organization that has not set a credit policy has not decided to
refuse the customer, and inventing a refusal on their behalf would be Fleet
making a commercial decision it does not own.

If a customer *should* be under policy, create the profile:

```
PUT /fleet-partner/v1/customers/{customerId}/credit-profile
```

A profile created for an already-restricted customer is **created restricted**,
so deleting and recreating a profile cannot launder a risk decision away.
