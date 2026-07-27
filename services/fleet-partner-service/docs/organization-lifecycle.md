# Organization Lifecycle and Closure

## State machine

```text
DRAFT -> CONTACT_VERIFIED -> BUSINESS_SUBMITTED -> UNDER_REVIEW -> ACTIVE
UNDER_REVIEW -> CHANGES_REQUIRED -> BUSINESS_SUBMITTED
ACTIVE <-> SUSPENDED
ACTIVE | SUSPENDED -> CLOSED
```

`PENDING_REVIEW` (created by the Sections 3–7 foundation) is accepted as a
synonym of `DRAFT` so existing rows stay valid. There is deliberately **no
immediate-delete state**.

## Enforced transitions

No handler updates `organizations.status`. Every change goes through
`Transition`, which in one transaction validates the move against the state
machine, performs a compare-and-swap on the aggregate version, appends
status history, writes audit and emits the outbox event.

## Closure is a workflow, not a delete

```text
REQUESTED -> ASSESSING -> BLOCKED | APPROVAL_REQUIRED -> APPROVED
         -> EXECUTING -> COMPLETED
         -> REJECTED | CANCELLED
```

A partial unique index allows only one open closure request per
organization, so a duplicate request returns the existing one rather than
creating a second.

### Blockers

Marketplace requests, assignments, trips, unsettled wallet operations,
pending allocations and withdrawals, active provider memberships, payroll,
invoices, claims, incidents, legal holds, retention requirements, pending
exports, compliance obligations and integration credentials.

Each blocker names the authoritative service that owns it. **"No local row"
never means "no external obligation":** a checker that cannot reach its
service returns `UNKNOWN`, which blocks closure exactly like a confirmed
blocker until it is reconciled. Until the closure contracts exist, the
default checker set returns `UNKNOWN` for every externally-owned obligation —
fail-closed by construction.

Active managed providers are counted locally and block closure until they
are offboarded.

### Execution

When blockers clear and approval exists: revoke corporate sessions, stop new
operations, offboard providers, revoke credentials and webhooks, archive
organization-owned mutable data per policy, anonymize eligible personal data,
**retain** immutable financial, trip, safety, compliance and audit records,
instruct `payment-wallet-service` per its closure contract, publish closure
events, and only then mark `CLOSED`.

Nothing is hard-deleted, and `CLOSED` is reached only after the required
external confirmation.
