# Prerequisite Stabilization Report

Tracks the work moving the repository from **NO-GO** to **READY FOR SECTIONS 13–15**.

## WP0 — Worktree inventory and backup

| Field | Value |
| --- | --- |
| Branch | `staging` |
| Starting commit | `7320aaaeb3bb7427b1ffb15c003bc6389d549c1f` |
| Tracked modified files | 5 |
| Untracked files | 246 |
| Total preserved | **251** |
| Backup location | `C:\KarryGoPush-backup-20260726-213208` |

The audit's reported counts were verified rather than assumed; both matched exactly.

### Tracked modifications (5)

```
go.work
services/driver-dispatch-delivery-service/.env.example
services/driver-dispatch-delivery-service/cmd/main.go
services/driver-dispatch-delivery-service/internal/config/config.go
services/driver-dispatch-delivery-service/internal/router/router.go
```

### Untracked distribution

| Area | Files |
| --- | --- |
| `services/fleet-partner-service` | 215 |
| `services/driver-dispatch-delivery-service` | 35 |
| `go.work` (tracked, modified) | 1 |

**No unrelated service has any change.** Taxi, hauling, wallet, customer,
media, notification, verification and analytics are all untouched.

### Backup method

Every tracked-modified and untracked file was copied outside the repository,
preserving relative paths, with a full inventory at `INVENTORY.txt`. Verified
251 inventory entries → 251 files copied.

`git clean` was not used. Nothing was staged, committed, reset or restored.

## WP1 — Dispatch schema mismatch: `trips.package_type`

### Root cause: stale test harness, not a schema defect

`migrations/000029_add_package_fields_to_trips.up.sql` adds `package_type`,
and the column is present in the real chain. The failure was in the test:

`internal/features/trip/repository_integration_test.go` built an isolated
schema and applied a **hand-picked subset** of migrations —
000004, 000019, 000021, 000022, 000023, 000024 — omitting 000029 entirely.

The test was therefore asserting against a schema **no deployment ever runs**.
Classification, per the work package's own list: *a migration not applied by
the test harness*. Not a missing migration, not a stale query, not a
production defect.

### Fix

The harness now globs and sorts the real `*.up.sql` chain and applies it in
order, plus an explicit assertion that `trips.package_type` exists before the
test proceeds. A hand-maintained subset silently stops covering whatever a
later migration adds — the same self-maintenance failure previously found in
the fleet integration TRUNCATE list. Deriving the list means a new migration is
covered the moment it lands.

No migration was rewritten. No expected field was removed from the test.

### Second finding: the legacy chain is not blindly re-appliable

A no-op second apply of the whole chain fails at
`000014_create_bikes.up.sql`. At least **eight tracked legacy migrations**
(000014, 000015, 000016, 000021, 000022, 000023, 000024, 000025) use bare
`CREATE TABLE` without `IF NOT EXISTS`.

This is pre-existing and by design: the chain is applied once per version by a
versioned runner, not re-executed. Blind re-application was the wrong test for
it. The three new marketplace migrations (000031/000032/000033) **are** fully
idempotent and were each verified apply → no-op → rollback → reapply
individually.

No legacy migration was modified, per the instruction not to rewrite applied
migrations.

### WP1 proof

| Check | Result |
| --- | --- |
| Fresh apply of full chain into a clean schema | PASS |
| `trips.package_type` present after chain | PASS |
| `000029` rollback removes the column | PASS (1 → 0) |
| `000029` reapply restores it | PASS (0 → 1) |
| `TestPostgresRepositoryTripFoundation` | **PASS** (was skipping) |
| Full dispatch suite, run twice | **1152 pass / 0 fail / 0 skip** |
| Customer booking + independent-provider regressions | green within the above |

### Third finding: a silently skipping test

`TestPostgresRepositoryTripFoundation` reads
`DISPATCH_TRIP_TEST_DATABASE_URL` or `TEST_DATABASE_URL` — not the
`DISPATCH_TEST_DATABASE_URL` used elsewhere. It had been skipping silently
rather than failing, which is why the schema drift went unnoticed.

