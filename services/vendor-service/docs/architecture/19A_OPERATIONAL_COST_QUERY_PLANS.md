# Section 16D - scale fixtures and query-plan evidence

Captured with `EXPLAIN (ANALYZE, BUFFERS)` against the synthetic scale corpus,
on real PostgreSQL 16. Reproduced by
`internal/features/cost/scale_test.go` and `queryplan_test.go`.

Plans over an empty table say nothing - PostgreSQL sequentially scans anything
small, correctly - so every measurement below is over the corpus.

## Fixture

Synthetic, built set-based with `generate_series`. Tenant and branch
relationships are real: every row belongs to an actual business and branch, so
the predicates under test have something to exclude.

| Table | Rows |
|---|---|
| `vendor_ingredients` | 5,000 |
| `vendor_packaging_materials` | 1,000 |
| `vendor_recipes` | 2,000 |
| `vendor_recipe_versions` | 4,000 |
| `vendor_recipe_components` | 20,000 |
| `vendor_recipe_packaging_requirements` | 4,000 |
| `vendor_purchase_cost_history` | 50,000 (1 in 20 reversed) |
| `vendor_production_batches` | 5,000 |
| `vendor_cost_snapshots` | 20,000 |
| `vendor_cost_snapshot_components` | 80,000 |
| `vendor_cost_policies` | 6,000 (5,000 business + 1,000 branch overrides) |
| Businesses / branches | 2 / 2 |

Build time: **44s**. The corpus is built once and reused - recipe versions and
cost snapshots are immutable, so re-seeding an existing corpus would be refused
by the database rather than merely wasteful.

## Plans

All twelve access paths reach their data by index and execute in **under 1ms**.

| # | Path | Top node | Execution |
|---|---|---|---|
| 1 | Business cost policy lookup | Index Scan `..._business_scope_uq` | 0.069 ms |
| 2 | Branch override lookup | Limit over index | 0.163 ms |
| 3 | Latest accepted purchase cost | Limit over index | 0.228 ms |
| 4 | Bounded weighted-average window | Limit (10 rows) | 0.134 ms |
| 5 | Recipe component costing | Index Scan `..._version_sequence_idx` | 0.074 ms |
| 6 | Latest recipe snapshot | Limit over index | 0.054 ms |
| 7 | Recipe cost history page | Limit over index | 0.072 ms |
| 8 | RecipeVersion cost history | Limit over index | 0.051 ms |
| 9 | Production expected cost | Index Scan `..._batch_idx` | 0.033 ms |
| 10 | Production actual cost | Index Scan `..._batch_idx` | 0.036 ms |
| 11 | Production cost history page | Limit over index | 0.129 ms |
| 12 | Component evidence for a snapshot | Sort over index | 0.097 ms |

Notably, path 3 answers a single-item cost question over 50,000 purchase rows
without scanning the table, and its `NOT EXISTS` reversal check does not
degrade that.

Assertions are deliberately narrow. The tests do **not** demand "no Seq Scan
anywhere" - that pins a planner decision rather than an architectural property.
They assert that the required indexes exist, that `business_id` reaches every
plan, that bounded reads carry a `Limit`, and that the two large tables are
never fully scanned to answer a bounded question.

## One real defect found and fixed

**Highest-cost ingredients read a full component scan.**

Written as a single statement with the snapshot window as a CTE, the planner
hash-joined the bounded 500-snapshot window against the *entire* component
table:

```
Hash Join  (rows=2000)
  ->  Seq Scan on vendor_cost_snapshot_components  (rows=80000)   27ms
Execution Time: 36.8 ms
```

The window was bounded but the work was not: cost grew with total evidence
rather than with the window, so at 1M component rows it would have been
~350ms.

Forcing the index nested loop measured **4.9ms - 7.3x faster** - proving the
better plan existed and the planner was simply not choosing it. Three
query-shape changes were tried and none moved it: an explicit `= ANY(array)` of
snapshot ids (24.8ms), a `LATERAL` join (24.0ms), and `WITH ... AS MATERIALIZED`
(39.5ms). At 2.5% selectivity the cost model genuinely prefers the scan.

The fix is a covering index, added to migration `000074` (still unreleased and
local-only at the time):

```sql
CREATE INDEX vendor_cost_snapshot_components_window_idx
    ON vendor_cost_snapshot_components (business_id,snapshot_id)
    INCLUDE (subject_type,subject_ref,component_cost_minor,currency);
```

Including the aggregated columns makes it an index-only scan, which the planner
does choose:

```
Index Only Scan using vendor_cost_snapshot_components_window_idx  (rows=2000)
Execution Time: 3.1 ms
```

**24.8ms -> 3.1ms**, and now bounded by the window rather than by total
evidence. `HighestCostSubjects` was also split into two bounded queries (the
window, then the aggregate over an explicit id set) so the planner sees an
accurate row estimate. Two round trips regardless of data size - not an N+1.

## Measured read timings

From `TestScaleReadsStayBoundedAndTenantScoped`:

| Read | Time |
|---|---|
| Branch-override policy lookup | 24.1 ms (first call, includes connection warm-up) |
| Latest accepted purchase cost over 50,000 rows | 3.9 ms |
| Bounded weighted-average window (10 rows) | 1.1 ms |
| Batched 40-item cost resolution | 11.7 ms vs 17.2 ms serial (1.5x) |
| Recipe cost history page over 20,000 snapshots | 13.2 ms |
| Production cost history page | 1.6 ms |
| Highest-cost subjects over 80,000 component rows | 20 ms -> 3.1 ms after the index |

The batched resolution is the N+1 guard: recipe costing resolves every
component's purchase evidence in one query, and the test fails if the batched
read is ever slower than doing it one item at a time.

## Isolation at scale

Every read above is also asserted to return nothing for a tenant that owns none
of the corpus, and nothing for a branch that owns none of it. Bounded pagination
is asserted by requesting 10,000 rows and checking the result is clamped.

## A note on what the tests assert about plans

An earlier version of the aggregate test asserted the plan SHAPE - that the
covering index must be chosen and no sequential scan may appear. That was
wrong, and it failed on a second database holding the same corpus, because the
planner's choice depends on statistics that differ between databases.

Pinning a planner decision is precisely what this file's own tests warn
against. The assertion was replaced with the property that must hold either
way: the aggregate is bounded to the rows reachable from the snapshot window,
never to the whole component table. The index's existence is asserted
separately, and the measured improvement when it is chosen is recorded above.

The covering index is kept because it makes the better plan available - on the
database where it was chosen it was 8x faster - not because any test forces the
planner to use it.
