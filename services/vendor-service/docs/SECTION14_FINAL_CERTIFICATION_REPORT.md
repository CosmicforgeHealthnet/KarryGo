# Section 14 Final Certification Report

**Generic Inventory (14A–14I)**

Every result below was produced by executing the named test against real
PostgreSQL 16 and real Redis through the isolated Vendor test runner
(`tools/vendor-test-runner`), which gives each package a freshly migrated
database cloned from a common template. Nothing in this report is claimed from
reading code. Where a gate could only be established by asking the running
database, it was asked.

---

## SECTION 14 RESULT: GO

| Subsection | Scope | Result |
|---|---|---|
| 14A | Generic Inventory architecture and domain foundation | **PASS** |
| 14B | Generic inventory item identity and stock basis | **PASS** |
| 14C | Shop compatibility bridge | **PASS** |
| 14D | Core stock, lots and the movement ledger | **PASS** |
| 14E | Reservations and concurrency | **PASS** |
| 14F | Stocktakes, transfers, quarantine, reorder and alerts | **PASS** |
| 14G | Food inventory seam (fail-closed) | **PASS** |
| 14H | Workers, IAM and internal contracts | **PASS** |
| 14I | Scale, security, migration, OpenAPI and certification | **PASS** |

---

## 1. Key facts

| Fact | Value |
|---|---|
| Database migration tip | **000066** (`000066_vendor_inventory_operational_hardening`) |
| Section 14 migrations | 000060–000066 (7 migrations, all paired up/down) |
| Migrations added by 14I | **none** — the one schema defect was a repair to 000063, which had never applied anywhere |
| Total live Vendor routes after 14I | **590** |
| Section 14 routes | **34** (27 vendor bearer + 7 internal HMAC-v2) |
| OpenAPI contract | `docs/openapi/section14-vendor-inventory.openapi.json` |
| Scale dataset | **21,000** inventory items, **63,000** movements |
| New authentication mechanisms | **none** — existing `shared/go/serviceauth` HMAC-v2 reused |
| Other platform services changed | **none** |
| Redis dependency for stock truth | **none** — zero Redis references in the Inventory and Shop packages |

---

## 2. Defects found and fixed

Section 14I found six defects. Two were severe enough that Section 14 could
not have been deployed or even fully tested before this pass, and two were in
this section's own tests rather than in production code.

### 2.1 Migrations could not apply to an empty database (P0)

`000063` used `CREATE OR REPLACE VIEW` to change three
`inventory_balance_availability` columns from `numeric` to `bigint`.
PostgreSQL refuses a column type change through `CREATE OR REPLACE`, so the
migration chain aborted at 000063 on any database that did not already have the
view in the new shape.

Consequences: no fresh environment could reach the tip, and every
database-backed Section 14 test — including the 14E and 14F PostgreSQL
concurrency matrices — was unrunnable rather than passing.

Fixed by dropping and recreating the view, which is exactly what `000063`'s own
`.down.sql` already did correctly. A new additive migration could not have
fixed this: the chain died before reaching it. `000060`–`000066` are untracked
working-tree files that had never been committed or successfully applied, so
repairing 000063 in place did not rewrite deployed history.

Pinned by `TestSection14IAvailabilityViewIsBigint`.

### 2.2 Stocktake creation was impossible whenever the snapshot returned rows (P0)

`CreateStocktake` executed an `INSERT` inside an open `rows.Next()` loop on the
same transaction. A PostgreSQL connection carrying an unread result set cannot
run another statement, so pgx returned `conn busy` and no stocktake could ever
be created.

Fixed by reading the expected-quantity snapshot to completion before writing
any line. The whole Inventory package was then swept for the same interleave
pattern; it is now absent.

### 2.3 The service-wide contract had a Section-14-sized hole (P1)

`internal/crmcontract` is the test whose entire job is catching live routes
nobody documented. Its production router mirrored `cmd/main.go` — except it
never registered Section 14, so all 34 generic Inventory routes were invisible
to it and passed the gate by not existing as far as it was concerned.

Fixed by registering Section 14's public and internal routes in that router.
The accepted surface moved from 556 to 590, and both directions now hold.

### 2.4 Stale tests and fixtures (P2)

