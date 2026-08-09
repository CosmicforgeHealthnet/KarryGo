# Provider withdrawals and statements (Section 13)

## Policy modes

| Mode | Provider may open | Approval required |
| --- | --- | --- |
| `PROVIDER_DIRECT` | yes | only above the configured threshold |
| `COMPANY_APPROVAL_REQUIRED` | yes | always |
| `COMPANY_INITIATED_ONLY` | no | always |
| `WITHDRAWALS_DISABLED` | no | n/a — every request is refused |

An organization that has never configured a policy gets
`COMPANY_APPROVAL_REQUIRED`. An unconfigured control must not be an open one.

Organization policy can only **narrow** what is possible. It cannot override
Wallet compliance restrictions, bank-account verification, platform withdrawal
limits, fraud controls, legal restrictions, account status, provider suspension
or provider verification requirements. Those are applied independently by
payment-wallet-service and are not consulted from Fleet's policy code at all —
there is no value an organization can set that reaches them.

## Routes

```
POST   /fleet-partner/v1/providers/{membershipId}/sub-wallet/withdrawals
POST   /fleet-partner/v1/providers/{membershipId}/sub-wallet/withdrawals/{id}/approve
POST   /fleet-partner/v1/providers/{membershipId}/sub-wallet/withdrawals/{id}/reject
GET    /fleet-partner/v1/providers/{membershipId}/sub-wallet/withdrawals
```

Provider-authenticated equivalents:

```
GET  /fleet-partner/v1/provider/sub-wallet
GET  /fleet-partner/v1/provider/sub-wallet/statement
POST /fleet-partner/v1/provider/sub-wallet/withdrawals
```

The provider routes resolve the membership from the provider's own
authenticated session through `ProviderMembershipResolver` — **never** from a
path parameter or request body. There is no code path where request input
selects whose money is read or moved. When no resolver is wired the routes are
not mounted at all, rather than mounted with a weaker check.

Corporate staff cannot impersonate a provider: the staff routes and the
provider routes are separate handlers with separate identity resolution, and a
staff-initiated withdrawal is recorded with `requestedByActorType` of
`ORGANIZATION_USER`.

## Bank details

Fleet stores an **opaque destination reference** and nothing else. No account
number, no sort code, no bank name, no account holder. The reference is not
included in list responses either: a handle that resolves to somebody's bank
account does not belong in a listing a broad read permission can reach, and it
is not published in any event.

## Reservation

Requesting a withdrawal reserves the funds immediately in
payment-wallet-service (payable → held). Reserving at request time rather than
at execution time is what makes two concurrent requests safe: the second finds
the balance already moved and is refused, instead of both passing a check that
was true when each of them read it.

Rejecting returns the reservation. Approving leaves it reserved for the
execution worker. A permanent failure releases it; an **ambiguous** outcome
does not — an unknown payout may well have happened, and releasing the funds
would let the same money leave twice.

Fleet additionally enforces one open withdrawal per membership through
`idx_fleet_withdrawal_requests_open`, a partial unique index rather than an
application check that could race.

## Statuses

```
REQUESTED → PENDING_APPROVAL → APPROVED → SUBMISSION_PENDING → PROCESSING
      ↓             ↓                              ↓
  CANCELLED     REJECTED         UNKNOWN → RECONCILING → COMPLETED
                                     ↓
                   FAILED_RETRYABLE → DEAD_LETTER
                   FAILED_PERMANENT
```

A resolved withdrawal does not accept a second decision. Two approvers pressing
the button concurrently produce one decision and one conflict.

## Provider statement

The statement DTO is the entire data-minimization boundary. Every field is
assigned explicitly from an allocation or withdrawal record. **No internal
record is ever serialized and then pruned** — with a prune-based approach, a
field added upstream next year would start reaching providers the day it was
added, and nobody would notice until it had.

Present: sub-wallet ID, currency, status, Wallet-reported balance, allocations
by type, approved deductions, reversals, withdrawals, dates, safe business
reference type and ID, reason code, provider-visible explanation, withdrawal
status, source version, freshness.

Absent by construction: customer fare, customer price, customer payment or
payment method, organization settlement amount, organization wallet balance,
company margin, platform commission or its percentage, contract price,
quotation price, organization profit, internal finance notes, internal approval
discussion, other providers' transactions, and any bank detail.

`TestProviderStatementExcludesCommercialData` asserts on the **serialized
bytes**, not on struct fields, because the question is what actually reaches
the provider. It also asserts a safe field is present, so the test cannot pass
by returning nothing.

Draft and rejected allocations are omitted: showing one would promise a payment
the company has not made.

## Workers

`StartCompanyWalletWorkers` runs allocation and withdrawal execution plus
projection refresh. Both the API binary and the dedicated worker binary build
the service through the same composition helper, so the two processes cannot
end up with different gates.

Workers require: a feature flag, kill-switch enforcement, bounded batch size,
a bounded interval, retry limits, redacted errors, a dead-letter state,
operator replay, tenant revalidation, idempotency and graceful shutdown. No
network call is made inside a database transaction.

Operator replay: `POST .../allocations/{id}/replay` and
`POST .../withdrawals/{id}/replay` return a dead-lettered record to the
execution queue.
