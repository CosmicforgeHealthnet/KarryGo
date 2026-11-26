# Fleet Sections 21-33: API and database report

## Routes added: 68

All under `/fleet-partner/v1`, tenant-scoped, deny by default, each with a permission and feature
flag, registered in the risk registry, query allowlist and object-authorization classifier, and
covered by the route-coverage and OpenAPI parity tests. Every write requires `Idempotency-Key`;
commands on existing objects require an expected version.

| Feature | Routes | OpenAPI file (repo root) |
|---|---|---|
| Warehouse operations (21/51) | 24 | `docs/openapi/section21-warehouse-fulfilment.openapi.json` |
| Finance analysis + invoice submit (22/52) | 7 | `docs/openapi/section22-finance-billing-payroll.openapi.json` |
| Exceptions + SLA breaches (23/53) | 8 | `docs/openapi/section23-communications-support-sla.openapi.json` |
| Notification preferences (23/53) | 3 | same |
| Analytics, reports, recommendations, automation (24/54) | 20 | `docs/openapi/section24-analytics-reporting-automation.openapi.json` |
| Security events, privacy requests (28/56) | 6 | `docs/openapi/section28-56-governance.openapi.json` |

Exact operations are listed in 08-frontend-api-contract.md.

## Migrations: 000100-000104 (head `000104_privacy_requests`)

| Migration | Tables / columns | Permissions seeded |
|---|---|---|
| 000100 warehouse_inbound_outbound_operations | warehouse_receipts, warehouse_receipt_lines, putaway_tasks, pick_waves, pick_tasks, packing_units, gate_events (append-only) | warehouse_receipts.read/manage, warehouse_tasks.read/execute, pick_waves.manage, packing.manage, yard.gate |
| 000101 finance_analysis_cost_allocation | payroll_periods.calculation_hash; cost_allocations, profitability_snapshots (both append-only) | profitability.read/snapshot, cost_allocations.read/manage |
| 000102 operational_exceptions_notification_preferences | operational_exceptions, operational_exception_history, notification_preferences | exceptions.read/manage/resolve, notification_preferences.manage/manage_organization |
| 000103 analytics_recommendations_automation | report_definitions, report_runs, model_versions (seeded), recommendations, automation_rules, automation_rule_versions (append-only), automation_executions (unique rule+trigger), automation_cursors | analytics.read, reports.read/create, recommendations.read/generate/decide, automation.read/manage/approve |
| 000104 privacy_requests | privacy_requests, privacy_request_history | privacy.manage/decide, security_events.read |

All tables are prefixed `fleet_partner_`, carry `organization_id`, and are owned by fleet-partner-service.
No cross-service foreign key or join was added. Permissions are granted to the existing system roles
appropriate to each area (owner always; warehouse/finance/compliance/analyst/operations as relevant).

## Reversibility

Every migration has a down file. `TestMigrationsReplayAndRollback` applies all, rolls back, verifies
the down files actually remove their objects, and reapplies, against a real PostgreSQL 18.
The 000103 down file restores the earlier inactive `reports.read` permission row rather than deleting it.
