# Fleet Sections 21-33: current-state audit (before this work)

Scope: `services/fleet-partner-service`, specification "KarryGo Corporate Logistics Complete Backend
Implementation Specification v2.0", Sections 21-33, with Part II contracts 51-71 as reference.
Audit baseline: `staging` at `219bab9b` (migration head `000099`).

## Method

Every requirement in Sections 21-33 was traced to code, migration, route manifest, test and
documentation. A requirement counted as present only when a route or worker actually exercised it
and a test proved it. Comments, boundary markers and registered-but-unproduced events did not count.

## Findings by section

| Section | Area | State at baseline | Gap closed in this work |
|---|---|---|---|
| 21 / 51 | Warehouse, fulfilment, yard, dock | Warehouses, zones, bins, stock ledger, reservations, fulfilment orders, yard/dock appointments existed. | No inbound receipt, put-away, pick wave/task, packing or gate-event model. Built (migration 000100, 24 routes). |
| 22 / 52 | Finance, billing, payroll, expenses, claims | Invoices, payments, payroll periods, expenses, claims existed. | Payroll approval did not detect inputs changed after calculation; no receivables ageing, profitability or cost allocation; invoice `submit` missing; cancel event misspelt `invoice.canceld.v1`. Built (000101, 7 routes) and fixed. |
| 23 / 53 | Communication, support, quality, SLA | Threads, messages, notification intents, support tickets, SLA measurements existed. | No operational exception register or SLA-breach view; no notification preferences. Built (000102, 11 routes, SLA escalation job). |
| 24 / 54 | Analytics, reporting, AI, automation | `features/reports` and `features/automation` were boundary markers only; `section27_runner.go` was a stub runner. | Built `features/analytics` (000103, 20 routes): dashboards, report definitions/runs, rules-based recommendations with model registry, approved automation rules with an outbox-driven evaluator. Stub runner removed. |
| 25 / 55 | Files, search, imports, exports | Implemented in earlier work (`documents`, `search`, `imports`, `exports`; `section25contract`). | None required; re-verified by the section25 contract suite. |
| 26 | Events and synchronisation | Outbox, inbox, relay existed; seven Section 26.2 core events were registered in the catalogue but never produced. | Core events now derived in the same transaction (`internal/events/canonical.go`). |
| 27 | Background jobs, scheduling | Durable job queue, leases, scheduler existed (`section27contract`). | Automation runner and report generator were documented as pending; now real jobs (`REPORT_RUN_GENERATE`, `AUTOMATION_EVALUATE`, `AUTOMATION_RULE_EXECUTE`, `ANALYTICS_HOUSEKEEPING`, `SECTION23_EXCEPTION_SLA`, warehouse lease release). |
| 28 / 56 | Security, privacy, audit | Audit log, sensitive-column inventory, legal holds existed (`section28contract`). | No tenant security-event feed and no privacy-request workflow. Built `features/governance` (000104, 6 routes). |
| 29 | Reliability, observability | Implemented earlier (`section29contract`, DR runbook). | None required. |
| 30 | Migrations, compatibility | Implemented earlier (`section30contract`). | Contract tip advanced to `000104`; every new migration has a tested down file. |
| 31 | Testing strategy | Implemented earlier (`section31contract`, mandatory CI workflow). | New integration and contract suites added (see 06). |
| 32 | Work packages WP20-WP24 | WP20-WP22 partially, WP23-WP24 absent. | See requirements matrix. |
| 33 | Release gates | Gate tooling existed (`masterreleasecontract`). | Gates re-run; see 09. |

## Things deliberately not built

- Sections 1-20 and 34+ were not developed. One dependency fix outside scope: the Section 26.2 core
  events belong to Sections 7-19 aggregates but are a Section 26 requirement, so their producer was added.
- No frontend changes.
- "AI" recommendations are deterministic, explainable rules with a versioned model registry, not a
  trained model. The spec permits this ("explainable, versioned, human-approved"); it is stated
  explicitly so nobody assumes ML exists.
