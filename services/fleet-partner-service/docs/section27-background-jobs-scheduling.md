# Section 27 — Background Jobs, Leases and Scheduled Processing

## Scope

Section 27 owns durable background-job records, scheduling, claiming/leases,
heartbeats, attempts, retry scheduling, dead-letter state, replay, recurring
scheduled-task state, worker registration and safe operator controls.

Section 27 does **not** own business effects. Every worker calls into the
owning domain's own service/use case; the generic job framework never writes
directly into an unrelated domain's tables. See "Workers" below for exactly
which domain each worker calls.

## Canonical entities

Section 27 extends the pre-existing durable-jobs schema (migration
`000001_platform_foundation`, widened by `000047_fulfilment_terminal_returns`
for `fleet_partner_job_attempts`) rather than building a second one, per the
explicit instruction to reuse and enhance existing tables:

- **`fleet_partner_jobs`** (extended by `000063_section27_jobs_completion`):
  `id, organization_id, branch_id, job_type, payload, payload_version,
  status, priority, dedupe_key, idempotency_key, correlation_id,
  causation_id, scheduled_task_id, scheduled_for, attempts, max_attempts,
  next_attempt_at, lease_owner, lease_expires_at, heartbeat_at, started_at,
  completed_at, cancelled_at, dead_lettered_at, last_error_class,
  last_error_code, last_error_redacted, external_outcome_status,
  authoritative_readback_required, version, created_at`. Status values:
  `PENDING, RETRY_SCHEDULED, CLAIMED, RUNNING, SUCCEEDED, FAILED,
  DEAD_LETTERED, CANCELLED` (the pre-existing names; the master
  specification permits reusing compatible existing state names rather than
  maintaining two job-state machines).
- **`fleet_partner_job_attempts`** (widened by `000063`): immutable,
  append-only (`fleet_partner_forbid_mutation()` trigger), one row per
  attempt: `id, job_id, organization_id, attempt_number, worker_owner,
  status, error_redacted, error_class, error_code, started_at, heartbeat_at,
  completed_at`.
- **`fleet_partner_scheduled_tasks`** (new, `000064_section27_scheduled_tasks`):
  `id, organization_id, branch_id, task_type, job_type, schedule_type
  (CRON|INTERVAL|ONCE), schedule_expression, timezone, payload,
  payload_version, enabled, next_run_at, last_scheduled_at, last_result,
  failure_count, concurrency_policy (ALLOW|FORBID|REPLACE), misfire_policy
  (SKIP|RUN_ONCE|CATCH_UP_LIMITED), max_catch_up, max_attempts, priority,
  lease_owner, lease_expires_at, created_by, updated_by, version,
  created_at, updated_at`.
- **`fleet_partner_scheduled_task_runs`** (new, `000064`): append-only
  scheduler-decision history: `id, scheduled_task_id, organization_id,
  scheduled_for, outcome (ENQUEUED|SKIPPED_FORBID|SKIPPED_MISFIRE|
  REPLACED_PRIOR|ENQUEUE_FAILED), job_id, safe_error_code, occurred_at`.

Outbox/inbox (`fleet_partner_outbox`, `fleet_partner_inbox`) are untouched —
Section 27 does not duplicate them.

## Error classification (`internal/jobs/errorclass.go`)

`ErrorClass` values: `TRANSIENT, PERMANENT, CONFLICT,
DEPENDENCY_UNAVAILABLE, RATE_LIMITED, AUTHENTICATION, AUTHORIZATION,
VALIDATION, UNKNOWN_EXTERNAL_OUTCOME, LEASE_LOST, CANCELLED`.
`Retryable()` is false for `VALIDATION`, `PERMANENT` and `AUTHORIZATION`;
true otherwise. `RequiresReadback()` is true only for
`UNKNOWN_EXTERNAL_OUTCOME`. A handler returns a `*jobs.ClassifiedError`
wrapping `{Class, Code, SafeMessage, Err}`; an unclassified plain error
defaults to `TRANSIENT` for backward compatibility with handlers written
before Section 27.

