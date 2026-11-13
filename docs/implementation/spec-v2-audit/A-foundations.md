# Fleet backend vs. Corporate Logistics Spec v2.0 - Foundations (Sections 1-7, 34-36, 57)

Read-only audit of `services/fleet-partner-service` at committed HEAD `9e3f4ba` (2026-11-13 17:25 UTC, clean worktree). No repository file was changed. Sections 8-25 were covered in `F-sections-8-25.md` and are only cross-referenced here. Paths are relative to `services/fleet-partner-service/` unless stated; `f/` = `internal/features/`, `mig/` = `migrations/`.

Status: IMPLEMENTED (I) / PARTIAL (P) / MISSING (M) / EXTERNAL (E, owned by another service) / N/A (a statement, nothing to build).

## Summary

| Section | Title | Implemented | Partial | Missing | External | Notes |
|---|---|---|---|---|---|---|
| 1 | Purpose, scope, authority | 3 | 3 | 0 | 0 | 6 rows N/A. Gaps: structure APIs, dispatch/taxi provisioning, reports/automation |
| 2 | Frozen product decisions | 8 | 2 | 0 | 1 | Managed provider creation works only for HAULING |
| 3 | Platform boundaries | 10 | 4 | 0 | 0 | No other service consumes corporate events (analytics) |
| 4 | Target architecture and runtime | 12 | 0 | 0 | 0 | Routes, health, ready, metrics and workers all present |
| 5 | Repository structure and conventions | 8 | 4 | 0 | 0 | 13 feature dirs are `doc.go` placeholders; handlers and usecases use concrete types |
| 6 | API and data conventions | 11 | 3 | 0 | 0 | Money object unused; unknown-filter rejection only in CRM |
| 7 | Multi-tenancy and isolation | 10 | 3 | 0 | 0 | Branch, department and settings tables exist but have no API |
| 34 | Binding scope / completion standard | 5 | 4 | 0 | 1 | Missing tables (Sec. 57) and riskregistry gaps |
| 35 | Canonical service position and runtime | 23 | 10 | 3 | 0 | No `seeds/`, no `.air.toml`; automation module missing |
| 36 | Global API and data conventions | 12 | 7 | 2 | 0 | Canonical `/api/v1/corporate-logistics` paths not used; meta.version/total missing |
| 57 | Canonical PostgreSQL schema | 65 same-name + 39 renamed | 12 tables | 26 tables | - | IDs are prefixed text, not UUID; no DB-role/GRANT separation |

**Most important findings**

1. **Canonical path deviation (36).** The service serves `/fleet-partner/v1` (corporate), `/internal/fleet-partner/v1` (internal), `/fleet-partner/integrations/v1` (organization API) and `/fleet-partner/public/v1` (secure customer links) (`internal/app/app.go:188-303`). The api-gateway routes `/fleet-partner/` (`services/api-gateway/internal/features/routing/models/routes.go:83`). The paths match Part I Sec. 4.2, but not the binding Part II Sec. 36 paths (`/api/v1/corporate-logistics`, `/internal/v1/corporate-logistics`, `.../customer-portal`, `.../integrations`). This needs a product decision. Renaming would be a cross-service change, because dispatch, taxi, hauling and shared clients hard-code `/internal/fleet-partner/v1`.
2. **26 schema-catalogue tables have no equivalent (57).** They include teams, cost_centres, operating_locations, sales_leads, provider_availability_projections, provider_shift_templates, training_enrollments, asset_defects, work_order_tasks, inventory_locations/balances, meter_readings, incident_investigations, the warehouse receipt/putaway/pick/pack set, provider_statements, cost_allocations, profitability_snapshots, report_definitions/runs and automation_rules/executions.
3. **Structure APIs are absent (7, 35).** `fleet_partner_branches` and `fleet_partner_departments` exist (`mig/000002`), and migration 000087 plus `f/organizations/usecases/onboarding_service.go:371-373` now give every org an ACTIVE `HQ` branch. There are still no routes for branches, locations, departments, teams, cost-centres or `organizations/{id}/settings` (see `f/organizations/http/handlers.go:53-103`). The matching web/fleet settings pages are 9-line TODO stubs.
4. **Managed provider provisioning is hauling-only (2, 3).** Only `contracts.Providers[CategoryHauling]` is wired (`internal/app/wiring.go:271`). DISPATCH and TAXI use `UnconfiguredProvider` and always fail closed (`internal/platformclients/contracts.go:375-384`). Dispatch and taxi expose no provisioning or link endpoint.
5. **The convention helpers exist but are not applied consistently (6, 36).**
   - `platform/money.Money{amountMinor,currency}` is used by 0 feature files. DTOs use flat `*Minor` fields instead.
   - `f/finance/ingestion.go:78-82` decodes amounts through `float64`.
   - `platform/filters` (allowlist, rejects unknown filters) is imported nowhere. Only CRM rejects unknown query parameters (`f/crm/http/handlers.go:275-285`).
   - The success envelope has no `meta.version`, and lists have no `page/limit/total` (`platform/respond`).
6. **Corporate events go nowhere outside fleet (3).** The outbox publishes to Redis stream `fleet-partner:events` (`internal/events/redis_publisher.go:16`). Only fleet's own Section 26 consumer reads it. analytics-service, admin-backoffice-service and the others do not consume it.
7. **No database-role separation (57.8).** No migration contains GRANT/REVOKE. Migrations run from the runtime pool when `FLEET_PARTNER_MIGRATION` is set (`cmd/main.go:71-79`). Append-only behaviour is enforced by triggers (`fleet_partner_forbid_mutation`, in 47 migrations), not by role privileges.

---

## Section 1 - Purpose, Scope and Implementation Authority (spec l.90)

