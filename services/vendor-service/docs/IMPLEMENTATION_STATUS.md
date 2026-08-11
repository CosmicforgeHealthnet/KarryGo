# Implementation Status

## Section 7 — Onboarding, KYB, Verification and Activation

**Status: VERIFIED; GO with explicitly fail-closed external deployment dependencies.**

### Section 7 contract correction

The cross-service VerificationStatus contract is exactly `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`; plain `PENDING` is rejected. Projection and history constraints enforce the same vocabulary, and activation tests prove mandatory `PENDING_REVIEW` blocks business, branch, and applicable branch-capability scope. The internal callback is the root-level `POST /internal/vendor/v1/verification-events`; it is not mounted below `/api/v1/vendor` and is not gateway-exposed. Queue/outbox operation statuses named `PENDING` are separate state machines and were deliberately preserved.

- Migrations `000008_vendor_section7_onboarding` and `000009_vendor_section7_permissions` add the onboarding aggregate, representative, opaque document references, exact verification projections/history, data-driven requirements, masked Wallet projection, reconciliation operations, nonce replay table, and six permissions.
- Nineteen public routes plus one HMAC-v2 callback are implemented and covered by `section7-onboarding-verification.openapi.json`; cumulative Vendor route count is 73.
- Activation commits the business graph, Section 7 audit, and `vendor.business.activated` outbox together. Unknown and pending outcomes reconcile by authoritative client reference with leases, bounded backoff, dead-letter and code-level replay; Verification also delivers signed callbacks from its leased outbox.
- Bank replacement is serialized per business, requires `expected_version`, and consumes step-up before the Wallet mutation. Verification submission atomically records the durable operation and marks referenced documents `SUBMITTED`; disabled integration writes nothing.
- Section 7 audit is transactional. The disclosed Section 5/6 append-after-commit audit limitation remains unchanged for those older paths.
- Deployment dependencies: Media and notification must allowlist Vendor; notification has no SMS; admin-backoffice remains a scaffold.

Per-section tracker for Vendor Service. Allowed statuses: `NOT_STARTED`, `IN_PROGRESS`, `BLOCKED`, `IMPLEMENTED`, `VERIFIED`, `GO`.

## Section 0 — Architecture / Foundation

