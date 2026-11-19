# Dispatch 070 transition package (staging)

Prepared 2026-11-17, refreshed 2026-11-19 (section 5), for the reviewed procedure in
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
| Approved image digest | Candidate: `ghcr.io/cosmicforgehealthnet/driver-dispatch-delivery-service@sha256:7acd0c138b096e66bdbc2dc02f0a667d7d6565845b11b065aeb4a0454728b715` (revision `8f992eee`, build run 44479021476). The reviewer confirms or asks for a rebuild. |
| Evidence run before | Latest: 44479278733 (2026-11-19). Re-run at the start of the window. |
| Evidence run after | _run id_ |
| Decision | _approved / refused_ |

The existing mechanism for the separate approval is GitHub environment
`staging` required reviewers with self-review prevention. As of 2026-11-17
that rule is not configured (only a branch policy; admin bypass is off), so
the approval must be recorded here until it is.

## 5. Status on 2026-11-19

Evidence run 44479278733 (`8f992eee`):

| Precondition | Status |
|---|---|
| 1. `dispatch.env` owned by the deploy account, 600 | **Done.** Deploy-owned, 600, readable by deploy. |
| 2. Dispatch image from current staging | **Done.** Built by "Build Dispatch image (no deploy)" (`.github/workflows/build-dispatch-image.yml`), run 44479021476: digest above, carries 000069-000079. The workflow cannot reach the server (`scripts/release/test_build_dispatch_image.py`). Nothing was deployed. |
| 3. Fleet receipt schema/API | **Done.** Fleet `24f9b6a6`, migrations 000099 (includes 000081). |
| 4. Operator, separate reviewer, window | **Open.** The staging environment still has no required-reviewer rule (only a branch policy; admin bypass off), so the approval is recorded in section 4. |
| 5. Fresh backup + restore check | At execution (step 3 of section 3). |

Inventory unchanged since 2026-11-17: relay table 0 rows, 0 rows to fence,
`readback_required` absent, read-back audit table absent, migration head
000068 (41 rows). One running consumer (`9caa16b6`, restart
`unless-stopped`); the exited `...-before-email-20261016-090932` container
(restart `unless-stopped`, image `:latest`) is still the hazard step 2 must
remove; `karrygo-dispatch-email-diagnostic` is exited with restart `no`. No
systemd or cron supervisor runs Dispatch (`karrygo-close-public-ports` is a
firewall unit).

Dispatch is not paired with Fleet in `deploy/platform/staging-pairings.txt`;
add its PAIR lines after the transition, not before.
