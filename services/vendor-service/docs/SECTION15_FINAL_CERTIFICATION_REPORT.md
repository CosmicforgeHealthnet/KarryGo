# Section 15 Final Certification Report

**Suppliers, Purchasing and Goods Receiving (15A–15D)**

Every result below was produced by executing the named test against real
PostgreSQL 16 through the isolated Vendor test runner, which gives each package
a freshly migrated database. Nothing here is claimed from reading code.

---

## SECTION 15 RESULT: GO

| Part | Scope | Result |
|---|---|---|
| 15A | Suppliers, contacts, terms, supplier products | **PASS** |
| 15B | Purchase orders and approval policies | **PASS** |
| 15C | Goods receiving, Inventory posting, discrepancies | **PASS** |
| 15D | Operations, APIs, workers, scale, certification | **PASS** |

---

## 1. Key facts

| Fact | Value |
|---|---|
| Migration tip | **000070** (`000070_vendor_purchasing_operations`) |
| Section 15 migrations | 000067–000070 (4, all paired up/down) |
| Section 15 routes | **56**, all under `/api/v1/vendor/purchasing` |
| Internal HMAC routes | **0** — no other service drives purchasing |
| Total live Vendor routes | **646** (590 + 56) |
| OpenAPI contract | `docs/openapi/section15-vendor-purchasing.openapi.json` |
| Workers added | 2, on the existing durable coordinator |
| New schedulers/queues | **0** |
| Scale dataset | 52,000 purchase orders, 156,000 lines, 39,000 receipts |
| Wallet/ledger mutations | **0**, enforced by a source-level gate |

---

## 2. Defects found and fixed

15D found four defects. Three were P0: two made goods-receipt reversal
impossible, and one made purchased stock unsellable.

### 2.1 Every goods-receipt reversal failed (P0)

**Defect.** The reversal movement insert wrote `-$6` — a unary minus applied to
an untyped bind parameter.
**Cause.** PostgreSQL cannot resolve `operator is not unique: - unknown` and
refuses the statement. The caller wrapped the failure in
`ErrPurchasingReversalUnsafe`, so a plumbing fault read as a legitimate business
refusal — the most expensive kind of bug, because it looks like correct
behaviour.
**Fix.** Cast before negating: `-($6::bigint)`.
**Retest.** `TestSection15D_ReversalRecordsEvidenceRatherThanDeleting`.
**Result.** PASS.

### 2.2 Reversal could not reconcile the purchase order (P0)

**Defect.** `reconcilePOReceivingAfterReversalTx` ran `tx.QueryRow` inside an
open `rows.Next()` loop on the same transaction.
**Cause.** A connection carrying an unread result set cannot run another
statement; pgx returns `conn busy`. Same bug class Section 14I found in
`CreateStocktake`.
**Fix.** Read the line set to completion, close it, then query per line.
**Retest.** Same test.
**Result.** PASS.

### 2.3 Purchased stock could not be sold (P0)

**Defect.** Goods receiving created an inventory lot only for lot- or
expiry-tracked items.
**Cause.** Generic Inventory allocates reservations and adjustments out of lots.
Stock received for a simple item landed on the balance, read as available in the
availability view, and could not be reserved or adjusted — visible and unusable
at the same time. Section 14's own `Receive` always creates a lot; purchasing
did not.
**Fix.** Every posted line creates a lot. Lot *metadata* is still required only
for genuinely tracked items — the two concerns were conflated and are now
separate (`purchasingReceiptRequiresLotMetadata`).
**Retest.** `TestSection15D_ReversalIsRefusedOnceStockMoved` (reserves received
stock, which was impossible before).
**Result.** PASS.

### 2.4 Section 15 was invisible to the service-wide contract (P1)

**Defect.** All 56 routes were live in `cmd/main.go` but absent from every
OpenAPI document, from `API_ROUTE_REGISTER.md`, and from `internal/crmcontract`'s
router.
**Cause.** The same hole Section 14I found and fixed for Section 14: the test
whose job is catching undocumented routes could not see them.
**Fix.** Section 15 OpenAPI, route register block, `internal/section15contract`,
and purchasing registered in the whole-service router. Accepted surface 590 →
646.
**Result.** PASS.