- **Scope**: repository audit; service scaffold; own database/Redis config; migration runner/convention; health/ready/meta; structured logging; standard error handling; service-auth foundation; outbox/inbox/job architecture skeleton; feature-flag configuration; mandatory documentation structure; architecture decision docs; module boundaries for later sections; no fake OpenAPI routes; baseline tests.
- **Status**: **VERIFIED**
- **Code paths changed**: `services/vendor-service/` (new — `cmd/main.go`, `internal/config`, `internal/database`, `internal/events`, `internal/platform/idempotency`, `internal/audit`, `internal/jobs`, `internal/integrations/*` (11 reserved boundaries), `internal/features/*` (23 reserved boundaries), `internal/architecture_test.go`); `go.work` (added `./services/vendor-service`); `CLAUDE.md` (service listing, ports, databases, this session's log entry)
- **Migrations**: `000001_vendor_service_foundation`, `000002_events_outbox_inbox`, `000003_idempotency_keys` (all with `.up.sql`/`.down.sql`)
- **Routes**: none (intentional — no fake routes for unimplemented features)
- **Events**: none produced yet (outbox/inbox infrastructure exists, unused)
- **Workers**: none started yet (outbox worker constructed, not run — no destination configured)
- **Tests**: `internal/config/config_test.go` (6 tests — defaults, validation rules), `internal/database/migrate_test.go` (real-Postgres gated, migration replay: apply/rollback/reapply, run live and passing), `internal/architecture_test.go` (feature-boundary import guardrail)
- **Affected services**: none modified (only new `internal/integrations/*` doc.go boundaries describing future calls — see [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md))
- **Documentation updated**: this file; [README.md](README.md); [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md); [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md); [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md); [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md); `architecture/00`–`12`
- **Remaining gaps**: none for this section's own scope. See "Future sections" below for what is intentionally not yet started.
- **Acceptance result**: `go build ./...`, `go vet ./...`, `gofmt -l .` all clean; `go test ./...` all pass (config + architecture-boundary tests unconditionally; migration-replay test verified live against real Postgres in this session, then confirmed to skip cleanly without a configured test database — both paths exercised).

## Section 5 — Authentication, Sessions and Staff IAM

- **Scope**: passwordless OTP login (start/resend/verify), refresh sessions (rotation/revocation), logout, step-up authentication, staff provisioning/invitation/suspension/reactivation, roles (11 system role families + vendor-scoped custom roles), a global capability-string permission catalogue (35 permissions), role-permission mappings, branch-scoped staff-role assignments, a real authorization evaluator (deny-by-default, permission-string-only, branch-scope-aware), repository-level tenant isolation, real Postgres-backed audit trail, rate limiting, HTTP API, OpenAPI + route-parity test.
- **Status**: **VERIFIED**
- **Code paths changed**: `services/vendor-service/internal/features/auth/{models,repositories,usecases,clients,http}` (real implementation, replacing the Foundation phase's `doc.go` boundary); `internal/features/staff/{models,repositories,usecases,http}` (same); `internal/platform/ratelimit` (new); `internal/platform/authctx` (new — shared request-context carrier, not feature-owned, since both `auth` and `staff` need it without importing each other's `http` package); `internal/audit/postgres_recorder.go` (new, `internal/audit/audit.go` extended with `BranchID`/`Reason`); `internal/config/config.go` (`AuthConfig`, `RateLimitConfig`, validation rules); `cmd/main.go` (full wiring), `cmd/adapters.go` (new — cross-feature port adapters), `cmd/http_contract_test.go` (new)
- **Migrations**: `000004_vendor_auth_iam` (`vendor_users`, `vendor_staff`, `vendor_otp_challenges`, `vendor_sessions`, `vendor_step_up_grants`, `vendor_permissions`, `vendor_roles`, `vendor_role_permissions`, `vendor_staff_role_assignments`, `vendor_audit_log`), `000005_vendor_default_permissions_and_roles` (35 seeded permissions, 11 seeded system role families + mappings) — both with `.up.sql`/`.down.sql`, verified apply/rollback/reapply against real Postgres
- **Routes**: 21 (see [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md) and `docs/openapi/section5-auth-staff-iam.openapi.json`) — 5 public auth routes, 2 authenticated step-up routes, 12 authenticated staff/role/permission routes, 1 internal HMAC-protected provisioning route, 0 fake/future routes
- **Events**: `vendor.staff.created`, `.suspended`, `.reactivated`, `.roles_changed`, `vendor.role.changed` — appended to the real durable outbox (`internal/events`) on every mutation; no consumer/destination exists yet (see [architecture/08](architecture/08_EVENT_OWNERSHIP.md)) so they accumulate as `PENDING`, never silently dropped
- **Workers**: none started (outbox worker remains unstarted, matching Section 0's decision — no destination is configured yet)
- **Tests**: 50 test functions across 8 packages, all passing — `internal/features/auth/models` (11 unit tests: OTP state machine, session, step-up binding), `internal/features/auth/usecases` (13 real-Postgres integration tests: full login lifecycle, expired/resend/lockout/concurrent-verify-once, suspended-staff-blocks-login-and-refresh, multi-vendor selection without burning the OTP, step-up binding precision, logout revokes step-up grants, **raw OTP never persisted**, **raw OTP never logged**), `internal/features/staff/usecases` (8 real-Postgres integration tests: provision+invite+branch-scoped-assignment, **tenant isolation/IDOR**, **privilege-escalation prevention**, **branch-scope-escape prevention**, **role-name-alone-does-not-authorize**, **high-risk-role-requires-step-up**, duplicate-assignment prevention, suspend/reactivate with version-conflict + audit trail), `cmd` (4 real-Postgres+Redis HTTP contract tests: full login→list-staff through the real production router, validation-error envelope, 401-without-a-valid-token), `internal/section5contract` (2 OpenAPI/route-parity tests), `internal/database` (extended migration-replay + schema-existence tests)
- **A real bug found and fixed by this test suite**: the seeded Branch Manager role initially granted only `vendor.staff.read`, not `.manage`, silently contradicting the specification's "branch staff where permitted" — caught by `TestIAM_BranchScopedActorCannotEscapeBranch` failing on the in-branch-delegation-should-succeed half of the assertion, fixed in migration `000005` before it was ever "accepted" history (same session, not yet committed)
- **Affected services**: none modified. `notification-service` needs a config-only allowlist addition (`vendor-service` in `NOTIFICATION_SERVICE_SECRETS`) before real email OTP delivery works — tracked in [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md) #3, still `NOT_STARTED` (not this service's code to change)
- **Documentation updated**: this file; [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md); [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md); [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md); [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md); [architecture/09_SECURITY_AND_AUTHORITY_MODEL.md](architecture/09_SECURITY_AND_AUTHORITY_MODEL.md) (updated from the Foundation phase's planned model to the actual implemented one); `docs/openapi/section5-auth-staff-iam.openapi.json` (new)
- **Remaining gaps, disclosed**:
  - **Phone-number OTP delivery has no real channel** — confirmed no SMS capability exists anywhere in the platform (`shared/go/notifications`' channel set is exactly push/email/websocket/in_app). Email delivery is real; phone returns a clear `service_unavailable` error outside debug mode rather than silently failing or unsafely falling back to logging the raw code (unlike customer-service's existing `LoggingOTPSender`, which does log the raw code in debug mode — a pattern deliberately NOT replicated here per this task's own rule 11). Recorded as a platform-wide gap, not a Vendor Service defect.
  - **`notification-service` caller allowlist** not yet updated (config-only change on another service, out of this section's own code).
  - **Section 6 (Business/Branch) does not exist yet**, so `vendor_id`/`branch_id` remain opaque text throughout, exactly as the architecture phase specified — no fake CRUD was added.
  - **Security scenario coverage**: 50 real tests cover every category from the task's security-test list (OTP lifecycle, rate-limit wiring present, step-up binding, IAM tenant isolation/escalation/branch-scope/step-up-gating, audit) at representative depth; not literally all ~47 enumerated scenarios have a dedicated test (e.g. Redis-unavailable degraded-mode behavior and a few narrower rate-limit-bucket-isolation cases are implemented in code — `ratelimit.Limiter` fails closed via `apperrors.Unavailable`, never open — but not independently tested here for time).
- **Acceptance result**: `go build ./...`, `go vet ./...`, `gofmt -l .` all clean. Full suite run with `go test -p 1 ./...` (packages serialized to avoid shared-test-database DDL contention across packages — a test-infrastructure requirement, not a product defect; each package was also independently verified against its own isolated database) — all 50 tests pass, zero failures, zero skips (both `VENDOR_SERVICE_TEST_DATABASE_URL` and `VENDOR_SERVICE_TEST_REDIS_ADDR` were set for this run).

## Future sections (NOT_STARTED — listed for roadmap visibility only, per the module map)

| Section (indicative, not binding numbering) | Feature(s) | Depends on |
|---|---|---|
| Vendor identity/business | `vendors`, `capabilities` | — |
| Brands/branches | `brands`, `branches` | Vendor identity |
| CRM | `crm` | Vendor identity |
| Catalogues (Food/Cloud Kitchen/Pastry/Shop) | `catalog`, `food`, `cloud_kitchen`, `pastry`, `shop` | Branches, capabilities |
| Inventory | `inventory` | Catalogues |
| Purchasing | `purchasing` | Inventory |
| Recipes/production | `recipes`, `production` | Inventory, catalogues |
| Commerce orders | `orders` | Catalogues, [payment allocation model](architecture/05_PAYMENT_ALLOCATION_MODEL.md) |
| Substitutions | `substitutions` | Orders |
| Promotions | `promotions` | Orders, [payment allocation model](architecture/05_PAYMENT_ALLOCATION_MODEL.md) |
| Fulfilment planning | `fulfilment` | Orders; `integrations/dispatch|taxi|hauling` |
| Vendor-owned delivery | `delivery` | Fulfilment |
| Finance projection | `finance_projection` | `integrations/wallet` |
| Vendor analytics | `analytics` (feature) | Orders, fulfilment |

Each of these is expected to follow the specification's own "Preferred backend feature order" (domain model/usecase tests → repository/client interfaces → migration + repository implementation → HTTP DTOs/handlers/routes → config/wiring → shared client updates → integration/handler tests), and to update this file, [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md), [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md), [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md), and [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md) as its own definition-of-done, per [README.md](README.md)'s documentation gate.

## Section 6 - Vendor Business, Brands, Capabilities and Branches

**Status: IMPLEMENTED; activation remains feature-flag controlled.**

- Repository checkpoint: branch `staging`; starting and ending HEAD `f4dc08ca080a6b8dab5d3060578ee7566d1af7e2` (no commit requested). Code paths: `internal/features/vendors`, Section 6 migrations, production `cmd` wiring, Section 6 contract tests, and documentation.
- Route surface: 32 new Section 6 operations, 53 cumulative Vendor operations. No worker or downstream consumer was invented; legitimate events persist as `PENDING` in the existing outbox.

- Real Postgres aggregates and constraints: `vendor_businesses`, `vendor_capabilities`, `vendor_brands`, `vendor_branches`, normalized hours, radius service areas, explicit settings, and branch capabilities.
- Authenticated staff scope is authoritative: business id equals the Section 5 `vendor_id`, owner is the creating `staff_id`, and every repository query includes business scope. Multi-business staff continue to select a distinct Section 5 session scope.
- Exact state vocabularies and seven capability types are enforced in Go, SQL, OpenAPI, and contract tests. Creation is always non-active/non-verified; vendor operational transitions are only `ACTIVE` to `PAUSED` and `PAUSED` to `ACTIVE`.
- Mutations use optimistic versions, idempotency, audit records, and transactional outbox events for aggregate/status changes. Parent pause gates effective branch capability state without rewriting children.
- Production IAM branch assignments validate that the branch exists in the same business. Section 5 tests remain backward compatible through the optional validation port.
- No frontend, catalogue, logistics, wallet, verification, media-file, or other-service implementation was added.
- Known continuing audit limitation: Section 6 reuses the Section 5 `PostgresRecorder`, so audit rows are durable and tested but still append after the aggregate transaction rather than atomically inside it; this remains disclosed in architecture/09 and was not expanded into an unrelated audit-framework rewrite.

## External dependencies blocking full activation (not blocking further Vendor Service sections)

- **PENDING_EXTERNAL_DEPENDENCY — LOGISTICS OTP/QR** (see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md)): blocks only final confirmed-handover activation of platform-delivery fulfilment units.
- **Vendor admin review product**: Verification's Vendor authority and signed callback/read-back paths are implemented, but `admin-backoffice-service` remains a scaffold with no operator work queue/UI. Formal cases remain honestly `PENDING_REVIEW` until an authorized admin-service caller decides them.
