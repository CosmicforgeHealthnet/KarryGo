# Fleet Sections 5-20: API and database reconciliation

Date 2026-11-14. Branch `staging`. Route-level detail per requirement is in
the requirements matrix; this document records the shape of the API and the
schema changes.

## 1. Conventions now enforced on tenant routes

| Convention | Enforcement | Guard |
|---|---|---|
| Success envelope `{data, meta}` | `respond.OK/List` on every tenant route | section contract tests |
| camelCase JSON keys | 87 structs tagged (`aa4cfb9`) | `internal/architecture_json_tags_test.go` (fails on any untagged payload field; 7 internal contracts exempted by name) |
| Unknown query parameters refused | `platform/queryallow` middleware | `TestTenantListRoutesRejectUnknownQueryAndBadPaging` |
| Paging validated (422) and clamped | shared parser + `pagination.ClampOffset` | row tests |
| Versioned writes require If-Match/expectedVersion | providers, structure, staff, roles, settings, units, assignment cancel | row tests asserting 400 on a missing version |
| Idempotency-Key on commands | per manifest `IdempotencyRequired` | route coverage contract |
| Step-up on high-risk actions | `platform/stepup` map + `RequireStepUp` | step-up route list contract |
| Branch scope in SQL, NULL-safe | `RequireScoped` + `COALESCE(cardinality(...),0)=0` | `internal/architecture_branchscope_test.go` |
| Money in integer minor units | no float64 on money lines | `internal/architecture_money_test.go` |
| UTC timestamps; local zone stored separately | `12d7de8` | `TestRow6c_UTCTimestampsAndSeparateLocalTimezone` |

Open (matrix rows 6f, 6h, 6.1): offset-only lists in dispatch, workforce,
assets and `/marketplace/trips`; bespoke bodies on internal service routes
(kept by contract); money DTOs flatten to `xxxMinor` + `currency` instead of
a Money object.

## 2. Routes added in Phase 2 (selection; full list in the route-coverage registry)

- Approvals: `/approval-policies` (CRUD, versions), `/approvals/delegations`;
  approve/reject now step-up protected.
- Structure: `/cost-centres`, `/teams`, `/locations` (CRUD + archive).
- CRM: `/leads` (list, create, get, patch, assign, status, convert, history).
- Operations: `/rescue-cases` (open, offer replacement, resolve, cancel),
  geofence events, `/custody-events`, incident investigations, compliance
  checks, asset meter readings, defects, tyres, cost entries.
- Marketplace: `POST /marketplace/trips/:tripId/cancel` (new, FLEET-12.2).
- Providers: update, reactivate, remove, capabilities, policy, declines,
  resend login, provisioning status, history.
- Internal (other services): Dispatch
  `POST /api/v1/internal/marketplace/trips/:tripId/cancel`; Taxi
  `/api/v1/taxi/internal/restrictions/{apply,clear,operations/:ref,:org/:type/:ref}`;
  Wallet `GET` provider withdrawal by idempotency key, managed-settlement
  payment binding.

Every tenant route is in the risk registry, object-authorization registry,
query allowlist (collections) and, where high risk, the step-up map; the
contract tests fail otherwise.

## 3. Database (Fleet migrations, all reversible, contiguous)

| Migration | New tables | Altered tables |
|---|---|---|
| 000089 | shift_templates, training_enrollments | shifts |
| 000090 | approval_policy_versions, cost_centres, teams, operating_locations | approval_policies, approval_delegations, approval_requests, approval_decisions, departments |
| 000091 | sales_leads, sales_lead_history | |
| 000092 | rescue_cases, rescue_case_history, meter_readings, asset_defects, asset_defect_history, tyres, tyre_events, asset_cost_entries | asset_assignments, operational_alerts |
| 000093 | incident_investigations, compliance_checks | custody_events, incidents, specialized_records |
| 000094 | | provider_invitations (resend counters) |
| 000095 | | provider_provisioning_sagas (association read-back) |
| 000096 | | managed_work_settlement_instructions, temperature_events (per-record replay key) |
| 000097 | | completed_work_records, managed_work_settlement_instructions (payment_source_reference) |
| 000098 | | restriction_operations (TAXI authority allowed) |
| 000099 | | role permissions: operating-mode grants for the owner role (data) |

All table names carry the `fleet_partner_` prefix. The latest-migration
replay test rolls back 000099 to 000089 in order and re-applies them.

Other services: Wallet 000023 (managed settlement payment binding), Taxi 031
(restriction authority), Dispatch test harness now applies the legacy
rider-auth schema (no new Dispatch migration).

No replacement tables were created; every new table fills a gap the
Section 57 audit recorded.
