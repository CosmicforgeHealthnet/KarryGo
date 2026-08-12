# Sales opportunities (Section 14)

## Stage matrix

```
LEAD           → QUALIFIED, LOST, ON_HOLD
QUALIFIED      → NEEDS_ANALYSIS, LOST, ON_HOLD
NEEDS_ANALYSIS → PROPOSAL, LOST, ON_HOLD
PROPOSAL       → NEGOTIATION, WON, LOST, ON_HOLD
NEGOTIATION    → WON, LOST, ON_HOLD
ON_HOLD        → LOST, or any non-terminal stage previously occupied
WON · LOST     → (terminal)
```

Resuming from `ON_HOLD` is only permitted into a stage the opportunity
**actually reached**, read from the immutable history rather than inferred.
Without that check a hold would be a way to skip qualification.

`WON` and `LOST` are final. Reopening one silently would rewrite a closed
commercial outcome that reporting has already counted.

## Winning creates nothing

Marking an opportunity `WON` does **not** open a contract, activate one, or
commit the organization to anything. Winning a deal and signing an agreement are
separate decisions with separate approvals, and collapsing them would let a
salesperson bind the company.

`TestCRMOpportunityTransitionMatrix` asserts no contract row exists after a win.

## Required reasons

- `LOST` requires a reason code. A lost deal with no reason teaches nobody
  anything.
- `ON_HOLD` requires a reason code. A hold with no reason is indistinguishable
  from neglect.

## Money and probability

Expected value is **integer minor units plus an ISO 4217 currency**, and the two
are present together or absent together — in the request, in the database CHECK,
and after a partial update is merged. A value without a currency is a number
nobody can add up.

Probability is **basis points**, 0–10000. A probability stored as a float and
accumulated as `0.1 + 0.2` is a forecast that does not add up.

## Concurrency

Every stage change appends an immutable history row and requires both
`expectedVersion` and an `Idempotency-Key`.

Two concurrent transitions resolve to one winner: the `stage = $4` guard in the
UPDATE means the loser's expected stage no longer matches and it is refused
rather than overwriting the winner's outcome.

## Events

`organization.sales_opportunity.*` carries the opportunity ID, customer ID,
branch, stage and probability. The **expected value is deliberately absent**: a
sales forecast is commercial information, and no consumer of this event needs it
to react.

## Routes

| Method | Path | Permission |
| --- | --- | --- |
| GET | `/sales/opportunities` | `crm.opportunities.read` |
| POST | `/sales/opportunities` | `crm.opportunities.manage` |
| GET | `/sales/opportunities/{id}` | `crm.opportunities.read` |
| PATCH | `/sales/opportunities/{id}` | `crm.opportunities.manage` |
| POST | `/sales/opportunities/{id}/mark-won` | `crm.opportunities.manage` |
| POST | `/sales/opportunities/{id}/mark-lost` | `crm.opportunities.manage` |
| POST | `/sales/opportunities/{id}/hold` | `crm.opportunities.manage` |
| POST | `/sales/opportunities/{id}/resume` | `crm.opportunities.manage` |
| GET | `/sales/opportunities/{id}/history` | `crm.opportunities.read` |

`resume` takes an explicit target `stage`; the service verifies it against the
history rather than guessing.