| Req | Requirement | Status | Evidence / gap |
|---|---|---|---|
| 1.0 | Purpose; this doc supersedes the draft; backward compatibility | N/A | Statement. Backward compatibility is tracked in 34.1. |
| 1.1a | Org onboarding, verification state, branches, hubs, departments, staff, governance | P | Onboarding, verification, staff and approvals exist (see prior audit, Sec. 8/9). Branch/department tables exist and an HQ branch is auto-created (`mig/000087`), but there are no branch/hub/department/team APIs. |
| 1.1b | Company-created provider accounts linked to dispatch/taxi/hauling | P | `f/providers` with `fleet_partner_organization_provider_memberships` (`mig/000007`). Only the hauling contract is wired (`internal/app/wiring.go:262-272`). |
| 1.1c | Org-aware request acceptance, assignment, provider accept/decline | I | `f/marketplace` (prior audit Sec. 11: 12 I / 1 P). Provider routes need `FLEET_MARKETPLACE_ALLOWED_CALLERS`. |
| 1.1d | Master wallet + provider sub-wallet via payment-wallet | I | `f/companywallet`. `EnsureSubWallet` (`f/providers/usecases/lifecycle.go:77-119`). |
| 1.1e | Private operations (customers ... automation) | P | Most modules exist. Reports and automation are missing (prior audit Sec. 24: 2 I / 11 M). |
| 1.1f | Internal/external APIs, events, projections, workers, migrations, security, observability, tests | I | `internal/app`, `internal/events`, `internal/jobs`, `internal/observability`, 87 migrations, `internal/integration`, `section*contract`. |
| 1.2 a-e | Out of scope: pricing formulas, customer booking redesign, replacing services, UI design, punitive decline action | N/A | Respected. `internal/architecture_test.go:139` `TestNoPricingFormulas` passes. Safety declines go to `fleet_partner_safety_decline_inbox` / `decline_followup_actions` with no punitive mutation. |

## Section 2 - Frozen Product Decisions (l.129)

| Req | Decision | Status | Evidence / gap |
|---|---|---|---|
| 2.0 | Every provider can accept/decline every offer and assignment; company cannot remove Decline or force a start | I | Provider accept/decline for offers and assignments (`f/marketplace/http/provider_response_handlers.go`, `fleet_partner_assignment_responses`, `provider_offer_decision_receipts` in `mig/000081`). `provider_decline_required boolean DEFAULT true CHECK(=true)` (`mig/000007:117-118`). Caveat: these routes are mounted only when `FLEET_MARKETPLACE_ALLOWED_CALLERS` is non-empty (prior audit finding 7). |
| 2.1 | Corporate service is additive | I | Sibling service with its own DB (`internal/config/config.go:546`). `TestNoOtherServiceInternalsImported`. |
| 2.2 | Customer flow unchanged | E | Owned by dispatch/taxi/hauling. Fleet adds no customer-booking surface. |
| 2.3 | No special corporate queue | I | Offers arrive from the execution services' own matching via internal calls (`/internal/fleet-partner/v1/marketplace-offers/...`, called from dispatch, taxi and hauling clients). |
| 2.4 | Company provisions/links provider by name, phone, email | P | `POST /providers` with `onboarding_source IN ('ORGANIZATION_SPONSORED','EXISTING_PROVIDER_LINKED')` (`mig/000007`). Succeeds only for HAULING. DISPATCH/TAXI fail closed (`platformclients/contracts.go:375-384`). |
| 2.5 | No repeated self-verification (ORGANIZATION_SPONSORED mode) | P | The mode is recorded in fleet only. `ORGANIZATION_SPONSORED` appears in no provider service. Dispatch and taxi have no sponsored-onboarding endpoint; hauling uses provider-link calls. |
| 2.6 | Accept/decline on offers and assignments; decline needs a reason and is auditable | I | Prior audit Sec. 11. `fleet_partner_assignment_responses`, `assignment_state_history`, audit records. |
| 2.7 | Company owns marketplace settlement; trips separate the executor from the settlement org | I | `fleet_partner_trip_projections.executing_provider_id` / `settlement_organization_id` (`mig/000010`). `fleet_partner_managed_work_settlement_instructions`. Execution services have `ORGANIZATION_MANAGED` settlement routes. |
| 2.8 | Sub-wallet for every managed provider, created on activation, allocations only | I | `EnsureSubWallet` on activation (`f/providers/usecases/lifecycle.go:77`), `fleet_partner_provider_subwallet_links`, `provider_allocation_instructions`. Provider self-service sub-wallet routes are not mounted (prior audit). |
| 2.9 | Provider DTOs never contain fare, commission, settlement or contract price | I | Shaped views per `internal/platform/fieldvisibility/registry.go`. Asserted in `internal/integration/field_visibility_test.go` and `provider_response_test.go:138-140`. |
| 2.10 | No commission/subscription/fare formula hard-coded | I | `TestNoPricingFormulas`. Compensation rules are org-configured data. |

## Section 3 - Existing KarryGo Platform Boundaries (l.162)

| Req | Domain -> corporate responsibility | Status | Evidence / gap |
|---|---|---|---|
| 3.a | customer-service: store refs and org-owned external customers only | I | `fleet_partner_company_customers` (`mig/000023`). The customer-service client is Ping-only (`platformclients/clients.go`). There is no `platform_customer_id` reference column (see 57.4). |
| 3.b | Dispatch: provision/link riders, assign work, consume trip events | P | Assignment, offer and trip integration exists (dispatch calls `/internal/fleet-partner/v1/assignments`, `.../marketplace-offers`). Rider provisioning is unwired (finding 4). |
| 3.c | Taxi: provision/link, consume ride lifecycle | P | Offer/assignment integration exists (`driver-taxi-service` calls `/providers/%s/offers|assignments`). Provisioning is unwired. |
| 3.d | Hauling: provision/link, consume lifecycle | I | `platformclients/hauling_provider.go`, `HaulingLinks` contract, `hauling_execution.go`. |
| 3.e | payment-wallet: request wallets/sub-wallets, projections, no local ledger | I | `platformclients/payment_wallet.go`; `fleet_partner_organization_wallet_provisioning`, `finance_projections`, `wallet_transaction_projections`; `TestNoLocalFinancialLedger` (`internal/architecture_auth_test.go:205`). |
| 3.f | notification-service: intents, org templates/preferences | I | `fleet_partner_notification_intents`, `notification_rules`, `notification_delivery_attempts` (`mig/000051`); notification adapter (`internal/app/wiring.go:419`). |
| 3.g | media-file-service: file IDs + metadata + ownership | I | `fleet_partner_file_references(organization_id, file_id, purpose, subject_*)` (`mig/000001`); `platformclients/media_file.go`. |
| 3.h | verification-compliance: org-sponsored verification path; consume status changes | P | Org verification requests and issues (`mig/000004`, `platformclients/verification.go`). There is no organization-sponsored *provider* verification path (see 2.5). |
| 3.i | support-dispute: org context + projections | I | `platformclients/support_reference.go`, `fleet_partner_support_case_projections`, `support_case_events_inbox`. support-dispute-service has a corporate client (`support-dispute-service/internal/features/support/clients/corporate.go`). |
| 3.j | analytics: publish corporate events, consume aggregated metrics | P | The analytics client is Ping-only. Fleet is an allowed reader of network analytics (`analytics-service/internal/config/config.go:45`). No analytics consumer exists for `fleet-partner:events`. |
| 3.k | admin-backoffice: internal review APIs/events; no admin functions in corporate web | I | Admin-owner and investigation routes are on the HMAC internal group only (`cmd/main.go:438-441`, `f/adminresources`, `f/admininvestigations`). |
| 3.l | api-gateway Partner API stays separate; org integration is scoped | I | Separate `/fleet-partner/integrations/v1` namespace and auth (`app.go:214-222`), `fleet_partner_organization_api_clients`; `TestPartnerAPINotImported` (`architecture_test.go:110`). |
| 3.1 | Source-of-truth: immutable external IDs; cached fields carry source version | I | `source_version` in 29 migrations (memberships, trip/request projections, finance projections, ...). `internal/platform/sourceref`. |
| 3.2 | No direct DB access to other services | I | Only `FLEET_PARTNER_DATABASE_URL`. `TestNoForeignServiceDSNsInConfig` (`architecture_test.go:161`); `migrations_test.go:83 TestNoCrossServiceReferences`. |