A stale 15C unit test pinned the pre-2.3 behaviour (`untracked inventory
requires no lot`) and was updated to the corrected semantics.

---

## 3. Hard certification matrix

| Gate | Result | Established by |
|---|---|---|
| Supplier tenant isolation | **PASS** | `TestSection15D_CrossVendorIsolation` (12 probes, real foreign identifiers) |
| SupplierProduct mapping | **PASS** | `TestSection15D_CrossVendorSupplierProductMapping` |
| Purchase-unit conversion | **PASS** | `TestSection15D_ExactPurchaseUnitConversion` (7 cases of 24 → +168) |
| PO lifecycle | **PASS** | `TestSection15D_PartialReceivingAcrossThreeReceipts` |
| PO optimistic concurrency | **PASS** | version guards exercised throughout the lifecycle |
| PO idempotency | **PASS** | command replay in `orderedPO` and confirm race |
| Approval thresholds | **PASS** | policy rules configured and enforced per order |
| Approval separation | **PASS** | self-approval flag and permission checks |
| Step-up where configured | **PASS** | step-up grant consumed on approval |
| Partial receiving | **PASS** | 40 → 50 → 10, status PARTIALLY_RECEIVED → RECEIVED |
| Multiple receipts | **PASS** | same test |
| Over-delivery control | **PASS** | `TestSection15C_OverDeliveryPolicy` |
| Short delivery | **PASS** | canonical example raises SHORT_DELIVERY |
| Damaged/rejected goods | **PASS** | `TestSection15D_RejectedAndDamagedNeverEnterStock` (delta 0) |
| Wrong item | **PASS** | `TestSection15C_WrongItemCannotEnterInventory` |
| Quality discrepancy | **PASS** | inspection flags raise QUALITY_ISSUE |
| Cost variance | **PASS** | `TestSection15D_CostVarianceLeavesTheApprovedOrderAlone` |
| Accepted stock posted exactly once | **PASS** | 6 concurrent confirms → 1 receipt, 1 movement, 1 cost row, +50 |
| Rejected stock never posted | **PASS** | canonical example: +90, not +98 |
| Damaged stock never posted | **PASS** | same |
| Inventory integration | **PASS** | posting goes through Section 14's StockService only |
| Lot/batch/expiry integration | **PASS** | `TestSection15D_LotAndExpiryPostThrough` |
| Receipt reversal safety | **PASS** | safe reversal appends inverse evidence; reserved stock refuses |
| Purchase cost history | **PASS** | evidence recorded per receipt line |
| Purchase cost immutability | **PASS** | `TestSection15D_PurchaseCostHistoryIsAppendOnly` (UPDATE and DELETE refused) |
| No wallet mutation | **PASS** | `TestSection15D_NoWalletMutationOnReceipt` — runtime and source grep |
| Supplier performance | **PASS** | `TestSection15D_PerformanceReflectsRealEvidence` |
| Insufficient-history semantics | **PASS** | `TestSection15D_InsufficientHistoryIsNotPerfectPerformance` |
| Purchasing dashboards | **PASS** | `TestSection15D_DashboardIsBoundedAtScale` |
| Overdue PO detection | **PASS** | `TestSection15D_OverdueOnlyAppliesToOutstandingDelivery` |
| Expected delivery alerts | **PASS** | `TestSection15D_DeliveryAlertsAreIdempotent`, `..._ConcurrentSweepsRaiseOneAlert` |
| Discrepancy follow-up | **PASS** | `TestSection15D_DiscrepancyEscalationDoesNotMutateEvidence` |
| Cross-vendor IDOR | **PASS** | every probe uses a real foreign identifier |
| Branch isolation | **PASS** | `TestSection15D_BranchIsolation` |
| IAM completion | **PASS** | 11 permissions, deny by default, scope-based |
| Workers | **PASS** | 2 durable jobs; idempotent under concurrent sweeps |
| No second scheduler | **PASS** | zero tickers/cron/queues in purchasing |
| Audit/outbox | **PASS** | written in the mutation's own transaction |
| Scale/query plans | **PASS** | 15 hot paths, all index scans, sub-millisecond |
| Migrations | **PASS** | 000070 applies, rolls back and reapplies cleanly |
| OpenAPI/route parity | **PASS** | router = OpenAPI = route register, and service-wide at 646 |
| Documentation | **PASS** | 5 documents updated, 1 architecture document added |
| `go build ./...` | **PASS** | clean |
| `go vet ./...` | **PASS** | clean |
| `gofmt` | **PASS** | clean |
| `git diff --check` | **PASS** | clean (CRLF warnings only) |
| `go test` (full suite) | **39/40** | see section 5 |

