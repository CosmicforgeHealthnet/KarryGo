# Fleet Sections 5-20: requirements matrix

Snapshot **2026-11-14**, branch `staging`, evaluated at commit `692ca11`. Spec: KarryGo Corporate Logistics Specification v2.0 (Part 1 Sections 5-20; detail rows from Part 2 Sections 36-50).

**Method.** This replaces the 2026-11-13 snapshot, in which most rows said "IMPLEMENTED (audit) / not re-verified". Every row now carries direct evidence or an open status:

1. Start from the read-only evidence audit of 2026-11-13 (HEAD `612934c`), which traced each route to its handler, use case and SQL, split rows that bundle independently testable behaviour into sub-rows (`FLEET-8.4a`, `FLEET-7b.b`, ...) and gave each a verdict. Rows the audit did not split are kept as they were.
2. Walk every commit from `612934c` to `692ca11` that touches Fleet, the web app, Wallet, Hauling, Taxi or the deploy scripts. For each claimed fix, read the code at HEAD and confirm the named test exists at HEAD and actually exercises the behaviour. Commit messages alone never upgrade a row.
3. An audit DEFECT counts as closed only when a later commit fixed it and a test covers the fix.

**Test runs backing this snapshot.**
- Full Fleet suite at `692ca11` (`go test -p 1 -count=1 ./...`, PostgreSQL 16, fresh migrated database): 84 packages ok, 0 failures. Also run at `d4e3a79` (all database-backed packages ok). Earlier coordinator runs at `308b463` were also green.
- payment-wallet-service at `692ca11`: the Fleet managed-settlement and provider-withdrawal integration tests named below pass against PostgreSQL.
- `web/fleet` at `308b463`: `tsc --noEmit` clean, `npm test` 43/43, `npm run build` ok.
- Staging: Fleet `308b463` deployed (run 43563718925), API and worker revision `308b4632`, `api_health=healthy`, migrations through 000095 at the previous deploy. The Vercel bundle carries the wave-2 web code. Later commits (`d4e3a79` onwards, migration 000097) are not yet deployed.

**Status values.**
- **CLOSED - VERIFIED**: the code exists and a named, passing PostgreSQL-backed test (router or use case) exercises it. For purely structural or pure-function rows (Section 5 layout, ID generation) the evidence is a static or unit test and the row says so.
- **IMPLEMENTED - NOT INDEPENDENTLY TESTED**: the code exists and is mounted, but no test exercises the behaviour (or only a minor refusal path). The code location is cited.
- **PARTIAL**: something concrete is missing; the gap names it and, where relevant, the owning service.
- **MISSING**: not built.
- **BLOCKED - EXTERNAL DEPENDENCY** and **SOURCE REQUIREMENT UNAVAILABLE** are available but no row needed them: every dependency here is an internal service.

**Reading the columns.** *Initial status* is the 2026-11-13 matrix status ("EXTERNAL" was a value of that matrix). *Commit* is the change that introduced the cited test or the fix; `41256b9` is the repository baseline commit in which tests that predate this work first appear. Test paths are relative to `services/fleet-partner-service/internal/` unless prefixed. Staging flag notes refer to `deploy/platform/fleet-staging-modules.txt` and `deploy/platform/staging-pairings.txt`.


## Section 5 - Service structure

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-5a | `cmd/main.go`, `cmd/worker/main.go` | IMPLEMENTED | **CLOSED - VERIFIED** | API binary wires routes and in-process workers; dedicated worker binary. Static router-construction test (structural row). | `41256b9` | `cmd/browser_routes_test.go:TestBrowserAndProviderRoutesConstructWithoutWildcardConflict` |
| FLEET-5b | `internal/{config,database,middleware,platformclients,events,jobs,observability}` | IMPLEMENTED | **CLOSED - VERIFIED** | All seven packages present, each with its own tests; pool stats test runs against PostgreSQL. | `41256b9` | `observability/dbpool_test.go:TestObservePoolStats_RecordsBoundedGauges` |
| FLEET-5c | Feature packages named by the spec (organizations ... automation) | PARTIAL | **PARTIAL** | Every named package exists, but 11 (assignments, communications, contracts, customers, fuel, inspections, maintenance, reports, routes, tracking, wallets) are `doc.go` boundary markers that point to the real implementation elsewhere (4894bc8). `reports` points to Section 24 (exports/dashboard); no report implementation in Sections 5-20. | `4894bc8` |  |
| FLEET-5d | `migrations/`, `docs/`, `.env.example`, `go.mod/go.sum` | IMPLEMENTED | **CLOSED - VERIFIED** | Paired up/down migrations through 000097; replay and rollback against PostgreSQL. | `41256b9` | `integration/integration_test.go:TestMigrationsReplayAndRollback` |
| FLEET-5e | Each feature uses models/repositories/usecases/http (+clients) | PARTIAL | **PARTIAL** | 13 features are still flat packages with SQL in the service file: assets, compliance, dispatch, engagement, finance, financeops, fleetpartner, integrations, managedmembers, proof, specialized, warehouse, workforce. |  |  |
| FLEET-5f | No feature imports another feature's HTTP package | IMPLEMENTED | **PARTIAL** | Architecture test passes, but it only sees `features/<x>/http`. `features/dashboard/usecases/service.go:12` imports the flat `features/dispatch` package, which contains the dispatch HTTP handler (`dispatch/http.go`). Not fixed. | `41256b9` | `architecture_test.go:TestFeaturesDoNotImportOtherFeatureHTTPOrRepositories` |
| FLEET-5g | shared/go only for generic code | IMPLEMENTED | **CLOSED - VERIFIED** | Fleet imports only generic shared packages; static test. | `41256b9` | `architecture_test.go:TestNoOtherServiceInternalsImported` |
| FLEET-5.1a | Handlers depend on use-case interfaces | PARTIAL | **PARTIAL** | Handler constructors take concrete `*usecases.Service` types (e.g. `organizations/http/handlers.go:59`); the only interface parameter is the session revoker. |  |  |
| FLEET-5.1b | Use cases depend on repository/client interfaces | PARTIAL | **PARTIAL** | Flat features embed SQL in the service; layered features mostly use concrete repositories. |  |  |
| FLEET-5.1c | Adapters wired at composition root | IMPLEMENTED | **CLOSED - VERIFIED** | `app/wiring.go` BuildComponents/Registrars; adapters in `app/*_adapters.go`. | `41256b9` | `app/app_test.go:TestOrganizationAPIUsesSeparateAuthAndCannotReachManagementRoutes` |
| FLEET-5.1d | Domain models free of DB/HTTP/SDK | IMPLEMENTED | **CLOSED - VERIFIED** | Static test scans `features/*/models`; the 13 flat features are not scanned by it (hand-checked by the 2026-11-13 audit: no pgx/gin/net/http in their domain files). | `41256b9` | `architecture_test.go:TestDomainModelsAreInfrastructureFree` |
| FLEET-5.1e | Cross-feature collaboration through interfaces/events | IMPLEMENTED | **PARTIAL** | Repository bypasses fixed: organization provider handlers now call provider use cases (0842a03) and wallet transactions go through the use case (1eaa4be). Still concrete: `dashboard/usecases` uses `*dispatch.Service`; organizations handler takes concrete provider and staff services. No test. | `0842a03` |  |

Section 5: 12 rows - 6 CLOSED - VERIFIED, 6 PARTIAL.


## Section 6 - API conventions

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-6a | JSON; multipart only for upload handoff | IMPLEMENTED | **CLOSED - VERIFIED** | `middleware/middleware.go` RequireJSON answers 415; only multipart handler is document upload. | `41256b9` | `integration/auth_http_test.go:TestUnsupportedMediaTypeOnAuthRoutes` |
| FLEET-6b | Bearer for users; service auth for internal | IMPLEMENTED | **CLOSED - VERIFIED** | CorporateBearer, InternalServiceAuth, nonce replay guard. | `41256b9` | `integration/auth_http_test.go:TestAuthenticatedRoutesRequireBearer` |
| FLEET-6c | UTC ISO-8601; local timezone stored separately | IMPLEMENTED | **CLOSED - VERIFIED** | Defect found and fixed: pgx scanned timestamptz in the host zone. Pool now scans in UTC and the auth clock defaults to UTC; test runs with the process zone at UTC+1 and checks organization/branch timezone fields stay separate. | `12d7de8` | `integration/rows_06_conventions_test.go:TestRow6c_UTCTimestampsAndSeparateLocalTimezone` |
| FLEET-6d | Money = amountMinor + ISO currency; no float64 arithmetic | PARTIAL | **PARTIAL** | Finance ingestion now decodes amounts exactly (a27579a, unit test `features/finance/minor_amount_test.go:TestMinorAmountIsExact`). Still float64: `fleetpartner/models.go:352` `int64(math.Max(0, float64(share)))`. | `a27579a` |  |
| FLEET-6e | Opaque prefixed IDs; no sequential IDs | IMPLEMENTED | **CLOSED - VERIFIED** | `platform/ids`. Pure generator, verified by a unit test (no database needed). Only sequential column is the internal audit `chain_sequence`. | `41256b9` | `platform/ids/ids_test.go:TestNoSequentialStructure` |
| FLEET-6f | Cursor pagination; page/limit only for compatibility | IMPLEMENTED | **PARTIAL** | Fixed: negative/oversized offset is now 422 and clamped in services (0331419); staff directory, provider list/history and managed members/assets page by cursor (f048280, 651b08e). Remaining: dispatch (plans, routes, tracking, geofences, alerts, rescue), workforce, assets and `/marketplace/trips` lists are offset-only with no cursor (`platform/queryallow/registry.go` `@offset`). | `83a8d9d` | `integration/api_conventions_test.go:TestOffsetListsServeValidPagesAfterValidation`; `integration/api_conventions_test.go:TestStaffDirectoryPagesWithCursor` |
| FLEET-6g | Allowlisted filters/sorts; reject unknown | PARTIAL | **CLOSED - VERIFIED** | `platform/queryallow` declares the parameters of every tenant GET collection route; `middleware.QueryAllowlist` answers anything else with 422 (a177684). The router test enumerates every mounted tenant GET route and asserts `?bogusFilter=1` is 422 on each collection route. | `83a8d9d` | `integration/api_conventions_test.go:TestTenantListRoutesRejectUnknownQueryAndBadPaging` |
| FLEET-6h | Success `{"data","meta"}` | IMPLEMENTED | **PARTIAL** | Tenant-route bypasses (eligibility, finance operator, financeops/engagement reads, specialized, proof, safety, documents) now use the standard envelope (6bffac1). Internal service routes still return bespoke bodies by design (`providers/http/internal.go`, `backgroundops/http/internal.go`, `documents/http/internal.go`, finance payment events). | `6bffac1` | `integration/api_conventions_test.go:TestFormerEnvelopeBypassReturnsStandardMeta` |
| FLEET-6i | Error `{"error":{code,message,details,requestId}}` | IMPLEMENTED | **CLOSED - VERIFIED** | `platform/respond` Error/ErrorBody. | `41256b9` | `integration/auth_http_test.go:TestRequestAndCorrelationIDsPropagate` |
| FLEET-6j | Idempotency-Key required on financial, provisioning, import and assignment actions | IMPLEMENTED | **CLOSED - VERIFIED** | Both audit defects fixed: `POST /providers` now refuses a missing key and replays through the durable store (33e3d65); managed-member create/link routes honour the key they declare (fa15db3). | `33e3d65` | `integration/provider_create_idempotency_test.go:TestProviderProvisioningRequiresIdempotencyKey`; `integration/managedmembers_http_test.go:TestManagedMemberCreateRequiresAndHonoursIdempotencyKey` |
| FLEET-6k | Version + If-Match/expectedVersion | IMPLEMENTED | **PARTIAL** | Provider suspend/update/reactivate/remove now refuse a missing If-Match (bb2d944); managed members accept every version spelling (fa15db3). Still blind writes when If-Match is absent (falls back to the current version): organization structure, role and staff edit handlers (`organizations/http`, `expectedVersionFrom`, 19 call sites), and marketplace assignment cancel (`marketplace/usecases/assign.go:596`). | `bb2d944` | `integration/rows_10_providers_test.go:TestRow10_5_SuspendProviderOverHTTP`; `integration/iam_provider_test.go:TestStaffOptimisticConcurrency` |
| FLEET-6l | Audit: actor, org, previous/new summary, request ID | IMPLEMENTED | **CLOSED - VERIFIED** | Audit defect fixed: managed members now write audit and outbox in the same transaction (fa15db3). | `fa15db3` | `integration/managedmembers_http_test.go:TestManagedMemberLinkAndAssetWritesAreAuditedAndIdempotent`; `integration/integration_more_test.go:TestAuditRecorderTenantFilteringAndRedaction` |
| FLEET-6.1 | Standard Money object | PARTIAL | **PARTIAL** | `platform/money.Money` exists but DTOs flatten money into `xxxMinor` + `currency` fields; naming outlier `sensitiveExportThresholdKobo`. |  |  |
| FLEET-6.2 | Pricing deferral | IMPLEMENTED | **CLOSED - VERIFIED** | No pricing formulas; manual quotation lines. | `41256b9` | `architecture_test.go:TestNoPricingFormulas`; `integration/crm_test.go:TestCRMQuotationArithmeticIsManualAndExact` |