## Section 4 - Target Architecture and Deployment Position (l.202)

| Req | Requirement | Status | Evidence |
|---|---|---|---|
| 4.0 | Apps -> API Gateway -> fleet -> downstream services | I | Gateway route `/fleet-partner/` -> UpstreamFleet (`api-gateway/.../routes.go:83`). |
| 4.1a | Go HTTP service, `cmd/main.go` + `internal/features` | I | `cmd/main.go`, `internal/app/app.go BuildRouter`. |
| 4.1b | Dedicated PostgreSQL | I | `config.go:546`; `internal/database`. |
| 4.1c | Redis for bounded caches, locks, invitation state, presence (where approved) | I | Namespaced TTL cache (`platform/cache/cache.go`, used by `f/accesscontrol/authorizer.go`), Redis rate limiter (`app/wiring.go:243-245`), Redis Streams outbox/consumer. Locks use Postgres `SKIP LOCKED`; invitations live in Postgres. There is no realtime presence (optional). |
| 4.1d | Transactional outbox + consumer inbox | I | `fleet_partner_outbox`, `fleet_partner_inbox` (`mig/000001`); `internal/events/outbox.go`, `inbox.go`. |
| 4.1e | Worker with leased jobs, heartbeats, recovery | I | `cmd/worker/main.go` (dedicated) and in-process workers (`cmd/main.go:105-205`); `internal/jobs` with lease/heartbeat/attempts/DLQ; `fleet_partner_job_attempts`. |
| 4.1f | Service clients: service auth, timeouts, retry classification, circuit-breaker metrics | I | `platformclients/base.go:26-200` (backoff + jitter, Retry-After, per-service breaker, `OnBreakerTransition`); `TestExternalHTTPClientsHaveExplicitTimeout`. |
| 4.2a | `/fleet-partner/v1/*` corporate web | I | `app.go:225-300`. |
| 4.2b | `/internal/fleet-partner/v1/*` service API | I | `app.go:303-327`; HMAC (+ optional scope verifier) + nonce replay protection. |
| 4.2c | `/health` liveness | I | `app.go:145` (+ `/live`). |
| 4.2d | `/ready` DB, config and worker readiness | I | `app.go:150-174`; checks in `cmd/main.go:207-405` (postgres, migrations current, redis, worker, stale leases, DLQ backlog, outbox backlog, registries, audit persistence, flag-dependent clients). |
| 4.2e | `/metrics` protected | I | `app.go:177-181` with `middleware.MetricsAuth` bearer (`middleware/auth.go:240-253`). Config validation refuses enabled metrics without a token (`config.go:976`). |

## Section 5 - Repository Structure and Coding Conventions (l.246)

| Req | Requirement | Status | Evidence / gap |
|---|---|---|---|
| 5a | `cmd/main.go`, `cmd/worker/main.go` | I | Both present. |
| 5b | `internal/{config,database,middleware,platformclients,events,jobs,observability}` | I | All present. |
| 5c | Feature packages: organizations, staff, accesscontrol, approvals, providers, assignments, marketplace, wallets, customers, contracts, servicecatalog, orders, shipments, dispatch, routes, tracking, workforce, assets, inspections, maintenance, fuel, compliance, safety, proof, warehouse, finance, communications, reports, automation | P | Every directory exists, but `assignments, communications, contracts, customers, fuel, inspections, maintenance, reports, routes, tracking, wallets` contain only `doc.go`, and `accesscontrol` only `authorizer.go`. Their logic lives in `marketplace`, `engagement`, `crm`, `assets`, `dispatch`, `companywallet`. `reports` has no implementation and `automation` only has `section27_runner.go`. |
| 5d | `migrations/`, `docs/`, `.env.example`, `go.mod/go.sum` | I | Present. |
| 5e | Each feature uses models/repositories/usecases/http (+clients) | P | 29 of 58 feature dirs are flat (`service.go`/`domain.go`/`http.go`), e.g. `workforce`, `warehouse`, `assets`, `financeops`, `dispatch`, `engagement`. |
| 5f | No feature imports another feature's HTTP package | I | `TestFeaturesDoNotImportOtherFeatureHTTPOrRepositories` (`architecture_test.go:88`). |
| 5g | shared/go only for generic code | I | Uses `shared/go` apperrors, httpx, auth, serviceauth, redisx, logging only (`docs/architecture.md:69`). |
| 5.1a | Handlers depend on use-case interfaces | P | Handlers hold concrete `*orgusecases.Service`, `*Service` (`f/organizations/http/handlers.go:32-37`, `f/workforce/http.go:71-74`). |
| 5.1b | Use cases depend on repository/client interfaces | P | Platform clients are interfaces (`platformclients/clients.go`), but repos are concrete (`*orgrepos.OrganizationRepository`, `onboarding_service.go:42-44`). Flat features embed SQL in `service.go`. |
| 5.1c | Adapters wired at composition root | I | `internal/app/*wiring*.go`, `*_adapters.go`, `*_ports.go`, invoked from `cmd/main.go:103`. |
| 5.1d | Domain models free of DB/HTTP/SDK | I | `TestDomainModelsAreInfrastructureFree` (`architecture_test.go:69`). Flat `*/domain.go` files also import no pgx, gin or net/http (checked). |
| 5.1e | Cross-feature collaboration through interfaces/events | I | Ports in `internal/app/*_ports.go`; architecture test as 5f. |

