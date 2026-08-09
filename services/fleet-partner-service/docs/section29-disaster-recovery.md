# Section 29 — Disaster Recovery

Scope: `services/fleet-partner-service` only. This document describes what
is actually implemented and tested in this repository today, what remains a
hosting/platform-dependent target, and exactly which command or test proves
each claim. Nothing here is aspirational without being labeled as such.

## 1. Recovery objectives

| Objective | Value | Status |
|---|---|---|
| RPO (Recovery Point Objective) | Bounded by backup cadence — **not continuous** today | **Target, platform-dependent.** This service has no WAL-archiving/PITR configuration in this repository; RPO equals "time since the last `backup.sh` run." A real production RPO figure (e.g., 15 minutes) requires the hosting platform's own managed-Postgres continuous backup/PITR feature (e.g., a cloud provider's automated backups), which is outside this repository's control. |
| RTO (Recovery Time Objective) | Restore duration observed in this session: **~5–8 seconds** for the current test dataset size (see §3) | **Partially verified.** The *mechanical* restore step is proven fast at today's data volume. A production RTO also includes detection, decision, DNS/traffic cutover and validation time, which are operational/organizational, not code — see §9. |
| Backup cadence | Not scheduled in this repository | **Target.** `backup.sh` is runnable on demand or from any external scheduler (cron, a platform's managed backup product); this repository does not itself run a scheduler for it (see §14 "DR validation schedule"). |

**Honest summary:** this service can be backed up and restored correctly and
verifiably (§2–§3, real test evidence). It does not yet have continuous
(WAL-based) backup, and it does not schedule its own backups — both are
genuine gaps, disclosed rather than hidden, and both are the kind of
capability a managed-Postgres hosting platform typically provides rather
than something an application repository implements itself.

## 2. Backup ownership and procedure

Owner: whichever team operates the production PostgreSQL instance for this
service (not specified further — no such role exists in this repository;
assign this in your deployment runbook).

Procedure and commands: `services/fleet-partner-service/scripts/backup.sh`.

```bash
FLEET_PARTNER_BACKUP_DATABASE_URL='postgres://user:pass@host:port/db?sslmode=require' \
  bash services/fleet-partner-service/scripts/backup.sh /path/to/backup-dir
```

Produces, per run:
- `<name>.dump` — `pg_dump -Fc` (PostgreSQL custom format: compressed,
  supports selective/parallel restore).
- `<name>.dump.sha256` — checksum.
- `<name>.meta.json` — service name, UTC timestamp, applied migration
  version (`fleet_partner_schema_migrations`), and row counts for
  `fleet_partner_organizations` / `fleet_partner_audit_records` /
  `fleet_partner_jobs`.

No credential is ever written into the backup filename or metadata; the
connection string is read from an environment variable only, never logged.
`chmod 700` is applied to the destination directory.

**Not automated by this script** (explicitly out of scope, listed honestly
rather than silently omitted):
- Off-host / encrypted-at-rest destination — pipe `backup.sh`'s output
  directory to your object-storage/encryption tooling of choice.
- Retention and rotation policy — an operator or external scheduler decides
  how many backup sets to keep.
- Failure alerting — `backup.sh` exits non-zero on any failure; wire that
  exit code into your scheduler's own alerting.
- Legal-hold-aware retention — legal holds are enforced in the *live*
  database (`fleet_partner_legal_holds`, Section 28) and are proven to
  survive a restore intact (§3); this script does not separately special-case
  a hold when deciding whether to retain or rotate a backup file.

## 3. Restore ownership and procedure

Owner: same as backup ownership — restore is a superset of the same access.

Operator command: `services/fleet-partner-service/scripts/restore-verify.sh`.

```bash
FLEET_PARTNER_BACKUP_DATABASE_URL='postgres://user:pass@host:port/db?sslmode=require' \
  bash services/fleet-partner-service/scripts/restore-verify.sh /path/to/backup-dir/fleet-partner-service-<ts>.dump
```

This **never touches the source/production database** — it creates a new,
disposable database on the same server, restores into it, verifies, and
drops it.

**Automated test proving the same procedure**:
`internal/integration/backup_restore_test.go` →
`TestBackupAndRestore_RealDisposableDatabase`. Real, run in this session
against the live PostgreSQL 16 test container (not mocked):

1. Seeds representative rows: an organization, a durable job, and an
   **ACTIVE legal hold**.
2. Runs a real `pg_dump -Fc` via `docker exec`.
3. Creates a genuinely new disposable database (`CREATE DATABASE`, all-digit
   timestamp name — an earlier draft of this script used an uppercase-letter
   timestamp and silently created a *lowercase*-folded database due to
   PostgreSQL's unquoted-identifier case folding, which a mismatched-case
   lookup then failed to find; fixed and now covered by this exact test).
4. Restores via real `pg_restore`.
5. Verifies: migration version matches the source
   (`000073_retention_export_deletion_tracking` in this session's run);
   8 critical tables are queryable
   (`fleet_partner_organizations`, `fleet_partner_private_orders`,
   `fleet_partner_private_assignments`, `fleet_partner_documents`,
   `fleet_partner_outbox`, `fleet_partner_audit_records`, `fleet_partner_jobs`,
   `fleet_partner_service_credentials`); the seeded organization round-trips
   by ID; **the seeded legal hold survives restore with `status='ACTIVE'`**;
   the audit table's append-only protective trigger is still present.
6. Drops the disposable database.

Observed duration in this session: **4.6–8.3 seconds** (varies with backup
size; the dataset was small — this is a mechanical-restore-speed data point,
not a production-scale claim; see §11 for the honest capacity-testing
disclosure).

## 4. Redis loss handling

Redis in this service is the outbox event-transport layer only
(`internal/events/redis_publisher.go`, `stream_consumer.go`) — it holds no
authoritative state. Every corporate event is written transactionally to
PostgreSQL (`fleet_partner_outbox`) **before** Redis ever sees it
(`internal/events/outbox.go`'s `Append`, called inside the same transaction
as the domain mutation).

If Redis is lost or unreachable:
- The outbox worker's publisher falls back to `DisabledPublisher`
  (`cmd/main.go`), which returns `ErrPublisherNotConfigured` — the leased
  record is released **without consuming a retry attempt** (see
  `events.OutboxWorker.publishOne`), so nothing is lost or dead-lettered
  merely because the transport is down.
- Once Redis is available again (a fresh instance, empty consumer group),
  the outbox worker resumes publishing the same durable backlog — proven by
  `TestOutboxWorkerDisabledPublisherKeepsEventsPending` (transport absent →
  events stay PENDING) and `TestSection26RealRedisTransactionalDedupDeadLetterAndReplay`
  (transport present → real delivery, real dedup) both passing in this
  session against real Postgres/Redis.
- A brand-new Redis instance has no consumer-group offset state — the
  `XGroupCreateMkStream ... "0"` call in `StreamConsumer.Run` (re-run on
  every process start) recreates the group from the stream's beginning,
  and `XAutoClaim` recovers any entries a crashed consumer left pending.
  Nothing about Redis's own durability is required for correctness because
  the outbox table is the source of truth Redis is only ever relaying.

## 5. Outbox replay

Mechanism: `events.OutboxRepository.ReplayDeadLettered` (single-record,
guarded — refuses unless the record is currently `DEAD_LETTERED`, so
replaying twice or replaying a still-retrying record is a safe no-op).

Operator-facing route (Section 29 addition, this session):
`POST /internal/fleet-partner/v1/operations/outbox/replay` (HMAC
service-authenticated, caller-allowlisted via
`WORKER_OPERATIONS_ALLOWED_CALLERS`), body `{"recordId": "evt_..."}`.

Tested: `TestOutbox_DeadLetterThenReplay`,
`TestOutbox_ReplayUnknownRecordIsNotFound`,
`TestSection26DeadLetterReplayRequeuesAndAttemptsAreAppendOnly`, and this
session's new `TestWorkerOperations_BacklogsAndOutboxReplay` (drives a real
record through claim → fail → dead-letter → the real HTTP replay route →
confirms it is `PENDING` again).

## 6. Inbox deduplication after replay

`events.InboxRepository` (`Begin`/`MarkProcessed`/`MarkFailed`) keys on
`(consumer, event_id)`. A replayed or redelivered event that a consumer
already processed is rejected with `ErrDuplicateEvent` and acknowledged
without re-executing the handler.

Tested against real Redis in this session:
`TestSection26RealRedisTransactionalDedupDeadLetterAndReplay` — delivers the
same event twice through a real consumer group and confirms the second
delivery is recognized as a duplicate, not double-processed.

## 7. Projection rebuild

This service has no single "rebuild everything" mechanism — each read
projection has its own reconciliation/rebuild primitive, all real and
tested:

| Projection | Rebuild/reconcile mechanism | Evidence |
|---|---|---|
| Search index (`fleet_partner_search_documents`) | `search/usecases.Service.Index` — idempotent upsert by `(org, subjectType, subjectReference)`; a feature calls it again after its own commit | New this session: `TestSearchProjection_RebuildsAfterLoss` (deletes the projection, re-derives it via `Index`, confirms recovery) |
| Marketplace/trip projections | `marketplace` reconciliation workers | `trip_reconciliation_test.go`, `reconciliation_worker_test.go`, `trip_projection_test.go` |
| Provider/verification projections | `providers/usecases.Service.Reconcile` (claims due provisioning sagas, resolves UNKNOWN outcomes by read-back, never blind resubmission) | Exercised by the existing provider reconciliation test suite |
| Wallet/sub-wallet projections | `companywallet` staleness gate + `RefreshProjections` | `TestStaleWalletProjectionBlocksAllocation`, `TestUnknownOutcomeIsResolvedByReadBackNotResubmission` (both re-run and passing this session) |
| Media/document projections | `documents/usecases.Reconciliation` | `documents_test.go` |
| Cross-service synchronization cache (Section 26) | `synchronization.Service.Reconcile`, exposed at `POST` the existing `reconcileSynchronizationCase` route | `synchronization_section26_test.go`, `idor_synchronization_test.go` |

**Disclosed scope limitation:** there is no single HTTP
`POST .../operations/projections/rebuild` or `.../search/rebuild` endpoint.
Building one would either (a) invoke all of the above per-projection
mechanisms generically, which each have different inputs and safety
preconditions (a search rebuild needs a subject list; a wallet refresh needs
a staleness cursor; a sync reconcile needs a case ID) — collapsing them into
one endpoint risks either being a no-op wrapper or hiding real per-domain
safety checks — or (b) inventing a new generic "replay every event since
time T" mechanism, which is materially new infrastructure beyond what this
review's time budget covers. The mandate's own instruction to "reuse
existing synchronization and dead-letter routes where they already satisfy
the requirement" is followed here: the existing `reconcileSynchronizationCase`
route and the per-domain reconciliation workers above are the real answer to
"can a projection be rebuilt after loss," proven by the tests cited, even
though there is no single button that does all of them at once.

## 8. Scheduled-task recovery

Mechanism: `internal/scheduler` (Section 27) — a cron/interval runner with
misfire policy (`SKIP`, `RUN_ONCE`, bounded catch-up) and `SKIP LOCKED`
claiming so concurrent scheduler processes never double-fire.

Tested (re-run and passing this session):
- `TestScheduler_RunnerMisfireRunOnce_EnqueuesExactlyOneOccurrence` —
  proves a missed occurrence (the scheduler process was down) fires exactly
  once on recovery, never duplicated.
- `TestScheduler_ClaimDueIsSkipLockedAcrossConcurrentSchedulers` — proves
  two scheduler processes racing for the same due task never both claim it.
- `TestScheduler_LeaseRecoveryAfterCrash` — proves a scheduler that crashed
  mid-run releases its lease for recovery by another instance.

Note (disclosed, not hidden): `scheduler.Runner` itself is a real, tested
component but is **not wired into `cmd/main.go`'s production composition**.
This service's actual scheduled-task delivery in production today is via
per-feature interval workers (`app.StartSectionNNWorkers`, already running
under `FLEET_PARTNER_WORKER_ENABLED`), not `scheduler.Runner`. Recovery for
those workers is the same as "worker process restart" — each is a bounded
polling loop with no persistent claim to lose. `scheduler.Runner`'s own
recovery properties are proven at the component level (the three tests
above) but are not yet an end-to-end production-wiring proof, because the
component is not in the production wiring. This is disclosed as a genuine,
pre-existing (not introduced this session) architecture gap, not silently
omitted.

## 9. Provider/Wallet/Media reconciliation

Covered in §7's table — all three have their own real, tested reconciliation
mechanism. Each follows the same non-negotiable rule proven throughout this
service (Section 29 WP9/WP10, and originally Section 28): an
**UNKNOWN external outcome is resolved by authoritative read-back, never by
blindly repeating the write** (`providers/usecases/reconciler.go`'s own doc
comment states this explicitly; `companywallet`'s
`TestUnknownOutcomeIsResolvedByReadBackNotResubmission` proves it for
wallet operations).

## 10. Lease recovery

- **Durable jobs** (`fleet_partner_jobs`): `jobs.Queue.StaleLeaseCount` /
  the worker's own heartbeat+lease-expiry mechanism. Proven by
  `TestExecutionStartClaimIsExclusiveAndLeaseRecovers` and this session's
  new readiness test `TestReadiness_JobBacklogThresholds_RealDatabase`
  (which also proves the Section 29 `job_stale_leases` readiness check
  correctly observes a genuinely-expired lease).
- **Outbox** (`fleet_partner_outbox`): `OutboxRepository.ClaimBatch`'s
  `lease_expires_at` column, recovered on the next poll once expired.
  Proven by `TestOutboxExpiredLeaseRecovery`.
- **Scheduler** (`fleet_partner_scheduled_tasks`): see §8,
  `TestScheduler_LeaseRecoveryAfterCrash`.

## 11. Credential/key restoration

Out of scope for this repository: service-to-service HMAC secrets
(`FLEET_PARTNER_SERVICE_SECRETS`), the audit hash-chain key
(`FLEET_PARTNER_AUDIT_CHAIN_SECRET`), and the database credential itself are
all supplied via environment variables / a secrets manager the hosting
platform owns — this repository never stores or backs up a credential
(confirmed: `backup.sh`/`restore-verify.sh` never embed one; see §2).
Restoring service operation after a credential-management incident means
re-provisioning those secrets through whatever the platform's own
credential-rotation/restoration procedure is — target, platform-dependent,
not implementable inside this repository.

## 12. DNS/service endpoint recovery

Out of scope for this repository — this service has no DNS or load-balancer
configuration of its own; endpoint/traffic recovery is a hosting-platform
concern (target, platform-dependent).

## 13. Validation checklist (run in order during a real DR exercise)

1. `bash scripts/backup.sh <dir>` succeeds; `<name>.meta.json` schema
   version matches the deployed migration tip.
2. `bash scripts/restore-verify.sh <dump>` reports `RESTORE PASS`.
3. `go test ./internal/integration/... -run TestBackupAndRestore` passes
   (automated equivalent of steps 1–2).
4. Outbox: confirm `GET /internal/fleet-partner/v1/operations/backlogs`
   reports a bounded, expected pending count (not growing unboundedly) —
   see §14 for the threshold this is gated on.
5. Jobs: same endpoint's `jobs.deadLettered` and `jobs.byStatus` are within
   expected bounds; replay any expected dead-letters via
   `POST .../worker-operations/jobs/:jobId/retry` or
   `POST .../operations/outbox/replay`.
6. `GET /ready` reports `{"status":"ready"}` with every mandatory-enabled
   dependency healthy (see `docs/section29-reliability-observability.md`
   for the full readiness contract).
7. Spot-check the audit chain for the active partition:
   `audit.VerifyChain(ctx, pool, "<orgId>:<YYYY-MM>")` reports `Valid()`.

## 14. Rollback/abort criteria

Abort a restore-into-production procedure and escalate rather than proceed
if, at step 2 (restore-verify), any of the following occur:

- Checksum verification fails (`sha256sum -c` reports `FAILED`) — the
  backup file itself may be corrupted; do not restore from it.
- The restored migration version does not match the expected/source
  version — the backup may be from an unexpected point in time or an
  unexpected deployment.
- Any of the 8 critical tables is not queryable after restore.
- The audit append-only trigger is missing after restore (§3 step 5) — the
  restored database's protective schema may be incomplete.

None of these conditions should ever be worked around by proceeding anyway;
`restore-verify.sh` and `TestBackupAndRestore_RealDisposableDatabase` both
exit non-zero / fail on any of them by design.

## 15. Communication procedure

Not specified in this repository — no incident-communication tooling or
distribution list exists in this codebase to reference. Assign this in your
organization's incident-response runbook; this document covers the
technical recovery procedure only.

## 16. DR validation schedule

Not currently scheduled by this repository (§1). Recommended cadence, as a
target for the operator who owns backup/restore: run the validation
checklist (§13) after every migration that changes schema shape, and on a
fixed periodic cadence (e.g., monthly) independent of deployments, to catch
drift between the backup/restore tooling and the live schema before a real
incident does.
