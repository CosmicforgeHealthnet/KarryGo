# Runbook — Organization closure

Closure is a controlled workflow. There is no delete endpoint and no hard
deletion anywhere in it.

## Check where a closure request stands

```text
GET /fleet-partner/v1/organizations/{organizationId}/closure-status
```

```sql
SELECT c.status, b.blocker_code, b.owning_service, b.state, b.detail
FROM fleet_partner_organization_closure_requests c
LEFT JOIN fleet_partner_organization_closure_blockers b
       ON b.closure_request_id = c.id
WHERE c.organization_id = $1
ORDER BY c.created_at DESC;
```

## Reading blocker states

| State | Meaning | Action |
| --- | --- | --- |
| `CLEARED` | the owning service confirmed no obligation | none |
| `BLOCKING` | a real obligation exists | resolve it in the owning service |
| `UNKNOWN` | the owning service could not be reached | **treated exactly like BLOCKING** |

`UNKNOWN` is not a soft state. An unreachable service is never read as "no
obligation" — that is how a company with unsettled money would otherwise be
closed. Until the closure contracts are implemented, externally-owned
blockers report `UNKNOWN` by design and closure cannot complete.

## Typical resolution order

1. Offboard managed providers (`POST /providers/{id}/remove`) — active
   memberships are counted locally and block closure.
2. Let in-flight trips and assignments finish in the owning services.
3. Settle wallet operations and withdrawals in `payment-wallet-service`.
4. Close support cases and compliance obligations.
5. Re-run the assessment; blockers are re-evaluated from scratch each time.

## Execution

Only an `APPROVED` request executes. Execution revokes corporate sessions,
stops new operations, instructs the wallet service per its closure contract,
publishes closure events and marks the organization `CLOSED` — retaining all
immutable financial, trip, safety, compliance and audit records.

## Cancelling

A closure request can be cancelled while it is open; the organization keeps
its current lifecycle state throughout, so cancelling costs nothing.

## Do not

- Do not delete organization rows to "finish" a closure.
- Do not clear a blocker row by hand to unblock execution; fix the
  underlying obligation in the owning service and re-assess.
- Do not mark `CLOSED` without the required external confirmation.
