# Provider allocations and approved deductions (Section 13)

An allocation is a company decision to move value from the organization wallet
to one provider sub-wallet. Fleet records the decision; payment-wallet-service
performs the movement.

## Allocation types

`SALARY` · `WAGE` · `PER_JOB` · `ALLOWANCE` · `REIMBURSEMENT` · `BONUS` ·
`ADJUSTMENT_CREDIT` · `APPROVED_DEDUCTION` · `REVERSAL`

`APPROVED_DEDUCTION` and `REVERSAL` debit the provider; the rest credit them.

## Business references

Accepted: `TRIP` · `ASSIGNMENT` · `PAYROLL_REFERENCE` · `EXPENSE_REFERENCE` ·
`MANUAL_APPROVED_REFERENCE`

`PRIVATE_ORDER` and `SHIPMENT` are Section 15 and are **not** accepted. They
are refused at three independent layers — the Fleet service, the Wallet service
and the database CHECK constraint — so enabling them is a deliberate act rather
than an oversight.

## Route

```
POST /fleet-partner/v1/providers/{membershipId}/sub-wallet/allocations
```

Permission `wallet.provider_allocations.create`. An `APPROVED_DEDUCTION` or
`REVERSAL` additionally requires `wallet.provider_allocations.approve` — holding
the permission to pay a provider is not holding the permission to charge one.

Requires `Idempotency-Key`. Accepts `expectedVersion` in the body or `If-Match`.

## Validation order

The idempotency replay lookup runs **first**, before every business check. A
retry of a request that already succeeded returns the same instruction; it is
not rejected by the very duplicate rule its own first attempt created. Getting
this order wrong is a real defect that the integration suite caught during
development.

After replay, in order:

1. Allocation type and business reference type are supported.
2. Amount is positive; currency is a valid ISO code.
3. A deduction carries a provider-visible explanation and a legal, policy or
   contract basis reference.
4. The durable eligibility gate passes.
5. The organization wallet link is ACTIVE and CURRENT (with one read-back
   attempt).
6. The provider sub-wallet link is ACTIVE and CURRENT.
7. Currencies match across wallet, sub-wallet and request.
8. `expectedVersion` matches the sub-wallet's source version.
9. For `PER_JOB` against a `TRIP`, Dispatch confirms the delivery completed.
10. No live allocation exists for the same protected business purpose.
11. Approval policy is evaluated.

Steps 9 and 10 are the ones that matter most. Work that failed, was cancelled
or was never confirmed cannot fund a per-job allocation, and the completion
fact is asked of the Dispatch-owned projection rather than inferred locally.

## Duplicate protection

`idx_fleet_allocation_instructions_business_purpose` enforces one live
allocation per organization, membership, reference and type — but only for
`TRIP`/`ASSIGNMENT` combined with `PER_JOB`/`BONUS`. A payroll period or an
expense claim can legitimately be paid against more than once; a completed trip
funds exactly one per-job payment.

`ProtectedBusinessPurpose` in the repository mirrors the index exactly, so the
application check and the constraint cannot drift apart.

## Statuses

```
DRAFT → PENDING_APPROVAL → APPROVED → SUBMISSION_PENDING → PROCESSING
                  ↓                              ↓
              REJECTED              UNKNOWN → RECONCILING → COMPLETED
                                              ↓
                        FAILED_RETRYABLE → DEAD_LETTER
                        FAILED_PERMANENT
                                REVERSED
```

Terminal states are guarded in SQL: an instruction that already COMPLETED
cannot be moved by a late worker whose read happened before the completion.

## UNKNOWN outcomes

An instruction in `UNKNOWN` or `RECONCILING`, or one that already carries a
Wallet operation reference, is **read back** — never resubmitted. Re-issuing a
payment because the first answer was lost is how a provider gets paid twice,
and the provider is not the party who would notice.

A timeout classifies as `UNKNOWN`, never as a failure. Treating "we did not
hear back" as "it did not happen" is precisely what makes a retry into a second
payment. `TestUnknownOutcomeIsResolvedByReadBackNotResubmission` asserts that
the submit count does not increase across the recovery pass.

## Approved deductions

A deduction requires all of:

- `wallet.provider_allocations.create` **and**
  `wallet.provider_allocations.approve`
- a dedicated reason code
- a legal, policy or contract basis reference
- a provider-visible explanation
- approval, always, regardless of amount — it takes money from someone who is
  not present to object
- a separate approver: the requester may not approve their own deduction

The system **cannot** create a deduction automatically from any of: a provider
decline, illness, fatigue, an unsafe-vehicle report, a safety or security
concern, a failed network response, or an UNKNOWN external outcome. There is no
code path from any of those to an allocation instruction. Repeated declines may
be reported for human review; they authorize nothing on their own.

## Separation of duties

The actor who requested an allocation cannot approve it. This is checked
against the actor identity, not the role — holding both permissions is still
not enough, because one person making and unmaking the same financial fact is
exactly what the control exists to prevent.

Concurrent approvals produce one decision and one conflict, enforced by the
status guard in `SetAllocationStatus` rather than by a check that could race.

## Events

Produced: `organization.provider_allocation.requested.v1`,
`organization.provider_allocation.decided.v1`,
`organization.provider_allocation.confirmed.v1`,
`organization.provider_allocation.failed.v1`.

Payloads carry safe references only. Never: a bank detail, a Wallet secret, a
customer fare, a company margin, a platform commission or an internal note. A
consumer that learns an allocation happened must not thereby learn what the job
was worth.
