# Fleet backend audit D: state machines, tests, configuration, migration and rollout

Scope: spec Sections 30, 31, 32, 33, Appendix B, Appendix D, 58, 66, 67, 68, 69, 70, 71, 72 (`the specification text`).
Service: `services/fleet-partner-service` at HEAD `9e3f4ba` (2026-11-13 17:25 UTC; seven fleet commits after the prior audit's `2f6932f`, including the Head Office branch migration 000087 and the staff directory API). Read-only; nothing in the repository was changed.
Paths below are relative to `services/fleet-partner-service/` unless they start with `services/`, `apps/`, `web/`, `deploy/` or `.github/`. `f/` = `internal/features/`.

Status values: IMPLEMENTED / PARTIAL / MISSING / EXTERNAL / N/A.

## Summary

| Section | Implemented | Partial | Missing | Notes |
|---|---|---|---|---|
| 30 Migration and backward compatibility | 3 | 4 | 0 | 87 up/87 down, replay tests exist. No backfill framework (000087 backfills inside the migration), no `CONCURRENTLY` (banned by policy), no migration advisory lock. Account delete becomes a closure that can never complete. |
| 31 Testing strategy (19 suites + 6 levels) | 14 | 11 | 0 | Large real-Postgres suite. Gaps follow the code gaps: onboarding activation, provider association, order/shipment lifecycle, warehouse receipt, incident lifecycle. No `-race`, no secret scan in CI; the dispatch end-to-end harness is allowed to skip. |
| 32 Work packages WP0-WP21 | 7 | 14 | 1 | WP3 (company can become ACTIVE) and WP5/WP6 (provider linked, sub-wallet) exit conditions are not met. WP20 (automation/AI) deferred by ADR-002. |
| 33 Release, rollout and acceptance gates (9+10+1) | 7 | 11 | 2 | Marketplace accept/assign/provider-direct are hard-coded ON (not flag-gated). No per-organization allowlist for controlled acceptance. 33.3 definition of done is not met. |
| App. B Domain states (17 domains) | 5 | 12 | 0 | Shipment, asset, work order, incident, invoice vocabularies differ from App. B; several states exist in CHECKs but nothing can reach them. |
| App. D Sequence flows (5) | 0 | 4 | 1 | D.1 stops at PENDING_PROVIDER_ACCEPTANCE for every provider. D.5 order never leaves CONFIRMED. |
| 58 Enforced state machines (preamble + 12) | 1 | 10 | 2 | Only 58.12 (assignment response contract) is fully met. Shipment has no transitions at all; warehouse receipt aggregate does not exist. Incident status is free-form. |
| 66 Mandatory tests (103 IDs) | 37 | 53 | 13 | No test references a Section 66 ID. Self-mapping (`docs/section31-test-coverage.json`) uses Section 31 names and only checks that each suite cites at least one existing test. |
| 67 Environment contract (90 vars) | 3 exact | 47 renamed | 40 missing | Service uses `FLEET_PARTNER_*`/module flags; JWT/JWKS, KMS key IDs, event transport, pricing mode, commercial-field safety switch are absent. Default port `:8113` vs spec `:8110`; base paths differ. |
| 68 Migration/rollout/back-compat plan | 8 | 19 | 4 | R0-R1 done; R2-R12 only partly (flags exist for most modules but not for onboarding/provisioning/marketplace writes). Taxi/dispatch identity contracts and verification read-back missing. |
| 69 Work packages WP0-WP24 + 69.1 | 7 | 17 | 2 | Same picture as 32; WP23 (reports/automation) and WP24 (all S66 + S71 pass) are not met. |
| 70 End-to-end journeys (12) | 0 | 10 | 2 | No journey runs end to end on a default or staging deployment. First break in each journey listed below. |
| 71 Final readiness checklist (40) | 12 | 23 | 5 | Self-report `docs/release1-master-spec-acceptance.json` marks all 40 PASS; at least 7 are false (see 71). |
| 72 Final implementation authority | 0 | 1 | 0 | Universal accept/decline holds where providers exist; undocumented deviations from Sections 58/67 have no ADR (only ADR-001 Taxi, ADR-002 AI). |

### Five findings that block the most

1. **A company cannot become ACTIVE through the system.** Activation requires `verification.Status == "APPROVED"` (`f/organizations/usecases/submission_activation.go:358`). Fleet only ever writes SUBMITTED / UNKNOWN_OUTCOME / PENDING_DISPATCH (`:215-232`). Verification-compliance records `VERIFIED` (`services/verification-compliance-service/internal/features/organizations/http.go:157`) and emits `organization.verification.status_changed.v1` (`:203`), which Fleet never consumes. `TryActivate` (`submission_activation.go:405`) has no production caller. Activation also needs an ACTIVE company wallet (`:367`).
2. **No managed provider can become ACTIVE.** `CreateProvider` leaves the membership at PENDING_PROVIDER_ACCEPTANCE (`f/providers/usecases/provider_service.go:391`). The only code that moves it on, `RecordAssociationOutcome` (`f/providers/usecases/lifecycle.go:26`), has no caller and no route; Fleet never calls `ReadAssociation` (`internal/platformclients/contracts.go:218`) and has no invitation-redeem route. The sub-wallet is requested only after acceptance (`lifecycle.go:77`), so it is never requested either. Only HAULING has a real provider contract (`internal/app/wiring.go:271`; DISPATCH/TAXI stay `UnconfiguredProvider`, `internal/platformclients/contracts.go:375-385`).
3. **Private orders and shipments stop after confirmation.** Order routes are create/confirm/cancel/stops only (`f/orders/http/manifest.go:23-41`); shipment routes are create/patch/cargo only (`f/shipments/http/manifest.go:22-31`). Both repositories' `SetStatus` (`f/orders/repositories/repository.go:186`, `f/shipments/repositories/repository.go:172`) have no caller, and private execution never updates the order. Shipments can never leave DRAFT.
4. **Marketplace assignments never reach IN_PROGRESS or COMPLETED**, and source cancellation closes only the offer, not the assignment (`f/marketplace/usecases/event_ingestion.go:396-420`). Offers never expire by worker: `OfferRepository.ExpireDue` (`f/marketplace/repositories/offers.go:140`) is not registered anywhere.
5. **Fleet's dispatch adapter calls routes dispatch does not expose.** Fleet calls `GET /api/v1/internal/marketplace/eligibility?...`, `GET /reservations/{sourceRequestId}?idempotency_key=` and `POST /reservations/{sourceRequestId}/release` (`internal/platformclients/dispatch_execution.go:43,98,119,134`). Dispatch exposes `POST /eligibility/providers/validate`, `GET /reservations/:reservationId`, `GET /reservations/by-idempotency/:key` and `POST /reservations/:reservationId/release` (`services/driver-dispatch-delivery-service/internal/features/marketplace/http.go:62-77`). Assignment calls the eligibility check (`f/marketplace/usecases/assign.go:87`, `eligibility.go:187`), so dispatch-sourced assignment is expected to fail. Fleet tests use fakes; confirm with one live call before relying on this.

---

## Section 30. Database migration and backward compatibility

| Req | Status | Evidence |
|---|---|---|
| 30.1 Additive first; destructive needs later cleanup release | IMPLEMENTED | `internal/section30contract/migration_policy_test.go:84,115,184` (destructive / constraint-tightening require cleanup metadata). |
| 30.1 Down migration + replay test | IMPLEMENTED | 87 `.up.sql` / 87 `.down.sql`; `migrations/migrations_test.go:19`; `internal/section30contract/migration_replay_test.go:25-312`; `internal/integration/integration_test.go` `TestMigrationsReplayAndRollback`. Replay tests need `FLEET_PARTNER_TEST_DATABASE_URL` (provided in CI). |
| 30.1 Large backfills in bounded batches through jobs | PARTIAL | No backfill job framework (only a lint, `internal/platform/migrationlint`). `migrations/000087_default_head_office_branch.up.sql:6-10` backfills every organization inside the deployment transaction. |
| 30.1 Production-safe index strategy | PARTIAL | Policy forbids `CONCURRENTLY` (`migration_policy_test.go:268`); all 322 indexes are plain `CREATE INDEX` in a transaction. Fine for small tables, locks writes on large ones. |
| 30.1 Enum evolution via safe CHECK changes | IMPLEMENTED | DROP/ADD CHECK pattern (e.g. `migrations/000008`, `000020`, `000022`); widening-only policy test `migration_policy_test.go:290`. |
| 30.2 Original-draft mapping | PARTIAL | Documented in `docs/section30-original-api-mapping.md`, tested by `compatibility_contract_test.go:117`. Real behaviour lags: "Drivers -> provider resolve/provision" works for HAULING only; "Account delete -> closure" exists but `DefaultClosureCheckers` always returns UNKNOWN blockers (`f/organizations/usecases/closure.go:334-343`), so closure is always BLOCKED. |
| 30.3 Provider app compatibility (providerMode, organization, assignment policy, field visibility) | PARTIAL | Internal membership summary `GET /providers/:providerId/services/:providerService/membership-summary` (`f/providers/http/internal.go:54`); golden fixtures in `f/providers/http/internal_test.go`. Dispatch and truck apps parse the fields (`compatibility_contract_test.go:170`); taxi app and `driver-taxi-service` have no `PROVIDER_ORGANIZATION_MODE_ENABLED` (`compatibility_contract_test.go:189` only checks hauling and dispatch). Accept/decline UI exists only in `apps/dispatch_provider`. |

Also missing: migration runner has no advisory lock (`internal/database/migrate.go:26-70`), so two replicas started with `MIGRATION=true` can race (spec `DB_MIGRATION_LOCK_ID`).

---

## Section 31. Testing strategy and mandatory suites

1,220 `Test*` functions. CI `section31-mandatory.yml` runs the whole fleet suite against Postgres 16 + Redis 7, plus tenant/security/Redis subsets, govulncheck, gofmt, vet, build.

| Suite | Status | Evidence / gap |
|---|---|---|
| Tenancy and authorization | IMPLEMENTED | `internal/integration/idor_*_test.go`, `TestStaffCrossTenantIsolation`, `TestMarketplaceTenantIsolation`, `TestFleetFinanceBranchScope`, `TestProviderFacingViews_NeverSerializeCommercialOrInternalFields`, `TestGetMembershipSummaryForProvider_SuspendedMembershipIsNotLive`. Repository-scoping static test is narrow (see TEN-003). |
| Organization onboarding | PARTIAL | `organization_onboarding_resume_test.go`, `organization_lifecycle_gate_test.go`, `organization_closure_http_test.go`, `default_branch_test.go`. No test drives verification decision -> ACTIVE because the code path does not exist. |
| Provider provisioning | PARTIAL | `provider_provisioning_reconciliation_test.go`, `TestProviderRepositoryEnforcesOneLiveMembership`, `TestFleetClientToRealHaulingAuthority`. No split-identifier, association-acceptance or sub-wallet-provisioning-failure test. |
| Acceptance and decline | IMPLEMENTED | `marketplace_test.go`, `offer_response_test.go`, `assignment_acknowledgement_test.go`, `provider_response_test.go`, `assignment_expiry_section27_test.go`. |
| Safety decline | PARTIAL | `TestDeclineReasonCatalogueAndSafetyClassification`, `TestOfferDeclineValidatesReasonAndFlagsSafety`; asset block + work order only for private assignments (`TestSection17And18EndToEnd` "unsafe decline inbox"). |
| Assignment and reassignment | IMPLEMENTED | `auto_reassignment_section27_test.go`, `execution_start_test.go`, `TestAcceptanceToOneCanonicalTrip`. |
| Pricing visibility | IMPLEMENTED | `TestProviderOfferViewCarriesNoCommercialFields`, `TestProviderAssignmentViewCarriesNoCommercialFields`, `TestTripProjectionStoresNoCustomerCommercialData`, `field_visibility_test.go`. |
| Wallet integration | IMPLEMENTED | `companywallet_test.go` (idempotency, self-approval, unknown outcome read-back, stale projection, no local ledger), `finance_cross_service_test.go`. |
| Orders and shipments | PARTIAL | `orders_test.go`, `shipments_test.go`, `packages_section15_matrix_test.go`, `recurring_orders_section15_matrix_test.go`, `bulk_imports_section15_matrix_test.go`. Order/shipment transitions beyond confirm/draft cannot be tested (no commands). |
| Dispatch/routes | IMPLEMENTED | `section16_dispatch_test.go` (7 tests). Hard capacity/time-window rejection not covered (see ROUTE-003). |
| Fleet/maintenance | IMPLEMENTED | `section17_18_test.go`, `section17_18_concurrency_test.go`, `idor_assets_maintenance_test.go`. |
| Compliance/safety/proof | PARTIAL | `section19_21_test.go` (restriction, proof), `safety_reference_*`. Incident lifecycle is free-form, not tested as a machine. |
| Warehouse | PARTIAL | `TestSection21WarehouseReceiptConcurrencyAndNegativeStock`, `idor_warehouse_fulfilment_test.go`. No receipt/put-away/pick concurrency (no such aggregates). |
| Finance | IMPLEMENTED | `section22_23_test.go`, `finance_operations_test.go`, `cmp027_double_pay_test.go`. |
| Events | IMPLEMENTED | `TestOutboxTransactionAtomicity`, `TestInboxDeduplication`, `event_ingestion_test.go`, `trip_projection_test.go`, `section26_stream_redis_test.go`. |
| Jobs/leases | IMPLEMENTED | `TestDurableJobsLeasing`, `TestExpiredLeaseRecovery`, `TestJobRetryAndDeadLetter`, `jobs_section27_test.go`. |
| Security | PARTIAL | IDOR, SSRF (`f/integrations/security_test.go`), rate limits, nonce replay, service credential scope. No JWT audience/algorithm test (Fleet signs its own HMAC tokens), no mass-assignment test. |
| Performance | PARTIAL | `section29_capacity_test.go`, `section31_capacity_profiles_test.go` - development-machine profiles, no SLO evidence. |
| Migration | PARTIAL | Replay/rollback tests exist; no staging-baseline replay, no least-privilege runtime role test. |
| Level: unit | IMPLEMENTED | e.g. `f/marketplace/models`, `f/companywallet/models/models_test.go`, `internal/platform/*`. |
| Level: repository integration (Postgres) | IMPLEMENTED | `internal/integration` (~110 files). |
| Level: contract tests per service client/event | PARTIAL | `internal/section30contract`, `f/providers/http/internal_test.go`, `fleet_hauling_blackbox_test.go`, `section26` asyncapi. No contract test against the real dispatch internal API (finding 5 would have been caught). |
| Level: end-to-end gateway + corporate + existing services | PARTIAL | `acceptance_to_canonical_trip_e2e_test.go` skips without `DISPATCH_TEST_DATABASE_URL`, and the CI gate explicitly allows it to skip (`.github/workflows/section31-mandatory.yml:155-162`, `-allow TestAcceptanceToCanonicalTripDispatchHarness`). |
| Level: failure injection | IMPLEMENTED | Unknown-outcome tests (`execution_start_test.go`, `companywallet_test.go`, `section19_21_test.go:83`), `section29_degraded_modes_test.go`. |
| Level: security + tenant regression in CI | IMPLEMENTED | `section31-mandatory.yml:163-168`. |

---

## Section 32. Work packages and build order (exit conditions)

| WP | Status | Evidence |
|---|---|---|
| WP0 Readiness audit | IMPLEMENTED | `docs/foundation-audit.md`, `docs/auth-onboarding-iam-provider-audit.md`, `docs/marketplace-assignment-trip-audit.md`. |
| WP1 Skeleton/config/health/CI | IMPLEMENTED | `cmd/main.go`, readiness incl. migrations (`cmd/main.go` readyChecks), CI workflows. |
| WP2 IDs, money, errors, tenancy, idempotency, outbox/inbox | IMPLEMENTED | `internal/platform/{ids,money,tenant,idempotency}`, `internal/events`. |
| WP3 Verified organization can become ACTIVE | PARTIAL | Head Office branch now auto-created (`f/organizations/usecases/onboarding_service.go:374`, migration 000087), but the verification decision is never read back and `TryActivate` is never called (finding 1). No branch CRUD routes. |
| WP4 IAM + cross-tenant/branch security | PARTIAL | Security suite passes; custom roles, role bindings, approval policies/delegations still have no routes (`docs/implementation/fleet-end-to-end-activation-report.md` "Still not working" 3). |
| WP5 Company creates/links provider, no repeated onboarding | PARTIAL | HAULING only; association never completes (finding 2). |
| WP6 Active provider always has sub-wallet | PARTIAL | Gate enforced (`f/providers/usecases/lifecycle.go` gate `subwallet_active`) but the sub-wallet is never requested (finding 2). |
| WP7 Receive and atomically accept current-service request | PARTIAL | Only dispatch sends offer events (`services/driver-dispatch-delivery-service/internal/config/config.go:452` `/internal/fleet-partner/v1/events`); hauling and taxi send none. Dispatch adapter path mismatch (finding 5). |
| WP8 Accept/decline, reasons, timeout, reassignment | IMPLEMENTED | `f/marketplace/usecases/assignment_service.go`, `assign.go`, expiry worker `internal/app/reconciliation_workers.go:140` (flag default off). |
| WP9 End-to-end marketplace company trip | PARTIAL | Execution start exists; assignment never reaches IN_PROGRESS/COMPLETED; E2E harness skippable. |
| WP10 CRM/quotations/contracts/catalogue | PARTIAL | Implemented except leads (prior audit row 14.10). |
| WP11 Private order lifecycle works | PARTIAL | Lifecycle stops at CONFIRMED (finding 3). |
| WP12 Ops control-centre APIs | IMPLEMENTED | `f/dispatch` (37 operations, `f/dispatch/http.go:106`). |
| WP13 Workforce scheduling/eligibility | PARTIAL | Shift templates, training enrolments missing; performance only from private assignments. |
| WP14 Safety block / return-to-service | IMPLEMENTED | `f/assets/operations.go:210-270` (RTS requires COMPLETED WO, no restriction, passing follow-up inspection). |
| WP15 Compliance, incidents, proof, custody | PARTIAL | Restrictions and proof real; incident status free-form (`f/safety/operations.go:255-274`). |
| WP16 Specialized modules | IMPLEMENTED | `f/specialized`; taxi/medical off by ADR. |
| WP17 Warehouse/dock/yard | PARTIAL | No receipt/put-away/pick-wave aggregates; receipt is a single stock command (`f/warehouse/service.go:357`). |
| WP18 Invoices/expenses/payroll/budgets/claims | PARTIAL | Profitability and receivables ageing missing (prior audit Sec 22). |
| WP19 Comms/quality/SLA/analytics/exports | PARTIAL | Reports missing (prior audit Sec 24). |
| WP20 Automation/AI | MISSING | Deferred, `docs/adr/ADR-002-release1-ai-automation-deferral.md`. |
| WP21 Hardening/perf/migration/rollout/runbooks | PARTIAL | See Sections 33, 68, 71. |

32.1 delivery rule (code + migrations + tests + API docs + event contracts + env docs + observability + rollback per package): PARTIAL. OpenAPI exists for Sections 14-18, 25-30, 55, onboarding v2, browser auth, admin resources (`docs/openapi/`), but not for organizations/staff/providers/marketplace/wallet/finance (Sections 8-13, 22-23).

---

## Section 33. Release, rollout and acceptance gates

### 33.1 Feature-flag rollout

| Step | Status | Evidence |
|---|---|---|
| 1 Foundation with all corporate writes disabled | PARTIAL | 174 of 179 boolean flags default off (prior audit), but marketplace `OrganizationAcceptEnabled`, `AssignmentEnabled`, `ProviderDirectAcceptEnabled`, `ProjectionEnabled` are hard-coded `true` (`internal/app/marketplace_wiring.go:93-131`). Onboarding, staff and provider creation have no flag. |
| 2 Onboarding for internal test orgs | PARTIAL | Onboarding is open to any signed-up user; no tenant allowlist. |
| 3 Provider provisioning sandbox | PARTIAL | No `FEATURE_PROVIDER_SPONSORED_ACCOUNTS`; provisioning always on, HAULING only. |
| 4 Wallet/sub-wallet with test funds | PARTIAL | `PROVIDER_SUBWALLET_ENABLED`, `ORGANIZATION_WALLET_READ_ENABLED` etc. exist; sub-wallet never requested. |
| 5 Marketplace projection read-only | PARTIAL | `FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED` gates ingestion, but writes cannot be held off independently (step 1). |
| 6 Organization acceptance for controlled orgs | MISSING | No per-organization/service/region allowlist; acceptance is global. |
| 7 Provider accept/decline | IMPLEMENTED | Provider routes mount only with `FLEET_MARKETPLACE_ALLOWED_CALLERS` (prior audit finding 7). |
| 8 Live settlement after finance approval | IMPLEMENTED | `FLEET_FINANCE_INTEGRATION_ENABLED`, `FLEET_SETTLEMENT_INSTRUCTION_ENABLED`, `FINANCE_POSTING_ENABLED` default false; kill switches. |
| 9 Private modules by capability | IMPLEMENTED | Per-module flags (`internal/config/config.go`), staging set in `deploy/platform/fleet-staging-modules.txt`. |

### 33.2 Mandatory release gates

| Gate | Status | Evidence |
|---|---|---|
| No regression in customer/independent flows | PARTIAL | `TestSections89_IndependentProviderIsUnaffected`, `TestGetMembershipSummaryForProvider_IndependentProviderHasNoMembership`; CI runs dispatch/hauling/wallet/customer suites. |
| Every managed-provider assignment exposes accept and decline | IMPLEMENTED | `TestAssignmentAlwaysOffersAcceptAndDecline`, `TestProviderDeclineRightCannotBeDisabled`, `f/privateoperations/http/provider_view_test.go`. |
| Provider APIs carry no price/settlement | IMPLEMENTED | Provider view tests above. |
| Marketplace settlement routes to organization wallet | PARTIAL | Contract exists (payment-wallet managed settlement, CLAUDE.md 2026-09-14), flags default off, never exercised end to end. |
| Every active managed provider has a sub-wallet | IMPLEMENTED (vacuous) | Activation gate requires it; no provider can activate (finding 2). |
| Cross-tenant penetration + automated isolation | PARTIAL | Automated isolation yes; no penetration-test evidence. |
| Outbox/inbox, assignment timeout, reconciliation workers operational | PARTIAL | Exist; `ASSIGNMENT_EXPIRY_WORKER_ENABLED`, `FLEET_MARKETPLACE_RECONCILIATION_ENABLED`, section workers default off and are not set in the staging file. Outbox delivers only when Redis is configured (`cmd/main.go` `DisabledPublisher`). |
| Migration replay and rollback/forward verified | PARTIAL | Replay tests; no staging-baseline replay. |
| Dashboards, alerts, backups, runbooks ready | PARTIAL | `docs/runbooks/` has 11 runbooks (CRM/onboarding/provisioning); most of the 12 required by 65.5 (DB unavailable, wallet discrepancy, cross-tenant suspicion, unsafe asset, regional disaster...) exist only as sections of `docs/section29-*.md`. No dashboards in repo. |
| Pricing/subscription configuration-only | IMPLEMENTED | No platform commission formula in Fleet; compensation rules are company data. |

### 33.3 Definition of backend completion — MISSING
A verified company cannot activate, cannot create an active managed provider, hauling/taxi offers never reach Fleet, and the order lifecycle stops at CONFIRMED.

---

## Appendix B. Domain states and enumerations

| Domain | Spec values | Implemented (CHECK / Go) | Status | Mismatch |
|---|---|---|---|---|
| Organization status | DRAFT, CONTACT_VERIFIED, BUSINESS_SUBMITTED, UNDER_REVIEW, CHANGES_REQUIRED, ACTIVE, SUSPENDED, CLOSED | `migrations/000008...:20-31` + PENDING_REVIEW, ARCHIVED; `f/organizations/usecases/lifecycle.go:19-56` | PARTIAL | Values match App. B, but 58.1 adds REJECTED, RESTRICTED, CLOSURE_PENDING (absent). UNDER_REVIEW -> ACTIVE unreachable in production. |
| Provider membership | INVITED, ACTIVE, SUSPENDED, REMOVED, EXPIRED, CANCELLED | `000007:29-30` + PENDING_PROVIDER_ACCEPTANCE, PENDING_SETUP, DECLINED, ENDING | PARTIAL | EXPIRED unreachable (invitation sweep updates invitations only, `f/providers/repositories/sagas.go:112`). ACTIVE unreachable (finding 2). REMOVED/ENDING only via `ChangeStatus` whose routes are not mounted. |
| Provider mode | INDEPENDENT, ORGANIZATION_MANAGED | `000007` `provider_mode IN ('ORGANIZATION_MANAGED')`; INDEPENDENT implicit (no membership) | IMPLEMENTED | — |
| Acceptance mode | CENTRALIZED, DELEGATED, HYBRID | `000010:115-124`, `000009:71` | IMPLEMENTED | — |
| Offer status | OFFERED, ACCEPTED, DECLINED, EXPIRED, CANCELLED | `000009:73` + RESERVED | PARTIAL | EXPIRED not produced: `ExpireDue` unused; source expiry maps to CANCELLED (`event_ingestion.go:341-345`). |
| Assignment status | CREATED ... REASSIGNMENT_REQUIRED ... CANCELLED (10) | Marketplace `000009:182-186` (no REASSIGNMENT_REQUIRED; adds EXECUTION_START_PENDING, RECONCILIATION_REQUIRED). Private `000032:7` has all 10. | PARTIAL | Marketplace IN_PROGRESS/COMPLETED unreachable (no Transition call; `f/marketplace/repositories/assignments.go:256-257` columns only). |
| Acceptance source | ORGANIZATION_DISPATCHER, PROVIDER_DIRECT, AUTOMATION_DRAFT, PLATFORM_RECOVERY | `000009:35-37,136-138` | IMPLEMENTED | — |
| Order status | DRAFT ... CANCELLED (10) | `000026:47-49`, `f/orders/models/models.go:93-104` | PARTIAL | Values match; PLANNED/ASSIGNED/IN_PROGRESS/PARTIALLY_COMPLETED/COMPLETED/FAILED unreachable. |
| Shipment status | DRAFT, READY, PLANNED, LOADED, IN_TRANSIT, AT_HUB, OUT_FOR_DELIVERY, DELIVERED, FAILED, RETURNING, RETURNED, CANCELLED | `000027:31-32`: DRAFT, READY, PLANNED, ASSIGNED, IN_TRANSIT, COMPLETED, FAILED, PARTIALLY_COMPLETED, CANCELLED | PARTIAL | Missing LOADED, AT_HUB, OUT_FOR_DELIVERY, DELIVERED, RETURNING, RETURNED; extra ASSIGNED, COMPLETED, PARTIALLY_COMPLETED. Only DRAFT reachable. |
| Package status | 13 values | `000028:24-26`, `f/packages/models/models.go:27-39` | IMPLEMENTED | Exact match; DELIVERED proof-gated (`f/packages/usecases/service.go:200`). |
| Asset status | 13 values | `000040:6-7`: operational AVAILABLE, ASSIGNED, IN_USE, BLOCKED, UNDER_MAINTENANCE, OUT_OF_SERVICE; lifecycle ACTIVE, RETIRED, ARCHIVED | PARTIAL | Missing RESERVED, INSPECTION_DUE, MAINTENANCE_DUE, BREAKDOWN, SAFETY_BLOCKED vs COMPLIANCE_BLOCKED (merged into BLOCKED), SUSPENDED. No Go transition matrix; raw SQL updates. |
| Inspection result | PASS, PASS_WITH_ADVISORY, FAIL_NONCRITICAL, FAIL_CRITICAL | `000041:10`: PASS, PASS_WITH_ADVISORY, FAIL, CRITICAL_FAIL | PARTIAL | Naming differs. |
| Work order status | DRAFT, AWAITING_APPROVAL, APPROVED, SCHEDULED, IN_PROGRESS, WAITING_PARTS, COMPLETED, RETURN_TO_SERVICE_PENDING, CLOSED, CANCELLED | `000041:31-32`: DRAFT, PENDING_APPROVAL, APPROVED, IN_PROGRESS, COMPLETED, CANCELLED + separate `return_to_service_status` | PARTIAL | Missing SCHEDULED, WAITING_PARTS, CLOSED (and 58.8 QUALITY_CHECK). |
| Incident status | OPEN, TRIAGED, UNDER_INVESTIGATION, ACTION_REQUIRED, RESOLVED, CLOSED | `000044:21`: OPEN, INVESTIGATING, CONTAINED, CORRECTIVE_ACTION_REQUIRED, RESOLVED, CLOSED | PARTIAL | No TRIAGED; names differ; no transition matrix. |
| Invoice status | DRAFT, AWAITING_APPROVAL, ISSUED, PARTIALLY_PAID, PAID, OVERDUE, VOID, CREDITED | `000048:15`: DRAFT, PENDING_APPROVAL, APPROVED, ISSUED, PARTIALLY_PAID, PAID, OVERDUE, CANCELLED, CREDITED, WRITTEN_OFF | PARTIAL | VOID named CANCELLED; PENDING_APPROVAL and OVERDUE never set on invoices (overdue sweep updates receivables only, `f/financeops/invoices.go:398`). |
| Job status | 8 values | `000001:53-55` | IMPLEMENTED | Exact match. |

---

## Section 58. Enforced state machines and transition contracts

Preamble (named command, one transaction, row version, history, audit, outbox for every transition): **PARTIAL**. Organizations, memberships, marketplace assignments, orders and packages do this. Offers update by version CAS with no from-state guard in SQL (`f/marketplace/repositories/offers.go:88-125`) and the source-cancel path writes no history/audit/outbox event (`event_ingestion.go:406`). Assets, incidents, invoices update status with raw SQL in service code.

| Entity | Status | Missing / extra states | Missing guards / effects |
|---|---|---|---|
| 58.1 Organization | PARTIAL | Missing REJECTED, RESTRICTED, CLOSURE_PENDING. Implements App. B vocabulary instead (CONTACT_VERIFIED, BUSINESS_SUBMITTED, CHANGES_REQUIRED); extra PENDING_REVIEW, ARCHIVED. | under_review -> active has no trigger (verification never read back; `TryActivate` uncalled). `ExecuteClosure` sets CLOSED via `UpdateStatus` without `RequireTransition` (`f/organizations/usecases/closure.go:274`); closure always BLOCKED by UNKNOWN checkers. No "owner active" check found in `EvaluateActivation`. |
| 58.2 Provider membership | PARTIAL | Missing TEMPORARILY_UNAVAILABLE, ENDED (uses REMOVED). Extra CANCELLED. | pending_provider_acceptance -> pending_setup has no caller; invited|pending -> expired never applied (invitation-only sweep); ending -> ended worker missing; compliance/service-role gates absent from activation (`f/providers/usecases/lifecycle.go:137-188` checks identity, association, branch, org ACTIVE, sub-wallet, platform status). |
| 58.3 Organization offer | PARTIAL | `rejected` named DECLINED. | Expiry worker never registered; RESERVED offers excluded from `ExpireDue` anyway; source cancellation does not handle the downstream assignment; delegated acceptance marks ACCEPTED before the source confirms (execution start is queued afterwards, `f/marketplace/usecases/offer_response.go:194`). |
| 58.4 Provider assignment | PARTIAL | Uses PENDING_PROVIDER_RESPONSE/READY_FOR_EXECUTION/EXECUTION_START_PENDING/IN_PROGRESS for spec pending_response/starting/active; extra CREATED, RECONCILIATION_REQUIRED. | No transition to IN_PROGRESS or COMPLETED from source trip events; source trip cancellation does not cancel the assignment. Accept recheck, unique response, deadline CAS are implemented (`assignment_service.go:246,418,637`; `assign.go:241`). |
| 58.5 Private transport order | PARTIAL | Uses App. B values. Missing SUBMITTED, REJECTED, READY_FOR_DISPATCH, EXECUTION_REQUESTED, IN_EXECUTION. | Only draft -> awaiting_approval/confirmed -> cancelled reachable. No command for planned/ready/execution; private execution never updates the order. |
| 58.6 Shipment | MISSING | See App. B row (6 spec states absent). | No status command at all; `SetStatus` unused; history only on create/update. |
| 58.7 Asset | PARTIAL | Missing DRAFT, PENDING_VERIFICATION, RESTRICTED, RESERVED, INSPECTION_DUE, MAINTENANCE_HOLD (BLOCKED used instead). | No transition matrix; `UPDATE ... SET operational_status=...` from several places (`f/assets/operations.go:264,302,654`, `safety_declines.go:88`, `service.go:259,513`). `safety_declines.go:88` guards `operational_status NOT IN('BLOCKED','RETIRED','ARCHIVED')` but RETIRED/ARCHIVED are lifecycle values, so a retired asset can be set BLOCKED. Retirement needs no approval (`service.go:180-210`). |
| 58.8 Maintenance work order | PARTIAL | Missing SUBMITTED (PENDING_APPROVAL), SCHEDULED, WAITING_PARTS, QUALITY_CHECK, RETURNED_TO_SERVICE as a status. | APPROVE allowed straight from DRAFT and by anyone with the permission (no cost threshold, no requester != approver) (`f/assets/operations.go:173-178`). RTS correctly requires COMPLETED + no restriction + passing follow-up inspection (`:210-270`). |
| 58.9 Customer invoice | PARTIAL | Missing REJECTED, VOIDED (CANCELLED); extra WRITTEN_OFF. | No submit command (PENDING_APPROVAL unreachable); no self-approval guard; `cancel` allowed from ISSUED/PARTIALLY_PAID/OVERDUE with no approval or reversal document (`f/financeops/invoices.go:213-217`); invoice OVERDUE never set. |
| 58.10 Warehouse receipt | MISSING | No receipt aggregate/table. `Receipt()` immediately creates/increments AVAILABLE stock (`f/warehouse/service.go:357-414`); no expected/arrived/receiving/exception/putaway states. | — |
| 58.11 Safety incident | PARTIAL | Missing REPORTED, TRIAGED, CORRECTIVE_ACTION (named CORRECTIVE_ACTION_REQUIRED), ESCALATED. | Status set freely from PATCH body (`f/safety/operations.go:255-274`); only RESOLVED/CLOSED require resolution text; no claim/regulatory/approval gate before CLOSED. |
| 58.12 Mandatory assignment response contract | IMPLEMENTED | — | Provider-only actor (internal HMAC routes, company users get 403: `TestServiceCredentialScope_OrganizationUserDeniedInternalRoute`); unique response; catalogue reasons; safety reasons flagged and no deduction; timeout CAS; pull after reconnect; no pricing. Caveat: safety declines on **marketplace** assignments only publish `organization.assignment.safety_followup_requested.v1` (`assignment_service.go:496-512`); asset block/work order is created only for private assignments (`f/assets/safety_declines.go:20-24`). |

---

## Section 66. Mandatory test specification (103 IDs)

No test file references these IDs. Mapping by test name and assertions:

| ID | Status | Test(s) / gap |
|---|---|---|
| TEN-001 | IMPLEMENTED | `TestStaffCrossTenantIsolation`, `Test*_ObjectAuthorization` (idor_*), `TestCrossTenantAllocationReadsAreNotFound` |
| TEN-002 | IMPLEMENTED | `TestCrossTenantApprovalIsDenied`, `TestFleetFinanceRoutesPreventCrossTenantAccess`, `TestCompanyDecisionCannotNameAnotherTenant` |
| TEN-003 | PARTIAL | `TestSQLAlwaysTenantScoped` (`internal/architecture_test.go:183`) checks only branches/departments in one repo dir |
| IAM-001 | IMPLEMENTED | `TestAuthorizationIsDenyByDefaultAndScoped`, `TestSafetyManagePermissionRecordsReference` |
| IAM-002 | IMPLEMENTED | `TestSafetyRoutesDenyWithoutPermission`, `TestFleetFinanceRoutesDenyByDefault`, `TestEligibilityRoutesDenyByDefault` |
| IAM-003 | IMPLEMENTED | `TestEligibilityBranchScopeIsEnforced`, `TestFleetFinanceBranchScope`, `TestProviderMembershipsAreBranchScoped` |
| IAM-004 | IMPLEMENTED | `TestSelfApprovalIsRefused`, `TestAllocationSelfApprovalIsRefused` |
| IAM-005 | PARTIAL | `TestConflictingDecisionIsRejected`, `TestConcurrentAllocationDecisionsProduceOneWinner`; no concurrent-approver quorum test |
| IAM-006 | PARTIAL | `TestRevokedAndExpiredBindingsGrantNothing`, `TestResolveTenant_Revoked*`; no "remove role then reuse token within cache TTL" test |
| IAM-007 | PARTIAL | `TestProviderInvitationTokensAreHashedAndSingleUse` (repo level); staff invitation replay/expiry only via `TestAuthV2StaffInvitationAndMultipleOrganizations` |
| PROV-001 | PARTIAL | `TestFleetClientToRealHaulingAuthority` (hauling client only) |
| PROV-002 | PARTIAL | same; no `CreateProvider` sponsored-create test |
| PROV-003 | MISSING | no split-identifier conflict test |
| PROV-004 | PARTIAL | `TestProviderRepositoryEnforcesOneLiveMembership`, `TestDuplicateProviderLinkIsRefused` |
| PROV-005 | PARTIAL | `TestProviderApps_ParseAdditiveProviderModeContract` (static string check) |
| PROV-006 | MISSING | — |
| PROV-007 | PARTIAL | `TestProviderProvisioningUnknownOutcomeReconcilesByContactWithoutDuplicateIdentity` (identity, not wallet) |
| PROV-008 | MISSING | ENDING route not mounted |
| PROV-009 | IMPLEMENTED | `TestProviderRepositoryEnforcesOneLiveMembership` |
| ASN-001 | IMPLEMENTED | `TestAssignmentAlwaysOffersAcceptAndDecline` |
| ASN-002 | IMPLEMENTED | `TestOfferAcceptanceCommitsTheWholeDelegatedConsequence` |
| ASN-003 | PARTIAL | `TestConcurrentOfferAcceptanceProducesOneWinner` (provider vs provider); no dispatcher-vs-provider hybrid race |
| ASN-004 | IMPLEMENTED | `TestServiceCredentialScope_OrganizationUserDeniedInternalRoute`, `TestInternalRouteRejectsUserTokens` |
| ASN-005 | IMPLEMENTED | `TestConcurrentProviderResponsesSingleWinner`, `TestExactlyOneProviderResponsePerAssignment`, `TestOfferResponseReplaysOnSameClientResponseID` |
| ASN-006 | IMPLEMENTED | `TestDeclineReasonCatalogueAndSafetyClassification`, `TestOfferRecipientDeclineDoesNotCloseOrganizationOffer` |
| ASN-007 | PARTIAL | marketplace: classification only; private: `TestSection17And18EndToEnd` (inspection + work order) |
| ASN-008 | PARTIAL | `TestSection27_ExpireAssignments_*` (no expiry-vs-accept race) |
| ASN-009 | MISSING | — |
| ASN-010 | PARTIAL | provider pull routes exist; no push-failure test |
| ASN-011 | IMPLEMENTED | `TestProviderOfferViewCarriesNoCommercialFields`, `TestProviderAssignmentViewCarriesNoCommercialFields`, `TestTripProjectionStoresNoCustomerCommercialData` |
| ASN-012 | PARTIAL | `TestTripCancellationReleasesCapacityOnlyWhenTrusted`; assignment not converged |
| ASN-013 | IMPLEMENTED | `TestExecutionStartUnknownOutcomeKeepsOriginalIdempotencyKey`, `TestAcceptanceToOneCanonicalTrip` (skippable) |
| TRIP-001 | IMPLEMENTED | `TestIdenticalDuplicateTripEventIsIdempotent` |
| TRIP-002 | IMPLEMENTED | `TestMissingTripVersionMarksGapDetected`, `TestIngestEventGapMarksGapDetected` |
| TRIP-003 | IMPLEMENTED | `TestSection16TrackingGapConflictFreshnessGeofenceAndAlerts`, `TestStaleSweepFlagsSilentProjections` |
| TRIP-004 | PARTIAL | `TestSections89_ManagedCompletionIsRecordedFromBothSources` |
| PRICE-001 | PARTIAL | no explicit test; code has no commission formula |
| PRICE-002 | IMPLEMENTED | `TestProviderFacingViews_NeverSerializeCommercialOrInternalFields` |
| WAL-001 | IMPLEMENTED | `TestFinanceEventDuplicateConflictAndGap` |
| WAL-002 | IMPLEMENTED | `TestFleetOwnsNoLocalLedgerOrBalance`, `TestProviderSubWalletProjectionHoldsNoBalance` |
| WAL-003 | IMPLEMENTED | `TestAllocationIsIdempotentAndFingerprintChecked` |
| WAL-004 | IMPLEMENTED | `TestUnknownOutcomeIsResolvedByReadBackNotResubmission` |
| WAL-005 | PARTIAL | `TestArithmeticRejectsCurrencyMismatch`, `TestPerJobInAnotherCurrencyIsRefused` (unit) |
| WAL-006 | PARTIAL | `TestProviderStatementIsScopedToOneMembership`; provider self-service routes unmounted |
| WAL-007 | MISSING | — |
| ORD-001 | PARTIAL | `TestOrders_CreateConfirmCancelLifecycle` (no single atomic order+shipments+packages command) |
| ORD-002 | PARTIAL | `TestShipments_CargoQuantityAndCoordinateValidation`, `TestShipments_CargoComplianceFailsClosed` |
| ORD-003 | PARTIAL | `private_execution_worker_test.go` |
| ORD-004 | PARTIAL | draft cancel only |
| SHIP-001 | IMPLEMENTED | `TestPackages_CustodyContinuityCorrectionEvidenceAndAppendOnly` |
| SHIP-002 | PARTIAL | package proof gate (`f/packages/usecases/service.go:200`); shipment has no delivered state |
| SHIP-003 | PARTIAL | package FAILED_ATTEMPT -> OUT_FOR_DELIVERY allowed; no dedicated test |
| ROUTE-001 | PARTIAL | `TestSection16OptimizationDurableUnknownOutcomeReadBack` |
| ROUTE-002 | PARTIAL | `TestSection16RoutesStopsPublicationAndReplan` |
| ROUTE-003 | MISSING | candidate facts set `CapacityOK`, `CargoAllowed` etc. all from one `eligible` bool (`internal/app/section16_wiring.go:83`); no hard-constraint rejection |
| ASSET-001 | IMPLEMENTED | `TestSection17And18Concurrency` (asset assignment) |
| ASSET-002 | IMPLEMENTED | `TestSection17And18EndToEnd` (blocked after failed inspection) |
| ASSET-003 | IMPLEMENTED | `TestSection17And18EndToEnd` (returned asset eligibility) |
| ASSET-004 | PARTIAL | `TestExpiryWorkerBlocksMandatoryDocuments`, asset worker counts |
| MNT-001 | PARTIAL | approval always required; no cost threshold |
| MNT-002 | MISSING | — |
| FUEL-001 | PARTIAL | `TestSection17And18EndToEnd` ("invalid/unsafe fuel accepted") |
| COMP-001 | PARTIAL | `TestSection19ComplianceRestrictionSafetyAndProof` |
| COMP-002 | PARTIAL | same; no revocation event consumer test |
| SAFE-001 | PARTIAL | append-only triggers on evidence tables (`TestPackages_*AppendOnly`) |
| SAFE-002 | PARTIAL | worker sets OVERDUE (`f/safety/operations_workers.go:47`); no test found |
| PROOF-001 | PARTIAL | `TestBulkMediaAdapterEnforcesOwnershipPurposeSafetyAndReadiness` |
| PROOF-002 | PARTIAL | package correction evidence test |
| WH-001 | MISSING | no receipt exception state |
| WH-002 | MISSING | no put-away tasks |
| WH-003 | PARTIAL | inventory reservations exist; concurrency test only for movements |
| WH-004 | PARTIAL | overlap check (`f/warehouse/operations.go:365`), non-locking, untested |
| FIN-001 | PARTIAL | `TestSection22FinancePayrollAndAuthorityBoundaries` |
| FIN-002 | IMPLEMENTED | `TestFinanceEventDuplicateConflictAndGap` |
| FIN-003 | PARTIAL | `TestSection22FinancePayrollAndAuthorityBoundaries` |
| FIN-004 | PARTIAL | `TestFinanceOperationsIdempotencyLeaseRecoveryAndReplay`, `cmp027_double_pay_test.go` |
| EVT-001 | IMPLEMENTED | `TestOutboxTransactionAtomicity`, `TestApprovalHistoryAndOutboxAreWrittenAtomically` |
| EVT-002 | IMPLEMENTED | `TestInboxDeduplication`, `TestIngestEventRedeliveryIsDuplicate` |
| EVT-003 | PARTIAL | `TestIngestEventUnknownTypeRejectedPermanently` (type, not schema version) |
| EVT-004 | MISSING | — |
| JOB-001 | IMPLEMENTED | `TestExpiredLeaseRecovery`, `TestOutboxExpiredLeaseRecovery` |
| JOB-002 | IMPLEMENTED | `internal/integration/integration_test.go` non-owner completion check |
| JOB-003 | IMPLEMENTED | `TestJobRetryAndDeadLetter`, `TestRedactBoundsErrorText` |
| SEC-001 | MISSING | no audience/alg test (HMAC self-issued tokens) |
| SEC-002 | IMPLEMENTED | `Test*_ObjectAuthorization` |
| SEC-003 | MISSING | — |
| SEC-004 | PARTIAL | `TestValidationErrorNeverContainsSecrets`, `TestServiceCredentialAdmin_NoSecretLeakage`, `TestAuditRecorderTenantFilteringAndRedaction` |
| SEC-005 | IMPLEMENTED | `TestDestinationValidatorRejectsSSRFIPv4IPv6AndUnsafeURLs` (webhooks) |
| SEC-006 | PARTIAL | `ratelimit_test.go`, `TestRateLimit_RedisBackedLimiterSharesStateAcrossTwoInstances`; no provider-safety exemption test |
| EXP-001 | PARTIAL | `TestExports_WorkerGeneratesFileAndDownloadReauthorizes` |
| IMP-001 | PARTIAL | `TestGenericImports_ValidateAndCommitAreSeparateCommands` |
| IMP-002 | IMPLEMENTED | `TestBulkImports_HeartbeatCancellationDuringImportAndCommittedRowsPreserved` |
| BC-001 | PARTIAL | `TestSections89_IndependentProviderIsUnaffected` |
| BC-002 | PARTIAL | customer suites in CI only |
| BC-003 | PARTIAL | `TestCompatibilityFixtures_CoverRequiredProviderContracts` |
| MIG-001 | IMPLEMENTED | `TestMigrationReplay_EmptySchemaAppliesCleanly` |
| MIG-002 | PARTIAL | `TestMigrationReplay_ExistingDataSurvivesUpgrade` (synthetic data, not a staging copy) |
| MIG-003 | IMPLEMENTED | `TestMigrationReplay_*RollbackAndReapply` |
| MIG-004 | MISSING | no runtime-role test; service runs its own migrations (`MIGRATION`, `internal/config/config.go:542`) |
| PERF-001 | PARTIAL | `TestCapacity_*` (dev machine) |
| PERF-002 | PARTIAL | `TestSection31Capacity_OperationalProfiles` |
| DR-001 | PARTIAL | `TestBackupAndRestore_RealDisposableDatabase` (no PITR) |
| DR-002 | PARTIAL | `TestOutbox_DeadLetterThenReplay`, `TestSearchProjection_RebuildsAfterLoss` |

Totals: IMPLEMENTED 37, PARTIAL 53, MISSING 13.

66.1 command layers: gofmt (changed files), vet, build, govulncheck, full `go test` against Postgres+Redis — present. `go test -race` — absent in all workflows. staticcheck — absent. Secret scanning — absent (`grep gitleaks|trufflehog .github/workflows` -> none).

---

## Section 67. Environment and configuration contract

Fleet reads `FLEET_PARTNER_*` and module flags from `internal/config/config.go` (338 names). Exact = same name; Renamed = equivalent under another name; Missing = no equivalent.

| Spec var | Status | Fleet name / note |
|---|---|---|
| APP_ENV | EXACT | — |
| SERVICE_NAME | MISSING | constant `config.ServiceName` |
| HTTP_ADDR | EXACT | default `:8113` (`config.go:541`), spec `:8110` (8110 is admin-backoffice) |
| PUBLIC_BASE_PATH | MISSING | hard-coded `/fleet-partner/v1` (spec `/api/v1/corporate-logistics`) |
| INTERNAL_BASE_PATH | MISSING | hard-coded `/internal/fleet-partner/v1` |
| SHUTDOWN_TIMEOUT | Renamed | `FLEET_PARTNER_SHUTDOWN_TIMEOUT` |
| REQUEST_BODY_MAX_BYTES | Renamed | `FLEET_PARTNER_MAX_REQUEST_BODY_BYTES` |
| TRUSTED_PROXY_CIDRS | Renamed | `FLEET_PARTNER_TRUSTED_PROXIES` (not required in production) |
| BUILD_VERSION | MISSING | — |
| DATABASE_URL | Renamed | `FLEET_PARTNER_DATABASE_URL` |
| DB_MAX_CONNS | Renamed | `FLEET_PARTNER_DATABASE_MAX_OPEN_CONNECTIONS` |
| DB_MIN_CONNS | Renamed (different semantics) | `FLEET_PARTNER_DATABASE_MAX_IDLE_CONNECTIONS` |
| DB_MAX_CONN_LIFETIME | Renamed | `FLEET_PARTNER_DATABASE_CONNECTION_MAX_LIFETIME` |
| DB_STATEMENT_TIMEOUT | MISSING | — |
| DB_MIGRATIONS_ENABLED | Renamed | `MIGRATION` |
| DB_MIGRATION_LOCK_ID | MISSING | no advisory lock |
| REDIS_URL | Renamed | `FLEET_PARTNER_REDIS_ADDR/_PASSWORD/_DB/_REQUIRED` |
| REDIS_KEY_PREFIX | MISSING | — |
| CACHE_DEFAULT_TTL | MISSING | (`FLEET_PARTNER_AUTHORIZATION_CACHE_TTL` only) |
| JWT_ISSUER / JWT_AUDIENCE / JWT_JWKS_URL / JWT_CLOCK_SKEW | MISSING (design differs) | Fleet issues HMAC tokens: `FLEET_PARTNER_ACCESS_TOKEN_SECRET`, `_ACCESS_TOKEN_TTL`, `_REFRESH_TOKEN_TTL` |
| SERVICE_AUTH_AUDIENCE / SERVICE_AUTH_JWKS_URL | MISSING (design differs) | HMAC `FLEET_PARTNER_SERVICE_SECRETS` + per-client secrets |
| MTLS_CA_FILE | MISSING | — |
| FIELD_ENCRYPTION_KEY_ID | MISSING | `internal/platform/fieldcrypto` exists but is not wired anywhere |
| WEBHOOK_SECRET_KEY_ID | Renamed (raw key, not KMS ID) | `FLEET_PARTNER_WEBHOOK_SECRET_ENCRYPTION_KEY` |
| STEP_UP_MAX_AGE | Renamed | `FLEET_PARTNER_STEP_UP_TTL` |
| AUDIT_HMAC_KEY_ID | Renamed (raw secret) | `FLEET_PARTNER_AUDIT_CHAIN_SECRET` (required in production, `config.go:1243`) |
| DISPATCH_SERVICE_URL | EXACT | also `FLEET_PARTNER_DISPATCH_SERVICE_BASE_URL` |
| TAXI / HAULING / WALLET / NOTIFICATION / MEDIA / VERIFICATION / SUPPORT / ANALYTICS / ADMIN `_SERVICE_URL` (9) | Renamed | `FLEET_PARTNER_{TAXI,HAULING,PAYMENT_WALLET,NOTIFICATION,MEDIA_FILE,VERIFICATION_COMPLIANCE,SUPPORT_DISPUTE,ANALYTICS,ADMIN_BACKOFFICE}_SERVICE_BASE_URL`. None is required in production (empty URLs are skipped, `config.go:1220-1227`); clients then fail closed with `ErrNotConfigured`. |
| DOWNSTREAM_READ_TIMEOUT / DOWNSTREAM_WRITE_TIMEOUT | Renamed (single value) | `FLEET_PARTNER_HTTP_CLIENT_TIMEOUT`, `DISPATCH_SERVICE_TIMEOUT_SECONDS` |
| EVENT_TRANSPORT | MISSING | Redis streams only |
| EVENT_STREAM_PREFIX | Renamed | `FLEET_PARTNER_OUTBOX_STREAM_KEY` |
| OUTBOX_BATCH_SIZE | MISSING | — |
| OUTBOX_POLL_INTERVAL | Renamed | `FLEET_PARTNER_OUTBOX_POLL_INTERVAL` |
| INBOX_MAX_ATTEMPTS | MISSING | per-feature `*_MAX_ATTEMPTS` only |
| WORKER_INSTANCE_ID | MISSING | generated (`ids.New("wrk")`) |
| JOB_BATCH_SIZE | MISSING | — |
| JOB_LEASE_DURATION / JOB_HEARTBEAT_INTERVAL | Renamed | `FLEET_PARTNER_JOB_LEASE_DURATION`, `FLEET_PARTNER_JOB_HEARTBEAT_INTERVAL` |
| DEAD_LETTER_RETENTION_DAYS | MISSING | — |
| WORKER_TYPES_ENABLED | Renamed | `FLEET_PARTNER_WORKER_ENABLED` + `SECTION1x_WORKERS_ENABLED` + per-worker flags |
| OFFER_RESERVATION_TTL | MISSING | — |
| ASSIGNMENT_RESPONSE_TTL | Renamed (private only) | `FLEET_PRIVATE_ASSIGNMENT_RESPONSE_TTL_SECONDS` |
| ASSIGNMENT_EXPIRY_INTERVAL | Renamed | `FLEET_ASSIGNMENT_EXPIRY_INTERVAL_SECONDS` |
| LOCATION_FRESHNESS_THRESHOLD | Renamed | `FLEET_SECTION16_TRACKING_FRESH_SECONDS` / `_STALE_SECONDS` |
| TRIP_RECONCILIATION_INTERVAL | Renamed | `FLEET_MARKETPLACE_RECONCILE_INTERVAL_SECONDS` |
| PROVIDER_DECLINE_NOTE_MAX_LENGTH | MISSING | — |
| MAX_ACTIVE_ASSIGNMENTS_PER_PROVIDER | MISSING | one live assignment enforced in code |
| PRICING_ENGINE_MODE | MISSING | — |
| PROVIDER_COMMERCIAL_FIELDS_ENABLED (must be false, startup rejects true) | MISSING | enforced structurally by DTOs instead |
| WALLET_OPERATION_TIMEOUT | MISSING | — |
| WALLET_RECONCILIATION_INTERVAL | Renamed | `FLEET_COMPANY_WALLET_WORKER_INTERVAL_SECONDS`, `FLEET_FINANCE_WORKER_INTERVAL_SECONDS` |
| WALLET_ALLOCATION_KILL_SWITCH (default true) | Renamed, default differs | `FLEET_COMPANY_WALLET_KILL_SWITCH` default **false** (`config.go:660`); `PROVIDER_ALLOCATION_ENABLED` default false |
| WALLET_STALE_AFTER | Renamed | `FLEET_COMPANY_WALLET_STALE_SECONDS` |
| DEFAULT_CURRENCY | MISSING | (`FLEET_PARTNER_DEFAULT_COUNTRY_CODE` is a country) |
| MAX_ALLOCATION_MINOR_WITHOUT_APPROVAL | MISSING | approval policy is per provider |
| FINANCIAL_EXPORT_TTL / SIGNED_FILE_URL_TTL / REPORT_MAX_RUNTIME | MISSING | — |
| EXPORT_FILE_TTL | Renamed | `FLEET_EXPORTS_RETENTION_HOURS` |
| IMPORT_MAX_BYTES / IMPORT_MAX_ROWS | Renamed | `FLEET_BULK_IMPORT_MAX_BYTES`, `FLEET_BULK_IMPORT_MAX_ROWS` |
| PRECISE_LOCATION_RETENTION_DAYS | Renamed | `FLEET_SECTION16_TRACKING_RETENTION_SECONDS` |
| AUDIT_RETENTION_DAYS / DEFAULT_OPERATIONAL_RETENTION_DAYS | MISSING | retention is DB policy data (`f/retentionpolicy`) |
| PRIVACY_ERASURE_WORKER_ENABLED | Renamed (no erasure) | `FLEET_PARTNER_RETENTION_EXECUTION_ENABLED`, `DATA_RETENTION_WORKER_ENABLED`; erasure/anonymisation not implemented (`f/retention/service.go:6`) |
| LOG_LEVEL | Renamed | `FLEET_PARTNER_LOG_LEVEL` |
| LOG_REDACTION_ENABLED | MISSING | always-on redaction, not configurable |
| OTEL_EXPORTER_OTLP_ENDPOINT | MISSING | only `FLEET_PARTNER_TRACE_SAMPLE_RATE` |
| METRICS_AUTH_TOKEN | Renamed | `FLEET_PARTNER_METRICS_AUTH_TOKEN` |
| SENTRY_DSN | MISSING | — |
| FEATURE_CORPORATE_MARKETPLACE | Renamed (partial) | `FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED` + `FLEET_MARKETPLACE_ALLOWED_CALLERS`; accept/assign hard-coded on |
| FEATURE_PROVIDER_SPONSORED_ACCOUNTS | MISSING | provisioning ungated |
| FEATURE_PROVIDER_SUBWALLETS | Renamed | `PROVIDER_SUBWALLET_ENABLED` |
| FEATURE_PRIVATE_OPERATIONS | Renamed | `PRIVATE_ORDERS_ENABLED`, `ORDERS_ENABLED`, ... + `FLEET_PRIVATE_OPERATIONS_KILL_SWITCH` |
| FEATURE_WAREHOUSE | Renamed | `WAREHOUSES_ENABLED`, `WAREHOUSE_INVENTORY_ENABLED` |
| FEATURE_AUTOMATION | Renamed | `AUTOMATION_RUNNER_WORKER_ENABLED` (job type always fails permanently, prior audit) |
| FEATURE_AI_RECOMMENDATIONS | MISSING | deferred (ADR-002) |

Counts: 3 exact, 47 renamed, 40 missing (of 90). 67 preamble: "production never falls back to logging/no-op" — IMPLEMENTED for clients (`internal/platformclients/base.go:9,96`, fail closed), outbox stays PENDING without Redis (not silent success). Production required-service URLs are not validated at startup — PARTIAL.

---

## Section 68. Migration, rollout and backward-compatibility plan

68.1 original-draft mapping: PARTIAL — documented (`docs/section30-original-api-mapping.md`); `/drivers/*` mapping works for hauling only; `/verification/*` facade exists but no read-back; `/account/delete` -> closure is always blocked.

68.2 additive changes in existing services:

| Component | Status | Evidence |
|---|---|---|
| driver-dispatch-delivery-service | PARTIAL | Offer events, reservations, executions, provider relays, `PROVIDER_ORGANIZATION_MODE_ENABLED` exist. No identity resolve/sponsored-create endpoint; internal routes differ from Fleet's client (finding 5). |
| driver-taxi-service | PARTIAL | Provider offer/assignment relay client to Fleet (`services/driver-taxi-service/internal/features/fleetprovider/client.go:103-158`), private-execution verifier. No offer feed to Fleet, no identity contract, no organization-mode flag. Fleet side deferred (ADR-001). |
| driver-hauling-service | PARTIAL | `fleetauthority` resolve/sponsored/provider-links/reservations/executions (`.../fleetauthority/http.go:38-57`). No marketplace offer events to Fleet; association decision not pushed or read back. |
| payment-wallet-service | PARTIAL | Managed settlement contract and allocation/withdrawal endpoints exist per CLAUDE.md; organization wallet provisioning reachable from Fleet; flags off. |
| notification-service | PARTIAL | Fleet sends invitations; no organization audiences/assignment templates verified here. |
| verification-compliance-service | PARTIAL | Organization submissions + decisions + outbox event exist; Fleet does not consume the decision. |
| media-file-service | IMPLEMENTED | Fleet media client used throughout (ownership/purpose checks tested). |
| support-dispute-service | PARTIAL | Support reference client exists (`internal/platformclients/support_reference.go`); support projections off. |
| analytics-service | MISSING | Fleet analytics client configured by URL only; no curated ingestion found. |
| api-gateway | IMPLEMENTED | `/fleet-partner/` upstream (`services/api-gateway/internal/features/routing/models/routes.go:83`). |
| customer app/service | N/A | No change required. |
| provider apps | PARTIAL | Dispatch app: organization offers + company assignments accept/decline (`apps/dispatch_provider/lib/features/requests/data/requests_api.dart:100-272`). Truck app: parses mode fields only, no managed assignment UI. Taxi app: nothing. |

68.3 rollout stages:

| Stage | Status | Note |
|---|---|---|
| R0 Audit and freeze | IMPLEMENTED | audit docs + this audit |
| R1 Foundations dark | IMPLEMENTED | flags default off except marketplace writes |
| R2 Organization pilot | PARTIAL | open signup; activation blocked |
| R3 Managed provider shadow | PARTIAL | hauling only; association never completes |
| R4 Marketplace projection | PARTIAL | dispatch only |
| R5 Controlled company acceptance | MISSING | no allowlist |
| R6 Delegated/hybrid | PARTIAL | code + tests; no load evidence |
| R7 Financial enablement | PARTIAL | kill switches exist; wallet allocation kill switch default false |
| R8 Private ops core | PARTIAL | enabled on staging; lifecycle stops at CONFIRMED |
| R9 Fleet/maintenance/compliance | PARTIAL | enabled on staging; RTS tests pass |
| R10 Warehouse/specialized | PARTIAL | enabled on staging; receipt/put-away missing |
| R11 Automation/AI | MISSING | deferred |
| R12 General availability | MISSING | no SLO evidence, DR exercise is dev-only |

68.4 rollback rules: flags stop entry while reads continue — IMPLEMENTED for most modules (writes/reads split flags, kill switches). Applied migrations not edited — IMPLEMENTED (policy test). Disabling marketplace — PARTIAL (writes hard-coded on). Disabling allocations keeps reconciliation — IMPLEMENTED (`FLEET_COMPANY_WALLET_KILL_SWITCH`, `TestKillSwitchOverridesEnabledFlags`). Pause consumers per family — PARTIAL (per-feature ingestion flags). Rollback never converts managed settlement to provider wallet — IMPLEMENTED (wallet refuses managed work in `CompleteJob`, per CLAUDE.md 2026-09-14).

---

## Section 69. Work packages and definition of done

| WP | Status | Note |
|---|---|---|
| WP0 audit | IMPLEMENTED | docs |
| WP1 foundation | IMPLEMENTED | — |
| WP2 tenancy/org core | IMPLEMENTED | — |
| WP3 branches/structure | PARTIAL | branches/departments tables + Head Office; no CRUD routes; teams/hubs/cost centres missing |
| WP4 IAM/approvals | PARTIAL | approvals real; roles/bindings/policies/delegations not routed |
| WP5 platform foundations | IMPLEMENTED | idempotency, outbox/inbox, jobs, DLQ, operator replay |
| WP6 provider identity integration | PARTIAL | hauling only; no contract tests for split/ambiguous |
| WP7 managed memberships | PARTIAL | cannot reach ACTIVE |
| WP8 sub-wallet provisioning | PARTIAL | never requested |
| WP9 marketplace read projections | PARTIAL | dispatch only |
| WP10 organization acceptance | PARTIAL | dispatch adapter mismatch |
| WP11 provider assignments (ASN-001..013 for all three services) | PARTIAL | ASN tests are service-agnostic fakes; taxi deferred |
| WP12 canonical execution and rescue | PARTIAL | execution start real; assignment completion and rescue missing |
| WP13 commercial core | PARTIAL | leads missing |
| WP14 private order/custody core | PARTIAL | lifecycle stops at CONFIRMED |
| WP15 dispatch/route planning | PARTIAL | capacity/time-window not evaluated |
| WP16 workforce | PARTIAL | templates/enrolments missing |
| WP17 fleet/asset lifecycle | IMPLEMENTED | (vocabulary differs from App. B) |
| WP18 maintenance/parts/fuel | IMPLEMENTED | (no cost-threshold approval) |
| WP19 compliance/safety/claims | PARTIAL | incident free-form |
| WP20 warehouse/yard/dock | PARTIAL | receipt/put-away missing |
| WP21 finance/payroll | PARTIAL | profitability missing |
| WP22 communication/cases | IMPLEMENTED | Section 23 real (prior audit) |
| WP23 reports/integrations/automation | MISSING | reports and automation missing; API clients/webhooks exist |
| WP24 hardening/GA | MISSING | Section 66/71 not met |

69.1 definition of done: PARTIAL — up/down + tenancy fields + Postgres tests are consistent; OpenAPI and event catalogue cover only some sections; "feature disabled by default" is violated by marketplace writes; several state transitions lack history/outbox (58 preamble).

---

## Section 70 and Appendix D. End-to-end journeys

"Default" = default config; "staging" = `deploy/platform/fleet-staging-modules.txt` (no marketplace, wallet, provider or section-worker flags set).

| Journey | Runs end to end? | First break |
|---|---|---|
| 70.1 Organization onboarding and activation | No (PARTIAL) | Steps 9-11 work (draft, profile, documents, submit, verification request). Step 12-13: admin decides in verification-compliance (`VERIFIED`), Fleet expects `APPROVED` and never reads it back; `TryActivate` never called; activation also needs an ACTIVE company wallet (staging has no wallet secret). Company stays UNDER_REVIEW and `RequireOperationalOrganization` blocks operational writes. |
| 70.2 Create and activate a managed provider | No (PARTIAL) | DISPATCH/TAXI: `POST /providers` fails closed (no contract). HAULING: resolve/provision and invitation work, membership -> PENDING_PROVIDER_ACCEPTANCE; provider acceptance is never recorded (no caller of `RecordAssociationOutcome`, no read-back, no invite redeem route) so sub-wallet and activation never happen (step 18-20). |
| 70.3 Centralized marketplace request | No (PARTIAL) | Needs an ACTIVE org and ACTIVE membership (above). Only dispatch sends offers; assignment calls a dispatch eligibility route that does not exist (finding 5). If those pass: provider accept -> execution start -> trip created; assignment stays READY_FOR_EXECUTION (never IN_PROGRESS/COMPLETED); settlement to org wallet needs finance/settlement flags. |
| 70.4 Centralized assignment declined | No (PARTIAL) | Decline + reassignment case work; UNSAFE_VEHICLE on a marketplace assignment only emits a follow-up event — no asset block/work order (step 31); "no provider before deadline -> release source opportunity" relies on offer expiry, which no worker runs. |
| 70.5 Delegated provider acceptance | No (PARTIAL) | Code real (`offer_response.go`), dispatch app UI exists; blocked by membership activation; offer becomes ACCEPTED before source confirmation. |
| 70.6 Hybrid race | No (PARTIAL) | Row-locked offer prevents double win; no dispatcher-vs-provider race test; same upstream blockers. |
| 70.7 Private order to platform execution | No (PARTIAL) | Order create/confirm work; no plan/ready/execution-requested commands; shipments stuck in DRAFT; private assignment + execution exist (flags `PRIVATE_*_EXECUTION_ENABLED` off on staging) but never update the order projection (step 49). |
| 70.8 Provider sub-wallet allocation | No (PARTIAL) | Allocation commands, approval, idempotency, unknown-outcome read-back are real; requires an ACTIVE sub-wallet link, which is never provisioned; `PROVIDER_ALLOCATION_ENABLED` off. |
| 70.9 Unsafe asset and rescue | No (PARTIAL) | Private-assignment UNSAFE decline -> asset BLOCKED + inspection + work order (worker, `SECTION18_WORKERS_ENABLED`); work order + RTS gate real. Marketplace decline path does not block the asset; rescue is a breakdown record with `rescue_status` only (no custody transfer/source notification). |
| 70.10 Warehouse fulfilment and dispatch | No (MISSING) | No yard-linked receipt, receiving session, exception state or put-away tasks; manifest dispatch does not request canonical execution. |
| 70.11 Incident, claim and legal hold | No (PARTIAL) | Incident, corrective actions, claims, legal holds exist; incident status free-form; closure not gated on claims/approval. |
| 70.12 Organization closure | No (MISSING) | `DefaultClosureCheckers` always UNKNOWN -> BLOCKED; no step-up on request; no provider offboarding/wallet handling. |

| Appendix D flow | Status | Break |
|---|---|---|
| D.1 Company creates managed provider | PARTIAL | Hauling only; stops before sub-wallet (needs association) and before ACTIVE. Notification step works. |
| D.2 Centralized acceptance + decline | PARTIAL | "create safety alert / inspection candidate" only as an outbox event for marketplace; dispatch adapter mismatch. |
| D.3 Delegated acceptance | PARTIAL | 409 for second provider works; "accept organization + provider atomically at source" happens later via execution start. |
| D.4 Trip completion and settlement | PARTIAL | Completed-work and finance projections exist; assignment never COMPLETED; all finance flags default off. |
| D.5 Private order executed by platform provider | MISSING | Order never progresses past CONFIRMED and never reflects execution. |

---

## Section 71. Final release and production readiness checklist

Self-report: `docs/release1-master-spec-acceptance.json` marks all 40 items PASS, enforced only for presence by `TestSection71MachineChecklistIsCompleteAndEvidenceBacked` (`internal/masterreleasecontract/coverage_contract_test.go:147`). Items marked **False PASS** below contradict code.

| # | Status | Evidence |
|---|---|---|
| 71.1.1 Sibling service, not Partner API | Done | separate service; gateway `/fleet-partner/` route |
| 71.1.2 Customer/independent back-compat | Partly | `TestSections89_IndependentProviderIsUnaffected`; other-service suites in CI |
| 71.1.3 Ownership boundaries, no cross-service DB | Done | `TestNoCrossServiceReferences`, architecture tests |
| 71.1.4 No hard-coded pricing | Done | no platform formula in Fleet |
| 71.1.5 Accept/Decline everywhere; no impersonation | Partly | true for assignments that exist; truck/taxi apps lack UI; prov. never ACTIVE |
| 71.2.1 Section 57 tables reviewed | Partly | many tables; shipment/asset/incident/WO/invoice vocabularies diverge; no warehouse receipt tables |
| 71.2.2 Fresh apply, baseline replay, rollback | Partly | no staging-baseline replay |
| 71.2.3 Runtime role cannot DDL/mutate audit | Not done (**False PASS**) | append-only triggers exist, but no role separation; service runs its own migrations (`MIGRATION`) |
| 71.2.4 Tenant/branch scoping in repos and tests | Partly | `TestSQLAlwaysTenantScoped` covers 2 tables |
| 71.2.5 Integer minor units | Done | no float/numeric money columns |
| 71.3.1 Every route in OpenAPI with examples/permission | Partly | no OpenAPI for Sections 8-13, 22-23; 36 mounted routes missing from riskregistry (prior audit) |
| 71.3.2 Strict validation, pagination, idempotency, If-Match | Partly | idempotency/expected-version widespread; no mass-assignment tests |
| 71.3.3 Role/branch/field/financial visibility tests | Done | `field_visibility_test.go`, branch tests |
| 71.3.4 Provider DTOs no commercial fields | Done | provider view tests |
| 71.3.5 High-risk step-up/approval/SoD | Partly | approvals + SoD real; work-order and invoice approval have no SoD |
| 71.4.1 Typed clients with auth/timeout/retry/idempotency/reconciliation | Partly | yes for wallet/hauling/media; dispatch client paths wrong (finding 5) |
| 71.4.2 Versioned, contract-tested schemas with each service | Partly | asyncapi for Sec 26/55 only; no real-dispatch contract test |
| 71.4.3 Outbox/inbox duplicate, gap, crash, replay | Done | events/trip/synchronization tests |
| 71.4.4 Unknown writes cannot duplicate trips/money | Done | execution start + wallet read-back tests |
| 71.4.5 Freshness and source-of-truth visible | Done | projection freshness fields, tracking freshness |
| 71.5.1 Durable async work with lease/heartbeat/DLQ/recovery | Partly | jobs/outbox durable; marketplace expiry/reassignment are ticker loops (no lease); offer expiry not run |
| 71.5.2 Worker metrics/alerts and kill switches | Partly | metrics + kill switches; no alert rules in repo |
| 71.5.3 No production no-op/logging clients | Done | `ErrNotConfigured` fail-closed |
| 71.5.4 Section 65 runbooks exercised in staging | Not done (**False PASS**) | 11 runbooks, mostly CRM/onboarding; required unsafe-asset, wallet-discrepancy, cross-tenant, DB-unavailable runbooks not present as runbooks; no staging exercise record |
| 71.5.5 Backup restore + replay meets RPO/RTO | Partly | disposable-DB restore test; no PITR/staging restore evidence in this repo (`certify-staging-restore.yml` exists, not evaluated) |
| 71.6.1 JWT/service-auth audience, revocation, algorithm tests | Partly (**False PASS** for audience/alg) | revocation tested; no audience/alg tests |
| 71.6.2 IDOR/privilege/mass-assignment/injection/SSRF/rate-limit | Partly | IDOR, privilege escalation, SSRF, rate limit tested; mass assignment/injection not |
| 71.6.3 Secrets/PII redaction | Partly | redact package + tests; traces/dead letters partly |
| 71.6.4 Encryption/KMS, rotation, signed files, webhook signing | Partly | webhook encryption/signing yes; `fieldcrypto` not wired; no KMS |
| 71.6.5 Retention, legal hold, privacy export/erasure, audit access | Partly (**False PASS** for erasure) | legal hold/retention/audit access tested; no privacy export/erasure |
| 71.7.1 Unsafe/expired/restricted providers/assets cannot accept | Partly | restriction gates exist; `RESTRICTION_PROPAGATION_ENABLED` default off; no ASN-009 test |
| 71.7.2 Safety decline no automatic punishment | Done | `SafetyProtected` + no deduction path |
| 71.7.3 RTS needs independent gates | Partly | gates real; same user may complete and return to service |
| 71.7.4 Wallet authoritative; Fleet cannot edit balances | Done | `TestFleetOwnsNoLocalLedgerOrBalance` |
| 71.7.5 Sub-wallet for every active provider; no auto-credit | Partly | structurally true because no provider is ever ACTIVE |
| 71.8.1 All Section 66 tests pass | Not done (**False PASS**) | 13 of 103 have no test and 53 are only partly covered; IDs not traceable |
| 71.8.2 gofmt, vet, build, race, dependency + secret scans | Not done (**False PASS**) | no `-race`, no secret scan; gofmt only on changed files |
| 71.8.3 Performance meets SLOs | Not done (**False PASS**) | dev-machine profiles, docs say "no production SLA claim" |
| 71.8.4 Flags default safely; rollout/rollback tested | Partly | marketplace writes hard-coded on; wallet kill switch default false |
| 71.8.5 Open risks/deferrals documented and approved | Partly | ADR-001/002 only; activation, association, order lifecycle, warehouse receipt gaps undocumented as deviations |

Totals: Done 12, Partly 23, Not done 5 (7 items flagged False PASS against the self-report).

## Section 72. Final implementation authority — PARTIAL
The universal Accept/Decline rule is honoured for every assignment the system can create (`TestProviderDeclineRightCannotBeDisabled`). Section 72 also requires a written ADR for every deviation; the state-vocabulary changes (58 vs App. B), the env naming (67), the missing warehouse-receipt aggregate and the hard-coded marketplace flags have none (only `docs/adr/ADR-001`, `ADR-002`).

---

## 1. Cross-service work required

**fleet-partner-service (own)**
- Consume `organization.verification.status_changed.v1` (or poll `GET /organizations/:id/submissions/:id`), map `VERIFIED` -> APPROVED, call `TryActivate`; add REJECTED/RESTRICTED/CLOSURE_PENDING per 58.1.
- Record provider association outcome: internal route or event consumer that calls `RecordAssociationOutcome`, plus reconciler read-back via `ReadAssociation`; invitation redeem route; then request sub-wallet.
- Wire DISPATCH and TAXI provider contracts; fix dispatch execution client paths (eligibility, reservation read-back, release).
- Register `OfferRepository.ExpireDue` (incl. RESERVED) with history/outbox; move marketplace assignments to IN_PROGRESS/COMPLETED/CANCELLED from trip events; cancel assignment on source cancellation.
- Order/shipment command set (submit/approve/plan/ready/execution-requested; shipment ready/manifested/.../delivered/returned) and order projection from private execution events.
- Warehouse receipt aggregate + put-away tasks; incident transition matrix; asset/work-order/invoice state machines aligned with 58; SoD on work-order/invoice approval; invoice void with reversal document.
- Flag-gate marketplace writes and provider provisioning; per-organization allowlist; migration advisory lock; `fieldcrypto` wiring; production validation of required service URLs; CI `-race` + secret scan; least-privilege runtime role test; tests tagged with Section 66 IDs.

**verification-compliance-service**: deliver the organization decision to Fleet (signed callback or published stream Fleet subscribes to); agree vocabulary (VERIFIED vs APPROVED); provider/asset revocation events for COMP-002.

**driver-dispatch-delivery-service**: either expose `GET /api/v1/internal/marketplace/eligibility` and request-keyed reservation read/release, or Fleet adopts the existing routes; add provider resolve + sponsored-create endpoints; association accept/decline relay to Fleet.

**driver-hauling-service**: publish marketplace offer/request events to `/internal/fleet-partner/v1/events`; push or expose association (provider-link) acceptance so Fleet can complete membership.

**driver-taxi-service**: organization-mode flag, identity resolve/sponsored contract, offer event feed (Fleet side gated by ADR-001).

**payment-wallet-service**: confirm organization wallet provisioning endpoint used by onboarding and sub-wallet ensure are reachable from Fleet with staging credentials; settlement events consumed by Fleet's finance ingestion.

**notification-service**: organization/provider audiences and assignment/association templates (spec 68.2).

**admin-backoffice-service**: operator UI/command to decide company verification that ends in Fleet activation (currently verification decision is recorded but nothing activates).

**support-dispute-service / analytics-service**: organization-context references and curated ingestion (68.2), currently client-only.

**deploy / CI**: staging flags for marketplace ingestion, assignment expiry, reconciliation and section workers; Fleet wallet secret on staging (activation gate).

## 2. Frontend work required

**web/fleet** (see also `(audit working file)`)
- Onboarding/company page: show verification decision states (UNDER_REVIEW, CHANGES_REQUIRED, REJECTED, RESTRICTED) and wallet provisioning status/retry; activation progress once the backend reads the decision back.
- Branch/department management (settings/branches is mock today) once routes exist; custom roles, role bindings, approval policies/delegations UIs once backend routes exist.
- Providers: provisioning status per saga step (association awaiting provider, sub-wallet), resend/reactivate/remove actions; DISPATCH/TAXI category once contracts are wired; replace mock availability/compliance/declines/work-setup tabs.
- Operations: offers inbox (reserve/accept/reject with expiry), assignment board showing provider response state, decline reasons, reassignment cases; link trips (currently `/operations/trips` exists per activation report).
- Private ops: order lifecycle actions (submit/approve/plan/ready/request execution), shipment status actions, execution status on order detail (16 TODO stubs per prior audit).
- Warehouse: receiving session and put-away task screens once receipt aggregate exists.
- Compliance: incidents/corrective actions/claims wired to real API with a guarded lifecycle (currently mock).
- Wallet: replace 13 mock wallet pages with organization wallet projection, allocations, withdrawals (backend exists, flags off).
- Settings/closure: closure request with step-up, blocker list, approval/execute.

**apps/dispatch_provider**: already has organization offers and company assignments accept/decline; add company association accept/decline, organization identity and field-visibility handling, and private assignment acknowledgement if not present.

**apps/truck_provider**: managed-mode UI is missing — company association accept/decline, pending marketplace and private assignments with mandatory Accept/Decline + reason catalogue (UNSAFE_VEHICLE etc.), organization identity banner, own sub-wallet statement/withdrawal (needs Fleet provider self-service routes mounted).

**apps/taxi_provider**: no organization mode at all — parse `providerMode`/organization/assignment policy, association acceptance, offers/assignments with Accept/Decline (blocked on ADR-001 Taxi deferral).