`Queue.Fail` dead-letters immediately on a non-retryable class, regardless
of `attempts` vs `max_attempts` — a validation or permanent error cannot
succeed on a later attempt, so bounded retry does not apply to it.

## Lease, heartbeat, recovery

`Queue.ClaimBatch` claims with `FOR UPDATE SKIP LOCKED`, ordered by
`priority DESC, scheduled_at`; expired leases (status `CLAIMED`/`RUNNING`
with `lease_expires_at < now()`) are reclaimed by the same query. Only the
lease owner may heartbeat (`Heartbeat`), complete (`Complete`), fail
(`Fail`) or dead-letter (`DeadLetter`) a job — every mutating method is
`WHERE id = $1 AND lease_owner = $2`. `Worker.execute` heartbeats on a
ticker at `HeartbeatInterval` while a handler runs.

`scheduler.Repository.ClaimDue` uses the identical `FOR UPDATE SKIP LOCKED`
pattern for scheduled tasks, so multiple scheduler instances (API-binary
in-process option and the dedicated worker binary) never process the same
due task concurrently — proven by
`TestScheduler_ClaimDueIsSkipLockedAcrossConcurrentSchedulers` and
`TestScheduler_LeaseRecoveryAfterCrash` against real PostgreSQL.

## Graceful shutdown

A handler that observes `ctx.Done()` before any externally-visible,
not-safely-repeatable effect returns the sentinel `jobs.ErrShutdownSafeToRelease`.
`Worker.execute` recognizes it and calls `ReleaseForShutdown` (status back
to `PENDING`, lease cleared, **no backoff** — nothing about the job failed,
only the process hosting it stopped) instead of the ordinary `Fail` path. A
handler that already made an external call whose outcome is unconfirmed
must instead return a `*ClassifiedError{Class: UNKNOWN_EXTERNAL_OUTCOME}`,
which sets `authoritative_readback_required` and preserves the job (and its
attempt history) for read-back rather than releasing it for a second worker
to potentially repeat the same external effect. Proven by
`TestJobWorker_ShutdownSentinelReleasesWithoutBackoff` and
`TestJobs_UnknownExternalOutcomeRequiresReadback`.

`scheduler.Runner.Run`'s shutdown is simpler by construction: a `Tick` only
claims and immediately decides/enqueues/advances — it never holds a task
lease across a long-running external effect — so stopping the poll loop on
`ctx.Done()` is sufficient; a task claimed mid-`Tick` at shutdown is
recovered by the next scheduler instance exactly like a crashed instance.

## Scheduler: CRON / INTERVAL / ONCE, DST, misfire, concurrency

`internal/scheduler/cron.go` is a hand-written 5-field cron parser
(`*`, a value, a comma-list, or `*/N` — ranges are deliberately
unsupported). `CronSchedule.Next` steps forward one real minute of
absolute time at a time and only uses `time.Date` reconstruction as an
equality check to detect and skip the repeated ("second pass") instant of a
fall-back DST transition, resolving every schedule to its first
chronological occurrence. A spring-forward gap is skipped automatically
because no absolute instant's local reading ever falls inside it. Both
transitions are pinned by
`TestCronSchedule_DSTForwardGapIsSkipped` and
`TestCronSchedule_DSTBackwardOverlapResolvesToFirstOccurrence` against
`America/New_York` (`time/tzdata` embedded via
`internal/features/organizations/repositories`, so this works on Windows
and slim containers without an OS zoneinfo database).

`INTERVAL` is an integer count of seconds from the last occurrence; `ONCE`
is an RFC3339 instant in the task's own timezone, and once passed leaves
`next_run_at` `NULL` (no further occurrences, not an error).

