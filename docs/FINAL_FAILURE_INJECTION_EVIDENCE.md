# FINAL_FAILURE_INJECTION_EVIDENCE

Phase baseline `9ebad225ea9ec25397778a4dc8d3ff842f2938a3`.

## FAIL-001 … FAIL-009

The section asks for controlled failure injection **in staging**. There is
none (`FINAL_DEPLOYED_STAGING_EVIDENCE.md`), so no row below claims deployed
evidence. Where the same failure is injected at a lower level, the row says
what was injected, at what level, and what was observed — that is a statement
about the behaviour, not about the deployment.

| ID | Failure | Deployed | Injected elsewhere | Observed |
|---|---|---|---|---|
| FAIL-001 | Fleet unavailable during completion | `NOT RUN` | `PASS — DB-BACKED`: transport error, terminal rejection and duplicate all injected into completed-work submission | Local completion stays authoritative; the idempotency key is stable across retries; ambiguity classifies `UNKNOWN` → `RECONCILIATION_REQUIRED` and redelivers under the **same** key, so recovery produces one record, not two. Assuming failure there is how a second record gets written. |
| FAIL-002 | Wallet unavailable after instruction | `NOT RUN` | `PASS — DB-BACKED` | The instruction stays pending and reconcilable; Fleet's `OUTCOME_UNKNOWN` path **reads the settlement back** rather than reposting. `TestSettlementCanBeReadBackByEveryRoute`, `TestAManagedJobHeldWithNoSettlementIsSurfaced`, `TestAStuckJobThatLaterSettlesIsResolved`. No money is lost and no second journal is posted. |
| FAIL-003 | Vendor callback lost | `NOT RUN` | `PASS — DB-BACKED` | Stale and duplicate callbacks do not regress state; an unmapped status advances nothing rather than being guessed at; Vendor's reconciliation converges from the authority. |
| FAIL-004 | Notification down | `NOT RUN` | `PASS — DB-BACKED` | `TestNotificationDegradedMode_CoreMutationPersistsRetriesDeadLettersAndReplays`. The core mutation persists, so the state a provider must recover is already durable and reachable by API. No business flow depends on push. |
| FAIL-005 | Media temporarily down | `NOT RUN` | `PASS — DB-BACKED` | An expired or replayed upload authorization is refused; nothing records evidence as uploaded when it was not. Retry is explicit. |
| FAIL-006 | Verification down | `NOT RUN` | `PASS — DB-BACKED` | Unavailable stays distinct from rejected; no path turns an unreachable verification service into an approval. Corporate membership is evaluated separately and never substitutes for readiness. |
| FAIL-007 | Redis interruption | `NOT RUN` in staging | `PASS — LIVE LOCAL` | Hauling boots and serves `/health` 200 with Redis unreachable — optional integrations do not fail liveness. Certified live in the Sections 15–18 phase and unchanged. |
| FAIL-008 | Service restart with pending operations | `NOT RUN` in staging | `PASS — DB-BACKED` | `TestScheduler_LeaseRecoveryAfterCrash`, `TestExpiredLeaseRecovery`, `TestReconciliationRecoversAbandonedLease`, `TestDeletionWorkerRecoversAbandonedLease`. Leases are reclaimed by expiry, so a process that dies holding one strands nothing. |
| FAIL-009 | Database interruption | **`NOT RUN — RISK CONTROL`** | — | The section permits this only where safe and isolated. Interrupting connections under a live balanced-journal write on the shared certification database risks leaving the certification data itself in a state that would make every other result unreliable. Lease expiry and crash recovery (FAIL-008) cover the observable half of the same class. This is a deliberate refusal, not an omission. |

## The five conditions, per injection

For each injected failure the section requires: local state correct, retry
identity stable, no duplicate financial posting, reconciliation surfaces it,
convergence after recovery. Each was checked.

The one worth stating plainly, because it is the difference between a
recoverable outage and a double payment: **an ambiguous outcome is never
treated as a failure.** A lost response after a write means the write may have
landed. Every path that can see one — completed-work submission, settlement
posting, commerce mission creation — classifies it `UNKNOWN`, retries under the
identical idempotency key, and where money is involved reads the result back
from the authority instead of re-posting. That is what makes FAIL-001 and
FAIL-002 safe rather than merely retried.

## What must still be injected in staging

All nine, against deployed services, once the environment exists. In
particular FAIL-007 and FAIL-008 change character when the process boundary is
real: a container restart is not a goroutine restart, and a Redis interruption
across a network is not a client returning an error inline.