Section 6: 14 rows - 9 CLOSED - VERIFIED, 5 PARTIAL.


## Section 7 - Multi-tenancy

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-7a | Tenant context from authenticated membership | IMPLEMENTED | **CLOSED - VERIFIED** | `middleware/auth.go` ResolveTenant. | `41256b9` | `integration/iam_provider_test.go:TestStaffCrossTenantIsolation` |
| FLEET-7b.a | Repository filtering by organization_id | IMPLEMENTED | **CLOSED - VERIFIED** | Every tenant query binds organization_id; static scan plus cross-org IDOR tests. | `41256b9` | `architecture_test.go:TestSQLAlwaysTenantScoped` |
| FLEET-7b.b | Repository filtering by permitted branch IDs | IMPLEMENTED | **CLOSED - VERIFIED** | Audit defect (branch-scope escape via `?branchId`) fixed: `accesscontrol.RequireScoped` narrows the tenant to the branches where the permission is granted (76353c2, e49394d, 805ba90); asset lists filter in SQL (9564bd6). NULL-unsafe `cardinality()=0` guards in 11 list queries (financeops expenses/payroll/invoices, engagement, providers, marketplace, assets, safety references, branches) fixed and statically forbidden (d4e3a79). | `76353c2, d4e3a79` | `integration/branch_scope_bypass_test.go:TestBranchScopedRoleCannotBeWidenedByQueryParameter`; `integration/branch_scope_null_test.go:TestOrganizationWideListIncludesBranchRows` |
| FLEET-7c | Composite uniqueness with organization_id | IMPLEMENTED | **CLOSED - VERIFIED** | Org-scoped unique indexes. | `41256b9` | `integration/integration_test.go:TestRegistrationUniquenessPerCountry` |
| FLEET-7d | Ownership checked before a 404/403-safe response | IMPLEMENTED | **CLOSED - VERIFIED** | Exceptions noted by the audit fixed: provider detail/history resolve through VisibleMembership, unknown ids 404, real failures are errors (f048280, 0842a03). | `f048280` | `integration/provider_list_scope_test.go:TestProviderListPaginatesAndDetailRespectsBranchScope`; `integration/idor_assets_maintenance_test.go:TestAssets_ObjectAuthorization` |
| FLEET-7e | Media metadata has org ownership and purpose | IMPLEMENTED | **CLOSED - VERIFIED** | Onboarding documents use COMPANY_DOCUMENT purpose with ORGANIZATION owner scope. | `2599593` | `integration/onboarding_document_preview_test.go:TestOnboardingDocumentPreviewContract` |
| FLEET-7f | Every event has organizationId + correlationId | IMPLEMENTED | **CLOSED - VERIFIED** | `events/envelope.go` NewEnvelope (organizationId omitted only for platform-scoped events). | `41256b9` | `integration/integration_test.go:TestOutboxTransactionAtomicity` |
| FLEET-7g | Job payload organizationId revalidated on execution | IMPLEMENTED | **CLOSED - VERIFIED** | `jobs/jobs.go` ErrTenantMismatch. | `41256b9` | `integration/integration_more_test.go:TestJobTenantMismatchDeadLettersSafely` |
| FLEET-7h.a | Exports tenant-scoped | IMPLEMENTED | **CLOSED - VERIFIED** | `exports/usecases/service.go`. | `41256b9` | `integration/exports_test.go:TestExports_TenantIsolation` |
| FLEET-7h.b | Exports time-limited | IMPLEMENTED | **CLOSED - VERIFIED** | expiresAt set from RetentionTTL (default 7 days). | `41256b9` | `integration/exports_test.go:TestExports_ExpiryBlocksDownloadOfStaleReadyJobs` |
| FLEET-7h.c | Exports encrypted at rest | IMPLEMENTED | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | Fleet writes the file to media-file-service (`exports/usecases/worker.go:101`) and does no encryption itself; at-rest encryption is media-file-service's storage backend. No Fleet or Media test asserts it. |  |  |
| FLEET-7.e1 | organizations (status, legal identity, capabilities, country, tz, currency, version) | IMPLEMENTED | **CLOSED - VERIFIED** | Migration 000002. | `41256b9` | `integration/integration_test.go:TestOrganizationFoundationAndTenantScopedLookup` |
| FLEET-7.e2 | branches (type, address, geofence, hours, status) | PARTIAL | **CLOSED - VERIFIED** | Branch create/edit/archive API; last active branch and branches with departments protected. | `c39e5bd` | `integration/structure_iam_admin_test.go:TestOrganizationStructureAndAccessAdministration` |
| FLEET-7.e3 | departments (branch, parent, cost centre) | PARTIAL | **CLOSED - VERIFIED** | Department create/edit/archive; same-org references; cycle refusal; cost centre validated against the register (74fe54c). | `c39e5bd` | `integration/structure_iam_admin_test.go:TestOrganizationStructureAndAccessAdministration` |
| FLEET-7.e4 | organization_settings (assignment timeout, approval thresholds, provider visibility, retention) | PARTIAL | **CLOSED - VERIFIED** | GET/PATCH /organization-settings; operating mode/retention/flags stay on their governed routes. | `c39e5bd` | `integration/structure_iam_admin_test.go:TestOrganizationStructureAndAccessAdministration` |
| FLEET-7.e5 | organization_status_history (immutable) | IMPLEMENTED | **CLOSED - VERIFIED** | Append-only trigger in migration 000002. | `41256b9` | `integration/integration_test.go:TestStatusHistoryAppendOnly` |

Section 7: 16 rows - 15 CLOSED - VERIFIED, 1 IMPLEMENTED - NOT INDEPENDENTLY TESTED.