`scheduler.Runner.processOne` applies misfire policy when `now -
next_run_at` exceeds a grace window (default 30s): `SKIP` records
`SKIPPED_MISFIRE` and advances past the missed occurrence without
enqueueing; `RUN_ONCE` enqueues exactly one job for the missed window no
matter how many occurrences were actually missed; `CATCH_UP_LIMITED`
enqueues up to `max_catch_up` missed occurrences (bounded — proven by
`TestScheduler_RunnerMisfireCatchUpLimited_BoundedByMaxCatchUp`).
Concurrency policy governs whether a still-in-flight job from a prior
occurrence blocks a new one: `ALLOW` never blocks; `FORBID` skips
(`SKIPPED_FORBID`); `REPLACE` cancels a not-yet-started (`PENDING`/
`RETRY_SCHEDULED`) prior occurrence and enqueues the new one, but never
force-cancels a `CLAIMED`/`RUNNING` job that may already have an external
effect in flight.

An enqueue failure (for example a task pointed at an unregistered job type)
is recorded as `ENQUEUE_FAILED` in run history, increments
`failure_count`, and **still advances the schedule** rather than leaving the
task's scheduler lease held until timeout — proven by
`TestScheduler_RunnerEnqueueFailure_RecordsAndAdvancesWithoutStuckLease`.

Job occurrences are enqueued idempotently via the existing
`scheduled_task_id + scheduled_for` uniqueness added to `fleet_partner_jobs`
in `000063` — re-processing the same due task (after a lease timeout) never
double-enqueues.

## Workers

| Worker | Owning domain call | Status |
|---|---|---|
| Outbox publisher | `events.OutboxWorker` (pre-existing) | Standardized: `cmd/worker/main.go` now uses the real Redis transport when configured, matching `cmd/main.go` (previously hardcoded `DisabledPublisher`). |
| Assignment expiry | `marketplace.Service.ExpireAssignments` | New: `app.StartAssignmentExpiryWorker`, gated `ASSIGNMENT_EXPIRY_WORKER_ENABLED`, wired in both binaries. |
| Reassignment coordinator | `marketplace.Service.ScanStaleReassignmentCases` + `AutoReassignStaleCase` | New: `app.StartReassignmentStalenessWorker`, gated `REASSIGNMENT_WORKER_ENABLED`. Always surfaces staleness via a gauge; when `FLEET_AUTOMATIC_REASSIGNMENT_ENABLED` is also set, attempts automatic reassignment (see "Reassignment coordinator scope" below). Also fixed a real pre-existing gap: `Flags.ReassignmentEnabled` existed on the marketplace service but was never wired to any config source, so the manual, human-dispatcher `Reassign` path always refused in every deployment; now wired via `FLEET_REASSIGNMENT_ENABLED`. |
| Provider provisioning reconciliation | `providers.Service.Reconcile` | Standardized: `app.RunProviderReconciler` is now shared between both binaries (previously only in the API binary); `cmd/worker/main.go` was silently missing it entirely. |
| Marketplace projection reconciliation | `marketplace.Service.RunReconciliationLoop` | Standardized: `app.StartMarketplaceReconciliation` shared between both binaries (previously only in the API binary). |
| Document expiry | `documents` (pre-existing `app.StartDocumentWorkers`) | Already wired in both binaries; unchanged. |
| Preventive maintenance scheduler | `assets.Service.RunMaintenance` / `ScheduleMaintenance` | Already wired in both binaries via `app.StartSection17And18Workers`, real integration test coverage (`section17_18_test.go`); no change needed. |
| Recurring order generator | `recurringorders` (pre-existing, Section 15) | Already wired via `app.StartSection15Workers`; unchanged. |
| Notification dispatcher | `engagement.Service` notification-intent submit/reconcile handlers, plus CRM's `crmNotifier` | Already wired via `app.StartSections22And23Workers` calling `engagement.ScheduleMaintenance` every minute (confirmed live in this codebase, not dormant); unchanged. |
| Bulk import processor | `bulkimports` (pre-existing) | Unchanged. |
| Export generator | `exports` (pre-existing) | Unchanged. |
| SLA evaluator | `engagement.Service.handleSLAGenerate` | Already wired via the same `ScheduleMaintenance` fan-out; unchanged. |
| Automation runner | *none — Section 24 not implemented* | Infrastructure/registration-boundary only: `internal/features/automation` registers `AUTOMATION_RULE_EXECUTE`, whose handler always returns a classified `PERMANENT` "automation domain not implemented" failure so a stray enqueue dead-letters loudly instead of retry-looping or vanishing. Gated `AUTOMATION_RUNNER_WORKER_ENABLED` (default false). No rule execution capability exists to gate a GO on. |
| Data retention | `internal/features/retention` (new) | Report-only, scoped to Section 27's own operational tables (see "Data retention scope" below). Gated `DATA_RETENTION_WORKER_ENABLED` (default false). |

