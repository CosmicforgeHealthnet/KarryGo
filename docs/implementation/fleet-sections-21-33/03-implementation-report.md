# Fleet Sections 21-33: implementation report

All work is on `staging` in `services/fleet-partner-service` (plus repo-root OpenAPI files under
`docs/openapi/`). No other service was changed. Layout follows the service convention
`features/<feature>/{models,repositories,usecases,http}`; use cases depend on repository interfaces
declared in the use-case package (architecture test FLEET-5.1b).

## Commits

| Commit | Work package | Content |
|---|---|---|
| `a1fc7371` | WP20 | Section 21/51 warehouse receipts, leased put-away and pick tasks, waves, packing, gate events |
| `0c1cbab2` | WP21 | Section 22/52 payroll fingerprint, receivables ageing, profitability, cost allocation, invoice submit |
| `421b70a2` | Section 26 | Produce the seven Section 26.2 core events that were registered but never emitted |
| `19607d23` | WP22 | Section 23/53 operational exceptions, SLA breaches, notification preferences |
| (this push) | WP23, WP24 | Section 24/54 analytics, reports, recommendations, automation; Section 28/56 security events and privacy requests; docs |

## WP20 - Warehouse operations (Section 21 / 51)

- Receipt states `EXPECTED -> ARRIVED -> RECEIVING -> (EXCEPTION ->) RECEIVED -> PUTAWAY_IN_PROGRESS -> COMPLETED`,
  or `CANCELLED`. Receiving posts accepted quantity (received minus damaged minus quarantined, enforced by a
  CHECK) to the stock ledger in the same transaction; a line whose count differs from expected opens a
  discrepancy that blocks completion until resolved with a reason.
- Put-away and pick tasks are claimed with a lease (default 5 minutes, `Options.TaskLease`). Completion
  requires the scanned bin / item to match the task. An expired lease is released by the warehouse worker
  so another operator can take the task.
- Pick waves group released fulfilment orders; a short pick is recorded and holds the order
  (limitation: no automatic replan).
- Packing quantity can never exceed picked quantity. Gate events are append-only and ordered.

## WP21 - Finance analysis (Section 22 / 52)

- `calculation_hash` on payroll periods: a SHA-256 over the sorted calculation inputs. Approval
  recomputes it and refuses with 409 `payroll_inputs_changed` if anything moved since calculation, so a
  period cannot be approved against figures nobody reviewed.
- Receivables ageing per currency (currencies are never summed together) and per customer.
- Profitability read model and append-only snapshots stamped with the method version.
- Cost allocations (MANUAL or EVEN), append-only, exact in minor units.
- Invoice `submit` command; cancel event renamed from the misspelt `invoice.canceld.v1` to `invoice.cancelled.v1`.
- No ledger was added: payment-wallet-service remains the money authority.

## Section 26 dependency fix

`internal/events/canonical.go` derives `organization.status_changed.v1`, `organization.provider.provisioned.v1`,
`organization.provider.membership_changed.v1`, `organization.asset.blocked.v1`, `corporate.order.confirmed.v1`,
`corporate.shipment.status_changed.v1` and `corporate.invoice.issued.v1` from the domain events already being
written, inside the same transaction, so they are produced exactly when the underlying change commits.

## WP22 - Exceptions, SLA, preferences (Section 23 / 53)

- Exceptions dedupe on (organization, source type, source id) while live; commands are optimistic
  (expected version). A durable job `SECTION23_EXCEPTION_SLA` escalates exceptions past their SLA and
  queues a notification; notification failure never loses the escalation.
- Notification preferences narrow channels for a user or organization, but mandatory channels for
  critical templates cannot be switched off. Preferences are applied inside `QueueIntentTx`.

## WP23 - Analytics, reports, recommendations, automation (Section 24 / 54)

- New feature `features/analytics`; `features/reports` and `features/automation` remain boundary
  markers pointing to it. The old stub `automation/section27_runner.go` was removed.
- Dashboards are synchronous, bounded and branch-scoped; unknown query parameters are refused.
- Report runs are asynchronous (`REPORT_RUN_GENERATE`), freeze the requester's branch permissions at
  request time, and expire (`ANALYTICS_HOUSEKEEPING`). Scheduled definitions use cron.
- Recommendations are deterministic rules (maintenance due, fuel/odometer anomaly, stale live trip)
  against a seeded model registry (`fleet_partner_model_versions`). Each carries its inputs, explanation
  and model version; a human accepts or rejects with an expected version; undecided ones expire.
- Automation rules: draft -> submit -> approve by a different person -> active -> disabled. Every
  status change writes an append-only version. The evaluator (`AUTOMATION_EVALUATE`) reads committed
  outbox events after a per-organization cursor; a unique (rule, trigger) key makes each execution
  happen once; events produced by automation are skipped so rules cannot trigger each other in a loop.
  Actions are limited to raising an exception and queueing a notification.

## WP24 - Governance (Section 28 / 56)

- `GET /security-events`: the organization's own authentication and authorization security events.
- Privacy requests (EXPORT, ERASURE, RECTIFICATION, RESTRICTION) with a response deadline
  (default 30 days), decision by someone other than the submitter, and completion blocked while an
  active legal hold covers the subject.

## Configuration

New switches, all default off (`.env.example`): `ANALYTICS_REPORTS_ENABLED`,
`FEATURE_AI_RECOMMENDATIONS`, `FEATURE_AUTOMATION`, `SECTION24_WORKERS_ENABLED`,
`PRIVACY_REQUESTS_ENABLED`, `SECURITY_EVENTS_ENABLED`, `OPERATIONAL_EXCEPTIONS_ENABLED`,
`NOTIFICATION_PREFERENCES_ENABLED`. Deploying the code therefore changes no behaviour until a switch is
turned on.