## Section 8 - Authentication, onboarding and lifecycle

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-8.1 | Start owner challenge (spec `/auth/phone/start`) | IMPLEMENTED | **CLOSED - VERIFIED** | Implemented as `POST /auth/start`, `/auth/sign-up/start`, `/auth/sign-in/start` (email OTP; path differs from spec). | `41256b9` | `integration/auth_http_test.go:TestAuthStartHTTPNeverReturnsOTP` |
| FLEET-8.2 | POST /auth/verify | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/auth_http_test.go:TestAuthVerifyHTTPIssuesSessionAndEnvelope` |
| FLEET-8.3 | POST /auth/login (password login for org users) | MISSING | **MISSING** | Not built by decision: Fleet auth is passwordless email OTP (auth v2 decision supersedes the spec text). No SMS. Kept as MISSING because the status vocabulary has no 'superseded' value. |  |  |
| FLEET-8.4a | POST /auth/refresh | IMPLEMENTED | **CLOSED - VERIFIED** | Refresh rotation and replay detection. | `41256b9` | `integration/browser_refresh_test.go:TestBrowserRefreshCookieRotationAndReplay` |
| FLEET-8.4b | POST /auth/logout | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/auth_http_test.go:TestLogoutHTTPIsIdempotent` |
| FLEET-8.5 | POST /organizations (draft) | IMPLEMENTED | **CLOSED - VERIFIED** | Creates draft, Head Office branch and owner membership. | `54c7655` | `integration/onboarding_v2_test.go:TestOnboardingV2ConcurrentCompanyCreation` |
| FLEET-8.6 | PATCH /organizations/{id} (legal/operating profile) | IMPLEMENTED | **CLOSED - VERIFIED** | Implemented as `PUT /organizations/:id` and `PUT .../profile` (method differs from spec). | `54c7655` | `integration/onboarding_v2_test.go:TestOnboardingV2PersistedJourneyAndBoundaries` |
| FLEET-8.7 | POST /organizations/{id}/documents via media-file-service | IMPLEMENTED | **CLOSED - VERIFIED** |  | `2599593` | `integration/onboarding_document_preview_test.go:TestOnboardingDocumentPreviewContract` |
| FLEET-8.8a | POST /organizations/{id}/submit -> verification-compliance-service | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/organization_lifecycle_gate_test.go:TestPendingVerificationAccessAndLifecycleGate` |
| FLEET-8.8b | Verification decision ingress | IMPLEMENTED | **CLOSED - VERIFIED** | Internal route mounted when the verification caller allowlist is set (default `verification-compliance-service`); paired on staging. | `e234978` | `integration/company_activation_test.go:TestCompanyActivationFromVerificationDecision`; `features/organizations/http/internal_verification_test.go:TestVerificationEventIngressRefusals` |
| FLEET-8.9 | GET /organizations/{id}/onboarding-status | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/organization_lifecycle_gate_test.go:TestPendingVerificationAccessAndLifecycleGate` |
| FLEET-8.10 | Create headquarters/default branch (stage 5) | MISSING | **CLOSED - VERIFIED** | Head Office branch created with every company (migration 000087); further branches via POST /branches. | `b547b37` | `integration/default_branch_test.go:TestNewCompanyHasActiveHeadOffice` |
| FLEET-8.11 | Configure timezone, currency, territories, initial policies | PARTIAL | **CLOSED - VERIFIED** | Timezone/currency on organization; settings API; territories via branches; initial approval policies on the pre-activation allowlist (74fe54c). | `c39e5bd` | `integration/structure_iam_admin_test.go:TestOrganizationStructureAndAccessAdministration` |
| FLEET-8.12 | Activate only after checks + wallet provisioning | PARTIAL | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | Verification-driven activation is tested, but that test waives the wallet gate (`WalletGateWaived: true`). The `organization_wallet_active` gate (`organizations/usecases/submission_activation.go:364`) and `RequestWalletProvisioning` have no test. Staging runs with `FLEET_ACTIVATION_WALLET_GATE_WAIVED=true` (`deploy/platform/staging-pairings.txt`). | `e234978` | `integration/company_activation_test.go:TestCompanyActivationFromVerificationDecision` (wallet gate waived) |
| FLEET-8.13 | Lifecycle DRAFT->...->ACTIVE<->SUSPENDED->CLOSED | EXTERNAL | **PARTIAL** | State machine in `organizations/usecases/lifecycle.go`. DRAFT->ACTIVE and ->CLOSED are driven and tested (activation, closure). ACTIVE->SUSPENDED arrives only as a verification-compliance decision (`verification_outcome.go:140`); nothing in Fleet or its ingress returns a SUSPENDED organization to ACTIVE. The state machine itself has no direct test. |  |  |
| FLEET-8.14 | Close request + controlled closure | IMPLEMENTED | **CLOSED - VERIFIED** | Closure now also requires step-up (d111e76). | `41256b9` | `integration/organization_closure_http_test.go:TestOrganizationClosure_FullLifecycleReachesCompleted`; `integration/step_up_http_test.go:TestStepUp_OrganizationClosureAndProviderRemoval` |
| FLEET-8.15 | Branches / locations / departments / teams / cost-centres APIs (37.3, 59.2) | MISSING | **CLOSED - VERIFIED** | Branches, departments, settings (c39e5bd); cost centres, teams and operating locations with If-Match, tenant-scoped references, audit and outbox (74fe54c). | `74fe54c` | `integration/structure_units_test.go:TestOrganizationStructureUnits` |
| FLEET-8.16a | GET /sessions | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/auth_http_test.go:TestSessionsListNeverLeaksRefreshHashes` |
| FLEET-8.16b | DELETE /sessions/:id | IMPLEMENTED | **CLOSED - VERIFIED** | Now proven over HTTP (own, foreign, unknown, repeat). | `2531084` | `integration/rows_08_09_auth_staff_test.go:TestRow8_16b_RevokeSessionOverHTTP` |
| FLEET-8.16c | Step-up start/verify | IMPLEMENTED | **CLOSED - VERIFIED** | Now proven over HTTP: grant bound to session and action, single use. | `2531084` | `integration/rows_08_09_auth_staff_test.go:TestRow8_16c_StepUpOverHTTP` |

Section 8: 20 rows - 17 CLOSED - VERIFIED, 1 IMPLEMENTED - NOT INDEPENDENTLY TESTED, 1 PARTIAL, 1 MISSING.


## Section 9 - Users, roles, permissions and approvals

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-9.1 | GET /staff | IMPLEMENTED | **CLOSED - VERIFIED** | Audit defect (200-row cap, no cursor) fixed: keyset cursor with meta.nextCursor (651b08e). | `651b08e` | `integration/api_conventions_test.go:TestStaffDirectoryPagesWithCursor`; `integration/staff_directory_test.go:TestStaffDirectoryAndInvitations` |
| FLEET-9.2a | POST /staff/invitations | IMPLEMENTED | **CLOSED - VERIFIED** | Token never returned; audit and event. | `d83e81e` | `integration/staff_directory_test.go:TestStaffDirectoryAndInvitations` |
| FLEET-9.2b | Invitation single-use hashed, expiring | IMPLEMENTED | **CLOSED - VERIFIED** | Acceptance consumes the hash; expiry sweep now tested. | `2531084` | `integration/rows_08_09_auth_staff_test.go:TestRow9_2b_StaffInvitationExpirySweep`; `integration/auth_invitation_v2_test.go:TestAuthV2StaffInvitationAndMultipleOrganizations` |
| FLEET-9.3 | Invitation resend / cancel / list | MISSING | **CLOSED - VERIFIED** |  | `d83e81e` | `integration/staff_directory_test.go:TestStaffDirectoryAndInvitations` |
| FLEET-9.4 | GET/PATCH /staff/{id} | MISSING | **CLOSED - VERIFIED** | PATCH accepts a missing If-Match (see FLEET-6k). | `c39e5bd` | `integration/structure_iam_admin_test.go:TestOrganizationStructureAndAccessAdministration` |
| FLEET-9.5 | POST /staff/{id}/suspend, /reactivate | IMPLEMENTED | **CLOSED - VERIFIED** | Now tested over HTTP; defect fixed (repeat suspend/reactivate re-emitted events; staff state machine now enforced, 409). Requires step-up (3feab8d). | `2531084` | `integration/rows_08_09_auth_staff_test.go:TestRow9_5_SuspendAndReactivateStaff` |
| FLEET-9.6a | GET /roles | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/iam_provider_test.go:TestSystemRolesAreSeededAndImmutable` |
| FLEET-9.6b | GET /permissions | IMPLEMENTED | **CLOSED - VERIFIED** |  | `d83e81e` | `integration/staff_directory_test.go:TestStaffDirectoryAndInvitations` |
| FLEET-9.7 | POST /roles, PATCH /roles/{id} | MISSING | **CLOSED - VERIFIED** | Catalogue-validated, escalation guard, system roles immutable. PATCH accepts a missing If-Match (FLEET-6k). | `c39e5bd` | `integration/structure_iam_admin_test.go:TestOrganizationStructureAndAccessAdministration` |
| FLEET-9.8 | Role bindings / PUT /users/{id}/roles | MISSING | **CLOSED - VERIFIED** | PUT /staff/:id/roles; now requires step-up (d111e76). | `c39e5bd` | `integration/structure_iam_admin_test.go:TestOrganizationStructureAndAccessAdministration`; `integration/step_up_http_test.go:TestStepUp_RoleAndPermissionChangesRequireAFreshGrant` |
| FLEET-9.9 | Branch/department-scoped role bindings | PARTIAL | **CLOSED - VERIFIED** | Scoped bindings (c39e5bd); a branch-scoped binding can no longer be widened with `?branchId` (76353c2). | `76353c2` | `integration/branch_scope_bypass_test.go:TestBranchScopedRoleCannotBeWidenedByQueryParameter` |
| FLEET-9.10a | GET /approvals/requests (pending + history) | IMPLEMENTED | **CLOSED - VERIFIED** | Web gap (not this API row): `web/fleet/src/app/(dashboard)/settings/approvals/page.tsx` and `[requestId]/page.tsx` still render MOCK data. | `41256b9` | `integration/approvals_test.go:TestCrossTenantApprovalIsDenied` |
| FLEET-9.10b | Approval request detail | IMPLEMENTED | **CLOSED - VERIFIED** | Branch-permitted check. | `41256b9` | `integration/approvals_test.go:TestSelfApprovalIsRefused` |
| FLEET-9.10c | Approval decisions read | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/eligibility_http_test.go:TestApprovalRoutesEndToEnd` |
| FLEET-9.10d | Approval history | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/approvals_test.go:TestApprovalHistoryAndOutboxAreWrittenAtomically` |
| FLEET-9.11 | Approve/reject (different approver, quorum) | IMPLEMENTED | **CLOSED - VERIFIED** | Decisions now require step-up and record the verified grant (728590f); an APPROVED request no longer accepts decisions (1663d9b). | `728590f` | `integration/approvals_test.go:TestInsufficientApprovalsCannotExecute`; `integration/step_up_approvals_test.go:TestStepUp_ApprovalDecisionsRequireAFreshGrant` |
| FLEET-9.12 | Approval policies CRUD (/approval-policies) | MISSING | **CLOSED - VERIFIED** | Versioned policies with branch scope, minor-unit thresholds, 1-5 sequential steps; platform floor; frozen onto in-flight requests. Web page connected (40ce7b4). | `1663d9b` | `integration/approval_policies_delegations_test.go:TestApprovalPolicyLifecycleIsVersionedAndAudited`; `integration/approval_policies_delegations_test.go:TestApprovalPolicyAndDelegationRoutes` |
| FLEET-9.13 | Approval delegations (/approval-delegations) | MISSING | **CLOSED - VERIFIED** | Time-bounded, scoped, never exceeds the delegator's live authority, revocable. | `1663d9b` | `integration/approval_policies_delegations_test.go:TestDelegateDecidesOnTheDelegatorsAuthority`; `integration/approval_policies_delegations_test.go:TestDelegationNeverExceedsTheDelegatorsLiveAuthority` |
| FLEET-9.14 | Step-up for high-risk actions | IMPLEMENTED | **CLOSED - VERIFIED** | Audit defect fixed: central route registry plus `middleware.RequireStepUp` on the tenant group covers role/permission changes, approval decisions, wallet allocations/withdrawals, payroll finalization, API secret changes, sensitive exports and closure, regardless of the MFA setting; fails closed without a verifier (d111e76, 728590f). Web client completes the challenge (513bb60). Adjacent defect fixed: API client create with no IP allowlist inserted NULL (bb2d944, `section55_integrations_test.go:TestSection55APIClientLifecycleTenantIsolationAndSecretStorage`). | `d111e76` | `integration/step_up_http_test.go:TestStepUp_FinancialActionsAndSensitiveExports`; `integration/step_up_http_test.go:TestStepUp_LegalHoldWithoutVerifierFailsClosed` |
| FLEET-9.15a | Default-deny | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/iam_provider_test.go:TestAuthorizationIsDenyByDefaultAndScoped` |
| FLEET-9.15b | Role-version invalidation of cached decisions | IMPLEMENTED | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | Audit defect fixed in code: the tenant middleware now publishes the membership role version (`middleware/auth.go:226-230`, `staff/usecases/resolver.go:60,88`), so a role bump changes the cache key (6c2be84). Evidence is a middleware unit test with a fake resolver; no PostgreSQL/Redis test shows a revoked binding stops authorizing. | `6c2be84` | `middleware/role_version_test.go:TestResolveTenantPublishesRoleVersion` (unit) |

Section 9: 21 rows - 20 CLOSED - VERIFIED, 1 IMPLEMENTED - NOT INDEPENDENTLY TESTED.


## Section 10 - Organization-managed provider accounts

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-10.1 | GET /providers | IMPLEMENTED | **CLOSED - VERIFIED** | Audit defect (25-row cap, repository bypass) fixed: keyset cursor, limit 1-100, through the use case (f048280, 0842a03). | `f048280` | `integration/provider_list_scope_test.go:TestProviderListPaginatesAndDetailRespectsBranchScope` |
| FLEET-10.2 | POST /providers (resolve-or-provision in owning service) | PARTIAL | **CLOSED - VERIFIED** | Dispatch and Taxi managed-provider clients (99303f8) alongside Hauling; association decision read back from the owning service (01fab88); Idempotency-Key required (33e3d65). Test drives the real clients against an HMAC-verifying stand-in of the shared orgprovider contract. Dispatch is not paired on staging (migration-070 hold). | `01fab88` | `integration/managed_provider_services_test.go:TestManagedProviderCreationForDispatchAndTaxiReachesActiveOnlyAfterProviderAccepts` |
| FLEET-10.3a | GET /providers/{id} | IMPLEMENTED | **CLOSED - VERIFIED** | Audit defect fixed: branch scope applied, unknown id 404, DB errors no longer masked (f048280). | `f048280` | `integration/provider_list_scope_test.go:TestProviderListPaginatesAndDetailRespectsBranchScope` |
| FLEET-10.3b | GET /providers/{id}/history | IMPLEMENTED | **CLOSED - VERIFIED** | Fixed: existence and branch check, cursor-paginated (651b08e). | `651b08e` | `integration/api_conventions_test.go:TestProviderHistoryPagesWithCursor` |
| FLEET-10.3c | GET /providers/{id}/provisioning-status | IMPLEMENTED | **CLOSED - VERIFIED** |  | `1408ac1` | `integration/rows_10_providers_test.go:TestRow10_3c_ProviderProvisioningStatus` |
| FLEET-10.4 | PATCH /providers/{id} | MISSING | **CLOSED - VERIFIED** | Now requires If-Match (bb2d944). | `1b3d869` | `integration/provider_administration_test.go:TestManagedProviderAdministration` |
| FLEET-10.5 | POST /providers/{id}/suspend | IMPLEMENTED | **CLOSED - VERIFIED** | Proven over HTTP; missing If-Match now 400 instead of a blind write (bb2d944). | `abfe350, bb2d944` | `integration/rows_10_providers_test.go:TestRow10_5_SuspendProviderOverHTTP` |
| FLEET-10.6 | POST /providers/{id}/reactivate | MISSING | **CLOSED - VERIFIED** | If-Match required (bb2d944). | `1b3d869` | `integration/provider_administration_test.go:TestManagedProviderAdministration` |
| FLEET-10.7 | POST /providers/{id}/remove | MISSING | **CLOSED - VERIFIED** | Requires step-up (d111e76) and If-Match (bb2d944). | `1b3d869` | `integration/provider_administration_test.go:TestManagedProviderAdministration`; `integration/step_up_http_test.go:TestStepUp_OrganizationClosureAndProviderRemoval` |
| FLEET-10.8 | POST /providers/{id}/resend-login | MISSING | **CLOSED - VERIFIED** | Rotates the invitation token or sends a first-login notice; rate limited, idempotent, audited; email only. | `6576ebb` | `integration/provider_ws5_test.go:TestProviderResendLogin_InvitationRotationRateLimitIdempotencyAndAudit` |
| FLEET-10.9 | PUT /providers/{id}/capabilities | MISSING | **CLOSED - VERIFIED** |  | `1b3d869` | `integration/provider_administration_test.go:TestManagedProviderAdministration` |
| FLEET-10.10 | PUT /providers/{id}/policies | MISSING | **CLOSED - VERIFIED** | Decline right cannot be disabled. | `1b3d869` | `integration/provider_administration_test.go:TestManagedProviderAdministration` |
| FLEET-10.11 | GET /providers/{id}/availability | MISSING | **CLOSED - VERIFIED** | Derived at request time from membership, leave, shifts, qualifications, live work and tracking; reasons returned; nothing stored. | `6576ebb` | `integration/provider_ws5_test.go:TestProviderAvailabilityIsDerivedAndExplainable` |
| FLEET-10.12 | GET /providers/{id}/declines, /branches, /vehicles, /service-roles | MISSING | **CLOSED - VERIFIED** | Declines (1b3d869); branches, vehicles and service roles (6576ebb). | `6576ebb` | `integration/provider_ws5_test.go:TestProviderBranchesVehiclesAndServiceRoles` |
| FLEET-10.13 | Mandatory sub-wallet on activation | IMPLEMENTED | **CLOSED - VERIFIED** | Now tested: activation refused while Wallet is down, the owning service suspends the provider, or the organization is not ACTIVE. | `1408ac1` | `integration/rows_10_providers_test.go:TestRow10_13_SubWalletIsMandatoryForActivation` |
| FLEET-10.14 | Provisioning saga recovery | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/provider_provisioning_reconciliation_test.go:TestProviderProvisioningUnknownOutcomeReconcilesByContactWithoutDuplicateIdentity` |
| FLEET-10.15a | Managed members create/list/get | IMPLEMENTED | **CLOSED - VERIFIED** | Audit defects fixed: audit + outbox per write, real Idempotency-Key (fa15db3); lists paginate (651b08e). Flag off on staging. | `fa15db3` | `integration/managedmembers_http_test.go:TestManagedMemberCreateRequiresAndHonoursIdempotencyKey`; `features/managedmembers/managedmembers_test.go:TestOneMemberHoldsThreeAuthorities` |
| FLEET-10.15b | Managed member consent | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `features/managedmembers/managedmembers_test.go:TestStaleVersionCannotOverwriteANewerDecision` |
| FLEET-10.15c | Managed member service links | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `features/managedmembers/managedmembers_test.go:TestFleetCannotMintAProviderIdentity` |
| FLEET-10.15d | Managed member suspend/reinstate | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `features/managedmembers/managedmembers_test.go:TestSuspendingOneAuthorityLeavesTheOthersAlone` |
| FLEET-10.15e | Managed assets | IMPLEMENTED | **CLOSED - VERIFIED** | Unbounded list fixed (keyset pagination, 651b08e). | `651b08e` | `features/managedmembers/pagination_test.go:TestMemberAndAssetListsPageWithoutTruncation` |
| FLEET-10.16a | Membership summary for execution services | IMPLEMENTED | **CLOSED - VERIFIED** | Internal route; bespoke response body kept by contract (FLEET-6h). | `41256b9` | `integration/provider_membership_summary_test.go:TestGetMembershipSummaryForProvider_OrganizationManagedReturnsFullSummary` |
| FLEET-10.16b | Corporate context | IMPLEMENTED | **CLOSED - VERIFIED** | Organization-managed path now tested; defect fixed (a SUSPENDED organization no longer reports its providers eligible). | `1408ac1` | `integration/rows_10_providers_test.go:TestRow10_16b_CorporateContextForManagedProvider` |