## Section 6 - API and Data Conventions (l.296)

| Req | Concern | Status | Evidence / gap |
|---|---|---|---|
| 6a | JSON; multipart only for upload handoff | I | `middleware.RequireJSON` on all groups. Exempt only `/organizations/:id/documents/upload-content` (`app.go:226`). |
| 6b | Bearer for users; service auth for internal | I | `CorporateBearer`, `ResolveTenant`, `InternalServiceAuth`/`InternalServiceScopeAuth` + `RejectReplayedNonce` (`app.go:225-313`). |
| 6c | UTC ISO-8601; local timezone stored separately | I | `timestamptz` everywhere. `timezone` on organizations, branches, shifts, warehouses and scheduled tasks. |
| 6d | Money = amountMinor + ISO currency; no float64 arithmetic | P | Amounts are int64 `*_minor` plus currency CHECK `^[A-Z]{3}$`. JSON uses flat `amountMinor`/`xxxMinor` + currency rather than the Money object. `f/finance/ingestion.go:78-82` reads `amountMinor` as `float64` (precision loss above 2^53). `f/fleetpartner/models.go:345` routes a share through `math.Max(float64)`. |
| 6e | Opaque prefixed IDs; no sequential IDs | I | `internal/platform/ids/ids.go` (`org_`, `brn_`, ... + random UUID hex). |
| 6f | Cursor pagination; page/limit only for compatibility | I | `internal/platform/cursor` (sort value + ID tie-breaker, `MaxLimit=100`), used in 30 feature files. |
| 6g | Allowlisted filters/sorts; reject unknown | P | `internal/platform/filters` implements this but is imported by no feature. Only CRM rejects unknown parameters (`f/crm/http/handlers.go:275-285`). Other list handlers ignore unknown query parameters. |
| 6h | Success `{"data","meta"}` | I | `internal/platform/respond` (309 call sites). A few internal ingestion handlers write raw `gin.H{"data":...}` or `{"status","code"}` (e.g. `f/finance/http.go:17-36`, `f/backgroundops/http/internal.go`, 81 `c.JSON` call sites). |
| 6i | Error `{"error":{code,message,details,requestId}}` | I | `respond.Error` (adds `retryable`). Same internal-handler exceptions as 6h. |
| 6j | Idempotency-Key required on financial, provisioning, import and assignment actions | I | `internal/platform/idempotency` (`ExecuteHTTP` rejects a missing key; fingerprint mismatch -> 409), used in 38 feature files. Provider create and staff invite enforce the key (`f/organizations/http/handlers.go:165,260,887`). |
| 6k | Version + If-Match/expectedVersion | I | `internal/platform/concurrency`. `expectedVersion` in 900+ places. If-Match in CRM, packages, marketplace, warehouse, financeops, retention. |
| 6l | Audit: actor, org, previous/new summary, request ID | I | `fleet_partner_audit_records(previous_summary,new_summary,request_id,correlation_id)` with append-only trigger (`mig/000001:129`) and hash chain (`mig/000069`). |
| 6.1 | Standard Money object | P | `internal/platform/money.Money{AmountMinor,Currency}` exists (json `amountMinor`/`currency`) but no feature uses it. |
| 6.2 | Pricing deferral | I | As 2.10. Org-entered amounts are stored as snapshots. |

## Section 7 - Multi-Tenancy and Organization Isolation (l.337)

| Req | Control / entity | Status | Evidence / gap |
|---|---|---|---|
| 7a | Tenant context from authenticated membership | I | `middleware.ResolveTenant` (`app.go:276`); `meta/tenant` probe (`app.go:285-299`). |
| 7b | Repository filtering by organization_id plus permitted branch IDs | I | `PermittedBranchIDs` enforced in 44 feature files. `TestSQLAlwaysTenantScoped` only checks 2 tables (`architecture_test.go:183-197`), so the guard is narrow. |
| 7c | Composite uniqueness with organization_id | I | e.g. `UNIQUE(organization_id,code)` on branches, departments, warehouses, assets. |
| 7d | Ownership checked before a 404/403-safe response | I | `internal/platform/objectauth` registry plus ready-check `objectauth.SelfCheck` (`cmd/main.go:279-282`). |
| 7e | Media metadata has org ownership and purpose | I | `fleet_partner_file_references` (`mig/000001`). |
| 7f | Every event has organizationId + correlationId | I | `internal/events/envelope.go:21-31`. |
| 7g | Job payload organizationId revalidated on execution | I | `internal/jobs/jobs.go:4-5,39-41` (`ErrTenantMismatch`). |
| 7h | Exports tenant-scoped, encrypted at rest, time-limited | I | `fleet_partner_export_jobs(organization_id, expires_at)` (`mig/000055`, `000059`, `000061`). Files go to media-file-service private storage; encryption at rest is that service's responsibility (not verified here). |
| 7.e1 | organizations (status, legal identity, capabilities, country, tz, currency, version) | I | `mig/000002` + statuses `mig/000008` + `organization_type` `mig/000086`. |
| 7.e2 | branches (type, address, geofence, hours, status) | P | Table exists (`mig/000002`; `type IN BRANCH/HUB/DEPOT/WAREHOUSE/OFFICE`, `geofence_reference`, `operating_hours` text). Only the auto HQ branch is created; there is no create/update/archive API (prior audit 8.15). |
| 7.e3 | departments (branch, parent, cost centre) | P | Table exists (`mig/000002`, `cost_centre_id` is untyped text, no cost_centres table). No API. |
| 7.e4 | organization_settings (assignment timeout, approval thresholds, provider visibility, retention) | P | Table has all of these (`mig/000002`, `+operating_mode` `mig/000010/000077`, `+require_assignment_acknowledgement` `mig/000013`). There is no `GET/PATCH /organizations/{id}/settings` route. |
| 7.e5 | organization_status_history (immutable) | I | `mig/000002` with append-only trigger (`:108-110`). |

## Section 34 - Binding Scope and Completion Standard (l.2483)