### Reassignment coordinator scope

`ExpireAssignments`'s own doc comment establishes the product's existing
design: expiry "releases the provider and asset, opens a reassignment case
and notifies dispatchers, so the work goes back to a human rather than
being silently committed to an unresponsive provider." There is no
candidate *scoring/matching* capability anywhere in the marketplace domain
(dispatch-board candidate selection in Section 16 is a different bounded
context, operating on its own "source" concept, not marketplace
`RequestProjection`s — bridging the two would mean inventing a
cross-domain mapping the product has never defined, not reusing an
existing one). Building a scoring algorithm is out of Section 27's
boundary for the same reason automation-rule execution is.

What Section 27 *can* safely do, and now does, is sequence candidates
through the marketplace domain's own already-correct decision: `Reassign`
already performs full eligibility checking (`EligibilityChecker.CheckMembership`
plus the durable-eligibility gate) for whatever membership it is given, and
rolls back cleanly (its own transaction) if that membership turns out to be
ineligible. `AutoReassignStaleCase` (`internal/features/marketplace/usecases/assign.go`)
lists up to 5 active memberships (via the new `CandidateMembershipLister`
port, excluding the case's own excluded set — no scoring, just the
providers repository's existing membership list) and tries `Reassign` for
each in turn, stopping at the first success. Gated by
`Flags.AutomaticReassignmentEnabled` (config `FLEET_AUTOMATIC_REASSIGNMENT_ENABLED`,
default false) *and* `REASSIGNMENT_WORKER_ENABLED`; with either off, the
worker only surfaces staleness via the
`fleet_partner_reassignment_cases_stale` gauge and the case stays open for
a human, the pre-existing safe default. `ErrNoEligibleCandidate` (all
candidates tried, none eligible) is not a failure — the case correctly
remains open. Proven end-to-end against real PostgreSQL by
`TestSection27_AutoReassignStaleCase_TriesCandidatesAndSucceeds`,
`TestSection27_AutoReassignStaleCase_NoEligibleCandidateLeavesCaseOpen`,
`TestSection27_AutoReassignStaleCase_DisabledByDefault`, and
`TestSection27_ReassignmentEnabledFlagIsNowWired` (the flag-wiring fix).

### Data retention scope

No domain in this codebase defines a retention *policy* (eligibility,
legal basis, retention window) for business tables — customers, orders,
documents, financial records. Inventing one is out of Section 27's
boundary. `internal/features/retention` therefore implements a real
capability scoped to tables Section 27 itself owns and generates
(`fleet_partner_jobs`, `fleet_partner_job_attempts`,
`fleet_partner_scheduled_task_runs`): it counts terminal/aged rows on an
interval and emits `fleet_partner_retention_eligible_rows{table=...}`
gauges plus structured log lines.

This is **permanently report-only, by design, not a deferred capability**:
`fleet_partner_job_attempts` and `fleet_partner_scheduled_task_runs` both
carry a `BEFORE UPDATE OR DELETE ... fleet_partner_forbid_mutation()`
trigger (migrations `000047`, `000064`) that rejects deletion at the
database level, and `fleet_partner_job_attempts.job_id` references
`fleet_partner_jobs(id) ON DELETE RESTRICT` — so a `fleet_partner_jobs` row
with any attempt history cannot be deleted either, without first deleting
its (undeletable) attempts. These are deliberate audit-trail immutability
guarantees, not oversights; weakening either trigger to allow destructive
retention would remove a data-integrity guarantee this codebase
intentionally established, which is a call for whoever owns that
guarantee to make explicitly, not something a background-jobs section
should quietly do as a side effect. Zero deletes, zero anonymization —
proven by `TestRetention_ScanAndReport_NeverMutates`.

## Feature flags (all default false)

`ASSIGNMENT_EXPIRY_WORKER_ENABLED`, `REASSIGNMENT_WORKER_ENABLED`,
`FLEET_REASSIGNMENT_ENABLED` (the manual `Reassign` gate, now actually
wired), `FLEET_AUTOMATIC_REASSIGNMENT_ENABLED`,
`AUTOMATION_RUNNER_WORKER_ENABLED`, `DATA_RETENTION_WORKER_ENABLED` are new
in this pass. `WORKER_OPERATIONS_ALLOWED_CALLERS` (comma-separated caller
names, empty by default) gates whether the internal worker-operations
routes mount at all. `FLEET_MARKETPLACE_RECONCILIATION_ENABLED` and the
per-section worker flags (`FLEET_PARTNER_WORKER_ENABLED`,
`Section16WorkersEnabled`, etc.) pre-date Section 27 and are unchanged.

## Operator permissions and routes

Permissions (seeded by `000063`/`000064`, granted to
`rol_system_owner`/`rol_system_admin`/`rol_system_operations`):
`jobs.read`, `jobs.manage`, `jobs.retry`, `jobs.cancel`,
`scheduled_tasks.read`, `scheduled_tasks.manage`, `scheduled_tasks.run`,
`worker_operations.read`, `worker_operations.manage`.

`internal/features/backgroundops/http` mounts the tenant-scoped surface
under `/fleet-partner/v1`:

```
GET  /jobs                              jobs.read
GET  /jobs/:jobId                       jobs.read
GET  /jobs/:jobId/attempts              jobs.read
POST /jobs/:jobId/retry                 jobs.retry
POST /jobs/:jobId/cancel                jobs.cancel
GET  /scheduled-tasks                   scheduled_tasks.read
GET  /scheduled-tasks/:taskId           scheduled_tasks.read
GET  /scheduled-tasks/:taskId/runs      scheduled_tasks.read
POST /scheduled-tasks/:taskId/enable    scheduled_tasks.manage
POST /scheduled-tasks/:taskId/disable   scheduled_tasks.manage
POST /scheduled-tasks/:taskId/run-now   scheduled_tasks.run
```

Every route is organization-scoped by the underlying repository query
(`WHERE organization_id = $x`), so a job or task belonging to another
organization is invisible (404), never a cross-tenant view — proven by
`TestBackgroundOps_JobsRoutes_RequireJobsPermissionAndEnforceOrgScope`.
`JobView`/`TaskView` responses never include the raw payload; error text is
always the queue's own pre-redacted, size-bounded safe message.
Enable/disable/run-now require `expectedVersion` (body field or `If-Match`)
and return 409 on a stale version.

### `worker_operations.*` (service-global jobs)

The master specification requires service-global (`organization_id IS
NULL`) job/task operations to sit behind "an existing internal/platform-
operator authorization mechanism" distinct from ordinary organization-admin
permissions. No tenant-independent human-operator IAM exists anywhere in
this codebase — every `fleet_partner_roles` row, including the
highest-privilege system-template roles (`rol_system_owner` etc., defined
in `000005_staff_and_iam.up.sql`), is organization-scoped, so reusing the
tenant `Authorizer` would not satisfy the requirement.

What *does* already exist, used throughout this service for exactly this
class of problem (see `documents/http/internal.go`,
`buildMarketplaceInternalRoutes`), is the internal HMAC service-auth route
class: `/internal/fleet-partner/v1/...`, protected by
`middleware.InternalServiceAuth` (timestamp-freshness-checked HMAC via
`shared/go/serviceauth`) plus `middleware.RejectReplayedNonce`, with a
per-route caller allowlist on top (holding a valid service secret proves
identity, not authorization). `internal/features/backgroundops/http/internal.go`
mounts `worker_operations.*` there, gated on `WORKER_OPERATIONS_ALLOWED_CALLERS`
(empty by default → routes not mounted at all, matching the "absent
allowlist means no caller could ever pass" convention every other internal
route builder in this service follows). Every method is hardcoded to
`organizationID=""` (the platform scope) — never a caller-supplied
organization — so this surface cannot be used to reach into any specific
tenant's jobs; that would be a materially larger privilege-escalation
surface than "service-global jobs" calls for. A human platform operator
reaches this through an internal admin tool (e.g. admin-backoffice-service)
that itself authenticates as the HMAC caller, exactly the same shape
`support-dispute-service`'s admin surface already uses.

Routes (`/internal/fleet-partner/v1/worker-operations/...`):

```
GET  /jobs
GET  /jobs/:jobId
GET  /jobs/:jobId/attempts
POST /jobs/:jobId/retry
POST /jobs/:jobId/cancel
GET  /scheduled-tasks
GET  /scheduled-tasks/:taskId
GET  /scheduled-tasks/:taskId/runs
POST /scheduled-tasks/:taskId/enable
POST /scheduled-tasks/:taskId/disable
POST /scheduled-tasks/:taskId/run-now
```

Proven against real PostgreSQL by
`TestWorkerOperations_RequiresAllowlistedCallerAndStaysPlatformScoped`
(non-allowlisted caller refused despite a valid signature; a tenant-owned
job is invisible/404 through this surface) and
`TestWorkerOperations_ScheduledTaskEnableRequiresExpectedVersion`.

Note: the `worker_operations.read`/`worker_operations.manage` permission
rows seeded by `000064` into `fleet_partner_permissions`/
`fleet_partner_role_permissions` are not consulted by this HMAC-gated
surface (internal routes never check tenant permissions) — they remain as
forward-compatible metadata for a future human-facing admin UI that gates
*its own* access before relaying a request here.

## Observability

`scheduler.Runner.Run` emits `scheduled_tasks_due_total`,
`scheduled_task_misfires_total`, `scheduler_enqueue_failures_total`,
`fleet_partner_scheduler_tick_errors_total`. `jobs.Worker` already emitted
(pre-Section-27) `fleet_partner_job_claims_total`,
`fleet_partner_job_retries_total`, `fleet_partner_job_succeeded_total`,
`fleet_partner_job_lease_lost_total`; Section 27 added
`fleet_partner_job_shutdown_released_total`. `jobs.Queue.StaleLeaseCount`
and `DeadLetterCount` are read-backs for a health check (stale-lease and
dead-letter-backlog thresholds); `jobs.Queue.Counts` reports queue depth by
status. `retention.Worker` emits
`fleet_partner_retention_eligible_rows{table=...}`. The reassignment
coordinator additionally emits `fleet_partner_reassignment_auto_succeeded_total`,
`fleet_partner_reassignment_auto_no_candidate_total` and
`fleet_partner_reassignment_auto_error_total` when automatic mode is on.

Structured logs never include job payloads; the retry/reassignment/
retention log lines above carry only safe references (job/task ID, type,
status, counts).

## Remaining, permanent scope boundaries (not gaps to close — explained above)

1. **Business-domain data retention/anonymization** — no per-domain
   retention policy exists to drive it, and the two candidate history
   tables are deliberately DB-trigger-immutable; only Section 27's own
   operational tables are covered, permanently report-only (see "Data
   retention scope" above).
2. **Automation rule execution** — Section 24 is not implemented; only the
   registration boundary exists (see above). The moment Section 24 ships a
   real rule-execution capability, `internal/features/automation`'s
   handler is the one place to replace.

Both `worker_operations.*` platform routes and automatic reassignment
candidate selection — previously listed here as gaps — are now implemented
(see "Operator permissions and routes" and "Reassignment coordinator
scope" above).