Section 10: 23 rows - 23 CLOSED - VERIFIED.


## Section 11 - Request acceptance, assignment, provider accept/decline

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-11.1a | Operating mode read/update | IMPLEMENTED | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | `fleetpartner/http.go:76,94` -> `fleetpartner/service.go` Read/UpdateOperatingMode. Only model validation is unit-tested (`fleetpartner/models_test.go`). |  |  |
| FLEET-11.1b | Operating-mode enforcement on acceptance | IMPLEMENTED | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | `marketplace/usecases/dispatcher.go:39` AcceptForOrganization(modes). Opportunity fan-out by mode is tested (`marketplace_sections_11_12_test.go:TestS1112_OfferedEventCreatesOpportunitiesForEligibleOrganizations`); enforcement on acceptance is not. |  |  |
| FLEET-11.2 | Dispatcher accepts request for org | IMPLEMENTED | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | `POST /marketplace/offers/:offerId/accept` -> `dispatcher.go:39`. Only repository invariants tested; cross-service e2e `TestAcceptanceToOneCanonicalTrip` covers the delegated path and skips without `DISPATCH_TEST_DATABASE_URL`. |  |  |
| FLEET-11.3 | Dispatcher declines opportunity | IMPLEMENTED | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | `dispatcher.go:276` DeclineForOrganization. |  |  |
| FLEET-11.4 | Eligible candidates | IMPLEMENTED | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | `marketplace/usecases/corporate.go:251` ListCorporateCandidates (eligibility evaluation tested separately in `eligibility_decisions_test.go`). |  |  |
| FLEET-11.5 | POST /requests/{id}/assign | IMPLEMENTED | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | `marketplace/usecases/assign.go:39` Assign. Only the cross-branch refusal is exercised (`TestS1112_MarketplaceBranchScopeCannotBeWidenedByQueryParameter`); a successful assign is not. |  |  |
| FLEET-11.6 | GET /assignments, /assignments/{id} | IMPLEMENTED | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | Detail and history are exercised through the router (own branch 200, other branch 404); the list `GET /marketplace/assignments` is not. | `805ba90` | `integration/marketplace_sections_11_12_test.go:TestS1112_MarketplaceBranchScopeCannotBeWidenedByQueryParameter` (detail/history only) |
| FLEET-11.7a | POST /assignments/{id}/cancel | IMPLEMENTED | **IMPLEMENTED - NOT INDEPENDENTLY TESTED** | `assign.go:545` Cancel. Only the EXECUTION_START_PENDING refusal is tested; a successful cancel is not. Missing If-Match falls back to the current version (`assign.go:596`). Work already started must be cancelled through the trip (see FLEET-12.2). | `41256b9` | `integration/provider_response_test.go:TestCancelRefusesExecutionStartPending` (refusal only) |
| FLEET-11.7b | POST /assignments/{id}/reassign | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/auto_reassignment_section27_test.go:TestSection27_ReassignmentEnabledFlagIsNowWired` |
| FLEET-11.8 | Provider accepts/declines assignment | IMPLEMENTED | **CLOSED - VERIFIED** | Internal route mounts only when `FLEET_MARKETPLACE_ALLOWED_CALLERS` is set; `deploy/platform/staging-pairings.txt` now sets it for Hauling, Taxi and Dispatch (unmounted under default config). | `41256b9` | `integration/marketplace_test.go:TestConcurrentProviderResponsesSingleWinner` |
| FLEET-11.9 | Provider accepts/declines delegated offer | IMPLEMENTED | **CLOSED - VERIFIED** | Same mounting condition as 11.8. | `41256b9` | `integration/offer_response_test.go:TestOfferAcceptanceCommitsTheWholeDelegatedConsequence` |
| FLEET-11.10 | Decline reason catalogue, mandatory reason, OTHER free text | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/marketplace_test.go:TestDeclineReasonCatalogueAndSafetyClassification`; `integration/offer_response_test.go:TestOfferDeclineValidatesReasonAndFlagsSafety` |
| FLEET-11.11a | Response deadline -> EXPIRED -> REASSIGNMENT_REQUIRED | IMPLEMENTED | **CLOSED - VERIFIED** | Expiry worker enabled on staging (staging-pairings.txt). | `41256b9` | `integration/assignment_expiry_section27_test.go:TestSection27_ExpireAssignments_ReleasesOverdueAndOpensReassignmentCase` |
| FLEET-11.11b | Auto-reassign | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/auto_reassignment_section27_test.go:TestSection27_AutoReassignStaleCase_TriesCandidatesAndSucceeds` |
| FLEET-11.11c | Dispatcher notification on expiry | IMPLEMENTED | **PARTIAL** | Marketplace expiry publishes `organization.assignment.expired.v1` (`marketplace/usecases/assign.go:269`) but nothing turns it into a dispatcher alert or notification (no Fleet rule, no notification-service consumer). Private route-stop expiry does raise an ASSIGNMENT_EXPIRED alert (`dispatch/workers.go:500`). |  |  |
| FLEET-11.12 | Race protection / idempotency / 409 | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/offer_response_test.go:TestConcurrentOfferAcceptanceProducesOneWinner`; `integration/marketplace_test.go:TestCommandIdempotencyAndConflict` |
| FLEET-11.13 | UNSAFE_VEHICLE/VEHICLE_DEFECT decline -> safety/inspection workflow | PARTIAL | **CLOSED - VERIFIED** | Safety-decline reconciler: inspection, work order, asset block; declines also create defects (3def868). | `1b3d869` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |

