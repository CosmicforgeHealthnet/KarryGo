# FINAL_LOAD_TEST_EVIDENCE

Phase baseline `9ebad225ea9ec25397778a4dc8d3ff842f2938a3`.
No production data and no real money were involved in any measurement here.

## Summary

Seven of eight load gates are `NOT RUN`, for one reason: they are HTTP-level
load against deployed services, and there are none
(`FINAL_DEPLOYED_STAGING_EVIDENCE.md`). Putting load on a laptop process and
reporting the numbers would measure the laptop.

`LOAD-005` is the exception, because it is the **financially** critical path
and its contention lives at the database, which can be driven honestly at the
usecase boundary. It is reported as `PASS — DB-BACKED`, never as staging load.

Full per-gate status and the measured table are in
`FINAL_BACKEND_CERTIFICATION_MATRIX.md` §5 and §9; this document exists so the
measurement has a named home and is not buried.

## LOAD-005 — managed settlement posting

Environment: throwaway PostgreSQL 16 database, production pool settings
(`MaxConns=10`), 4 CPUs. Gated behind `WALLET_LOAD_TEST` so an ordinary build
never pays for it. Each request is a distinct real instruction posting one
balanced journal.

| Concurrency | Requests | Duration | Throughput | p50 | p95 | p99 | max | Errors | Timeouts | Pool empty-acquire waits |
|---|---|---|---|---|---|---|---|---|---|---|
| 4 | 400 | 1.025s | 390.4/s | 5.655ms | 28.215ms | 41.446ms | 54.114ms | 0 (0.00%) | 0 | 3 |
| 8 | 400 | 1.172s | 341.4/s | 5.788ms | 103.158ms | 203.744ms | 227.367ms | 0 (0.00%) | 0 | 7 |
| 16 | 400 | 1.220s | 327.9/s | 24.193ms | 170.465ms | 230.784ms | 325.803ms | 0 (0.00%) | 0 | 399 |

Duplicate storm — 400 replays of one instruction:

| Concurrency | Duration | p50 | p95 | p99 | Errors | Journals |
|---|---|---|---|---|---|---|
| 4 | 243ms | 2.247ms | 2.699ms | 5.495ms | 0 | 1 |
| 8 | 320ms | 5.918ms | 8.241ms | 18.485ms | 0 | 1 |
| 16 | 313ms | 11.532ms | 16.754ms | 31.828ms | 0 | 1 |

Raw transcript: `KARRYAM_SECTION19_24_LIVE_EVIDENCE.txt`.

### Reading it

Throughput does not rise with concurrency; it falls slightly (390 → 341 → 328
per second) across a 4× increase, while p99 grows 5.6×. That is a bottleneck
**downstream of the connection pool** — a single PostgreSQL serializing
balanced-journal writes. At concurrency 16, 399 of 400 acquisitions waited and
the pool sat at its maximum of 10.

The consequence matters more than the numbers: **raising `MaxConns` would not
add throughput.** It would move the same queue from the pool, where it is
visible as `empty_acquire_waits`, into the database, where it is harder to see
and where it competes with every other query.

The duplicate storm being ~10× cheaper at p95 than real postings (16.8ms vs
170.5ms at the same concurrency) is the intended shape: a replay
short-circuits on the unique constraint before doing journal work. A retry
storm is survivable, and it produced exactly one journal at every level.

### Proposed guardrail, not a met SLO

No official SLO exists, and none is invented. The measured baseline suggests
holding settlement concurrency at or below the connection-pool size, and
alerting on `empty_acquire_waits` rising rather than on latency alone — latency
is the symptom, saturation is the cause. This is a proposal for the launch
guardrail and must be re-derived from real staging traffic.

### Resource and backlog effects

Outbox backlog growth during the test: none — this path posts synchronously and
does not enqueue. Recovery after test: the database was dropped; no residue.
CPU and memory were not instrumented, which is another reason this is not
offered as SLO evidence.