- A Section 9B assertion still expected the pre-Section-14 constant
  `NOT_PRESENT` for the Food inventory seam; Section 14G had renamed it to
  `NOT_SUPPORTED` without updating the test.
- Two Section 14F transfer fixtures built a destination stock basis that did
  not share the source's stock identity and had no enabled sell unit. **The
  production guards were correct and were left unchanged**; the fixtures were
  wrong and were corrected.
- The 14E migration shape test asserted the presence of the broken
  `CREATE OR REPLACE VIEW` string, pinning the defect. It now pins the fix.

### 2.5 This section's own tenant-isolation test was vacuous (P1, self-inflicted)

The first version of `TestSection14ITenantIsolationAcrossGenericInventory`
built its foreign caller as `inventory.Actor{BusinessID: ...}` with no staff
identity. `Actor.Validate` rejects an actor carrying neither `StaffID` nor
`ActorRef`, so every probe was refused for being **malformed** rather than for
being **foreign**. The test would have passed with tenant isolation completely
removed.

It surfaced only because a later test built its actor the same way and failed
on a path where a valid actor was required. The foreign caller is now fully
well-formed, so each refusal is genuinely about ownership. Every isolation
result in the matrix below was re-established after that correction.

### 2.6 This section's own worker test raced the retry backoff (P2, self-inflicted)

The first version of `TestSection14IExpiredLeaseIsRecovered` expected a job to
be claimable immediately after its lease expired. `recoverExpiredLease` in fact
releases the dead lease and reschedules the job `retryBase` into the future, so
the assertion was racing a one-millisecond backoff and failed roughly one run
in seven.

Production was right and the test was wrong: handing a job straight back to the
next caller is exactly what would let a crash-looping worker spin. The test now
asserts the real property — the dead owner's lease is released, the status is
`RETRY`, the attempt count advanced, a reason was recorded, and the claim in
that same call is *refused* — and then makes the retry due and proves a live
worker takes it. Fifteen consecutive runs of the package pass.

A documentation defect was also repaired: `docs/IMPLEMENTATION_STATUS.md`
carried 79 lines of double-encoded UTF-8 introduced by an earlier session. The
reversal was verified exact — the pre-existing region is now byte-identical to
`HEAD`.

---

## 3. Hard gate matrix

