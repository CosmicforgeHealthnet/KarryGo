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
| Operator (executes) | CosmicforgeHealthnet (requested in `deploy/platform/dispatch-070-request.txt`) |
| Reviewer (separate person) | One of ainzkoji, davidodimayo7: whoever approves the run in environment `dispatch-070-transition` (recorded by GitHub and re-checked by the run) |
| Maintenance window (UTC) | Proposed 2026-11-20 08:00 to 2026-11-27 18:00; the transition runs when the reviewer approves inside it (downtime is minutes) |
| Approved image digest | Candidate: `ghcr.io/cosmicforgehealthnet/driver-dispatch-delivery-service@sha256:7acd0c138b096e66bdbc2dc02f0a667d7d6565845b11b065aeb4a0454728b715` (revision `8f992eee`, build run 44479021476). The reviewer confirms or asks for a rebuild. |
| Evidence run before | Latest: 44479278733 (2026-11-19). Re-run at the start of the window. |
| Evidence run after | _run id_ |
| Decision | _approved / refused_ |

The separate approval is a GitHub environment approval (section 6). The
shared `staging` environment is deliberately left without a reviewer rule so
routine automatic deploys are not blocked.

## 5. Status on 2026-11-19

Evidence run 44479278733 (`8f992eee`):

| Precondition | Status |
|---|---|
| 1. `dispatch.env` owned by the deploy account, 600 | **Done.** Deploy-owned, 600, readable by deploy. |
| 2. Dispatch image from current staging | **Done.** Built by "Build Dispatch image (no deploy)" (`.github/workflows/build-dispatch-image.yml`), run 44479021476: digest above, carries 000069-000079. The workflow cannot reach the server (`scripts/release/test_build_dispatch_image.py`). Nothing was deployed. |
| 3. Fleet receipt schema/API | **Done.** Fleet `24f9b6a6`, migrations 000099 (includes 000081). |
| 4. Operator, separate reviewer, window | **Requested** (section 6). Waiting for ainzkoji or davidodimayo7 to approve. |
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

## 6. Reviewed execution mechanism (added 2026-11-19)

**Corrected inventory.** Evidence run 44601578762 lists the files `migrate.sh`
has recorded in the Dispatch database: 000001-000068 as `baseline`, and
000069 (`offline_location_atomic`, one nullable column and two indexes) as
`applied`. So the transition applies exactly the ten files **000070-000079**.
The script refuses any other pending set, and checks that 000069's column
really exists.

**How it runs.**
- **Request:** the operator commits `deploy/platform/dispatch-070-request.txt`
  on staging. It holds the digest above, its revision, the operator, the
  reviewers and the window.
- **Plan job** (no secrets): validates the request, checks the digest is the
  one recorded in this package and exists in GHCR, and writes the plan to the
  run summary for the reviewer.
- **Review gate:** environment `dispatch-070-transition`. Required reviewers
  are ainzkoji and davidodimayo7, with self-review prevented, no admin bypass,
  and the staging branch only. Nothing reaches the server until one of them
  approves.
- **Server job** (environment `staging`, concurrency `staging-release-runtime`):
  re-checks that the starter is an authorized initiator and is the named
  operator. It reads the run's approval record from the GitHub API and requires
  an approval by a named reviewer who is neither the operator nor the
  re-runner. It refuses outside the window. Only then does it run
  `scripts/platform/dispatch-070-transition.sh`.
- **The script** carries out section 3:
  1. Read-only preflight. It refuses on any surprise: a relay row, a live
     lease, an extra running consumer, a supervisor, a wrong revision label, or
     an unexpected pending set.
  2. `restart=no` on every Dispatch container, then stop the running one and
     prove nothing can start.
  3. `pg_dumpall` backup (mode 600), restored into a throwaway container with
     no network and compared.
  4. `migrate.sh` from the new image, then checks of the fence column, the
     fenced-row count (0) and the read-back audit's refusal of a zero-row
     UPDATE.
  5. Start only the digest-pinned image, then check its health, revision and
     final consumer inventory. The old container is kept, renamed
     `-pre070-<time>`, stopped and `restart=no`.
- **On failure:**
  - Before migration nothing in the schema has changed, so the previous
    container is restarted as it was.
  - From migration on, the run **holds**: delivery stays stopped, every row is
    kept, the old image is never restarted and no down migration runs.
- **Tests:** `scripts/release/test_dispatch_070_transition.py` runs the real
  script against a simulated server, covering the happy path, five refusals
  and three holds, plus the workflow and request rules.

**What the reviewer does.** Open the waiting run of "Dispatch 070 transition
(separately reviewed)" in GitHub Actions. Read the plan summary and the
latest "Platform staging evidence (read only)" Dispatch lines. Then use
"Review deployments", choose `dispatch-070-transition`, and approve or reject.
Approving inside the window starts the transition immediately. After it
completes, the operator adds the Fleet-Dispatch PAIR lines to
`deploy/platform/staging-pairings.txt` and records the "Evidence run after"
row above.