Separately, `acceptance_to_canonical_trip_harness_test.go` appeared to be
incomplete scaffolding — it skipped unconditionally, seemingly waiting for a
test that did not exist — and was removed on that basis.

**That judgement was wrong, and WP2 caught it.** The file is the dispatch half
of a two-process end-to-end test, and `TestAcceptanceToOneCanonicalTrip` does
exist, in the fleet integration package. Removing it broke that test. It was
restored byte-identical from the WP0 backup during WP2. See the WP2 section.

## WP2 — Full fleet and dispatch PostgreSQL suites

### Environment

| Variable | Value |
| --- | --- |
| `FLEET_PARTNER_TEST_DATABASE_URL` | `karrygo_fleet_partner_test` on :5446 |
| `DISPATCH_TEST_DATABASE_URL` | `karrygo_dispatch_test` on :5447 |

Both are read by the fleet harness: the second is required by the
cross-service end-to-end test. Isolation verified — distinct ports, distinct
databases, and zero dispatch tables present in the fleet database.

### Baseline

329 s, **207 pass / 1 fail / 0 skip**. Skips were counted from verbose output;
a non-verbose run cannot display them and its skip count means nothing.

### Root cause 1 — a regression I introduced in WP1

`TestAcceptanceToOneCanonicalTrip` failed with
`dispatch helper exited before readiness`.

The fleet end-to-end test spawns the dispatch half as a child process:

```go
exec.CommandContext(childCtx, "go", "test", "-count=1",
    "-run", "^TestAcceptanceToCanonicalTripDispatchHarness$", "./internal/router")
```

That harness is exactly the file WP1 removed as "dead scaffolding". It was not
scaffolding — it is the dispatch process of a two-process test, and its
unconditional-looking skip is a guard on
`KARRYGO_CROSS_SERVICE_DISPATCH_HELPER`, which the fleet test sets when it
launches the child.

Restored byte-identical from the WP0 backup. The end-to-end test passes again.

**Lesson recorded:** a test that always skips standalone is not necessarily
dead. Before deleting one, find its caller.

### Root cause 2 — per-test migration re-application

No hang, no lock wait, no leaked goroutine, no pool exhaustion. The
`internal/integration` package accounted for 305 s of the 329 s, spread evenly
across 103 tests at roughly 3 s each; the slowest single test was 5.76 s.

`testPool` re-applied all 11 migrations on every call. They are idempotent, so
this was correct but wasteful — it changed no outcome. Isolation comes from
the per-test TRUNCATE and catalogue re-seed, which are retained unchanged.

Applied `sync.Once` so migrations run once per test binary.

**329 s → 208 s / 191 s (about 40 % faster) with no assertion weakened, no
test skipped and no coverage removed.**

### Migration verification

The fleet chain is 11 up migrations, each with a matching down file, no
duplicate numbers. Fresh apply verified. As recorded under WP1, the dispatch
legacy chain is versioned rather than blindly re-appliable; the fleet chain
applies cleanly and the test schema matches repository expectations, proven by
the suite passing.

### Results

| Run | Result | Runtime |
| --- | --- | --- |
| Fleet run 1 | **208 pass / 0 fail / 0 skip** | 208 s |
| Fleet run 2 | **208 pass / 0 fail / 0 skip** | 191 s |
| Dispatch regression | **1152 pass / 0 fail / 1 skip** | — |

Idle-in-transaction sessions after the runs: **0** — no leaked database state.

The single dispatch skip is `TestAcceptanceToCanonicalTripDispatchHarness`,
the helper process described above. Skipping when run standalone is its
correct behaviour, and its coverage *is* exercised — through the fleet
end-to-end test, which passes. It is not a missing-coverage skip.

`go vet` and `go build` pass for both services.

## Remaining packages

WP3 through WP14 are not started. The next package is **WP3** — the durable
dispatch marketplace outbox delivery worker.