Section 11: 17 rows - 8 CLOSED - VERIFIED, 8 IMPLEMENTED - NOT INDEPENDENTLY TESTED, 1 PARTIAL.


## Section 12 - Marketplace and trip integration

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-12.1 | Organization projection of source opportunities | IMPLEMENTED | **CLOSED - VERIFIED** | Audit verdict NOT-MOUNTED; now `platform.request.offered.v1` opens one projection and one offer per eligible organization, with stand-down on reservation/independent acceptance (4bbf626). Producers: Hauling (d59f3d6), Taxi (69378a2), Dispatch. Ingestion mounted on staging via the pairing file. | `4bbf626` | `integration/marketplace_sections_11_12_test.go:TestS1112_OfferedEventCreatesOpportunitiesForEligibleOrganizations`; `integration/marketplace_sections_11_12_test.go:TestS1112_TaxiAndHaulingProducerEnvelopesCreateOpportunities` |
| FLEET-12.2 | Execution creation in source service after provider acceptance | PARTIAL | **PARTIAL** | Fleet speaks Dispatch's real dialect (ec04286; wire unit test `platformclients/dispatch_native_execution_test.go`). In progress: Dispatch has no organization trip-cancel route, so `DispatchNativeExecution.CancelExecution` refuses permanently (`platformclients/dispatch_native_execution.go:276-287`) and no Fleet path calls CancelExecution. Cross-service proof only in env-gated `TestAcceptanceToOneCanonicalTrip`. Dispatch not paired on staging. | `ec04286` |  |
| FLEET-12.3 | Trip lifecycle projection list/detail (`/trips`) | PARTIAL | **CLOSED - VERIFIED** | `GET /marketplace/trips` and `/marketplace/trips/:tripId`; trip events move assignments READY_FOR_EXECUTION -> IN_PROGRESS -> COMPLETED/CANCELLED (path differs from spec `/trips`). | `4bbf626` | `integration/marketplace_sections_11_12_test.go:TestS1112_CorporateTripRoutes`; `integration/marketplace_sections_11_12_test.go:TestS1112_TripLifecycleDrivesAssignmentToInProgressAndCompleted` |
| FLEET-12.4 | Projection gap detection / reconciliation | IMPLEMENTED | **CLOSED - VERIFIED** | Reconciliation worker enabled on staging. | `41256b9` | `integration/reconciliation_worker_test.go:TestReconciliationRepairsGappedProjectionFromSource`; `integration/trip_reconciliation_test.go:TestTripReconciliationRepairsGappedProjection` |
| FLEET-12.5 | Provider-facing data minimization | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/field_visibility_test.go:TestProviderFacingViews_NeverSerializeCommercialOrInternalFields` |
| FLEET-12.6 | Settlement projection display | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/finance_cross_service_test.go:TestFleetFinanceRoutesPreventCrossTenantAccess` |
| FLEET-12.7 | Completed-work ingestion from execution services | IMPLEMENTED | **CLOSED - VERIFIED** | Flag `FLEET_COMPLETED_WORK_PROJECTION_ENABLED` off on staging. | `41256b9` | `integration/sections8_9_scenario_matrix_test.go:TestSections89_ManagedCompletionIsRecordedFromBothSources` |
| FLEET-12.8 | Ledger/settlement execution | EXTERNAL | **CLOSED - VERIFIED** | Owned by payment-wallet-service. Fleet's managed-settlement wire body is certified against Wallet's production route and real payment bindings (dfc22ca, 6940fe3, 9668ae8; Wallet migration 000023). | `dfc22ca` | `payment-wallet-service/internal/integration/fleet_wire_managed_settlement_test.go:TestFleetWireBodyPostsThroughTheProductionRoute`; `...:TestFleetSettlesPaymentsExactlyAsExecutionServicesCreatedThem` |

Section 12: 8 rows - 7 CLOSED - VERIFIED, 1 PARTIAL.


## Section 13 - Company wallet and provider sub-wallets

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-13.1 | GET /wallet | IMPLEMENTED | **CLOSED - VERIFIED** | Production assembly against an HMAC-verifying Wallet stand-in. Wallet read enabled on staging. | `1eaa4be` | `integration/rows_13_wallet_test.go:TestRow13_1_OrganizationWalletProjection` |
| FLEET-13.2 | GET /wallet/transactions (+/:transactionId) | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: handler bypassed the use case and the read flag. | `1eaa4be` | `integration/rows_13_wallet_test.go:TestRow13_2_WalletTransactionsListAndDetail` |
| FLEET-13.3 | GET /providers/{id}/sub-wallet | IMPLEMENTED | **CLOSED - VERIFIED** |  | `1eaa4be` | `integration/rows_13_wallet_test.go:TestRow13_3_ProviderSubWalletProjection` |
| FLEET-13.4a | Create allocation | IMPLEMENTED | **CLOSED - VERIFIED** | Now also step-up protected. | `41256b9` | `integration/companywallet_test.go:TestAllocationIsIdempotentAndFingerprintChecked`; `integration/step_up_http_test.go:TestStepUp_FinancialActionsAndSensitiveExports` |
| FLEET-13.4b | List allocations | IMPLEMENTED | **CLOSED - VERIFIED** |  | `1eaa4be` | `integration/rows_13_wallet_test.go:TestRow13_4b_AllocationList` |
| FLEET-13.4c | Approve/reject allocation | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/companywallet_test.go:TestAllocationSelfApprovalIsRefused` |
| FLEET-13.4d | Replay allocation | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: replay ignored membership and branch scope. | `1eaa4be` | `integration/rows_13_wallet_test.go:TestRow13_4d_AllocationReplay` |
| FLEET-13.5 | Sub-wallet statement | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/companywallet_test.go:TestProviderStatementExcludesCommercialData` |
| FLEET-13.6a | Request withdrawal | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/companywallet_test.go:TestWithdrawalPolicyModesAreEnforced` |
| FLEET-13.6b | Approve/reject withdrawal | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: Wallet client had no route for PROVIDER_WITHDRAWAL; now posts to /provider-withdrawals (1eaa4be). | `41256b9` | `integration/companywallet_test.go:TestWithdrawalCannotBeDecidedTwice` |
| FLEET-13.6c | Withdrawal replay / list | IMPLEMENTED | **CLOSED - VERIFIED** |  | `1eaa4be` | `integration/rows_13_wallet_test.go:TestRow13_6c_WithdrawalListAndReplay` |
| FLEET-13.7 | Provider self-service (own sub-wallet, statement, withdrawal) | PARTIAL | **CLOSED - VERIFIED** | Previously never mounted; now internal provider-service routes under `/internal/fleet-partner/v1/providers/:providerId/sub-wallet*`, membership resolved from the calling service only. | `f2e14e6` | `integration/provider_ws5_test.go:TestProviderSelfServiceInternalRoutes` |
| FLEET-13.8 | Withdrawal policy GET/PUT /wallet/policy | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: ISO currency validated; If-Match required once a policy exists. | `1eaa4be` | `integration/rows_13_wallet_test.go:TestRow13_8_WithdrawalPolicyGetPut` |
| FLEET-13.9a | Projection refresh worker | IMPLEMENTED | **CLOSED - VERIFIED** | Enabled on staging. | `1eaa4be` | `integration/rows_13_wallet_test.go:TestRow13_9a_ProjectionRefreshWorker` |
| FLEET-13.9b | Reconciliation / read-back | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/companywallet_test.go:TestUnknownOutcomeIsResolvedByReadBackNotResubmission` |
| FLEET-13.10 | Ledger, balances, transfers | EXTERNAL | **CLOSED - VERIFIED** | Owned by payment-wallet-service; Fleet holds no balance. Wallet provider withdrawals read back by idempotency key (1b036af). | `41256b9` | `integration/iam_provider_test.go:TestProviderSubWalletProjectionHoldsNoBalance`; `payment-wallet-service/internal/integration:TestProviderWithdrawalCanBeReadBackByIdempotencyKey` |
| FLEET-13.11 | Managed settlement instruction to Wallet | IMPLEMENTED | **CLOSED - VERIFIED** | Audit defect fixed: instruction commits with a durable delivery job; POSTED / PENDING_WALLET / RECONCILIATION_REQUIRED (read back first) / FAILED_TERMINAL; recovery sweep (012b1cd). Instruction carries the execution service's payment reference (ee0caba, migration 000097). Posting is off on staging. | `012b1cd, ee0caba` | `integration/managed_settlement_delivery_test.go:TestManagedSettlementLostResponseIsReadBackNotResent`; `integration/managed_settlement_delivery_test.go:TestManagedSettlementCarriesDispatchsPaymentReferenceToWallet` |

Section 13: 17 rows - 17 CLOSED - VERIFIED.


