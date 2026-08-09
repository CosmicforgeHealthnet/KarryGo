# Section 22 — Finance, billing and payroll

Section 22 adds organization-scoped operational finance records to Fleet. It does not add a Fleet ledger, bank account model, payment rail, tax engine, or general accounting system. All amounts are signed integer minor units paired with a three-letter currency. Wallet and external accounting identifiers are opaque references.

## Capabilities and state

- Invoices contain server-calculated lines and totals. Drafts may be edited with `expectedVersion`; approval and issue are separate. Issue atomically freezes an append-only snapshot and creates the receivable projection. Issued invoices are corrected only with capped, immutable credit notes.
- Authoritative payment events update receivables by source event and source version. Duplicate or stale events cannot create a Fleet ledger movement.
- Expenses require safe organization-owned Media evidence when a receipt is supplied. Submitter and approver are separated. Reversal is an immutable compensating record and does not rewrite the approved expense history.
- Budgets move through draft, approved/active and closed states. A worker rebuilds committed and actual projections from Fleet expense records.
- Compensation rules preserve versioned calculation snapshots. Payroll calculation records the rule, rule version, source reference, source version, inputs and explanation for every item. Approval freezes the period.
- Payroll posts each approved provider item through Wallet's provider-allocation API with a stable per-item idempotency key. Unknown outcomes are read back before any retry. Provider statements contain only the member's period and item data.
- Subcontractor settlements reject incomplete, foreign or previously settled jobs. Claims retain Media evidence and opaque insurer/settlement references; Fleet performs no underwriting.

Automatic calculation never creates `DEDUCTION` or `MANUAL_ADJUSTMENT` items. Database constraints additionally prohibit negative compensation sourced from decline counts, decline rates, safety reports, safety declines or performance scores. A deduction must have an explicit approved basis and a separate authorized flow.

## Routes, security and concurrency

The canonical route list is machine-readable in [section22-finance-billing-payroll.openapi.json](openapi/section22-finance-billing-payroll.openapi.json). Corporate routes are under `/fleet-partner/v1`, require tenant resolution and a route-specific permission, and enforce branch scope where applicable. Every command requires `Idempotency-Key`; state transitions also require `expectedVersion`/`If-Match`. Payment ingestion is restricted to the HMAC-authenticated internal surface.

Permissions are grouped as `invoices.*`, `receivables.*`, `expenses.*`, `budgets.*`, `compensation.*`, `payroll.*`, `subcontractor_settlements.*`, and `claims.*`.

## Posting and workers

Posting operations and attempts are durable. A timeout is `OUTCOME_UNKNOWN`, not failure; reconciliation asks the authority for the same idempotency key. The shared jobs runtime provides leases, heartbeat, retry limits, dead-letter and replay. Section 22 schedules invoice-overdue, receivable/posting reconciliation, expense posting, budget projection and claim-sync work. Payroll allocations and reversals route to Payment Wallet Service. Expense and subcontractor posting routes to the provider-neutral HMAC-v2 accounting contract at `/api/v1/accounting/internal/postings`; startup fails closed when an enabled posting capability lacks its authoritative adapter.

Wallet migration `000008_fleet_payroll_reversal_authority` gives the Fleet service identity a narrowly scoped payroll-orchestrator role containing transaction read-back, provider allocation posting and provider allocation reversal. It does not grant settlement approval, refund approval, reconciliation, withdrawal or general finance administration.

## Flags, rollout and rollback

All flags default false: `INVOICES_ENABLED`, `RECEIVABLES_ENABLED`, `EXPENSES_ENABLED`, `BUDGETS_ENABLED`, `COMPENSATION_ENABLED`, `PAYROLL_ENABLED`, `SUBCONTRACTOR_SETTLEMENTS_ENABLED`, `CLAIMS_ENABLED`, `FINANCE_POSTING_ENABLED`, and `SECTION22_WORKERS_ENABLED`. The global integration kill switch stops writes and schedulers. Enable schemas first, then read capabilities, then narrowly enable writes, posting and workers after Wallet/Media credentials and permissions are verified. Rollback disables flags and workers before applying the paired migration downs; issued snapshots, financial history and posting attempts are intentionally append-only while installed.
