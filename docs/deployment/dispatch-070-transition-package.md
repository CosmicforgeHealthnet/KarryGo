# Dispatch 070 transition package (staging)

Prepared 2026-11-17, updated 2026-11-20 (owner-authorized staging exception), for the reviewed procedure in
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
| Migration head | `dispatch_schema_migrations` 000068 (41 rows). Corrected 2026-11-19: `migrate.sh`'s own record (`karrygo_deploy_migrations`, 72 rows) has 000001-000068 baselined and 000069 applied, so only 000070-000079 are pending (section 6) |
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
4. Explicit versioned authorization by the staging repository owner, the
   named operator, and a bounded execution window, recorded in section 4.
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
| Operator (executes) | CosmicforgeHealthnet (requested in `deploy/platform/dispatch-070-request.txt`) |
| Owner authorization | `OWNER_APPROVED_STAGING_DISPATCH_070` in the versioned request; GitHub trigger and operator must be `CosmicforgeHealthnet` |
| Execution window (UTC) | 2026-11-20 07:00 to 2026-11-27 18:00; owner-authorized one-time staging transition, guarded by preflight and restore checks |
| Approved image digest | Candidate: `ghcr.io/cosmicforgehealthnet/driver-dispatch-delivery-service@sha256:7acd0c138b096e66bdbc2dc02f0a667d7d6565845b11b065aeb4a0454728b715` (revision `8f992eee`, build run 44479021476). The reviewer confirms or asks for a rebuild. |
| Evidence run before | Latest: 44479278733 (2026-11-19). Re-run at the start of the window. |
| Evidence run after | _run id_ |
| Decision | _approved / refused_ |

The owner explicitly requested a one-time staging-only autonomous transition
on 2026-11-20. The protected `dispatch-070-transition` environment remains
unmodified but is not used by this one-time workflow. The ordinary `staging`
environment requires the named initiator; routine deployment rules are unchanged.

## 5. Status on 2026-11-19

Evidence run 44479278733 (`8f992eee`):

| Precondition | Status |
|---|---|
| 1. `dispatch.env` owned by the deploy account, 600 | **Done.** Deploy-owned, 600, readable by deploy. |
| 2. Dispatch image from current staging | **Done.** Built by "Build Dispatch image (no deploy)" (`.github/workflows/build-dispatch-image.yml`), run 44479021476: digest above, carries 000069-000079. The workflow cannot reach the server (`scripts/release/test_build_dispatch_image.py`). Nothing was deployed. |
| 3. Fleet receipt schema/API | **Done.** Fleet `24f9b6a6`, migrations 000099 (includes 000081). |
| 4. Named operator, explicit owner authorization, window | **Authorized by owner on 2026-11-20**, recorded in the versioned request. Execute only in the bounded window as the permitted staging initiator. |
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

## 6. Owner-authorized autonomous staging transition (2026-11-20)

**Reason for revision.** Run 44605516932 was independently approved by
`ainzkoji` but failed before any server changes because the maintenance
window had not opened. The repository owner explicitly requested that this
one-time **staging-only** transition operate autonomously, without another
human review. The routine release gate, rollback exclusions, and protected
`dispatch-070-transition` environment remain unchanged.

**Corrected inventory.** Evidence run 44601578762 lists migrations 000001-000068
as baseline, 000069 as applied and exactly 000070-000079 pending. The runtime
transition script rejects any unexpected pending set, relay row, live lease,
extra consumer, incompatible image or supervisor.

**How it runs.**
- **Versioned authorization.** A commit to
  `deploy/platform/dispatch-070-request.txt` on `staging` includes the exact
  Dispatch image digest, source revision, named owner operator, the literal
  `OWNER_APPROVED_STAGING_DISPATCH_070`, and a bounded execution window.
- **Plan without secrets.** Validates the authorization, operator, digest,
  image availability and request window, then records the plan in GitHub.
- **Staging-only execution.** The transition job checks that GitHub actor and
  named operator are both `CosmicforgeHealthnet`, that the actor is permitted
  by `STAGING_RELEASE_INITIATORS`, that authorization matches, and that the
  window is open. It runs inside environment `staging` with release
  concurrency serialization; there is no independent-review job.
- **No shortcut around migration safeguards.** The unchanged
  `scripts/platform/dispatch-070-transition.sh` performs strict read-only
  preflight; disables all incompatible Dispatch restart paths; stops old
  consumers; makes a mode-600 PostgreSQL dump and checks an isolated restore;
  applies only migrations 000070-000079; validates the read-back audit and
  fence; then starts only the digest-pinned healthy image.
- **Failure behavior.** Before any migration, safely restores the exact old
  container. From the first migration onward, HOLD prevents restarting an
  incompatible old binary or reversing an irreversible migration. No
  down-migration or lease/receipt reset is allowed.
- **Tests.** `scripts/release/test_dispatch_070_transition.py` exercises the
  real migration shell script with simulated-server happy path, refusals,
  restore failure and post-migration holds, plus static authorization guards.

On success, apply and verify the existing Fleet–Dispatch pairing, rerun
read-only staging evidence and record the new running revision, migration
head and signed read-back results. A healthy image alone does not certify
the signed-in customer/provider journeys.