| Req | Requirement | Status | Evidence / gap |
|---|---|---|---|
| 34.0 | Mandatory provider accept/decline right | I | As 2.0. |
| 34.0b | No hard-coded commercial percentages; versioned policy refs | I | `TestNoPricingFormulas`. Versioned compensation plans (`fleet_partner_compensation_plan_versions`, `rule_versions`). |
| 34.1a | Every entity has table, constraints, index, migration, repository and test | P | 26 catalogue tables are missing (see 57). Several reserved feature packages are empty (5c). |
| 34.1b | Every command has authn, authz, validation, idempotency, concurrency and audit | P | Generally present. The prior audit found 36 mounted routes without a riskregistry entry, and unknown-filter validation is missing (6g). |
| 34.1c | Every state change enforces its transition and writes history + outbox | P | History tables exist for most aggregates (e.g. `organization_status_history`, `assignment_state_history`, `workforce_history`, `asset_history`, `manifest_history`, ...). Branches, departments, settings and roles have no history table. |
| 34.1d | Downstream writes: timeout, retry, unknown-outcome reconciliation, operator recovery | I | `platformclients` outcome classification; reconciliation tables (`projection_reconciliation_cases`, `private_execution_reconciliation_cases`, `document_reconciliation_cases`, `synchronization_cases`); worker-operations internal routes (`cmd/worker_operations_routes.go`). |
| 34.1e | Financial movement only through payment-wallet, or explicit non-custodial entries | I | `TestNoLocalFinancialLedger`. Payroll posting goes through `financial_posting_operations` to Wallet. |
| 34.1f | Workers: durable, leases, heartbeats, bounded retry, DLQ | I | `fleet_partner_jobs` (lease_owner/expires, heartbeat_at, max_attempts, DEAD_LETTERED), `job_attempts`; outbox `DEAD_LETTERED`. |
| 34.1g | Existing customer and independent-provider behaviour unchanged | E | Must be verified in dispatch, taxi and hauling. |
| 34.1h | Mandatory security/migration/contract/resilience/E2E tests pass before rollout | P | Gate exists (`cmd/section31-testgate`, `docs/section31-test-coverage.json`, `internal/section31contract`). Not executed by this audit (needs Postgres). The prior audit shows the coverage JSON over-claims Sections 37-39/52-55. |

## Section 35 - Canonical Service Position and Runtime (l.2514)

| Req | Requirement | Status | Evidence / gap |
|---|---|---|---|
| 35a | Sibling Go service under services/fleet-partner-service | I | Present. Product name "Corporate Logistics" is used in docs; `ServiceName="fleet-partner-service"` (`config.go:25`). |
| 35b | Distinct from api-gateway Partner API (accounts, keys, billing, authz) | I | Own `fleet_partner_organization_api_clients`, own namespace; `TestPartnerAPINotImported`. |
| 35c | `internal/app/` wiring and lifecycle | I | `internal/app/`. |
| 35d | `internal/config/`, `internal/database/` | I | Present (env validation `config.go:932+`, migration runner `database/migrate.go`). |
| 35e | `internal/platform/` (auth, errors, audit, idempotency, outbox, inbox, jobs) | I | `internal/platform/{audit,idempotency,respond,errorcodes,serviceauth,...}`. Outbox/inbox live in `internal/events/` and jobs in `internal/jobs/` (location differs, content present). |
| 35f | `internal/integrations/` typed clients | I | Named `internal/platformclients/`. (`f/integrations` is the org integration API.) |
| 35g | `internal/features/` | I | 58 feature dirs. |
| 35h | `migrations/` | I | 87 up/down pairs. |
| 35i | `seeds/` | M | Directory absent. |
| 35j | `docs/openapi/` | P | 16 specs. Marketplace, providers, staff/IAM, company wallet and Sections 19-23 have no committed spec (only `generate_sections22_23.go`). |
| 35k | `scripts/` | I | `backup.sh`, `ci.sh`, `restore-verify.sh`, `smoke/`. |
| 35l | `.env.example` | I | Present. |
| 35m | `.air.toml` | M | Absent. |
| 35n | `go.mod`, `go.sum` | I | Present. |
| 35-M1 | organizations module | I | `f/organizations`. |
| 35-M2 | structure: branches, hubs, depots, warehouses, departments, teams, cost centres | P | Branch and department tables, warehouses (`f/warehouse`). No teams, cost centres or operating locations, and no branch/department API. |
| 35-M3 | access: users, roles, branch scope, approvals | P | `f/staff`, `f/approvals`, `f/accesscontrol`. No role CRUD, role-binding grant/revoke, approval policy or delegation routes (prior audit Sec. 9). |
| 35-M4 | providers: membership, service roles, shifts, employment info | P | `f/providers`. Reactivate, remove, PATCH, capabilities, policies, availability and declines routes are missing; DISPATCH/TAXI are unwired (prior audit Sec. 10). |
| 35-M5 | marketplace: offer/assignment orchestration | I | `f/marketplace`. |
| 35-M6 | customers | I | `f/crm` (`company_customers`, contacts, locations, credit). |
| 35-M7 | commercial: leads, quotations, contracts, SLAs, service catalogue | P | Quotations, contracts, SLAs, opportunities and service catalogue exist. Leads are missing. |
| 35-M8 | orders: orders, stops, shipments, packages, manifests | I | `f/orders`, `f/shipments`, `f/packages`, `f/manifests`. |
| 35-M9 | dispatch: routes, board, planning, rescue | I | `f/dispatch` (plans, routes, optimization, control). |
| 35-M10 | workforce: shifts, leave, training, qualifications, performance | P | `f/workforce`. No shift templates or training enrollments (prior audit Sec. 17). |
| 35-M11 | assets | I | `f/assets`. |
| 35-M12 | maintenance: inspections, defects, work orders, parts, return-to-service | P | Inspections, work orders, parts and RTS decisions exist. No defect register, no work-order tasks, no multi-location parts stock. |
| 35-M13 | fuel | I | `fleet_partner_fuel_records` + approval history (in `f/assets`). |
| 35-M14 | compliance | I | `f/compliance` (requirements, statuses, restrictions). |
| 35-M15 | warehouse: stock, fulfilment, returns | P | Warehouses, storage locations, inventory, reservations, fulfilment and returns exist. No receipts/putaway/pick/pack task tables. |
| 35-M16 | yard: gate, yard, dock, terminal | I | `yard_visits`, `dock_appointments`, `terminal_transfers` (in `f/warehouse`). |
| 35-M17 | finance: invoices, receivables, expenses, payroll, budgets | P | `f/financeops`. Profitability and cost allocation are missing; receivables ageing is missing (prior audit). |
| 35-M18 | wallet_facade | I | `f/companywallet`. |
| 35-M19 | communications | I | `f/engagement`. |
| 35-M20 | analytics: read models, reports, exports | P | `f/dashboard`, `f/exports`. `f/reports` is `doc.go` only. |
| 35-M21 | automation: governed workflow rules | M | `f/automation/section27_runner.go` only. No rule or execution tables or API (prior audit Sec. 24). |
| 35-M22 | integrations: API clients, webhooks, imports | I | `f/integrations`, `f/bulkimports`. |