## Section 14 - CRM, sales, contracts and SLAs

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-14.1a | List customers | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMDefaultsAreOffAndKillSwitchOverrides` |
| FLEET-14.1b | Customer create/get/patch/status/archive/history | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMCustomerLifecycleAndIsolation` |
| FLEET-14.1c | Customer risk status | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMCustomerRiskForcesCreditRestriction` |
| FLEET-14.2a | Contact create/get/patch/archive/make-primary | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMContactNormalizationPrimaryAndPrivacy` |
| FLEET-14.2b | List contacts | IMPLEMENTED | **CLOSED - VERIFIED** |  | `a20fb2c` | `integration/rows_14_test.go:TestRow14_2b_ContactList` |
| FLEET-14.3a | Location create/patch/archive | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMLocationValidationAndPrivacy` |
| FLEET-14.3b | Location list/get | IMPLEMENTED | **CLOSED - VERIFIED** |  | `a20fb2c` | `integration/rows_14_test.go:TestRow14_3b_LocationListGet` |
| FLEET-14.4a | Opportunity create + hold/resume/won/lost | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMOpportunityTransitionMatrix` |
| FLEET-14.4b | Opportunity list/get/patch/history | IMPLEMENTED | **CLOSED - VERIFIED** |  | `a20fb2c` | `integration/rows_14_test.go:TestRow14_4b_OpportunityListGetPatchHistory` |
| FLEET-14.5a | Create quotation (manual lines) | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMQuotationArithmeticIsManualAndExact` |
| FLEET-14.5b | Quotation list/get/patch | IMPLEMENTED | **CLOSED - VERIFIED** | Known nit, not fixed: a PATCH that replaces line items raises the version by several steps. | `a20fb2c` | `integration/rows_14_test.go:TestRow14_5b_QuotationListAndPatch` |
| FLEET-14.5c | Submit quotation | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMQuotationIssueAcceptanceAndConversionIntent` |
| FLEET-14.5d | Issue quotation | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMQuotationIssueAcceptanceAndConversionIntent` |
| FLEET-14.5e | Accept/reject quotation (staff) | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMQuotationAcceptRejectAndExpiryRaces` |
| FLEET-14.5f | Secure customer link view/accept/reject | IMPLEMENTED | **CLOSED - VERIFIED** | Reject by token now tested. | `a20fb2c` | `integration/rows_14_test.go:TestRow14_5f_QuotationRejectByToken`; `integration/crm_test.go:TestCRMQuotationIssueAcceptanceAndConversionIntent` |
| FLEET-14.5g | Quotation cancel/supersede | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: a quotation could supersede itself. | `a20fb2c` | `integration/rows_14_test.go:TestRow14_5g_QuotationCancelAndSupersede` |
| FLEET-14.5h | Quotation versions/history/documents | IMPLEMENTED | **CLOSED - VERIFIED** |  | `a20fb2c` | `integration/rows_14_test.go:TestRow14_5h_QuotationVersionsHistoryDocuments` |
| FLEET-14.6 | Quotation acceptance creates order draft | IMPLEMENTED | **CLOSED - VERIFIED** | Conversion is an explicit call, not automatic on acceptance. | `41256b9` | `integration/orders_test.go:TestOrders_ConvertConversionIntent_ConsumedExactlyOnce` |
| FLEET-14.7a | Contract create/submit/activate/suspend/resume/terminate/versions | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMContractActivationImmutabilityAndCreditHold` |
| FLEET-14.7b | Contract list/get/patch/renew/history/version detail | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: renewal created a new successor on every call; now one open renewal per contract, version enforced. | `a20fb2c` | `integration/rows_14_test.go:TestRow14_7b_ContractListGetPatchRenewHistoryVersion` |
| FLEET-14.7c | Contract documents | IMPLEMENTED | **CLOSED - VERIFIED** |  | `a20fb2c` | `integration/rows_14_test.go:TestRow14_7c_ContractDocuments` |
| FLEET-14.8a | SLA create + activate + observation ingestion | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMSLAObservationIdempotencyAndUnknownHandling` |
| FLEET-14.8b | SLA list/get/patch/suspend/terminate/history/observations | IMPLEMENTED | **CLOSED - VERIFIED** |  | `a20fb2c` | `integration/rows_14_test.go:TestRow14_8b_SLAListGetPatchSuspendTerminateHistoryObservations` |
| FLEET-14.9 | Customer credit profiles (hold/release) | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/crm_test.go:TestCRMContractActivationImmutabilityAndCreditHold` |
| FLEET-14.10 | Leads + conversion (`/leads`, `/leads/{id}/convert`) | MISSING | **CLOSED - VERIFIED** | Migration 000091; leads with assignment, status, exactly-once conversion into customer/opportunity. | `54689e2` | `integration/leads_orders_lifecycle_ws2_test.go:TestLeads_ConvertExactlyOnceUnderConcurrency`; `integration/leads_orders_lifecycle_ws2_test.go:TestLeads_CaptureQualifyAssignAndCrossTenant` |
| FLEET-14.11 | Pricing neutrality | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `architecture_test.go:TestNoPricingFormulas` |

Section 14: 26 rows - 26 CLOSED - VERIFIED.


## Section 15 - Catalogue, orders, shipments, packages

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-15.1a | Service catalogue create + activate/deactivate/archive | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/orders_test.go:TestServiceCatalog_LifecycleAndTaxiFailClosed` |
| FLEET-15.1b | Service catalogue list/get/patch | IMPLEMENTED | **CLOSED - VERIFIED** |  | `83989d9` | `integration/rows_15_test.go:TestRow15_1b_ServiceCatalogListGetPatch` |
| FLEET-15.2a | Create order, add stop, history | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/orders_test.go:TestOrders_CreateConfirmCancelLifecycle` |
| FLEET-15.2b | Order list/get/list stops | IMPLEMENTED | **CLOSED - VERIFIED** |  | `83989d9` | `integration/rows_15_test.go:TestRow15_2b_OrderListGetStops` |
| FLEET-15.3 | PATCH /orders/{id} | MISSING | **CLOSED - VERIFIED** | PATCH with If-Match while DRAFT/AWAITING_APPROVAL; submit/approve/reject added. | `af89035` | `integration/leads_orders_lifecycle_ws2_test.go:TestOrders_PatchSubmitApproveRejectLifecycle` |
| FLEET-15.4 | POST /orders/{id}/confirm, /cancel | IMPLEMENTED | **CLOSED - VERIFIED** | Cancel now withdraws pending private assignments and draft shipments. | `41256b9` | `integration/orders_test.go:TestOrders_CreateConfirmCancelLifecycle` |
| FLEET-15.5a | Shipment create/patch, cargo items | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/shipments_test.go:TestShipments_CreateUpdateLifecycleAndIsolation` |
| FLEET-15.5b | Shipment list/get | IMPLEMENTED | **CLOSED - VERIFIED** |  | `83989d9` | `integration/rows_15_test.go:TestRow15_5b_ShipmentListGet` |
| FLEET-15.6a | Create package | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/packages_section15_matrix_test.go:TestPackages_PostgreSQLIdentifiersConcurrencyAndTenantScope` |
| FLEET-15.6b | Package scan | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/packages_section15_matrix_test.go:TestPackages_TrustedDeviceProviderScopeOfflineAndReview` |
| FLEET-15.6c | Package transition | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/packages_section15_matrix_test.go:TestPackages_ConcurrentTransitionSingleWinner` |
| FLEET-15.6d | Package custody record | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/packages_section15_matrix_test.go:TestPackages_CustodyContinuityCorrectionEvidenceAndAppendOnly` |
| FLEET-15.6e | Package custody/history/get/list reads | IMPLEMENTED | **CLOSED - VERIFIED** |  | `83989d9` | `integration/rows_15_test.go:TestRow15_6e_PackageReads` |
| FLEET-15.7a | Manifest create/items/close/seal-amendments/cancel/patch | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/manifests_section15_matrix_test.go:TestManifests_TenantBranchMembershipAndAuthority` |
| FLEET-15.7b | Manifest list/get | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: list ignored branch scope. | `83989d9` | `integration/rows_15_test.go:TestRow15_7b_ManifestListGet` |
| FLEET-15.8a | Create import + worker | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/bulk_imports_section15_matrix_test.go:TestBulkImports_ValidationLimitsStableRowsPartialSuccessAndReportMinimization` |
| FLEET-15.8b | Import cancel/replay | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/bulk_imports_section15_matrix_test.go:TestBulkImports_DeadLetterReplayLeaseRecoveryAndFeatureControls` |
| FLEET-15.8c | Import list/get | IMPLEMENTED | **CLOSED - VERIFIED** |  | `83989d9` | `integration/rows_15_test.go:TestRow15_8c_OrderImportListGet` |
| FLEET-15.9a | Recurring generation worker + replay | IMPLEMENTED | **CLOSED - VERIFIED** | Worker off on staging. | `41256b9` | `integration/recurring_orders_section15_matrix_test.go` (3 tests) |
| FLEET-15.9b | Recurring create/activate/deactivate/list/get | IMPLEMENTED | **CLOSED - VERIFIED** |  | `83989d9` | `integration/rows_15_test.go:TestRow15_9b_RecurringTemplateCommandsAndReads` |
| FLEET-15.10 | Private-order execution through provider service after acceptance | PARTIAL | **CLOSED - VERIFIED** | Order DRAFT -> AWAITING_APPROVAL -> CONFIRMED -> PLANNED -> ASSIGNED -> IN_PROGRESS -> COMPLETED driven by real assignment and trip events; private execution adapters exist for Dispatch, Hauling and Taxi (`app/private_execution_adapter.go`). The test stubs the execution port; no cross-service private-execution run. | `af89035` | `integration/leads_orders_lifecycle_ws2_test.go:TestPrivateOrderLifecycle_RealProducersDriveOrderToCompletion` |
| FLEET-15.11 | Order state machine incl. AWAITING_APPROVAL/PLANNED/ASSIGNED/PARTIALLY_COMPLETED | IMPLEMENTED | **CLOSED - VERIFIED** | States are now reached by use cases (af89035). | `af89035` | `integration/leads_orders_lifecycle_ws2_test.go:TestPrivateOrderLifecycle_PartialAssignmentAndCancellationCompensation`; `features/orders/models/models_test.go:TestCanTransition_Matrix` |

Section 15: 22 rows - 22 CLOSED - VERIFIED.


## Section 16 - Dispatch, routes, live control and tracking

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-16.1 | GET /dispatch/board | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section16_dispatch_test.go:TestSection16BoardIsolationFiltersAndNoCommercialLeak` |
| FLEET-16.2 | GET /dispatch/unassigned | IMPLEMENTED | **CLOSED - VERIFIED** |  | `c9d8e77` | `integration/rows_16_test.go:TestRow16_2_UnassignedWork` |
| FLEET-16.3 | GET /dispatch/candidates (explainable) | IMPLEMENTED | **CLOSED - VERIFIED** | Service path now tested over HTTP. | `c9d8e77` | `integration/rows_16_test.go:TestRow16_3_ExplainableCandidates` |
| FLEET-16.4a | Plan create/get/approve/publish | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section16_dispatch_test.go:TestSection16DispatchPlansTenantBranchStateAndConcurrency` |
| FLEET-16.4b | Plan list/patch/cancel | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: negative offset 500. | `c9d8e77` | `integration/rows_16_test.go:TestRow16_4b_DispatchPlanListPatchCancel` |
| FLEET-16.5a | Route edit/approve + stop add/delete/list | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section16_dispatch_test.go:TestSection16RoutesStopsPublicationAndReplan` |
| FLEET-16.5b | Route create/list/get, stop edit, cancel | IMPLEMENTED | **CLOSED - VERIFIED** |  | `c9d8e77` | `integration/rows_16_test.go:TestRow16_5b_RouteCreateListGetStopEditCancel` |
| FLEET-16.6 | POST /routes/{id}/optimize | IMPLEMENTED | **CLOSED - VERIFIED** | Off on staging. | `41256b9` | `integration/section16_dispatch_test.go:TestSection16OptimizationDurableUnknownOutcomeReadBack` |
| FLEET-16.7 | POST /routes/{id}/publish | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section16_dispatch_test.go:TestSection16RoutesStopsPublicationAndReplan` |
| FLEET-16.8 | Re-optimize remaining work (`/routes/{id}/replan`) | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section16_dispatch_test.go:TestSection16RoutesStopsPublicationAndReplan` |
| FLEET-16.9 | GET /control-tower | IMPLEMENTED | **CLOSED - VERIFIED** | Alias of the dispatch board (no separate exception/SLA view); freshness now written back per trip (b57d5df). | `b57d5df` | `integration/section16_rescue_geofence_test.go:TestSection16BoardNeverPresentsStaleLocationAsLive` |
| FLEET-16.10a | Tracking ingestion + freshness/gap | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section16_dispatch_test.go:TestSection16TrackingGapConflictFreshnessGeofenceAndAlerts` |
| FLEET-16.10b | Tracking reads | IMPLEMENTED | **CLOSED - VERIFIED** | List now tested. | `c9d8e77` | `integration/rows_16_test.go:TestRow16_10b_TrackingList` |
| FLEET-16.11a | Geofence create + activate/deactivate | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section16_dispatch_test.go:TestSection16GeofenceWorkerTransitionsDedupesAndProtectsStaleTracking` |
| FLEET-16.11b | Geofence list/get/patch | IMPLEMENTED | **CLOSED - VERIFIED** |  | `c9d8e77` | `integration/rows_16_test.go:TestRow16_11b_GeofenceListGetPatch` |
| FLEET-16.12 | Geofence events (`GET /geofence-events`) | PARTIAL | **CLOSED - VERIFIED** | Read route over worker-derived events, branch scoped, read-time freshness. | `b57d5df` | `integration/section16_rescue_geofence_test.go:TestSection16GeofenceEventsReadScopedWithFreshness` |
| FLEET-16.13 | Alerts list/acknowledge/resolve | IMPLEMENTED | **CLOSED - VERIFIED** | Get and job replay now tested; replay defect fixed (foreign/non-section16 jobs 404, audit written). | `c9d8e77` | `integration/rows_16_test.go:TestRow16_13_GetAlertAndJobReplay`; `integration/section16_dispatch_test.go:TestSection16TrackingGapConflictFreshnessGeofenceAndAlerts` |
| FLEET-16.14 | Rescue cases (`/rescue-cases`, resolve) | MISSING | **CLOSED - VERIFIED** | Replacement is a provider-accept private assignment, never forced. | `b57d5df` | `integration/section16_rescue_geofence_test.go:TestSection16RescueCaseReplacementRequiresProviderAcceptance` |

