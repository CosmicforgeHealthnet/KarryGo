# Dispatch 070 transition package (staging)

Prepared 2026-11-17 for the reviewed procedure in
`docs/deployment/staging-release-approval.md`, "Dispatch 070
stop/migrate/verify/start and recovery hold". This package is preparation
only. Nothing here has been executed, and nothing here is an approval.

## 1. Inventory (procedure step 1), read-only evidence run 44185288548

Source: "Platform staging evidence (read only)" on `f0dbb379`, which now
includes a Dispatch relay inventory (aggregate counts only).

| Item | Value |
|---|---|
| Running image | `driver-dispatch-delivery-service:9caa16b6` (2026-10-21), healthy |
| Settings file | `dispatch.env` root:root 600, not readable by the deploy account |
| Migration head | `dispatch_schema_migrations` 000068 (41 rows); 000069-000079 pending |
| Relay rows (`dispatch_fleet_relay_operations`) | **0 rows** in any status |
| Rows 000070 would fence (UNKNOWN_OUTCOME, DELIVERING, DEAD_LETTER) | **0** |
| Live leases | 0 |
| `readback_required` column | absent (000070 not applied) |
| Read-back audit table (000071) | absent |
| Consumer containers (any state) | `karrygo-dispatch-delivery-service` running, restart `unless-stopped`; `karrygo-dispatch-delivery-service-before-email-20261016-090932` exited, restart `unless-stopped`, image `:latest`; `karrygo-dispatch-email-diagnostic` exited, restart `no` |
| Systemd / cron supervisors | none matching karrygo or dispatch-delivery (re-confirm with the narrowed match on the next run) |

What this means for risk: there is no in-flight or uncertain Fleet decision
to preserve, so the fence migration has nothing to fence and the backfill in
000070 updates zero rows. The transition is still a stop/migrate/start
because old binaries must not run against the new schema, even briefly.

One hazard found: the exited `...-before-email-20261016-090932` container
has restart policy `unless-stopped` and image tag `:latest`. A Docker daemon
restart does not start a container that was stopped manually, but it is an
old incompatible consumer with a restart path. Step 2 must remove it (or set
`--restart no`) and prove it cannot start.

## 2. Preconditions (all required before step 2 of the procedure)

1. `dispatch.env` owned by the deploy account, mode 600 (same owner action
   as Wallet/Taxi/Hauling/Customer).
2. A Dispatch image built from current `staging` (the automatic deploy
   excludes Dispatch; build it through the approved release path).
3. Fleet receipt schema and API in place: Fleet runs `24f9b6a6` with
   migrations through 000099, which includes offer receipts (000081).
4. Named operator, a separate reviewer, and an approved maintenance window,
   recorded in section 4.
5. Fresh Dispatch database backup taken immediately before step 3, and its
   restore checked.

## 3. Execution outline (maps to procedure steps 2-5)

1. Re-run the evidence workflow; confirm relay rows are still 0 and no new
   consumer exists.
2. Stop `karrygo-dispatch-delivery-service`; remove or set `restart=no` on
   both exited Dispatch containers; re-run the consumer inventory and prove
   zero Dispatch containers can start.
3. Back up the Dispatch database (`pg_dumpall` in `karrygo-dispatch-delivery-db`).
4. Apply 000069-000079 with the new image's `migrate.sh` while no Dispatch
   binary runs. Check: `readback_required` exists, 0 rows fenced, the
   read-back audit table exists and rejects UPDATE/DELETE.
5. Start only the new image. Verify health, signed GET read-back to Fleet,
   no POST retry fallback, lease exclusion, no duplicate decisions.
6. On any failure: stop delivery, keep every row, prefer a forward fix.
   000070/000071 down migrations refuse by design; do not restart the old
   image.

## 4. Approval record (to be completed by the people named)

| Field | Value |
|---|---|
| Operator (executes) | _name / GitHub login_ |
| Reviewer (separate person) | _name / GitHub login_ |
| Maintenance window (UTC) | _start - end_ |
| Approved image digest | _ghcr digest of the Dispatch build_ |
| Evidence run before | _run id_ |
| Evidence run after | _run id_ |
| Decision | _approved / refused_ |

The existing mechanism for the separate approval is GitHub environment
`staging` required reviewers with self-review prevention. As of 2026-11-17
that rule is not configured (only a branch policy; admin bypass is off), so
the approval must be recorded here until it is.
