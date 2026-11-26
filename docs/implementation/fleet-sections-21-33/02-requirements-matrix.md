# Fleet Sections 21-33: requirements matrix

Status values: **DONE** (code + test), **PRE-EXISTING** (implemented before this work and re-verified),
**PARTIAL** (implemented with a stated limitation), **NOT BUILT** (with reason).

## Section 21 / Contract 51 - Warehouse, fulfilment, yard, dock

| Requirement | Status | Evidence |
|---|---|---|
| Inbound receipt with expected lines, arrive / receive / resolve discrepancy / complete / cancel | DONE | `warehouse/usecases/inbound_outbound.go`, `TestSection51WarehouseJourney` |
| Received quantity posts to the stock ledger; over/under receipt raises a discrepancy that must be resolved before completion | DONE | same |
| Put-away tasks with leased claim, scan validation of bin, lease expiry release | DONE | `workers.go`, journey test |
| Pick waves over released fulfilment orders, pick tasks, short pick recorded | PARTIAL | Short pick is recorded and the order is held; automatic replanning of the shortfall is not implemented, an operator re-releases. |
| Packing units limited to picked quantity | DONE | journey test |
| Gate events (append-only, ordered: check-in before dock, dock before check-out) | DONE | migration 000100 trigger, journey test |
| Yard / dock appointments | PRE-EXISTING | `section19_21contract` |

## Section 22 / Contract 52 - Finance

| Requirement | Status | Evidence |
|---|---|---|
| Payroll calculation fingerprint; approval refuses changed inputs (409 `payroll_inputs_changed`) | DONE | `models/payroll_hash.go`, `TestSection52FinanceAnalysis` |
| Invoice submit command; correct `invoice.cancelled.v1` event name | DONE | `usecases/invoices.go`, `models/rules.go` |
| Receivables ageing (CURRENT, 1-30, 31-60, 61-90, over 90), per currency and customer | DONE | `TestAgeingBucketsAndCurrencyIsolation` |
| Profitability (revenue, direct cost, allocated cost, margin) and immutable snapshots | DONE | migration 000101 append-only trigger |
| Cost allocation (MANUAL or EVEN split, exact minor units, remainder assigned deterministically) | DONE | `TestProfitabilityMarginAndSplitEven` |
| Ledger authority stays in payment-wallet-service | DONE | No ledger tables added; finance reads are projections. |

## Section 23 / Contract 53 - Communication, support, quality, SLA

| Requirement | Status | Evidence |
|---|---|---|
| Operational exception register with dedupe per source, update / acknowledge / escalate / resolve | DONE | `engagement/usecases/exceptions.go`, `TestSection53ExceptionEscalationJourney` |
| SLA due time, automatic escalation by durable job, breach list | DONE | job `SECTION23_EXCEPTION_SLA`, `/sla-breaches` |
| Notification preferences (personal and organization), mandatory channels cannot be suppressed | DONE | `communications/models/preferences.go` (`NarrowChannels` test), applied inside `QueueIntentTx` |
| Notification failure does not lose the exception | DONE | integration test with failing notifier |

## Section 24 / Contract 54 - Analytics, reporting, AI, automation

| Requirement | Status | Evidence |
|---|---|---|
| Dashboards (executive, operations, finance, fleet, warehouse, workforce, sla, customers) with allowlisted filters, branch-scoped | DONE | `analytics/http/handler.go`, `TestSection24AnalyticsRecommendationsAutomationAndGovernance` |
| Report definitions, scheduled (cron) and on-demand runs, results expire | DONE | jobs `REPORT_RUN_GENERATE`, `ANALYTICS_HOUSEKEEPING` |
| Report runs freeze the requester's permissions at request time | DONE | `permissions` column on run, enforced on read |
| Recommendations: explainable, versioned model registry, human decision with expected version, expiry | DONE (rules-based) | `usecases/recommendations.go`; no trained model |
| Automation rules: draft -> submit -> approve (separation of duties) -> active -> disabled; versions append-only | DONE | `usecases/automation.go`, SoD test |
| Rule evaluation from committed outbox events, exactly once per (rule, trigger), loop guard | DONE | unique `(rule_id, trigger_ref)`, `sourceType=AUTOMATION_EXECUTION` skip |
| Allowed actions limited to raise exception and queue notification | DONE | `models.AutomationActions` |

## Sections 25-27

| Requirement | Status | Evidence |
|---|---|---|
| Files, search, imports, exports | PRE-EXISTING | `section25contract` |
| Section 26.2 core events produced | DONE | `internal/events/canonical.go`, `canonical_test.go` |
| Durable jobs for every new background workflow | DONE | see 03 |

## Section 28 / Contract 56 - Security, privacy, audit

| Requirement | Status | Evidence |
|---|---|---|
| Tenant security-event feed | DONE | `GET /security-events` |
| Privacy requests (EXPORT, ERASURE, RECTIFICATION, RESTRICTION), response window, decision with SoD, legal hold blocks erasure | DONE | `features/governance`, `TestSection24AnalyticsRecommendationsAutomationAndGovernance` (governance part) |
| Audit events endpoint | PRE-EXISTING | Served as `/audit-logs`; no separate `/audit-events` alias added. |
| Admin session management (`/security/sessions`) | NOT BUILT | Platform-admin surface, outside the tenant service scope of this work; recorded as follow-up. |

## Sections 29-33

| Requirement | Status | Evidence |
|---|---|---|
| Reliability, observability, DR | PRE-EXISTING | `section29contract` |
| Reversible migrations, compatibility | DONE | 000100-000104 replay/rollback test; `section30contract` tip `000104` |
| Testing strategy | DONE | 06-test-certification.md |
| WP20 warehouse, WP21 finance, WP22 communication/SLA, WP23 analytics/automation, WP24 governance | DONE (with the PARTIAL / NOT BUILT rows above) | commits listed in 03 |
| Release gates | See 09 | |