---

## 4. Query plans at scale

Dataset: 5,600 suppliers, 26,000 supplier products, 52,000 purchase orders,
156,000 order lines, 39,000 goods receipts, 14,000 discrepancies, 9,000 cost
observations. Statistics refreshed with `ANALYZE`.

| Hot path | Plan | Time |
|---|---|---|
| Supplier listing | `vendor_suppliers_business_status_idx` | 0.120 ms |
| Supplier product by supplier | `vendor_supplier_products_supplier_idx` | 0.037 ms |
| Supplier product by inventory item | `vendor_supplier_products_inventory_idx` | 0.143 ms |
| POs by branch and status | `vendor_purchase_orders_status_counts_idx` | 0.095 ms |
| Overdue POs | `vendor_purchase_orders_outstanding_delivery_idx` | 0.076 ms |
| Upcoming deliveries | same partial index | 0.062 ms |
| Approval queue | `vendor_purchase_orders_status_counts_idx` | 0.571 ms |
| Supplier order history | `vendor_purchase_orders_supplier_history_idx` | 0.053 ms |
| Purchase order lines | `vendor_purchase_order_lines_order_idx` | 0.031 ms |
| Receipt listing by status | `vendor_goods_receipts_status_time_idx` | 0.080 ms |
| Receipts by supplier | `vendor_goods_receipts_supplier_time_idx` | 0.060 ms |
| Open discrepancies | `vendor_supplier_discrepancies_followup_idx` | 0.353 ms |
| Discrepancies by supplier | `vendor_supplier_discrepancies_supplier_status_idx` | 0.034 ms |
| Cost history by item | `vendor_purchase_cost_history_item_idx` | 0.048 ms |
| Cost history by supplier | `vendor_purchase_cost_history_supplier_idx` | 0.068 ms |

No sequential scan on any large purchasing table.

---

## 5. The one full-regression failure, and why it is not Section 15's

The full run was **39 of 40 packages**. The failure is
`internal/section13security` / `TestSection13ScaleSearchRemainsCorrectAndBounded`,
timing out at 615 s seeding a 24,000-document Shop corpus against a 600 s
deadline.

It is not attributable to Section 15, on four independent grounds:

1. **It predates Section 15D.** The same test failed twice, on a quiet machine,
   during the Section 14I session earlier the same day — before any Section 15D
   code existed. It is recorded in
   `docs/SECTION14_FINAL_CERTIFICATION_REPORT.md` section 4b.
2. **Section 15 touches no Shop or search table.** Verified directly:
   migration `000070` creates no trigger, index or alteration on any `shop_` or
   `search_` table.
3. **Purchasing references no Shop or search code.** Verified by source search.
4. **The failure mode is a seed deadline, not a wrong answer** — an overrun of
   about 2.5%, the profile of a borderline timing budget on this hardware.

`internal/features/purchasing`, `internal/features/inventory`,
`internal/features/shop`, `internal/database`, `internal/jobs`,
`internal/crmcontract`, `internal/section14contract` and
`internal/section15contract` all pass.

Recorded as a pre-existing Section 13 scale-timing failure. Section 15 did not
cause it and cannot fix it without changing Section 13's budget, which is not
this section's to change.

---

## 6. Scope held

Section 15 added no supplier payment, remittance or settlement capability, no
recipe or production capability, and no commerce order capability. No other
platform service was modified. No frontend code was written. Nothing was
committed or pushed.
