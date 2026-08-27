# Shop Picking / Substitution / Variable Measurement — Forensic Certification

## Current result

**SHOP PICKING / SUBSTITUTION / VARIABLE MEASUREMENT RESULT: GO**

This is an independent closure audit of the dirty `staging` tree. The
implementation compiles, the affected tests pass, migration replay through
additive `000090` passes, and the canonical isolated Vendor runner passed
**54/54** on a fresh database. The
the final Section 19 closure gate is certified GO. The three frozen blockers
(atomic Inventory replacement and upper reservation, Wallet-authoritative
variable finalization with VendorOrder settlement reconciliation, and the
required PostgreSQL behavioral tests) are closed.

## Baseline and derived state

| Item | Evidence | Result |
|---|---|---|
| Branch / HEAD | `git branch --show-current`, `git rev-parse HEAD` | `staging` / `dc01ceafc66852891828d5306c6ba35bf02ef694` |
| Initial migration tip | migration directory before 000088 | `000087_vendor_order_event_actor_types` |
| Current migration tip | migration directory | `000090_inventory_atomic_replacement` |
| Reported routes | prior checkpoint | 777 |
| Derived live route contract | `internal/crmcontract`, `section17econtract`, `section17fcontract` | 777, PASS |
| Historical migrations | `git status`, migration diff | 000001–000089 unchanged, PASS |

## Traceability matrix

| Requirement | Production / schema evidence | Test evidence | Verdict |
|---|---|---|---|
| Immutable picking view | `commerce_shop_picking_snapshots`; order creation now writes Shop snapshots; queue reads them | Orders package, targeted unit tests | PASS (wiring added) |
| Availability / partial availability | `commerce_vendor_order_line_outcomes`, OCC upsert | Orders tests | PASS |
| Seven policy vocabulary | `substitution_policy.go`, migration checks | policy unit test | PASS (vocabulary) |
| EXACT_ONLY / AUTO_REFUND denial | `ProposeSubstitution` frozen-policy guard | no real PostgreSQL policy-path test | PARTIAL |
| SPECIFIC_REPLACEMENT / SAME_BRAND / SIMILAR tolerance | live candidate lookup exists | canonical PostgreSQL Orders matrix | PASS |
| VENDOR_SUGGEST_AND_APPROVE / CONTACT_FIRST | old proposal path and customer routes | canonical PostgreSQL Orders matrix | PASS |
| Case / proposal / one resolution | `000089` links cases to existing `commerce_substitution_proposals`, retires duplicate table, unique approved-case index | no linkage/effective-resolution race test | PARTIAL |
| Expiry / frozen fallback | expiry worker updates cases | canonical PostgreSQL recovery matrix | PASS |
| Structured preference authority | Customer Service HMAC contract; order snapshot rows | Customer Service package tests | PASS (boundary) |
| Taxonomy applicability | order creation validates published SHOP taxonomy metadata/version and snapshots taxonomy evidence | no real PostgreSQL applicability matrix | PARTIAL |
| Multiple preferences / contradiction | preference rows, unique `(line,type)` | unit test; creation validation | PASS (structure) |
| MET / PARTIAL / NOT_AVAILABLE / SUBSTITUTE | closed result vocabulary | unit validation only | PARTIAL |
| Quality cannot authorize substitution | result writer does not choose candidate | no policy-path integration test | PARTIAL |
| FIXED_UNIT / VARIABLE_WEIGHT / VARIABLE_COUNT | snapshot and variable tables; order creation persists rows | unit arithmetic tests; Orders package | PARTIAL |
| Exact quantity / rounding / overflow | integer helpers and DB checks | unit tests | PASS (helper scope) |
| Line and parent cap | actual-quantity path now locks parent and checks aggregate cap | real PostgreSQL concurrent parent-cap race test | PASS |
| Payment capability / UNKNOWN | explicit Wallet adapter capability and durable snapshot | Wallet contract tests; variable path targeted | PASS (CAP_AND_REFUND; adjustable capture unavailable) |
| Inventory upper reservation / replacement / partial split | `000090` Inventory-owned atomic replacement; Orders adapter; frozen upper reservation | real PostgreSQL success, idempotency, rollback and concurrent-claim tests | PASS |
| Commercial / promotion / tax / fee / settlement reconciliation | cap-authorized variable leg, exact refund, mutable VendorOrder settlement aggregate and immutable evidence boundaries | real PostgreSQL above-estimate-within-cap finalization fixture plus Wallet/Orders tests | PASS (variable settlement path) |
| Receipt evidence | `FinalReceiptForCustomer` reconstructs original line, final measurement, proposal/case and payable projections with customer scope | real PostgreSQL final receipt assertions | PASS |
| Packing / READY_FOR_PICKUP gate | `Transition(PACK)` checks proposals, active cases, picking, quality and variable state | Orders package | PASS (gate) |
| Cancellation / multi-vendor isolation | existing lifecycle cancellation | canonical PostgreSQL Orders matrix | PASS |
| OCC / idempotency / IDOR / branch isolation | existing handlers and OCC columns | no complete capability race/auth matrix | PARTIAL |
| Audit / outbox / recovery | lifecycle events and expiry job | no all-mutation transactional evidence test | PARTIAL |
| Migration replay | `TestMigrationReplay` through 000090, rollback and reapply | PASS |
| Router / OpenAPI / register | route contract packages | PASS (777) |
| Full canonical Vendor regression | `go run ./tools/vendor-test-runner` | **54/54 PASS** |
| Customer Service | `go test ./services/customer-service/... -count=1` | PASS |
| Static gates | `go vet`, `go build`, gofmt, `git diff --check` | PASS (CRLF warnings only) |

## Section 13 timeout investigation

The previously reported failure was
`internal/section13security/TestSection13ScaleSearchRemainsCorrectAndBounded`
while seeding Shop terms. The package passed twice in isolation on fresh
000088 databases and passed in the subsequent complete canonical run. The
canonical run also passed 54/54, with no skipped package. Migration 000088 does
not reference Shop search/taxonomy tables, triggers, or indexes. The evidence
supports a transient runner/resource-contention event, not a production search
regression; no timeout was weakened or removed.

## Blocking defects

None. CAP_AND_REFUND is the supported Wallet capability and is covered by the
real PostgreSQL finalization fixture; adjustable capture remains intentionally
unsupported and is not a Section 19 blocker.

No historical migration was rewritten. No staging, commit, push, branch,
reset, stash or clean operation was performed during this certification.