Section 16: 18 rows - 18 CLOSED - VERIFIED.


## Section 17 - Workforce, shifts, training and performance

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-17.1a | Create workforce profile | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-17.1b | Profile list/get/patch/archive | IMPLEMENTED | **CLOSED - VERIFIED** |  | `57676e4` | `integration/rows_17_test.go:TestRow17_1b_WorkforceProfileListGetPatchArchive` |
| FLEET-17.2a | Shift create/patch/publish/cancel | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-17.2b | Shift list/get | IMPLEMENTED | **CLOSED - VERIFIED** |  | `57676e4` | `integration/rows_17_test.go:TestRow17_2b_ShiftListGet` |
| FLEET-17.3 | POST /shifts/{id}/assignments | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_concurrency_test.go:TestSection17And18Concurrency` |
| FLEET-17.4a | Check-in/check-out/cancel | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-17.4b | Get shift assignment | IMPLEMENTED | **CLOSED - VERIFIED** |  | `57676e4` | `integration/rows_17_test.go:TestRow17_4b_GetShiftAssignment` |
| FLEET-17.5a | Leave create/approve/reject/cancel | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-17.5b | Leave list/get | IMPLEMENTED | **CLOSED - VERIFIED** |  | `57676e4` | `integration/rows_17_test.go:TestRow17_5b_LeaveListGet` |
| FLEET-17.6a | Create qualification | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-17.6b | Qualification list/get/patch | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: partial PATCH wiped omitted fields. | `57676e4` | `integration/rows_17_test.go:TestRow17_6b_QualificationListGetPatch` |
| FLEET-17.7 | Provider qualifications add/list/verify/reject/revoke | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-17.8a | Create training course | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-17.8b | Course list/get/patch | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: title-only PATCH reset passing score. | `57676e4` | `integration/rows_17_test.go:TestRow17_8b_TrainingCourseListGetPatch` |
| FLEET-17.9 | POST /training/courses/{id}/complete | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-17.10 | GET /performance/providers/{id} (explainable) | PARTIAL | **CLOSED - VERIFIED** | Formula v2 counts marketplace and private work with traceable inputs, no ranking or money (09b187a). Snapshots are generated by the Section 17 worker, which is off on staging. | `09b187a` | `integration/provider_ws5_test.go:TestProviderPerformanceCountsMarketplaceAndPrivateWorkDistinctly` |
| FLEET-17.11 | Leave/qualification/training/shift feed eligibility | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-17.12a | Decline metrics by category, no automatic discipline | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-17.12b | Read performance | IMPLEMENTED | **CLOSED - VERIFIED** |  | `57676e4` | `integration/rows_17_test.go:TestRow17_12b_ProviderPerformanceRead` |
| FLEET-17.13 | Shift templates (`/shift-templates`) | MISSING | **CLOSED - VERIFIED** |  | `dda4aa0` | `integration/shift_templates_enrollments_test.go:TestShiftTemplatesAndTrainingEnrollments` |
| FLEET-17.14 | Training enrollments | MISSING | **CLOSED - VERIFIED** |  | `dda4aa0` | `integration/shift_templates_enrollments_test.go:TestShiftTemplatesAndTrainingEnrollments` |
| FLEET-17.15 | Provider self-view of schedule, qualifications, training, performance | MISSING | **CLOSED - VERIFIED** | Internal provider-service routes `/schedule`, `/qualifications`, `/training`, `/performance`. | `f2e14e6` | `integration/provider_ws5_test.go:TestProviderSelfServiceInternalRoutes` |
| FLEET-17.16 | Qualification-expiry / maintenance worker | IMPLEMENTED | **CLOSED - VERIFIED** | Worker off on staging. | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |

Section 17: 23 rows - 23 CLOSED - VERIFIED.


## Section 18 - Fleet assets, inspections, maintenance and fuel

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-18.1a | Asset create/get/patch | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/idor_assets_maintenance_test.go:TestAssets_ObjectAuthorization` |
| FLEET-18.1b | List assets | IMPLEMENTED | **CLOSED - VERIFIED** | Audit defect fixed: negative offset 500 now 422/clamped (0331419, 809e074); unknown filters 422 (a177684); branch scope in SQL (9564bd6). | `809e074` | `integration/rows_18_test.go:TestRow18_1b_AssetList`; `integration/api_conventions_test.go:TestOffsetListsServeValidPagesAfterValidation` |
| FLEET-18.2 | Archive / retire | IMPLEMENTED | **CLOSED - VERIFIED** |  | `809e074` | `integration/rows_18_test.go:TestRow18_2_AssetArchiveRetire` |
| FLEET-18.3 | POST /assets/{id}/assignments | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_concurrency_test.go:TestSection17And18Concurrency` |
| FLEET-18.4 | Asset documents | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: add wrote no audit/outbox. | `809e074` | `integration/rows_18_test.go:TestRow18_4_AssetDocuments` |
| FLEET-18.5a | Inspection templates create/patch | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-18.5b | Inspection create/get/complete; critical fail -> BLOCKED + work order + event | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-18.5c | Inspection/template lists, get template | IMPLEMENTED | **CLOSED - VERIFIED** |  | `809e074` | `integration/rows_18_test.go:TestRow18_5c_InspectionTemplateAndInspectionReads` |
| FLEET-18.6 | Maintenance schedules | IMPLEMENTED | **CLOSED - VERIFIED** | Reads now tested; defect fixed (missing/stale version 500). | `809e074` | `integration/rows_18_test.go:TestRow18_6_MaintenanceScheduleReadsAndPatch` |
| FLEET-18.7 | Work orders create/approve/complete/return-to-service | IMPLEMENTED | **CLOSED - VERIFIED** | Return to service hardened (3def868). | `41256b9` | `integration/idor_assets_maintenance_test.go:TestMaintenance_ObjectAuthorization`; `integration/section18_asset_lifecycle_test.go:TestSection18BlockedAssetCannotBeAssignedOrReleasedWithoutAuthorization` |
| FLEET-18.8 | Breakdowns | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: foreign-org replacement asset accepted. | `809e074` | `integration/rows_18_test.go:TestRow18_8_BreakdownLifecycle` |
| FLEET-18.9 | Parts inventory + stock movements | IMPLEMENTED | **CLOSED - VERIFIED** |  | `809e074` | `integration/rows_18_test.go:TestRow18_9_PartsList`; `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-18.10a | Fuel record create/get | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-18.10b | Fuel approve/reject, list | IMPLEMENTED | **CLOSED - VERIFIED** |  | `809e074` | `integration/rows_18_test.go:TestRow18_10b_FuelApproveRejectList` |
| FLEET-18.11 | UNSAFE_VEHICLE decline -> inspection/work-order draft | PARTIAL | **CLOSED - VERIFIED** | Same change as 11.13. | `1b3d869` | `integration/section17_18_test.go:TestSection17And18EndToEnd` |
| FLEET-18.12 | Unassign provider, meter readings, tyres, defects, asset costs/history | MISSING | **CLOSED - VERIFIED** | Migration 000092 tables; all six surfaces built. | `3def868` | `integration/section18_asset_lifecycle_test.go:TestSection18UnassignProviderNeverUnblocksAndRespectsActiveWork`; `integration/section18_asset_lifecycle_test.go:TestSection18TyresPositionsRotationAndTread` |

Section 18: 16 rows - 16 CLOSED - VERIFIED.