## Section 36 - Global API and Data Conventions (l.2584)

| Req | Requirement | Status | Evidence / gap |
|---|---|---|---|
| 36.p1 | External web API `/api/v1/corporate-logistics` | M | Served at `/fleet-partner/v1` (`app.go:188,225`); gateway prefix `/fleet-partner/`. Deviation from the binding path. |
| 36.p2 | Internal `/internal/v1/corporate-logistics` | M | Served at `/internal/fleet-partner/v1` (`app.go:303`). Dispatch, taxi, hauling and shared clients hard-code this path. |
| 36.p3 | Customer portal `/api/v1/corporate-logistics/customer-portal` | P | Only the quotation secure-link API, at `/fleet-partner/public/v1` (`app.go:204-207`). No general customer portal. |
| 36.p4 | Organization API `/api/v1/corporate-logistics/integrations` | P | Exists at `/fleet-partner/integrations/v1` (`app.go:214-222`). Path differs. |
| 36.p5 | `/health`, `/ready` | I | As 4.2. |
| 36.p6 | `/metrics` internal only | I | Token-protected (4.2e). |
| 36.a | Bearer + service-signed internal credentials | I | As 6b. |
| 36.b | Org scope from session; body/X-Organization-ID never override | I | Corporate routes resolve the tenant from the session. The org API rejects `X-Organization-ID`/body org (`f/integrations/auth.go:125`). Some *internal HMAC* routes take org from `X-Organization-ID` (`f/financeops/http.go:357`, `f/engagement/http.go:263,275`, `f/documents/http/external.go:220`); that is a trusted-caller path, but the scope should be validated against the payload. |
| 36.c | Idempotency: same key + same hash replays; different hash -> 409 | I | `platform/idempotency/idempotency.go:133,178-180`. |
| 36.d | ETag/If-Match or explicit version on destructive/state-sensitive updates | I | `expectedVersion`/If-Match (6k). No `ETag` response header is emitted anywhere. |
| 36.e | Money int64 + ISO-4217; never inferred | P | As 6d. |
| 36.f | RFC 3339 UTC; IANA timezone kept for schedules; DST ambiguity resolved explicitly | P | timestamptz + timezone columns. Explicit DST handling only in `internal/scheduler/cron.go` and `f/crm/models/models.go`; shifts and maintenance schedules do not address it. |
| 36.g | UUID v7 preferred; no cross-service FK assumption | P | Prefixed random UUID (v4) hex in text columns (`ids.go:31-34`). No cross-service FK (`TestNoCrossServiceReferences`). |
| 36.h | Cursor preferred; max 200; stable order with id tie-breaker | I | `platform/cursor` (MaxLimit 100, id tie-breaker). |
| 36.i | Files via media fileId; no binary or permanent public URL | I | `media_file_id`/`file_id` references; preview/download via grants. |
| 36.j | Archive/deactivate by default | I | Only 3 DELETE routes (document detach, route stop, session revoke); everything else archives or changes status. |
| 36.k | Error envelope with code, message, details, retryable, requestId; no internals | I | `respond.ErrorBody` (`retryable` included); 5xx causes logged only. |
| 36.l | Versioned event envelope + outbox + inbox dedupe | I | `events/envelope.go` (`schemaVersion`, `aggregateVersion`), `fleet_partner_inbox PK(consumer,event_id)`. |
| 36.s1 | Success `{"data":{...},"meta":{"requestId","version"}}` | P | `respond.Meta` has only `requestId` and `nextCursor`; no `version`. |
| 36.s2 | List meta `page, limit, total, nextCursor, requestId` | P | Only `nextCursor` + `requestId`. |
| 36.s3 | Error envelope shape | I | Matches. |

## Section 57 - Canonical PostgreSQL Schema Catalogue (l.3797-4160)

All fleet tables carry the `fleet_partner_` prefix. 142 spec tables were compared against 306 `CREATE TABLE` statements across 87 migrations.

### 57.0 General rules

| Rule | Status | Evidence / gap |
|---|---|---|
| id UUID PK, organization_id NOT NULL, status, version, created_at/updated_at, archived_at | P | IDs are `text` (prefixed opaque). `version` and timestamps are near-universal. `archived_at` exists only on some tables (organizations, branches, departments, assets, warehouses, identities). Some child tables do not REFERENCE organizations (e.g. `fleet_partner_inventory_reservations`, `fulfilment_lines`). |
| Hot indexes begin with organization_id | I | About 243 of ~390 indexes start with `organization_id`; most of the rest are global infra (status, expires, lease). |
| Append-only tables deny UPDATE/DELETE | I | Enforced by `fleet_partner_forbid_mutation()` and similar triggers in 47 migrations, not by role privileges. |
| State change writes history + audit + outbox in one tx | I | Pattern followed in tx helpers (`internal/database/tx.go`). Exceptions as 34.1c. |
| PostGIS only if approved; otherwise numeric coordinates | I | `latitude/longitude double precision`; geofences store geometry JSON. |
| Case-insensitive email; phone E.164 + display | I | `fleet_partner_corporate_identities(email_normalized, email_display, phone_e164, phone_display)` (`mig/000003`). |

### 57.1 Organization and access (21 tables)

