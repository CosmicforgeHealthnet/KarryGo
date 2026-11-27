# Fleet Sections 21-33: integration report

## Cross-service boundaries

No new cross-service call, database access, foreign key or join was introduced. All new tables are
fleet-partner-owned. Specifically:

| Concern | Authority | How this work respects it |
|---|---|---|
| Money, ledger, payouts | payment-wallet-service | Profitability, ageing and cost allocation are read models over fleet-owned invoices, payments, expenses and payroll. Nothing posts to a ledger. Payroll posting still uses the existing financeops posting path. |
| Notifications | notification-service | Exceptions, SLA escalation and automation queue durable notification intents through the existing communications outbox; delivery stays with notification-service. Preferences narrow channels before the intent is queued. |
| Files | media-file-service | Report results are stored as JSON in the run row (bounded); no file upload added. |
| Execution (dispatch / taxi / hauling) | execution services | Not touched. The Dispatch end-to-end integration test in `internal/integration` still passes against a real Dispatch database. |

## Internal integration (inside fleet-partner-service)

- **Events**: every new mutation publishes domain events through the transactional outbox in the same
  transaction as the state change. Section 26.2 core events are derived in that transaction too.
- **Automation reads committed events only**: the evaluator consumes outbox rows after a stored cursor
  (timestamp + id), so a rolled-back transaction can never trigger a rule.
- **Cross-feature calls go through ports**: analytics raises exceptions and queues notices through
  adapters built in `internal/app/section24_wiring.go` (`automationExceptionAdapter`,
  `automationNoticeAdapter`); no feature imports another feature's `http` or `repositories` package
  (enforced by the architecture tests).
- **Jobs**: all new background work runs on the existing durable job queue with leases and retry:
  `REPORT_RUN_GENERATE`, `AUTOMATION_EVALUATE`, `AUTOMATION_RULE_EXECUTE`, `ANALYTICS_HOUSEKEEPING`,
  `SECTION23_EXCEPTION_SLA`, plus warehouse task-lease release in the warehouse worker. Workers start in
  both the API process (`cmd/main.go`) and the worker process (`cmd/worker/main.go`) according to config.

## Failure behaviour verified by tests

- Notification-service rejecting every submission during SLA escalation: the escalation stays committed and the notice is recorded as failed, not lost.
- Automation execution replayed: unique (rule, trigger) prevents a second execution.
- Automation action producing events: skipped by the loop guard.
- Payroll inputs edited after calculation: approval refused with 409.
- A task already claimed by one operator cannot be claimed or completed by another (409).
- Concurrent wave release of the same fulfilment orders: exactly one wins, no double allocation (WH-003, real goroutines).

## Final-closure additions (2026-11-27)

- **Sessions**: organization session management reuses the auth feature's single session table through the
  governance `SessionAuthority` port. No second session store exists, and revocation takes the same family
  advisory lock as refresh rotation, so a concurrent refresh cannot slip a new credential past it.
- **Audit**: `/audit-events` reuses the audit handler and the append-only audit records; nothing is copied.
- **Events**: report-run and legal-hold events now go through the same transactional outbox as every other
  domain event.
- **Cross-service dependency fixes** (owner-authorized, release-gate driven, no business logic changed):
  shared runtime image OpenSSL, media-file-service OpenTelemetry, Dispatch/Hauling websocket library.
  Dispatch's existing 15 socket tests and a new Hauling socket test pass on the new websocket version.
- **Not integrated (recorded)**: support-case messages to support-dispute-service (needs a new
  cross-service contract); the Section 23 notification delivery worker stays off on staging (see 07).