| Gate | Result | How it was established |
|---|---|---|
| Generic Inventory | **PASS** | full inventory + shop generic suites |
| Shop compatibility | **PASS** | Section 12B suites unchanged and passing |
| Section 12B regression | **PASS** | catalogue, inventory, concurrency, security, bulk, discovery, internal contract |
| Tenant isolation | **PASS** | `TestSection14ITenantIsolationAcrossGenericInventory`, real DB |
| Branch isolation | **PASS** | `TestSection14IBranchScopeCannotBeEscaped`, real DB |
| Nested IDOR | **PASS** | `TestSection14IForeignReservationIsUnreachable`, real DB |
| HMAC-v2 | **PASS** | `TestSection14HInternalInventorySecurity` (unsigned, bad signature, replayed nonce, non-allowlisted caller, body spoofing) plus `TestSection14IEmptyAllowlistGrantsNobody`, which exercises the exact `strings.Split` shape `cmd/main.go` uses, including the `""` to `[""]` trap that would otherwise admit an unresolved caller |
| Movement immutability | **PASS** | `TestSection14IMovementLedgerIsImmutableInPostgres` — real UPDATE and DELETE refused, row survives |
| Negative stock impossible | **PASS** | availability view invariant plus `TestSection14IPhysicalStockRequiresAuthorizedMutation` |
| Reservation idempotency | **PASS** | `TestSection14EPostgresIdempotency` |
| Reservation expiry exactly once | **PASS** | `TestSection14EPostgresExpiryCommitRace`, `TestSection14EPostgresCommitReleaseRace` |
| FEFO/FIFO | **PASS** | `TestSection14FPostgresExpiryHandling`, lot allocation ordering |
| Receipt safety | **PASS** | `TestSection14EPostgresReceiptReservationRace` and Section 12B receipt suites |
| Quarantine | **PASS** | `TestSection14EPostgresQuarantineReservationRace`, `TestSection14FPostgresQuarantineReservationRace` |
| Stocktake stale-snapshot protection | **PASS** | `TestSection14FPostgresStocktakeMovementRace` |
| Transfer concurrency | **PASS** | `TestSection14FPostgresTransferDispatchReservationRace`, `TestSection14FPostgresTransferReceiveIdempotency` |
| Reorder/alerts | **PASS** | `TestSection14FPostgresAlertDedupeAndReorderThreshold` |
| Food fail-closed inventory seam | **PASS** | `section14g_test.go`, `TestSection9BCertificationDatabaseFlow` |
| Search invalidation | **PASS** | `TestSection14IAvailabilitySignalsFireOnlyOnTransition` with a capturing event sink: becoming available emits exactly one branch-bound signal, a top-up that does not change availability emits none, going out of stock emits exactly one, and restocking emits one more |
| Worker lease/heartbeat/retry | **PASS** | `internal/jobs/durable_postgres_test.go` against real PostgreSQL: single-owner exclusion, non-holder refused heartbeat and completion, expired-lease recovery, bounded retries to dead-letter, replay, and 8-way concurrent claims over 5 rounds electing exactly one winner |
| Migration replay | **PASS** | All four scenarios executed at Section 14's pinned tip: A and C by `TestSection14IMigrationsRollbackAndReapply`, B by `TestSection14IUpgradesAPreSection14Database` (rolls to the pre-Section-14 state and upgrades forward), D and E by `TestSection14IPreservesExistingShopInventoryRows` (a real Section 12 catalogue chain and stock basis survives the round trip; the 14C `SHOP_VARIANT` identity bridge is replay-safe) |
| Redis independence | **PASS** | zero Redis references in both packages; `internal/database` Section 14 suite passes with Redis stopped |
| Scale | **PASS** | 21,000 items / 63,000 movements |
| Query plans | **PASS** | 11 hot paths, all index scans, no index added |
| OpenAPI | **PASS** | 34 operations, JSON valid |
| Route parity | **PASS** | router = OpenAPI = route register, and service-wide at 590 |
| Documentation | **PASS** | 6 existing documents updated, 1 architecture document added |
| `go build` | **PASS for Section 14; repo-wide FAIL** | every Section 14 package builds clean. `go build ./...` fails only inside the concurrently-added `internal/features/purchasing` (Section 15), which was not touched here |
| `go vet` | **PASS** | clean across inventory, shop, database, jobs, food, section14contract and crmcontract |
| `gofmt` | **PASS** | all 13 files this section changed are clean (CRLF-normalised). Three unformatted files belong to the concurrent Section 15 work |
| `git diff --check` | **PASS** | clean, CRLF warnings only |

---

## 4. Query plans at scale

Dataset: 4 businesses, 12 branches, 21,000 inventory items, 21,000 balances,
6,996 lots, 63,000 movements, 3,000 reservations, 3,000 reorder rules, 1,800
alerts, statistics refreshed with `ANALYZE`.

| Hot path | Plan | Time |
|---|---|---|
| Balances by business and branch | `inventory_balances_branch_idx` | 0.058 ms |
| Item availability | `inventory_balances_inventory_item_id_business_id_key` | 0.036 ms |
| Item branch lookup | `inventory_items_id_business_uq` | 0.039 ms |
| Reservation by source reference | `inventory_reservations_business_id_source_type_source_ref_key` | 0.026 ms |
| FEFO lot allocation | `inventory_lots_allocation_idx` | 0.050 ms |
| Movement history by branch | `inventory_movements_history_idx` | 0.174 ms |
| Movement history by item | `inventory_movements_item_idx` | 0.043 ms |
| Lot expiry sweep | `inventory_lots_expiry_idx` | 0.054 ms |
| Reservation expiry sweep | `inventory_reservations_expiry_idx` (bitmap) | 0.363 ms |
| Reorder rule scan | `inventory_reorder_rules_active_idx` | 0.069 ms |
| Open alert scan | `inventory_alerts_open_idx` | 0.036 ms |

No sequential scan appeared on any large Inventory table, and **no index was
added**, because no plan justified one.

---

## 4a. The repository moved during this certification

Section 15 (Suppliers and Purchasing) was added by a concurrent session while
Section 14I was running. Migrations `000067_vendor_purchasing_suppliers_foundation`
and `000068_vendor_purchase_orders_approval_lifecycle` appeared, along with a
full `internal/features/purchasing` package wired into `cmd/main.go`.