| Spec table | Fleet table | Status | Notable differences |
|---|---|---|---|
| organizations | organizations | I | No `onboarding_state` column (status covers it). Statuses lack `submitted`/`closing` from 37.2. UNIQUE(country,registration) is not partial on `archived_at`. |
| organization_profiles | organization_profiles | I | No `logo_file_id`, no `metadata`. |
| organization_capabilities | organization_capabilities | I | PK(org,capability) + `id` (`mig/000045`). Uses enabled_at/disabled_at instead of effective_at/suspended_at/reason_code; history in `capability_history`. |
| branches | branches | I | `address text` (not JSONB). Uniqueness is not partial on archived. No API. |
| operating_locations | - | M | |
| departments | departments | I | `cost_centre_id` is free text. |
| teams | - | M | |
| cost_centres | - | M | `budgets.cost_centre` is text. |
| organization_settings | organization_settings | I | No `provider_price_visibility`, no `provider_decline_required` (that column lives in `provider_policies`, `mig/000007:117`), `operating_mode` instead of `assignment_mode`. |
| organization_status_history | organization_status_history | I | Append-only trigger. |
| organization_users | organization_users | I | `identity_id` instead of `identity_subject`; email/phone on `corporate_identities`; no `default_branch_id`/`last_login_at`. |
| organization_invitations | organization_staff_invitations | I | Renamed. |
| roles / permissions / role_permissions | roles / permissions / role_permissions | I | |
| user_roles | role_bindings | I | Single `scope_type/scope_id` (ORG/BRANCH/DEPARTMENT) per grant instead of `branch_scope UUID[]`. |
| approval_policies / requests / steps / decisions / delegations | same (`fleet_partner_approval_*`) | I | Plus `approval_executions`, `approval_state_history`. |

### 57.2 Managed providers and workforce (21 tables)

| Spec table | Fleet table | Status | Notable differences |
|---|---|---|---|
| provider_memberships | organization_provider_memberships (+ managed_provider_members `mig/000079`) | I | Has onboarding_source, employment_type, employee_number, home_branch_id, source_version. |
| provider_invitations | provider_invitations | I | |
| provider_service_roles | provider_capabilities | I | service_type, vehicle_classes JSONB, validity. |
| provider_branch_assignments | - | M | Only `home_branch_id` on the membership. No history of primary/secondary branches. |
| provider_vehicle_assignments | asset_assignments | I | Keyed on `provider_membership_id`. |
| provider_availability_projections | - | M | No availability/location projection. |
| provider_compensation_profiles | compensation_plans / plan_versions / membership_compensation_assignments | I | Versioned plans instead of effective-dated profiles. |
| provider_subwallet_links | provider_subwallet_links | I | |
| provider_decline_events | assignment_responses + provider_offer_decision_receipts + safety_decline_inbox | P | No single append-only decline-event table with `safety_related` across offers, assignments and private work. |
| provider_shift_templates | - | M | |
| provider_shifts | shifts + shift_assignments | I | Shift is branch-level with per-membership assignments. |
| leave_requests | leave_records | I | No `approval_request_id`. |
| qualifications / provider_qualifications | same | I | |
| training_courses | training_courses | I | |
| training_enrollments | - | M | Only `training_completions`. |
| provider_performance_snapshots | performance_snapshots | I | Explanation and formula_version present. |

### 57.3 Marketplace, assignments and trips (10 tables)

| Spec table | Fleet table | Status | Notes |
|---|---|---|---|
| marketplace_request_projections, organization_request_offers, provider_offer_recipients, organization_acceptances, provider_assignments, assignment_responses, assignment_state_history, trip_projections, trip_projection_history | same names | I | `mig/000009/000010/000013/000081`. trip_projections has executing_provider_id + settlement_organization_id + source_version. |
| operational_rescue_cases | reassignment_cases (+ breakdowns.rescue_status) | P | Request-level reassignment only. No trip-level rescue case with replacement membership/asset or severity. |

### 57.4 Commercial, orders and custody (22 tables)

| Spec table | Fleet table | Status | Notes |
|---|---|---|---|
| customer_accounts | company_customers | I | No `platform_customer_id`. |
| customer_contacts, customer_locations, sales_opportunities, quotations, contracts, contract_versions, service_level_agreements, customer_credit_profiles, service_catalog_items, order_stops, shipments, packages, manifests, manifest_items | same names | I | |
| sales_leads | - | M | |
| quotation_items | quotation_line_items | I | |
| transport_orders | private_orders | I | |
| package_events | package_history + package_scans + custody_events | I | Split across three tables. |

### 57.5 Routes, assets and maintenance (22 tables)

| Spec table | Fleet table | Status | Notes |
|---|---|---|---|
| route_plans, route_stops, geofences, geofence_events, inspection_templates, inspections, stock_movements | same names | I | `stock_movements` covers parts only; `location_reference` is text. |
| route_assignments | private_assignments (work_origin ROUTE_STOP) / dispatch_plans | P | No route-level assignment row with response deadline per route plan. |
| organization_assets | assets | I | |
| asset_documents_projection | asset_documents | I | |
| inspection_item_results | inspection_results | I | |
| asset_defects | - | M | Partly covered by `inspection_results.critical` and `breakdowns`. |
| maintenance_plans | maintenance_schedules | I | Per-asset only (no asset_type plans). |
| maintenance_work_orders | work_orders | I | |
| work_order_tasks | - | M | |
| parts_catalog | parts | I | Single `current_quantity_projection`. |
| inventory_locations, inventory_balances | - | M | No per-location parts balance. |
| fuel_transactions | fuel_records | I | |
| meter_readings | - | M | Only `assets.odometer` / `operating_hours` scalars. |

### 57.6 Compliance, warehouse and finance (39 tables)

| Spec table | Fleet table | Status | Notes |
|---|---|---|---|
| compliance_requirements, incident_evidence, corrective_actions, proof_records, warehouses, inventory_items, dock_appointments, yard_visits, invoice_lines, expenses, payroll_periods, budgets | same names | I | `corrective_actions` requires an incident_id (no generic source). `budgets` is flat per `cost_centre`/`category` text. |
| compliance_checks | compliance_statuses | I | |
| operational_restrictions | restrictions | I | |
| safety_incidents | incidents (+ safety_reports) | I | |
| incident_investigations | - | M | Only an `INVESTIGATING` status on incidents. |
| insurance_claims | claims | I | |
| proof_requirements | proof_policies + proof_policy_versions | I | |
| warehouse_zones, storage_bins | storage_locations (location_type ZONE/BIN/...) | I | Hierarchical single table. |
| inventory_lots | inventory_items.lot_reference | P | No lot table with manufacture/expiry per lot. |
| warehouse_stock | inventory_items (quantity_projection, reserved_quantity) | I | |
| warehouse_receipts, warehouse_receipt_lines, putaway_tasks, pick_tasks, packing_units | - | M | Fulfilment has `fulfilment_lines.picked_quantity` + `inventory_reservations` only. |
| gate_events | yard_visit_history | P | Status/seal history, not gate events with gate_code/evidence. |
| customer_invoices | invoices | I | |
| payment_receipt_projections | receivable_payment_history | I | |
| receivable_ledger_entries | receivable_entries | P | One mutable row per invoice (`invoice_id UNIQUE`, outstanding updated), not an append-only subledger. |
| payroll_entries | provider_compensation_items | P | Line items per rule. No per-membership gross/deduction/net entry. |
| provider_statements | - | M | Computed on demand from compensation items. |
| budget_lines | - (folded into budgets) | P | |
| cost_allocations, profitability_snapshots | - | M | |