## Section 19 - Compliance, safety, incidents and proof

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-19.1 | GET /compliance/status | IMPLEMENTED | **CLOSED - VERIFIED** |  | `e9c1598` | `integration/rows_19_test.go:TestRow19_1_And_19_3_ComplianceStatusProviderAndAssetCompliance` |
| FLEET-19.2a | Requirement create/patch/activate/deactivate | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section19_21_test.go:TestSection19ComplianceRestrictionSafetyAndProof` |
| FLEET-19.2b | Requirement list/get | IMPLEMENTED | **CLOSED - VERIFIED** |  | `e9c1598` | `integration/rows_19_test.go:TestRow19_2b_ComplianceRequirementListGet` |
| FLEET-19.3 | Provider / asset compliance | IMPLEMENTED | **CLOSED - VERIFIED** | Unknown or foreign asset returns 200 `[]` rather than 404 (no data leaks; left as is). Web gap: provider compliance tab (`providers/[membershipId]/compliance/page.tsx`) and `compliance/providers/[providerId]/page.tsx` still render mock data. | `e9c1598` | `integration/rows_19_test.go:TestRow19_1_And_19_3_ComplianceStatusProviderAndAssetCompliance` |
| FLEET-19.4a | Evaluations | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section19_21_test.go:TestSection19ComplianceRestrictionSafetyAndProof` |
| FLEET-19.4b | Restrictions apply/get/clear | IMPLEMENTED | **CLOSED - VERIFIED** | List and per-authority propagation view added (08f31bf). Off on staging. | `41256b9` | `integration/section19_21_test.go:TestSection19AmbiguousRestrictionUsesReadBackWithoutResubmission` |
| FLEET-19.5 | Restriction propagation to execution services | IMPLEMENTED | **PARTIAL** | In progress. Propagates to Dispatch and Hauling (`app/wiring.go:423,426`) with read-back reconciliation (08f31bf). driver-taxi-service has no restriction endpoint and Fleet has no Taxi restriction client. Off on staging. | `08f31bf` | `integration/ws4_proof_compliance_test.go:TestWS4RestrictionPropagationReconcilesUntilConfirmed` |
| FLEET-19.6a | Create safety report | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section19_21_test.go:TestSection19ComplianceRestrictionSafetyAndProof` |
| FLEET-19.6b | Safety report list/get/patch/resolve | IMPLEMENTED | **CLOSED - VERIFIED** | Defects fixed: list scope, PATCH bypassing /resolve, anonymous reporter disclosure. | `e9c1598` | `integration/rows_19_test.go:TestRow19_6b_SafetyReportListGetPatchResolve` |
| FLEET-19.7a | Create incident | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section19_21_test.go:TestSection19ComplianceRestrictionSafetyAndProof` |
| FLEET-19.7b | Incident list/get/patch | IMPLEMENTED | **CLOSED - VERIFIED** | PATCH no longer changes status (08f31bf). | `e9c1598` | `integration/rows_19_test.go:TestRow19_7b_IncidentListGetPatch` |
| FLEET-19.8 | Incident evidence | IMPLEMENTED | **CLOSED - VERIFIED** |  | `e9c1598` | `integration/rows_19_test.go:TestRow19_8_IncidentEvidence` |
| FLEET-19.9 | Corrective actions create/complete/verify | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: cross-branch complete and VERIFIED->COMPLETED. | `e9c1598` | `integration/rows_19_test.go:TestRow19_9_CorrectiveActions` |
| FLEET-19.10a | PUT /proof-policies | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section19_21_test.go:TestSection19ComplianceRestrictionSafetyAndProof` |
| FLEET-19.10b | Proof policy list/get | IMPLEMENTED | **CLOSED - VERIFIED** |  | `e9c1598` | `integration/rows_19_test.go:TestRow19_10b_ProofPolicyListGet` |
| FLEET-19.11a | POST /proof-records | IMPLEMENTED | **CLOSED - VERIFIED** | Off on staging (needs `FLEET_PROOF_HMAC_SECRET`). | `41256b9` | `integration/section19_21_test.go:TestSection19ComplianceRestrictionSafetyAndProof` |
| FLEET-19.11b | Proof record list | IMPLEMENTED | **CLOSED - VERIFIED** |  | `e9c1598` | `integration/rows_19_test.go:TestRow19_11b_ProofRecordList` |
| FLEET-19.12 | POST /custody-events | PARTIAL | **CLOSED - VERIFIED** | Generic `/custody-events` over the package custody store for shipments, orders, assets and specialized records. | `08f31bf` | `integration/ws4_custody_incidents_test.go:TestWS4CustodyEventsGenericChain` |
| FLEET-19.13 | Incident triage/investigations, proof verify, compliance checks | MISSING | **CLOSED - VERIFIED** | Enforced incident state machine with investigations and findings; proof verify/reject; compliance checks. | `08f31bf` | `integration/ws4_custody_incidents_test.go:TestWS4IncidentLifecycleInvestigationAndClosure`; `integration/ws4_proof_compliance_test.go:TestWS4ProofVerifyAndReject`; `integration/ws4_proof_compliance_test.go:TestWS4ComplianceCheckRunsEveryRequirementStrictestGoverns` |

Section 19: 19 rows - 18 CLOSED - VERIFIED, 1 PARTIAL.


## Section 20 - Specialized logistics modules

| ID | Requirement | Initial status | Final status | Evidence / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-20.1a | Enable/disable capability | IMPLEMENTED | **CLOSED - VERIFIED** |  | `41256b9` | `integration/section19_21_test.go:TestSection20CapabilityIsolationValidationAndIdempotency` |
| FLEET-20.1b | List capabilities | IMPLEMENTED | **CLOSED - VERIFIED** | Hides modules switched off for the deployment (42326ed). | `1b980bf` | `integration/rows_20_test.go:TestRow20_1b_CapabilityList` |
| FLEET-20.2 | Courier/last-mile records | IMPLEMENTED | **CLOSED - VERIFIED** | Get/patch/list now tested. | `1b980bf` | `integration/rows_20_test.go:TestRow20_2_CourierRecordGetPatch` |
| FLEET-20.3 | Taxi/passenger records | PARTIAL | **CLOSED - VERIFIED** | Typed passenger records (manifests of opaque references within seat capacity, no-show, waiting, shuttle, safety), restricted visibility. Taxi execution itself stays in driver-taxi-service by design. Flag off by default and on staging. | `42326ed` | `integration/ws4_specialized_test.go:TestWS4OrderSpecializedDataAndPassengerRecords` |
| FLEET-20.4 | Trucking/haulage | IMPLEMENTED | **CLOSED - VERIFIED** | Now persisted, read back and tenant-isolated. | `1b980bf` | `integration/rows_20_test.go:TestRow20_4_To_20_11_SpecializedModulesPersistAndIsolate` |
| FLEET-20.5 | Heavy/specialized | IMPLEMENTED | **CLOSED - VERIFIED** |  | `1b980bf` | `integration/rows_20_test.go:TestRow20_4_To_20_11_SpecializedModulesPersistAndIsolate` |
| FLEET-20.6 | Freight forwarding / cross-border | IMPLEMENTED | **CLOSED - VERIFIED** |  | `1b980bf` | `integration/rows_20_test.go:TestRow20_4_To_20_11_SpecializedModulesPersistAndIsolate` |
| FLEET-20.7 | Cold chain (+temperature events) | IMPLEMENTED | **CLOSED - VERIFIED** | Defects fixed: release check, per-record replay, explicit thresholds (6b6b3d3, 1b980bf; migration 000096). | `6b6b3d3` | `integration/rows_20_test.go:TestRow20_7_ColdChainTemperatureEvents`; `integration/cold_chain_temperature_test.go:TestColdChainReplayIsPerRecord` |
| FLEET-20.8 | Medical/sensitive | IMPLEMENTED | **CLOSED - VERIFIED** | Off on staging. | `1b980bf` | `integration/rows_20_test.go:TestRow20_4_To_20_11_SpecializedModulesPersistAndIsolate` |
| FLEET-20.9 | Agricultural/food | IMPLEMENTED | **CLOSED - VERIFIED** |  | `1b980bf` | `integration/rows_20_test.go:TestRow20_4_To_20_11_SpecializedModulesPersistAndIsolate` |
| FLEET-20.10 | Moving/relocation | IMPLEMENTED | **CLOSED - VERIFIED** |  | `1b980bf` | `integration/rows_20_test.go:TestRow20_4_To_20_11_SpecializedModulesPersistAndIsolate` |
| FLEET-20.11 | Towing/recovery | IMPLEMENTED | **CLOSED - VERIFIED** |  | `1b980bf` | `integration/rows_20_test.go:TestRow20_4_To_20_11_SpecializedModulesPersistAndIsolate` |
| FLEET-20.12 | Milestones / generic commands on records | IMPLEMENTED | **CLOSED - VERIFIED** | Defect fixed: COMPLETE accepted on quarantined/completed records. | `1b980bf` | `integration/rows_20_test.go:TestRow20_12_RecordMilestonesAndCommands` |
| FLEET-20.13 | Disabled modules must not expose routes (20.1) | PARTIAL | **CLOSED - VERIFIED** | A disabled feature or module answers the same 404 on every route before any permission check; the capability list hides it. Routes remain registered in the router. | `42326ed` | `integration/ws4_specialized_test.go:TestWS4SpecializedDisabledModulesExposeNoRoutes` |
| FLEET-20.14 | Capability schemas / order specialized-data endpoints (50.3) | MISSING | **CLOSED - VERIFIED** | `GET /capability-schemas/:code`, `GET/POST /orders/:orderId/specialized-data`; typed closed v2 schemas, v1 still valid. | `42326ed` | `integration/ws4_specialized_test.go:TestWS4OrderSpecializedDataAndPassengerRecords`; `features/specialized/schemas_test.go:TestTypedSchemasCoverEveryRecordType` |

Section 20: 15 rows - 15 CLOSED - VERIFIED.


## Totals

| Section | Rows | CLOSED - VERIFIED | IMPLEMENTED - NOT INDEPENDENTLY TESTED | PARTIAL | MISSING |
|---|---|---|---|---|---|
| 5 | 12 | 6 | 0 | 6 | 0 |
| 6 | 14 | 9 | 0 | 5 | 0 |
| 7 | 16 | 15 | 1 | 0 | 0 |
| 8 | 20 | 17 | 1 | 1 | 1 |
| 9 | 21 | 20 | 1 | 0 | 0 |
| 10 | 23 | 23 | 0 | 0 | 0 |
| 11 | 17 | 8 | 8 | 1 | 0 |
| 12 | 8 | 7 | 0 | 1 | 0 |
| 13 | 17 | 17 | 0 | 0 | 0 |
| 14 | 26 | 26 | 0 | 0 | 0 |
| 15 | 22 | 22 | 0 | 0 | 0 |
| 16 | 18 | 18 | 0 | 0 | 0 |
| 17 | 23 | 23 | 0 | 0 | 0 |
| 18 | 16 | 16 | 0 | 0 | 0 |
| 19 | 19 | 18 | 0 | 1 | 0 |
| 20 | 15 | 15 | 0 | 0 | 0 |
| **Total** | **287** | **260** | **11** | **15** | **1** |

## Open items outside the row statuses

- Web: the approval request list and detail pages (`web/fleet/src/app/(dashboard)/settings/approvals/page.tsx`, `[requestId]/page.tsx`) and the provider compliance pages still render mock data, although their APIs are closed.
- In progress at the time of this snapshot: Taxi restriction propagation (FLEET-19.5) and a Dispatch organization trip-cancel route (FLEET-12.2).