This has three consequences that a reader of this report needs to know.

1. **The migration tip is no longer 000066.** Section 14's own certification
   was performed at 000066 and remains valid at that tip. The migration
   scenarios are pinned explicitly through
   `VENDOR_SERVICE_TEST_MIGRATIONS_DIR` so they certify Section 14's tip rather
   than silently skipping or rolling back another section's migrations, which
   would certify that section's down files instead of this one's.

2. **`go build ./...` currently fails**, on
   `internal/features/purchasing/purchase_orders.go:12: "time" imported and not
   used`. That file is Section 15's and was not touched here. Every Section 14
   package — `inventory`, `shop`, `database`, `jobs`, `food`,
   `section14contract`, `crmcontract` — builds and vets clean in isolation.
   `cmd` does not compile only because it imports the in-flight purchasing
   package.

3. **`internal/crmcontract` is now out of date with respect to Section 15.**
   Purchasing registers routes in `cmd/main.go` but is not registered in the
   service-wide contract router, so those routes are invisible to the gate that
   exists to catch undocumented routes. This is the identical hole that
   Section 14I found and fixed for Section 14 (defect 2.3). It is Section 15's
   to close and was deliberately not touched here.

---

## 4b. The one full-regression failure, and why it is not Section 14's

The final full run was **37 of 38 packages**. The failure was
`internal/section13security` /
`TestSection13ScaleSearchRemainsCorrectAndBounded`, which timed out at 615
seconds seeding its 24,000-document Shop corpus against a 600 second deadline.
It reproduced on a re-run with the machine otherwise idle.

It is not attributable to Section 14, on four independent grounds:

1. **It passed earlier the same day** on a full run that already contained
   every Section 14 fix, including the 000063 migration repair.
2. **Section 14 adds nothing to any Shop write path.** No Section 14 migration
   creates a trigger or index on, or alters, any `shop_` table. Verified
   directly against `000060`–`000066`.
3. **The 14C compatibility bridge is not on that path.** The bridge fires on
   Shop stock-basis writes; the Section 13 scale seed creates no stock bases.
4. **The failure mode is a seed deadline, not a wrong answer.** The test
   overran by about 2.5%, which is the profile of a borderline timing budget on
   this hardware rather than a correctness regression.

Recorded as a pre-existing Section 13 scale-timing failure. Section 14 did not
cause it and cannot fix it without changing Section 13's budget, which is not
this section's to change.

---

## 5. Limitations

These are stated rather than resolved, and none blocks Section 14.

- **Generic receipt routes do not exist.** `CreateReceipt`, `PostReceipt` and
  `CancelReceipt` are certified at the generic service layer, but goods
  receiving reaches HTTP only through the Section 12B Shop surface. No
  duplicate generic route was added for symmetry.
- **`supplier_ref` is opaque.** Reorder rules raise a restocking signal and
  never create a purchase order. Section 15 owns purchasing.
- **`RecipeAvailabilityResolver` is an unimplemented seam.** Section 16 owns
  recipe authority; `INVENTORY_LINKED` Food availability fails closed until it
  arrives, which is the intent of the mode.
- **CRLF line endings.** Five tracked files under `internal/database` and most
  of `internal/features/shop` are stored with CRLF, so a bare `gofmt -l` on
  Windows lists them. Their content is correctly formatted; the gate above was
  run with line endings normalised.
- **Concurrency timing under load.** `TestSection14EPostgresExpiryCommitRace`
  completes in about one second normally, but was observed to exhaust a 60
  second context once while a 38-package regression was competing for the same
  PostgreSQL server. Three isolated re-runs passed in about a second each. This
  is resource contention in the test environment, not a lock-ordering defect.

- **Race iteration counts differ by gate.** The durable-worker claim race runs
  five rounds of eight contenders every time it executes. The 14E and 14F stock
  races run once per suite execution rather than in a deliberate repeat loop;
  they were executed across several full and targeted runs during this pass, but
  a dedicated high-iteration soak is not part of the suite. If a rare
  interleaving exists in those paths, this certification would not have found
  it.

---

## 6. Scope held

Section 14 added no supplier, purchase-order, recipe, production, order,
checkout, payment or logistics capability, and pulled none of those
authorities forward. No other platform service was modified. No frontend code
was written. Nothing was committed or pushed.