### 57.7 Infrastructure and governance (21 tables)

| Spec table | Fleet table | Status | Notes |
|---|---|---|---|
| organization_wallet_projections | organization_wallet_provisioning + finance_projections | P | No single balance projection row (available/pending/last_event_id). |
| provider_subwallet_projections | provider_subwallet_links + finance_projections | P | Same as above. |
| wallet_allocation_commands | provider_allocation_instructions | I | |
| audit_events | audit_records | I | Has previous/new summary rather than before/after hash. Hash chain in `mig/000069`. |
| idempotency_records | idempotency_keys | P | PK(org,endpoint,key). `response_body bytea` is **not encrypted**. No resource_type/id. |
| outbox_events | outbox | I | |
| inbox_events | inbox | I | |
| durable_jobs | jobs (+ job_attempts) | I | Active dedupe-key uniqueness (`internal/jobs/jobs.go:149`). |
| dead_letter_messages | jobs/outbox `DEAD_LETTERED` + synchronization_dead_letters | P | No generic DLQ table with resolution note. Recovery is through worker-operations routes. |
| import_jobs / import_row_results | bulk_imports / bulk_import_rows | I | `import_type` added in `mig/000056`. |
| export_jobs | export_jobs | I | No `permission_snapshot`. |
| organization_api_clients, webhook_subscriptions, webhook_deliveries | same | I | |
| report_definitions, report_runs | - | M | |
| automation_rules, automation_executions | - | M | `scheduled_tasks` / `scheduled_task_runs` exist but are not rule/condition/action automation. |

### 57.8 Migration and database-role rules

| Rule | Status | Evidence / gap |
|---|---|---|
| Migration number ranges by domain (001-010 foundations ... 101+) | N/A (deviation) | Sequential 000001-000087 by delivery order. Not renumberable, because applied migrations are immutable. |
| Deployment identity runs migrations; runtime role has no DDL | M | Migrations run in-process from the runtime pool (`cmd/main.go:71-79`, `FLEET_PARTNER_MIGRATION`). No role split documented. |
| Append-only tables: grant INSERT/SELECT only | M | No GRANT/REVOKE in any migration. Triggers are used instead. |
| Applied migrations never rewritten | I | Additive changes such as `mig/000008` replacing a CHECK instead of editing 000002. |
| Fresh apply, replay and reversible rollback tests | I | `mig/migrations_test.go:19 TestEveryUpMigrationHasDown`, `cmd/test-migrate`, `internal/integration/section*_migration_test.go`. |

**57 totals:** 65 present under the same name + 39 present under another name = 104 implemented; 12 partial; 26 missing (142).

---

## Cross-service work required

1. **driver-dispatch-delivery-service and driver-taxi-service:** add the managed-provider provisioning/link contract hauling already exposes: create or link a provider in `ORGANIZATION_SPONSORED` mode (skipping self-onboarding), plus read and transition. Then fleet must wire `contracts.Providers[DISPATCH|TAXI]` (`internal/app/wiring.go:262-272`).
2. **dispatch / taxi / hauling:** honour the sponsored onboarding mode end to end (no repeat of independent-provider verification), and confirm that independent-provider and customer flows are unchanged (34.1g).
3. **verification-compliance-service:** an organization-sponsored provider verification path, with status-change events fleet can consume (3.h). Fleet already has org-level verification.
4. **analytics-service (and any other consumer):** consume corporate events from the `fleet-partner:events` Redis stream, or an HTTP ingest. Today nothing outside fleet reads them (3.j).
5. **payment-wallet-service:** if `organization_wallet_projections` / `provider_subwallet_projections` are built as specified, wallet must emit balance-change events carrying an event id and source version for available/pending balances.
6. **customer-service (optional):** a lookup or reference contract if `customer_accounts.platform_customer_id` linking is required (57.4).
7. **api-gateway and every fleet caller:** if the binding Section 36 paths are adopted, add `/api/v1/corporate-logistics` routing in the gateway. The internal path `/internal/fleet-partner/v1` is hard-coded in dispatch, taxi, hauling and shared clients, so a change needs a coordinated rename or alias.

## Frontend work required

**web/fleet**
- `app/(dashboard)/settings/{branches,locations,departments,policies,closure}/page.tsx` are 9-line "TODO: implement ORG1x" stubs.
  - Closure can be built now: the backend routes exist (`close-request`, `closure/assess|approve|reject|execute`, `closure-status`).
  - Branches, locations, departments, teams, cost-centres and org settings need the backend APIs first.
- The organization settings page (assignment timeout, approval thresholds, provider visibility, retention) needs `GET/PATCH /organizations/{id}/settings` on the backend.
- Send `Idempotency-Key` on every state-changing call. Only 5 of 23 `lib/services/*.ts` files send it (crm, breakdown, marketplace, inspection, asset).
- Surface `error.requestId` and `retryable` from the error envelope. `lib/api/client.ts:66` reads only `code`/`message`.
- Pages backed by `lib/mock-*.ts` instead of the API: `providers/[membershipId]/{availability,compliance,declines,work-setup}` and `operations/control-tower/exceptions`. Availability and declines also need backend support (57.2 missing tables).
- If the canonical paths change, update `lib/api/client.ts` (`AUTH_COOKIE_SCOPED_BASE='/fleet-partner/v1/auth'`) and `middleware.ts`.

**apps/* (provider apps)**
- `dispatch_provider`, `taxi_provider`, `truck_provider`: every organization-managed offer and company assignment must show both **Accept and Decline**, with a mandatory decline reason (2.0, 2.6).
  - `taxi_provider` has no organization awareness at all.
  - `truck_provider` has only an auth-model reference.
- Managed providers must not see customer fare, commission, settlement or contract price (2.9), and must see sub-wallet allocations rather than trip settlement (2.8). This needs provider-facing sub-wallet routes, which the backend has in code but does not mount (prior audit).
- Sponsored providers must skip the independent self-onboarding and verification screens (2.5) once the provider services support the sponsored mode.
- `apps/customer`: no work required (2.2, customer flow unchanged).
