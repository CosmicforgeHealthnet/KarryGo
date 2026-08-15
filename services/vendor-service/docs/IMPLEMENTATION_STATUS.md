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

## Sections 10/11 — Shared Production, Cloud Kitchen, and Pastry/Bakery

- **Status**: **GO**. Backend implementation is complete; no frontend was created.
- **Schema**: migrations `000033` through `000041`; current tip `000041_vendor_sections10_11_integrity_alerts`. The schema includes shared pools/lines/rules/templates/slots/blackouts, multi-pool reservations and allocations, production jobs/history/alerts, Cloud Kitchen packages/bands/version history/plans/occurrences, and Pastry products/options/specifications/quotes/accepted snapshots/conversion intents/milestones/history.
- **Capacity correctness**: one PostgreSQL transaction locks capacity pools in deterministic order, applies buffers/rules/slots/blackouts, and atomically grants or rejects all allocations. Blackout publication takes the same ordered pool locks, preserving prior commitments while making every reservation started after blackout commit fail. Holds, confirmation, release, partial release, rescheduling, preview, utilization, calendar revisions, and expiry share this authority.
- **Cloud Kitchen**: Section 9 catalogue references are tenant/branch checked; composition and price-band edits preserve immutable package/price history. Corporate recurrence uses branch IANA timezones, exception dates, horizon/lead-time policy, idempotent future materialization, durable conflicts, leases, retries, dead-letter replay, safe cancellation, jobs, audit, and outbox. Occurrence management is versioned compare-and-swap, including concurrent reschedule protection.
- **Pastry/Bakery**: typed products, reusable ordered options, standard capacity-backed batches, versioned custom requests/specifications/diffs/change decisions, bounded media references, controlled logistics/risk metadata, optional-approval quote/revision/expiry/hold flows, stale/superseded-spec rejection at issue/approval/acceptance, concurrency-safe acceptance, immutable accepted snapshot, `PENDING_EXTERNAL_DOMAIN` conversion intent, jobs, milestones/history, cutoff and cancellation policy.
- **IAM/security**: production read/manage/operate, Cloud Kitchen manage, Pastry read/manage/quote-manage/quote-approve remain distinct; high-value approval uses existing step-up. The Kitchen/Pastry role has production operation and visibility but not production administration or quote pricing/approval. Tenant, branch, nested-resource, capacity-pool, catalogue-item, and capability references are checked fail-closed.
- **Workers**: corporate materialization/recovery, expired holds/quotes, and operational alerts are bounded and outbox/audit backed. Corporate, occurrence-reschedule, and quote race regression tests prove one occurrence/reservation/job, exactly one winning occurrence CAS mutation, and one accepted snapshot/conversion intent/reservation/job respectively.
- **API**: 123 Sections 10/11 public operations, zero internal operations; fully enabled Vendor Service router is pinned at 385 routes. All module mutations use the shared Vendor `Idempotency-Key` store; job transitions require `If-Match`. Section OpenAPI declares authentication, idempotency, path parameters, command bodies, and bidirectional route parity; the global route contract is green.
- **Authority boundary**: `ORDER_AUTHORITY=NOT_PRESENT`; accepted cake quotes create only a durable conversion intent. No payment, Wallet, inventory-stock, dispatch, taxi, hauling, route, fee, ETA, or handover credential is created.
- **Contract handoff**: [SECTION10_11_HANDOFF.md](SECTION10_11_HANDOFF.md), [sections10-11-production.openapi.json](openapi/sections10-11-production.openapi.json), and [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md).

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

## Section 8A - CRM Foundation, Customer 360, Consent and Privacy

**Status: IMPLEMENTED; every external CRM integration is deliberately dormant because no authority publishes those events yet.**

- Repository checkpoint: branch `claude/vendor-crm-foundation-8a-uonlya` (based on `staging`); starting HEAD `aa8c7807d8f7b90aad33425add2ac92402830fcf`. Route baseline before 8A: **73**; after: **80** (6 public + 1 internal). No existing route was changed or removed.
- Migrations `000010_vendor_crm_foundation` and `000011_vendor_section8a_crm_permissions`. Fresh replay, Section 7 -> 8A upgrade, rollback and reapply all verified against real Postgres; the Section 7 pair test was updated to step back past 8A first so it still proves exactly what it always proved.

### What the CRM is, and is not

A CRM profile is the commercial relationship between one vendor business and one customer subject. It is a projection, not a customer master: no canonical email, phone, address, date of birth or credential is stored or returned anywhere, and no hash of a customer reference is used as a stand-in identity. `TestHTTPCustomer360ReturnsNoCanonicalPII` walks the whole response tree to enforce that at the contract level.

Relationships are created **only** from an authenticated authoritative event. There is no vendor-facing create-customer route, so a production database legitimately holds zero CRM profiles until a real commerce or identity authority is integrated. Nothing was seeded to make the feature look populated.

### Authority audit (the reason most integrations are dormant)

| Capability | Result | Evidence |
|---|---|---|
| `CUSTOMER_CONSENT_SUPPORT` | **NOT_PRESENT** | No consent concept anywhere in customer-service. notification-service's `notification_preferences` is a delivery toggle (`enabled boolean DEFAULT true`, no purpose, no source version, defaults opt-in) owned by the delivery service, not a marketing-consent authority — mapping it to ALLOW would be exactly the forbidden widening. |
| `CUSTOMER_PRIVACY_EVENT_SUPPORT` | **NOT_PRESENT** | No delete/restrict/redact/suppression concept in customer-service. |
| `CUSTOMER_IDENTITY_MERGE_SUPPORT` | **NOT_PRESENT** | No identity-link, guest-upgrade or account-merge concept in customer-service. |
| `CUSTOMER_PRESENTATION_SUPPORT` | **PARTIAL, unusable as-is** | `GET /internal/customers/:id` (HMAC) exists but returns `phone` and `email` alongside `name`. There is no PII-free presentation contract, so `safe_display_label` is left permanently null rather than populated from a payload that carries contact data. |
| `ORDER_CRM_PROJECTION_SUPPORT` | **NOT_PRESENT** | `internal/features/orders` is still a reserved `doc.go` boundary; no vendor commerce order publisher exists. The projection interface, ingestion contract, dedupe, source-version rules and rebuild path are implemented and fixture-tested; the live integration stays disabled. |
| `SUPPORT_INTERACTION_PROJECTION_SUPPORT` | NOT_PRESENT | Not needed by 8A (interactions are 8C); recorded for later packages. |
| `NOTIFICATION_REFERENCE_DELIVERY_SUPPORT` | PARTIAL | push/email/websocket/in_app exist; no SMS sender. 8A sends nothing. SMS **consent** remains representable even though SMS **delivery** does not exist — the two are separate concerns. |
| `MEDIA_EXPORT_SUPPORT` | Not applicable to 8A | Exports are 8H. |

Consequence, stated plainly: with no consent authority wired, every marketing decision reads UNKNOWN and every channel is ineligible. That is the designed fail-closed outcome, not a defect.

### Schema

`vendor_customer_profiles`, `crm_customer_identity_links`, `crm_customer_branch_relations`, `crm_customer_order_facts`, `crm_consent_projections`, `crm_privacy_projections`, `crm_privacy_processing_history`, plus one additive nullable `payload_fingerprint` column on the shared inbox (the existing inbox deduped by event id but could not detect the same id carrying a different body).

Every CRM table is tenant-scoped in PostgreSQL, not merely in middleware: each carries `business_id` and reaches its parent through a composite `(id, business_id)` foreign key, so a child row can never be attached across tenants. Vocabularies, money non-negativity, the one-primary-subject-reference invariant, the merge-state/privacy-state agreement, and the "redacted iff redacted_at" rule are all real CHECK constraints.

**No privacy tombstone table was added.** The redacted profile row *is* the suppression record: it retains its opaque subject reference (and nothing else usable), which is the minimum needed to recognise a late event and refuse recreation. Hashing that reference would not make the subject anonymous — it would only make suppression unreliable.

### Behaviour worth calling out

- **Marketing fails closed by construction.** `MarketingEligibility` takes no override input, so no caller can pass one. Precedence: redaction > merge > marketing suppression > restriction > archival > consent unavailable > DENY > UNKNOWN/stale > ALLOW. Only the last returns true.
- **Aggregates are derived, never incremented.** Totals are recomputed from `crm_customer_order_facts` on every change, which is what makes refunds, replays, out-of-order delivery and merges safe by construction rather than by careful arithmetic. A partial refund lowers net without creating a second order; a redelivered event changes nothing.
- **Cross-currency data is quarantined, never converted.** CRM has no FX authority. An order denominated differently from the business currency is rejected and the existing totals are proven uncorrupted.
- **Merges require authoritative evidence.** No heuristic, fuzzy or ML matching exists. Both profiles are locked in a fixed id order (no deadlock), evidence is unique per authority event (no double merge), and the survivor's totals are rebuilt from the merged fact set so an order both sides observed is counted once.
- **Anti-resurrection holds in both orderings**: a late event for a redacted subject is refused, and a redaction that arrives *before* any relationship still blocks the later creation. A merged-away reference routes to its survivor instead of spawning a duplicate.
- **Archive cannot launder a privacy decision.** Archive and restore are both refused on a redacted or merged-away relationship.
- **Privacy processing is a handler registry**, so 8B-8H register their own redaction behaviour instead of the engine being rewritten. 8A registers `customer_profile`, `consent`, `commercial_projection`, `identity_link`, and Customer 360 reports the covered domain list.
- **Customer 360 distinguishes zero from unknown.** With no observed commercial data the state is `NOT_AVAILABLE` and every figure is omitted. Future modules report `NOT_IMPLEMENTED` rather than an empty structure that would read as an operational feature with no data.

### IAM

Five permissions added in `000011`: `vendor.crm.customers.read`, `.customers.manage`, `.customer360.read`, `.consent.read`, `.privacy.read`. Mapped to Owner, General Manager, CRM/Support (full 8A surface); Marketing (safe read + consent eligibility only — no privacy read, no lifecycle); Branch Manager (read only, branch-scoped by its Section 5 assignment); Analyst/Auditor (read only, including privacy for compliance review). Finance, Kitchen/Pastry, Shop/Inventory, Procurement and Vendor Rider are deliberately unmapped.

No `vendor.crm.consent.override`, `vendor.crm.privacy.override` or `vendor.crm.identity.force_merge` permission exists, and a migration test asserts none is ever introduced.

### Tests

220 test functions across the service, **0 failures, 0 skips**, run with real Postgres and Redis (`go test -p 1 ./... -count=1`). Section 8A contributes unit tests for the pure rules plus real-Postgres suites for tenancy/IDOR, branch-scope escape, event-caller authority, privacy races (including out-of-order versions and both anti-resurrection orderings), merge concurrency and dedupe, consent decision/version matrices, Customer 360 honesty, refund and replay safety, HTTP contract behaviour, and OpenAPI route parity.

### Known dependencies (real, not hypothetical)

1. **No consent authority** — needed before any marketing feature (8E/8F) can target anyone. Until then eligibility is uniformly false.
2. **No privacy authority** — the redaction engine is built and tested but nothing currently instructs it.
3. **No identity-merge authority** — guest-to-customer linking cannot occur in production yet.
4. **No vendor commerce order publisher** — every commercial figure stays `NOT_AVAILABLE`. This is the dependency that makes CRM useful, and it belongs to the order section.
5. **No PII-free customer presentation contract** — `safe_display_label` stays null; a client needing display identity must ask the customer authority directly.

### Not started

At the time Section 8A closed: Section 8B and everything after it. Section 8B has since been implemented — see its own entry below.

## External dependencies blocking full activation (not blocking further Vendor Service sections)

- **PENDING_EXTERNAL_DEPENDENCY — LOGISTICS OTP/QR** (see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md)): blocks only final confirmed-handover activation of platform-delivery fulfilment units.
- **Vendor admin review product**: Verification's Vendor authority and signed callback/read-back paths are implemented, but `admin-backoffice-service` remains a scaffold with no operator work queue/UI. Formal cases remain honestly `PENDING_REVIEW` until an authorized admin-service caller decides them.

## Section 8B — Tags & Advanced Segmentation Engine

**Status: IMPLEMENTED; GO.**

Master CRM requirements 14-24. Built on the accepted Section 8A baseline (80 Vendor routes, migration tip `000011`), verified against the working tree before any code was written: the full Section 5-8A suite was run green on a fresh Postgres/Redis first, so nothing here is attributed to a failure that was already present.

### Scope

Vendor-defined customer tags, a safe JSON segmentation DSL with a closed field/operator registry, static and dynamic segments, immutable definition versions, privacy-safe preview, durable materialization with atomic generation swap, manual exclusions, and Customer 360 integration. At the Section 8B boundary, Sections 8C-8I had not started; Section 8C is now implemented below, while 8D-8I remain untouched.

### What was built

- **Migrations** `000012_vendor_section8b_tags_segments` and `000013_vendor_section8b_permissions` add seven tables — `crm_tags`, `crm_customer_tag_memberships`, `crm_segments`, `crm_segment_versions`, `crm_segment_evaluations`, `crm_segment_memberships`, `crm_segment_exclusions` — and five permissions. Migration tip is now `000013`.
- **21 public routes**, 0 internal. Cumulative Vendor surface: **101**. Documented in `openapi/section8b-crm-tags-segmentation.openapi.json` and pinned by `internal/section8bcontract`; `cmd`'s HTTP contract test independently counts what the production registration helper actually mounts, so the two cannot drift.
- **Code**: `internal/features/crm/segment_rules.go` (DSL, validation, canonicalisation, hashing, explanations), `segment_registry.go` (field/operator registry and the SQL compilers), `segments.go` (definitions and versions), `segment_membership.go` (static membership, exclusions, member reads), `segment_eval.go` (preview, materialization, worker, invalidation), `segment_privacy.go` (privacy and merge handlers), `tags.go`, `segments_http.go`. Two Section 8A files gained an extension point each: `privacy.go` registers the new handlers, `customer360.go` composes the new blocks.

### Design decisions worth recording

**The rule language is the entire attack surface, so it is a closed registry, not a parser.** A condition can only name a field that exists in `NewSegmentFieldRegistry`, and each field's SQL is a hand-written Go function in one file. Field keys, operators and values never become SQL text — values travel as bind parameters and identifiers are never derived from input. There is no free-text filter, no user-chosen sort column and no escape hatch anywhere in the surface. A rejection for an unknown field and a rejection for a forbidden one are the same error, so a probe learns nothing about the schema.

**Unknown is not zero, and SQL's three-valued logic enforces it.** Every commercial condition compiles to `CASE WHEN <observed> THEN (<expr>) ELSE NULL END`, and the whole rule is wrapped in `IS TRUE`. A relationship with no observed commercial data therefore matches neither `count >= 2` nor `count < 2`, and a `NOT` over such a condition does not resurrect it. This is native SQL semantics rather than bolted-on filtering, which is why it holds at every nesting level. Deployment-level missing sources are reported in preview `unavailable_fields`; an individual row that lacks observed data is simply `NOT_AVAILABLE` for evaluation and does not expose a per-customer reason.

**Consent eligibility exists in two places and a test proves they agree.** `MarketingEligibility` in Go remains the single definition of the policy; `consentEligibleCompiler` is its set-based twin. A real-Postgres test seeds allow/deny/unknown/expired relationships and asserts the segment answer matches Section 8A's own eligibility answer for every one, so drift between them fails the build rather than quietly mis-targeting customers.

**A failed rebuild never empties an audience.** Materialization writes a new generation alongside the live one and swaps in a single committed transaction. A failure at any point before the swap leaves the previous membership exactly as it was, and the segment reports `STALE` with an error code rather than a count of zero. An empty audience reads as "nobody qualifies"; a stale one reads as "this is out of date" — only one of those is recoverable by the person looking at it.

**Privacy and merge win every materialization race, in both orderings.** The swap transaction locks all candidate profile rows in stable id order and re-evaluates the complete compiled rule before changing the live-generation pointer. Privacy actions and identity merge use the same row locks. If privacy/merge commits first, certification excludes the relationship; if swap commits first, the following privacy/merge transaction removes or transfers its membership and repairs counts. A crash during the fenced swap rolls back the pointer and membership changes together.

**Dependency tracking is real, not decorative.** Each stored definition version carries the registry field keys its rule reads. Tag, commercial, consent and profile-creation mutations mark stale and durably queue only the segments that read the changed fields. Restriction and marketing suppression target only marketing-eligibility dependencies and immediately remove now-ineligible current membership. Duplicate invalidations reuse the queued evaluation rather than creating duplicate work.

**Five permissions, not nine.** Bulk tagging is the same authority as tagging; previewing and materializing are the same authority as reading and managing a definition. Splitting them would create ways for a role to be half-granted rather than any real protection. Member reads are separate, because naming the customers in an audience is a strictly larger disclosure than reading its definition and size.

**Branch scope reaches into the rule and every read surface.** A branch-scoped segment compiles its commercial conditions against that branch's relation row rather than the business-wide aggregate, so "two completed orders" means two at that branch. Preview counts, tag member counts, segment member lists and Customer 360 are branch-filtered. In Customer 360 a branch actor sees only branch-scoped classifications for an authorized branch; business-scoped tag and segment classifications are never disclosed, and tag member counts remain omitted.

**Section 8B extends the Section 8A privacy registry without changing its authority model.** Both new domains register through the existing `PrivacyHandler` port, and `PrivacySummary.CoveredDomains` grows automatically. `ApplyPrivacy` now locks the relationship profile row before dispatching handlers so privacy, merge and materialization share one database fence; decision ownership and handler extensibility are unchanged. That is the property 8C-8H depend on.

**Archived relationships are excluded by default but not made unreachable.** The mandatory filter drops them unless the rule explicitly names `profile.status`, so an operational audience over archived relationships stays expressible while the default stays safe.

### Deliberate scope choices

- **Computed traits are a versioned code registry, not a table.** Every registered field declares its key, label/description, type, source, scopes, operators, nullable/branch/privacy behavior, availability predicate, hand-written query compiler, deterministic explanation renderer, and field-contract version. No persisted trait table was added because these values are deterministic functions of Section 8A projections. No `VIP`, `Churn Risk` or `High Value` system tag was invented — those are segment logic, and scoring belongs to 8H.
- **No purpose/marketing-intent field on a segment.** It would not change privacy or consent enforcement, so it would be metadata pretending to be a control.
- **Archived tag names are reusable; archived tag identifiers are not.** A new tag with a reused name gets a new id, so historical membership and audit stay unambiguous.
- **`GenerationRetainer` is a port with a no-op default.** Section 8E implements it to pin the generation a campaign launched against; building the seam now is what stops cleanup from deleting an audience a campaign is still reconciling against later.

### Completion-audit fixes

1. **The canonical rule form did not re-parse.** Values were serialised as arrays and durations as raw seconds, so a stored definition failed revalidation at rebuild time — silently, long after the rule was accepted. Fixed by making the canonical form emit exactly the shapes the validator accepts, and pinned by a round-trip test over every value type.
2. **Parameter typing in the generation insert.** A bigint parameter was first used inside a text concatenation, so Postgres inferred `text` and pgx could not encode it. Fixed with explicit casts. The evaluation clock also moved from the database's `now()` to the service clock, so a run is reproducible and testable rather than dependent on container time.
3. **Lease renewal and replay were incomplete.** The worker claimed a lease but did not renew it during a long generation build, and a dead-letter had no operator recovery method. Materialization now heartbeats the lease, uses a unique worker identity per process, and exposes the code-level `ReplayMaterialization` hook. Tests prove renewal and dead-letter replay.
4. **Projection invalidation stopped at `STALE`.** Tag changes were marked stale but not durably enqueued, and commercial, consent, archive/restore, and merge changes did not consistently trigger the dependent audience rebuild. Invalidation now marks and queues in the same transaction, with field-level dependency selection where possible and tenant-wide restaking only for implicit profile-state/merge effects.
5. **Customer 360 over-disclosed business classifications to branch actors.** Branch reads now return only authorized branch-scoped tags and segments; business actors retain the full business view. Member counts remain absent from Customer 360 tag summaries.
6. **Profile creation and privacy eligibility changes missed segment invalidation.** New profiles now invalidate only `profile.subject_type`, `profile.status`, and `profile.acquisition_source_type` dependencies in the creation transaction. Restriction and marketing suppression invalidate only channel-eligibility dependencies, immediately remove current ineligible membership, repair counts, and leave consent decisions unchanged.
7. **Generation swap had a privacy/merge race.** The swap now shares profile-row locks with privacy and merge, certifies the pending generation by re-running the complete rule while fenced, and atomically changes membership, counts, evaluation state, and current-generation pointer. Tests cover both commit orderings and crash rollback/recovery.

### Tests

**257 top-level test functions across the service, 0 failures, 0 skips**, run with real Postgres and real Redis (`go test -p 1 ./... -count=1`) on a freshly created database after the targeted correctness pass. Section 8B contributes:

- **Rule DSL (unit)**: structural rejections, operator/type compatibility, every value-type rejection, all six complexity limits plus the ceiling being usable, canonical-hash stability, canonical round-trip, deterministic explanations, and the security set — SQL injected through field names, operators and values, unmapped column names, forbidden personal fields proven absent from the registry, and the registry proven not to leak table or column names.
- **Tags (real Postgres)**: normalisation and duplicate collapse, reserved namespaces, control/invisible/markup rejection, branch scope enforcement, cross-branch invisibility, cross-tenant isolation, all-or-nothing bulk semantics, bulk ceiling, archive clearing membership, version conflicts, branch-relationship requirement, and branch-scoped member counts.
- **Segments (real Postgres)**: static and dynamic lifecycles, terminal archive, static-only and dynamic-only refusals, preview counts and samples, the unknown-versus-zero matrix including negation, branch-scoped preview and the business-scope refusal, privacy exclusion, tag- and branch-reference scope enforcement, cross-tenant isolation, business-segment invisibility to branch actors, and version conflicts.
- **Materialization (real Postgres)**: first build, failure preserving the last good audience, dead-lettering and code-level replay, lease exclusivity, heartbeat renewal and adoption after expiry, partial-generation discard, fenced redaction and merge races in both commit orderings, crash rollback and recovery during the fenced swap, exclusion subtraction surviving rebuilds, rule-edit versioning, transactional dependency-scoped invalidation/enqueue (including commercial and profile-creation changes), duplicate-request collapsing, archive dropping membership, branch-scoped figures, a shared evaluation instant, single-generation reads, the disabled-worker behaviour, two concurrent workers producing one audience, generation cleanup, the retention port pinning a generation, and the consent cross-check against Section 8A.
- **Privacy and merge (real Postgres)**: domain registration, immediate removal of tags/membership/exclusions on redaction with dynamic restaking, restriction/suppression eligibility invalidation with immediate membership and count repair while consent decisions remain unchanged, tag and static-membership races in both orderings, tag and static-membership transfer on merge with deduplication, exclusion transfer, dynamic membership recomputed rather than copied, and Customer 360 scope tests proving branch actors never receive business-scoped classifications.
- **Scale (real Postgres, 1,200 relationships)**: materialization across batch boundaries with no loss or duplication, complete non-overlapping pagination, the preview count budget reporting an honest lower bound, a bulk operation at the 500 ceiling, a rule at the documented complexity ceiling evaluating, and the candidate query's real plan.
- **Contract**: bidirectional OpenAPI/route parity, route count, every operation declaring a permission, every command declaring `Idempotency-Key`, no campaign- or later-section-shaped route, exact enum vocabularies, a fully recursive documented rule grammar, and the error-code catalogue.
- **Migration**: fresh replay, 8A-to-8B upgrade, rollback and reapply as a pair, Section 8A schema and permissions proven undamaged by an 8B rollback, exact SQL vocabularies, eleven composite tenant foreign keys, thirteen indexes, and the role-mapping matrix including the roles that must hold nothing.
- **Config**: both flag prerequisites and the safe default.

Customer 360 availability tests advance with each implemented module: tags/segments are available under Section 8B flags, interactions/tasks under Section 8C flags, loyalty under the Section 8D flag, and 8E-8H remain unimplemented. Section 8E deliberately does not change that: campaign targeting history is not added to Customer 360, so `campaigns` still reports `NOT_IMPLEMENTED` there even though the section itself is implemented. The Section 7, 8A, and 8B rollback tests walk down to their own version boundaries rather than assuming they are the migration tip.

### Performance limits (configuration, not capacity claims)

Pagination default 50 / max 200. Bulk batch default 100 / max 500. Rule limits: depth 6, nodes 50, OR branches 25, IN values 100, NOT depth 3, tag references 25. Materializer batch 500, lease 2 minutes, 5 attempts, 30s base backoff. Preview sample default 25 / max 100, count budget 50,000. The scale tests ran against a 1,200-relationship fixture; **no figure from them is a production capacity claim**, and none should be quoted as one.

### Known dependencies

Unchanged from Section 8A, and none new. Segmentation runs entirely over Vendor-owned data, so it needs no external service. What it inherits:

1. **No vendor commerce order publisher** — every commercial field reports as unobserved, so commercial rules match nobody. This is the dependency that makes segmentation useful, and it belongs to the order section.
2. **No consent authority** — consent fields are unavailable, so a marketing-eligibility rule matches nobody. Fail-closed by design.
3. **No privacy or identity-merge authority** — the handlers are built and tested against fixtures; nothing currently instructs them in production.

Section 8B's own flags default off, and `VENDOR_CRM_SEGMENTATION_ENABLED` requires `VENDOR_CRM_ENABLED`, which `VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED` in turn requires — validated at startup, so a half-enabled configuration is a boot failure rather than a runtime surprise.

### Not started

At the Section 8C boundary, Sections 8D-8I had not started. Section 8D is now implemented below; Sections 8E (campaign intelligence), 8F (campaign execution), 8G (corporate CRM), 8H (intelligence/exports), 8I (final CRM integration), and everything in Section 9 onwards remain not started. No table, route, permission, flag, worker, or event for those sections was created; Section 8C contains no notification delivery, support-ticket authority, frontend, or future commerce domain.

## Section 8C — interactions, timeline, and CRM tasks

**Status: IMPLEMENTED and PostgreSQL-verified.** Section 8C extends `internal/features/crm` with Vendor-local relationship interactions, append-only corrections, staff follow-up tasks, immutable task history, privacy/merge handlers, bounded Customer 360 activity, and 16 public HTTP operations. It creates no internal route and changes no external service.

- **Migrations:** `000014_vendor_crm_interactions_tasks` adds `crm_interactions`, `crm_interaction_revisions`, `crm_tasks`, and `crm_task_history`; `000015_vendor_section8c_permissions` adds six permissions and established role mappings. Current tip is `000015`.
- **Interactions:** 12 controlled types, four directions, ten outcomes, `BUSINESS`/`BRANCH` visibility, 200-character subject, 5,000-character plain-text note, and optimistic correction with an immutable full-snapshot revision.
- **Tasks:** `OPEN`, `IN_PROGRESS`, `COMPLETED`, and `CANCELLED`; `LOW`, `NORMAL`, `HIGH`, and `URGENT`; same-tenant active eligible staff assignment; nullable UTC due date; derived `is_overdue`; explicit terminal transitions and optimistic version checks.
- **Privacy and merge:** interaction content becomes display-suppressed after redaction; open/in-progress tasks are cancelled and suppressed; merged records move transactionally to the canonical profile. Creation/correction/task mutation and privacy/merge share the profile-row lock fence.
- **Customer 360:** at most five recent permitted interactions with `has_more`, plus at most five open permitted tasks and an exact scope-safe count. Redacted and merged-source profiles expose no ordinary activity content.
- **Audit/outbox:** create, correction, task create/update/start/reassign/complete/cancel all write transactionally. Events are `vendor.crm.interaction.created`, `vendor.crm.interaction.corrected`, `vendor.crm.task.created`, `vendor.crm.task.updated`, `vendor.crm.task.started`, `vendor.crm.task.reassigned`, `vendor.crm.task.completed`, and `vendor.crm.task.cancelled`; free-text bodies and canonical PII are excluded from event/audit metadata.
- **Flags:** `VENDOR_CRM_INTERACTIONS_ENABLED` and `VENDOR_CRM_TASKS_ENABLED`, both safe-default false and requiring `VENDOR_CRM_ENABLED`; all commands also obey `VENDOR_CRM_WRITES_ENABLED` and `COMMERCE_GLOBAL_KILL_SWITCH`.
- **Rate limits:** the existing fail-closed Redis limiter is applied per business/staff: 60 interaction/task creates and 120 correction/task mutations per minute.
- **Tests:** real PostgreSQL coverage includes tenant/branch/customer/assignee/link/history IDOR, validation, idempotency, revision/history, privacy and merge, Customer 360 disclosure, deterministic concurrency races, migration replay/upgrade/rollback/reapply, OpenAPI parity, and a 300-profile/1,500-interaction/600-task scale fixture.
- **No authority expansion:** no notification delivery, support/dispute workflow, canonical customer PII, cross-service database read, reminder worker, attachment, bulk task mutation, frontend, Section 8D+, or Section 9+ implementation.

Known deployment dependencies remain explicit: Customer Service must eventually publish privacy/merge authority events, Notification Service would own any future reminder delivery, and the future order domain must publish commercial facts. Section 8C itself fabricates none of these integrations.

## Section 8D - loyalty points

**Status: IMPLEMENTED and PostgreSQL-verified.** Section 8D extends `internal/features/crm` with a per-business loyalty programme, lazily created customer accounts, an append-only points ledger, point lots with per-issuance expiry, recorded lot consumption, a durable expiry sweeper, bounded manual adjustments, full reversal, privacy and identity-merge handlers, a Customer 360 loyalty block, and 6 public HTTP operations. It creates no internal route and changes no external service.

- **Migrations:** `000016_vendor_crm_loyalty` adds `crm_loyalty_program_configs`, `crm_loyalty_accounts`, `crm_loyalty_entries`, `crm_loyalty_point_lots`, and `crm_loyalty_lot_consumptions`; `000017_vendor_section8d_permissions` adds four permissions and role mappings. Current tip is `000017`.
- **Points, not money.** No currency, minor-unit amount, wallet reference or conversion path exists anywhere in the schema, the API or the events. `payment-wallet-service` was not touched and remains the sole monetary authority. Both a schema-level test and an OpenAPI contract test reject a monetary-looking column or field, so the boundary fails the build rather than drifting.
- **Programme:** `DISABLED`/`ACTIVE`/`PAUSED`; `NEVER`/`FIXED_DAYS` expiry with a 1-3650 day window; configurable points nouns, redemption minimum and increment, and manual adjustment limit; optimistic concurrency where the first write must send `expected_version: 0` and a concurrent first write loses on the unique constraint rather than silently overwriting.
- **Accounts:** one per business/profile, created lazily on first real movement - a read never creates a row, so `NOT_ENROLLED` is a genuine fact rather than a fabricated zero. Concurrent first credits resolve to one account via the unique constraint.
- **Ledger:** append-only, eight closed entry types, per-account monotonic sequence, `balance_after` on every row, and a database constraint holding `current_points = lifetime_credited - lifetime_debited`. There is no update or delete path; corrections are compensating entries.
- **Lots and FEFO:** every credit issues a lot stamped with the expiry policy as it stood at issue time, never recomputed. Debits consume earliest-expiring first (non-expiring last) and record exactly which lot funded how much, which is what makes a later reversal exact rather than approximate.
- **Expiry:** `available_points` always excludes lots past their expiry whether or not the sweeper has settled them, and every balance-changing command settles due expiry before acting - so a sweeper outage delays bookkeeping without ever letting an expired point be spent. The sweeper needs no queue table and no lease: a lot with points left and a past expiry is itself the durable job, giving crash recovery and multi-worker safety (SKIP LOCKED on the account row) from the schema. Disabling the flag defers the backlog rather than discarding it.
- **Earn and redeem:** implemented as service-internal primitives with durable command-key and source-event deduplication. **No public earn or redeem route exists**, and the contract test rejects one structurally: an endpoint letting a caller name its own points total is a far larger authority than the bounded manual adjustment. No earning formula was invented, because the Vendor Order domain that would define one does not exist.
- **Manual adjustment:** dedicated high-risk permission, positive amount only (direction carries the sign), bounded by the programme's own limit, mandatory 500-character reason recorded in the audit log **only** - never in the ledger row, the outbox payload or any log line. A credit may enrol; a debit may not, and may not overdraw.
- **Reversal:** full only, exactly once per entry (a unique index, not a read-then-write check). Only `EARN`, `REDEEM`, `ADJUSTMENT_CREDIT` and `ADJUSTMENT_DEBIT` are reversible. Reversing a credit refuses when its lot has been touched rather than confiscating points from an unrelated issuance; reversing a debit restores the exact points to the exact funding lots with their original expiry, then immediately expires anything already past its window in the same transaction.
- **Privacy and merge:** redaction makes the account non-actionable and suppresses it from every ordinary read while retaining the ledger, which holds integer movements and vendor-local references with no personal data to erase. Marketing suppression and restriction are explicit no-ops - holding a balance is not marketing consent, and withdrawing consent is not forfeiture. Identity merge posts an auditable `MERGE_OUT`/`MERGE_IN` pair, transfers surviving lots with their original expiry preserved, and leaves the source ledger immutable and non-actionable.
- **Customer 360:** one loyalty block reporting programme status, enrolment, available/lifetime/pending-expiry points and the next expiry, contributed through the same composition point Sections 8B and 8C use. Redacted and merged-source profiles report `NOT_ENROLLED` with no figures.
- **Audit/outbox:** programme update, account creation, credit, debit, expiry, adjustment, reversal and merge all write transactionally. Events are `vendor.crm.loyalty.program.updated`, `.account.created`, `.points.credited`, `.points.debited`, `.points.expired`, `.adjustment.applied`, `.entry.reversed`, and `.account.merged`; free-text reasons, command keys, source references and canonical PII are excluded from every payload.
- **Flags:** `VENDOR_CRM_LOYALTY_ENABLED`, safe-default false and requiring `VENDOR_CRM_ENABLED` (validated at startup). Commands also obey `VENDOR_CRM_WRITES_ENABLED` and `COMMERCE_GLOBAL_KILL_SWITCH`.
- **Segmentation boundary:** the Section 8B field registry remains at exactly 22 fields, guarded by a test. Loyalty is deliberately not segmentable yet; adding a field would silently change what every stored audience means.
- **Tests:** real PostgreSQL coverage includes programme versioning and expiry-policy validation, non-retroactive lot expiry, ledger replay and aggregate agreement, overflow and zero/negative rejection, FEFO and multi-lot consumption, sweeper idempotence and multi-day outage catch-up, adjustment validation/limit/idempotency and reason non-disclosure, the full reversal matrix, tenant/branch/customer/entry/programme IDOR, Customer 360 disclosure, HTTP permission separation and edge idempotency, migration replay/upgrade/rollback/reapply with constraint and index assertions, OpenAPI parity, and eight deterministic barrier-synchronised concurrency races (debit vs debit, first-credit creation, expiry vs debit, double reversal, merge vs credit, merge vs debit, merge vs expiry, redaction vs adjustment) each closing with a ledger/aggregate/lot reconciliation.
- **No authority expansion:** no loyalty tiers, reward catalogue, voucher or coupon engine, gift cards, cashback, wallet conversion, cash redemption, campaign targeting, notification delivery, order integration, internal route, external-service change, frontend, or Section 8E+/Section 9+ implementation.

**Known dependencies, disclosed rather than fabricated.** The Vendor Order domain does not exist, so nothing awards points automatically today; `CreditPoints` and `DebitPoints` are ready for it and deduplicate producer events durably. Expiry-warning notifications would be Section 8E/8F work and require their own Notification Service integration. The pre-existing `go test -p 1 ./...` serialisation caveat documented under Section 5 still applies to the shared external test database.

## Section 8E - campaign intelligence and the audience engine

**Status: IMPLEMENTED and PostgreSQL-verified.** Section 8E extends `internal/features/crm` with campaign definitions, immutable campaign versions, segment-backed audience selection, privacy-safe preview, durable audience preparation, immutable audience snapshots, campaign targets, per-channel snapshot-time consent evidence, deterministic experiment cohorts, frequency and quiet-hour policy definition, a privacy/merge handler, a segment-generation retainer, and 10 public HTTP operations. It creates no internal route and changes no external service.

- **Migrations:** `000018_vendor_crm_campaign_intelligence` adds `crm_campaigns`, `crm_campaign_versions`, `crm_campaign_experiment_variants`, `crm_campaign_audience_snapshots`, `crm_campaign_targets`, and `crm_campaign_target_channels`; `000019_vendor_section8e_permissions` adds five permissions and role mappings. Current tip is `000019`.
- **Prepares, never delivers.** No scheduling code, send endpoint, notification handoff, template, provider client, `scheduled_at`, `sent_at`, `delivered_at` or destination column exists anywhere in the section. The campaign lifecycle is `DRAFT`/`PREPARING`/`PREPARED`/`PREPARATION_FAILED`/`ARCHIVED` with no delivery state, and a migration test rejects one structurally. `notification-service` was not touched.
- **Dual-time consent.** Preparation records what the authority's projection said, per target per channel, as immutable evidence. Section 8F must revalidate current consent before any delivery handoff. `send_time_recheck_required` carries a `CHECK (send_time_recheck_required = true)`, so a row asserting otherwise is unstorable rather than merely unwritten, and a later consent change never rewrites the frozen evidence.
- **Fail-closed eligibility.** Consent is decided by Section 8A's single `MarketingEligibility` rule, not a second copy: DENY, UNKNOWN, a missing projection, a lapsed ALLOW and a stale projection are all ineligible, and with the consent integration disabled every channel is ineligible with `CONSENT_SOURCE_UNAVAILABLE`. A database check additionally refuses any row whose stored decision and stored verdict disagree.
- **Suppression is absence, not a state.** Privacy- and service-recovery-suppressed relationships receive no target row at all and appear only in the snapshot aggregates, so the channel-eligibility vocabulary is exactly two values and there is no state meaning "cleared to send". Structural suppression reuses Section 8A's own reason codes rather than inventing coarser ones.
- **Audience source:** the Section 8B segment engine and nothing else. No rule column, no SQL parameter, no customer-list upload. A `BRANCH` campaign may only use a `BRANCH` segment for its own branch; a `STALE`, `FAILED` or unevaluated dynamic source is refused with `CRM_CAMPAIGN_AUDIENCE_NOT_CURRENT` rather than silently frozen. Exclusions are bounded at 10 segments; static exclusions use generation 0 and dynamic exclusions use only their current successful definition/generation.
- **Immutable versions:** editing a campaign mints a new version and leaves the old one, and every snapshot points at the exact version it was built from, so an edit mid-preparation cannot mix two definitions. The definition hash is order-independent, so reordering channels, exclusions or variants is correctly recognised as no change. A `PREPARED` campaign returns to `DRAFT` on edit rather than inheriting an audience the new definition never described. Scope is immutable after creation.
- **Generation pinning:** the `GenerationRetainer` port Section 8B deliberately left open is now implemented, so a segment rebuild cannot delete the membership a queued or current campaign snapshot depends on. Section 8B's `cleanupSupersededGenerations` was changed to take the evaluation row's lock before asking the retention question, and campaign enqueue takes the same lock before pinning - closing a genuine race in which both could have decided the other was not there.
- **Preparation worker:** the snapshot row is its own durable job (lease + `SKIP LOCKED` + heartbeat + bounded retry + dead-letter + a code-level replay hook), matching the Section 8B materializer rather than introducing a second worker system. A partial audience can never become current: the new snapshot is built alongside the old, promoted in one transaction that re-sweeps under a profile-row fence and recomputes the counts from the surviving rows, and a failure at any point before that leaves the previous `CURRENT` snapshot and the campaign's pointer to it untouched. Batch size 500. Only one preparation per campaign can be in flight, enforced by a partial unique index rather than a process-local guard.
- **Experiments:** integer basis points totalling exactly 10000, at most one control (at most 50%), one to five treatment variants. Assignment is `sha256(campaign_version, profile_ref, namespace) mod 10000` laid over cumulative ranges in a canonical order - deterministic, reproducible without a stored lookup, stable within a campaign version, and taking no contact data as input. A campaign without an experiment produces no cohort at all rather than a synthetic treatment group of everyone.
- **Policies:** frequency caps and quiet hours are validated, versioned and immutable, and enforced nowhere here. `VENDOR_CAMPAIGN_CONTACT_HISTORY = NOT_PRESENT`: nothing has ever been delivered, so a cap evaluated against an empty history would always pass. Frequency therefore reports `NOT_AVAILABLE` with a mandatory recheck, never "eligible". Quiet hours are minute-of-day plus a mandatory IANA zone (`time/tzdata` is embedded so validation works on Windows and slim containers), handle cross-midnight windows, and are evaluated only against an explicit hypothetical instant a caller supplies. No scheduler was written.
- **Service recovery:** the Vendor-local signal is an `OPEN`/`IN_PROGRESS` CRM task linked to a `SERVICE_FOLLOW_UP` interaction - both halves required, so an ordinary open task does not suppress an audience. It is evaluated live on reads rather than stored, because a complaint can be resolved at any moment and a stored flag would be stale. `EXTERNAL_SERVICE_RECOVERY_SUPPORT = NOT_PRESENT`; no support-dispute state is read or fabricated. Section 8C was not modified.
- **Privacy and merge:** registered through the same `PrivacyHandler` port Sections 8B-8D use. Unlike those domains it revokes current actionability rather than deleting evidence: the frozen snapshot stays auditable while the customer stops being contactable the moment the instruction commits. A merge revokes the source rather than moving its targets onto the survivor, leaving exactly one actionable row for the merged identity.
- **Audit/outbox:** created, updated, archived, preparation.started and prepared all write transactionally. Events carry the audience's identity and size and never its membership; a customer reference in a campaign event fails the build.
- **Flags:** `VENDOR_CRM_CAMPAIGNS_ENABLED` and `VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED`, both safe-default false. Campaigns require `VENDOR_CRM_ENABLED` **and** `VENDOR_CRM_SEGMENTATION_ENABLED` (the audience has no other source); preparation requires campaigns. Freezing preparation never disturbs an audience that is already prepared.
- **Segmentation boundary:** the Section 8B field registry remains at exactly 22 fields. Campaign membership, campaign counts and campaign response are deliberately not segmentable; adding a field would silently change what every stored audience means.
- **Customer 360:** unchanged. Campaign targeting history is not surfaced per customer, so the `campaigns` module still reports `NOT_IMPLEMENTED` - communication history belongs after execution exists.
- **Tests:** real PostgreSQL coverage includes the full consent matrix and fail-closed behaviour, per-channel decisions, dual-time evidence immutability, privacy/merge/service-recovery suppression and their post-snapshot revocation, static and dynamic sources, stale/failed source refusal, exact generation pinning, exclusion precedence, funnel reconciliation on every snapshot, last-good-snapshot preservation and terminal dead-lettering with replay, atomic supersede, edit-during-preparation version binding, tenant/branch/segment/snapshot/target IDOR, HTTP permission separation and edge idempotency, schema- and wire-level PII and delivery-machinery guards, audit and outbox payload inspection, migration replay/upgrade/rollback/reapply with constraint and index assertions, bidirectional OpenAPI parity, six deterministic lock-ordered concurrency races (duplicate prepare, redaction committing first, redaction committing second, merge during preparation, service recovery during preparation, segment rebuild versus a pinned generation), and a 1,000-relationship fixture covering batch boundaries, pagination completeness, cohort spread and index coverage.
- **No authority expansion:** no campaign scheduling, delivery, notification handoff, provider callback, delivery history, send-time enforcement, template, corporate CRM, RFM/churn scoring, sensitive export, internal route, external-service change, frontend, or Section 8F+/Section 9+ implementation.

**Known dependencies, disclosed rather than fabricated.** No consent authority publishes to this platform yet, so with the integration flag off every channel is correctly ineligible and a prepared audience contacts nobody - the section is built to be safe in exactly that state rather than to pretend otherwise. No vendor commerce order publisher exists, so commercial segment rules still match nobody (inherited from Section 8B, unchanged here). Section 8E preview correctly has no contact history; Section 8F now owns the real handoff history used for frequency enforcement. The pre-existing `go test -p 1 ./...` serialisation caveat documented under Section 5 still applies to the shared external test database.

## Section 8F — campaign execution and Notification handoff

**Status: IMPLEMENTED and PostgreSQL-verified.** Migrations `000020_vendor_crm_campaign_execution` and `000021_vendor_section8f_permissions` add durable runs, deliveries, append-only transition history, contact history, delivery-event dedupe, four permissions, nine public operations, and one internal HMAC ingress. The execution flag defaults false.

Run creation binds the current `PREPARED` snapshot and exact immutable version. Workers use `SKIP LOCKED`; every handoff rechecks current consent, privacy/identity, service recovery, quiet hours, frequency, cancellation/pause, and channel availability under database fences. Notification receives an opaque customer reference and fixed delivery idempotency key, never destination PII. `HANDED_OFF` is distinct from `DELIVERED`. SMS is suppressed because the real Notification contract has no SMS channel. The current Notification Service has no delivery-event producer, so the secured Vendor receiver is implemented but production callbacks remain a disclosed dependency.

Adversarial tests cover corrected current-generation exclusions, consent withdrawal, privacy after snapshot, quiet-hour boundaries, sequential and concurrent frequency caps, cancellation, kill-switch behavior, Notification timeout retry identity, and event dedupe/out-of-order protection. Migration tests cover fresh replay and 8E-to-8F rollback/reapply. Sections 5–8E remain in the full serial regression gate.

## Section 8G — corporate customer CRM

**Status: IMPLEMENTED and PostgreSQL-verified.** Migrations `000022_vendor_crm_corporate_customers` and `000023_vendor_section8g_permissions` add seven tables (code counters, profiles, branch scopes, locations, cost centres, contacts, memberships), an additive corporate subject on the Section 8C interaction and task tables, five permissions and 41 public operations. No internal route. The corporate flag defaults false and requires `VENDOR_CRM_ENABLED`.

**What it owns.** The commercial relationship with an organization: identity and vendor-local `CORP-` reference, lifecycle (`PROSPECT`/`ACTIVE`/`ON_HOLD`/`INACTIVE`/`ARCHIVED`), business or branch-set scope, an account manager, contacts referencing Section 8A relationships, memberships, the organization's own sites, and cost-centre labels.

**What it does not own, and cannot express.** No corporate order, invoice, credit, budget, contract, quotation, wallet, catalogue entitlement, approval workflow, RFM/churn score or export. A cost centre carries no budget, limit, currency or balance. The lifecycle contains no commercial state, and the migration test asserts the absence directly rather than trusting review.

**Isolation is structural.** Every child reaches its parent through a composite key: `(id, business_id)` for tenancy and `(id, business_id, corporate_customer_id)` for nested resources, so PostgreSQL — not only the HTTP layer — refuses a membership pointing at another account's contact, or an interaction naming another vendor's site. Branch-scoped operators see business-wide accounts plus branch-set accounts covering a branch they hold, and cannot widen an account past their own scope.

**Concurrency is proved by lock-ordered races, not by timing.** Six deterministic tests pin contact creation against redaction (both commit orderings), contact creation against identity merge, membership and activity against merge, archive against task creation, complete against cancel, and two simultaneous primary-contact promotions. Each blocks the second transaction on a real row lock and asserts the surviving state, so a passing run is evidence rather than a fast machine.

**Section 8C was extended, not modified.** `customer_profile_id`/`original_profile_id` became nullable under `crm_interaction_subject_ck` and `crm_task_subject_ck`; Go structs kept `string` fields with SQL `COALESCE`, so no accepted Section 8C call site changed and the full 8C suite passes unaltered. The 8G rollback restores both columns to `NOT NULL` and drops the corporate columns, which the migration test asserts.

**Known dependencies, disclosed rather than fabricated.** No corporate ordering, invoicing or credit domain exists on this platform, so `MEMBERSHIP_ROLE=APPROVER` is a CRM label and approves nothing — deliberately, because an approval that authorises nothing is safer than one that appears to. Corporate accounts are not customers of `customer-service`: the organization exists only inside this vendor's tenancy, and only its people are platform customers. The pre-existing `go test -p 1 ./...` serialisation caveat documented under Section 5 still applies to the shared external test database.

## Section 8H — CRM intelligence, analytics and secure exports

**Status: IMPLEMENTED and PostgreSQL-verified.** Migrations `000024_vendor_crm_intelligence_exports` and `000025_vendor_section8h_permissions` add four tables (evaluations, scope states, customer scores, exports), six permissions and 10 public operations. No internal route. Both feature flags default false.

**The models.** `RFM_V1` scores recency, frequency and monetary as scope-relative quintiles computed from PostgreSQL `cume_dist`, so equal inputs always receive equal scores and a rerun over unchanged facts is byte-identical. `CHURN_V1` is an integer 0-100 heuristic derived from how overdue a relationship is against its own observed purchase cadence, plus absolute inactivity and engagement depth. Both carry a model version and a definition hash taken over a canonical descriptor, so changing a threshold — or changing what the model claims to do — changes its identity and a queued run stamped with the old one is superseded rather than writing scores under a hash that no longer exists.

**What the models refuse to do.** A relationship with no commercial projection is `SOURCE_NOT_AVAILABLE`. One with a single observed order is `INSUFFICIENT_HISTORY`, never high churn risk — the rule that stops a follow-up list filling with people who arrived yesterday. A scored cohort below five members is `INSUFFICIENT_COHORT`, because a quintile over three people reads exactly like a quintile over thousands.

**Generation safety.** Reuses the Section 8B shape: a durable evaluation row is the job, the generation is built alongside the live one, and the swap is a single transaction that re-certifies against privacy and recomputes counts from the survivors. A forced-failure test proves the previous generation stays live and current, and that no row from the failed generation is ever `is_current`.

**Segmentation.** The Section 8B registry grows from 22 fields to exactly 30. Score predicates read through a correlated subquery requiring `state='CURRENT'`, so an unavailable, stale or failed score yields NULL and matches no numeric comparison. Invalidation is dependency-scoped: a churn rebuild leaves an RFM-only segment `CURRENT`.

**Exports.** Six closed datasets, two line-oriented formats, a per-dataset versioned field allowlist, typed filters, a 100k default and 250k hard row ceiling with a byte cap, keyset-streamed reads, a lease-based generator, cancellation, retention sweeping and full audit. Every dataset whose rows are about an individual excludes redacted and merged-away relationships centrally in `buildExportQuery`, so a dataset added later cannot forget. No canonical contact detail, payment credential, notification destination or free-text CRM note appears in any dataset, asserted against the real generated bytes in both formats.

CSV rendering deterministically prefixes a single quote to text cells beginning with `=`, `+`, `-`, `@`, tab or carriage return, neutralising spreadsheet formula interpretation without modifying CRM source data. JSONL preserves native JSON value semantics.

### Section 8H completion report

EXPORT STORAGE:

- Initial support status: `PARTIAL`
- Gap: Media's existing private object contract used magic-signature MIME detection and therefore rejected legitimate sniff-less CSV/JSONL; it also had no closed export purpose.
- Minimal Media File Service change: additive, purpose-scoped private textual artifact support only; no generic media redesign and no export generation moved out of Vendor Service.
- DATA_EXPORT purpose: added to Media's closed purpose registry and constrained private in the database and use case.
- CSV: exactly `.csv` + `text/csv`; bounded UTF-8/control-byte/signature checks and standard-library CSV structural parsing.
- JSONL: exactly `.jsonl` + canonical `application/x-ndjson`; every non-empty line must be one valid JSON object. No additional MIME alias is repository-established or accepted.
- Private-only enforcement: `DATA_EXPORT + PUBLIC` is rejected server-side and is unstorable under Media database checks.
- Safe-text validation: non-empty, valid UTF-8, NUL/C0-control rejection, executable/archive/binary-signature and markup rejection, exact extension/MIME pairing, bounded parsing, checksum and stored-size verification. Existing purpose signature detection is unchanged.
- Max artifact size: **20 MiB (20,971,520 bytes)**, purpose-specific and clamped to Media's global upload ceiling; Vendor uses the same 20 MiB ceiling.
- Content-Disposition: `attachment` with the validated, header-sanitised filename.
- Nosniff: `X-Content-Type-Options: nosniff`; private content also uses `Cache-Control: no-store`.
- Artifact reference: opaque Media object id only. Vendor persists `artifact_ref`, `checksum_sha256`, `artifact_bytes`, `row_count`, `completed_at` and `expires_at`; it persists no path, URL, credentials or bytes.
- Short-lived access: Vendor rechecks actor permission, tenant/branch scope, `READY`, expiry and purpose-bound single-use Step-Up, audits access, then asks Media for its object-specific token/path grant (five-minute default). Nothing permanent is stored.
- Artifact expiry: approximately 24 hours. Vendor supplies the same `retentionUntil` to Media, refuses and marks expired jobs, and invokes Media's existing idempotent deletion semantics on expiry/cancellation/failure cleanup.
- Media migrations: `000004_data_export_artifacts` adds visibility/file-name metadata, private-only checks and the export-retention index; no existing table semantics are weakened.
- Media tests: validator and real-PostgreSQL contract coverage proves private CSV/JSONL success; PUBLIC, wrong extension/MIME, unsupported alias, binary/NUL/control/invalid UTF-8, malformed/oversized content and HTML/SVG/executable bypass refusal; existing-purpose regression; short-lived private attachment access; completed-upload reconciliation; and deletion denial.
- Vendor↔Media contract: generated CSV/JSONL is checksum-verified, uploaded as private `DATA_EXPORT` under the stable export id, retained for 24 hours, and reconciled after an ambiguous completion. `READY` occurs only after verified persistence; storage failure remains non-READY; Step-Up gates download; expired artifacts are refused and deleted. Adapter tests also pin Media retention and deletion-idempotency headers.
- Final support status: `AVAILABLE`
- Result: `PRIVATE_EXPORT_ARTIFACT_SUPPORT = AVAILABLE`

EXTERNAL SERVICES:

- media-file-service: `MINIMAL SECTION 8H CHANGE — DATA_EXPORT private CSV/JSONL artifact support`
- All other external services: unchanged; no other concrete Section 8H requirement needs a cross-service modification.

**Two clock bugs the tests caught pre-commit.** The artifact expiry and the privacy fence were both computed from the service clock and compared against database-written values; on any host whose clock differs slightly that would have published artifacts past their retention and made every export look like it raced a redaction. Both now read the database clock, which is the one that stamped the values they are compared against.

**Known dependencies, disclosed rather than fabricated.** No vendor commerce order publisher exists, so a production deployment will legitimately report `NOT_AVAILABLE` for every score until authoritative order facts arrive — the section is built to be honest in exactly that state rather than to fill it with zeros. `media-file-service` required one minimal change to accept an export artifact (see the change register). The pre-existing `go test -p 1 ./...` serialisation caveat documented under Section 5 still applies to the shared external test database.

## Section 8I — final CRM integration, hardening and acceptance

**Status: CERTIFIED.** No new CRM module, no new public or internal route, no new
migration. Vendor surface remains **194 routes**, migration tip remains
`000025_vendor_section8h_permissions`, segment registry remains **30 fields**.

**Defects fixed.**

1. *Internal authority route mounted under the public base.* `staffhttp.RegisterInternalRoutes`
   was called with the `/api/v1/vendor` group, exposing `POST /internal/staff/provision-owner`
   at `/api/v1/vendor/internal/staff/provision-owner`. Its OpenAPI, the Section 5
   contract package and the route register all specified the engine root. The
   registrar now takes the engine and mounts at `/internal`, matching the rule
   Section 7 set for the verification callback. Guarded by
   `TestInternalRoutesAreNeverMountedUnderThePublicBase`.
2. *Notification handoff inside a database transaction.* Section 8F called
   `notifier.Send` while holding the delivery, run and customer-profile row
   locks. The profile lock is the shared privacy fence, so the remote call could
   block redaction. Restructured to reserve-and-commit, call, then record in a
   second transaction, with a `HandoffReservation` window (default 2 minutes,
   comfortably beyond the client timeout) preventing a second worker from
   adopting an in-flight delivery. Duplicate handoff is prevented by the stable
   `NotificationRequestRef` idempotency key the remote side deduplicates on.
   Contact history remains written under the profile lock inside the reservation
   transaction, because the frequency cap counts those rows.

**Verification gap closed.** Section 8H's broad serial run exceeded the execution
window without reporting. Section 8I measured why and ran the suite in
deterministic groups on separate databases: the CRM package alone takes ~834s,
which is what overran. Groups: (A+C) all 21 non-CRM packages, (B) the CRM
package, (G) media-file-service. All three: 0 failures, **0 skips**.

**Added tests.** The global route/OpenAPI/register drift guard
(`internal/crmcontract`), the cross-module redaction and anti-resurrection
certification, the marketing-suppression semantics test, and two live-schema
authority scans proving CRM holds no canonical customer PII and loyalty holds no
monetary column.

**Known dependencies remain external, documented and fail-closed** — see the
dependency matrix in `PLATFORM_SERVICE_CHANGE_REGISTER.md`. No production path
fabricates an unavailable external truth.


---

## Section 8 completion — remaining original Section 8 requirements

Sections 8A-8I delivered the CRM architecture. This final-completion pass audited the ORIGINAL
Section 8 specification line by line against what was actually built and
completed the remaining CRM-owned runtime slice. Producer-owned facts remain
fail-closed until their authoritative service sends them; that is an
availability state, not a fabricated zero.

**Migrations added:** `000026_vendor_crm_section8_completion`,
`000027_vendor_section8_completion_permissions`, and
`000028_vendor_crm_section8_final_completion` (reversible additive completion
columns, configuration, bounds and trigger lifecycle). Vendor migration tip is
now `000028`.

**Routes added:** 2 public, 0 internal. Vendor surface **194 -> 196**.

### What is COMPLETE and active

**8.4 Loyalty tiers** — implemented end to end and active today, with no
external producer required. A vendor defines an ordered ladder of at most 8
thresholds; a customer's standing is DERIVED at read time from the Section 8D
ledger and never stored, so a credit, a reversal, an identity merge or a
threshold change is reflected immediately with no recalculation job and no
second copy of the truth to drift.

Decisions worth recording:

- **The input is lifetime credited points, net of reversed credit** — not the
  spendable balance. The original specification permitted either and required
  the choice be made and documented. Status recognises what a customer has done
  over the relationship, so redeeming points must not demote them. A test pins
  this: a customer who earns 1200 and redeems 1000 keeps their rung, and a
  customer whose credit is *reversed* surrenders it.
- **That quantity is computed from the ledger entries, not from the account's
  `lifetime_credited_points` column** — a correction a test forced. Section 8D's
  ledger is immutable, so reversing a credit posts a compensating REVERSAL entry
  rather than decrementing the counter; reading the counter would have let a
  credit posted in error confer permanent status, which is exactly what a
  reversal exists to undo. Summing credit-type entries that have not been
  reversed gets all three cases right at once: debits never appear so redeeming
  cannot demote, a reversed credit is excluded, and `MERGE_IN` is included so a
  relationship that absorbed another keeps the standing it earned.
- **Ranks are derived from thresholds, never supplied.** A caller-supplied rank
  could disagree with its own ordering and the ladder would have two answers to
  "which tier is higher".
- **Thresholds must be distinct.** Two rungs at one entry point make membership
  ambiguous, and an ambiguous customer-facing status is worse than no tiers.
- **NOT_AVAILABLE, NO_TIER and CURRENT are three different answers.** No ladder
  configured is not the bottom rung; a configured ladder not yet reached is not
  the bottom rung either.
- **Recognition, not benefits.** No discount, entitlement, multiplier or
  monetary column exists, and a live-schema scan test fails the build if one
  appears. Points are not money; payment-wallet-service remains the only
  monetary authority.
- Replacement is atomic and version-checked under a per-business advisory lock;
  superseded rungs are archived rather than deleted so audit records that
  referenced them still point at something.

### Final-completion runtime status

> The historical paragraph immediately below this heading described the
> pre-handoff state. It is superseded by the completion table and verification
> below: the runtime projections, ingress handlers, Customer 360 blocks and
> privacy/merge repair paths are now active.

`000026`-`000028` create and complete the tables every remaining original Section 8 requirement
needs, with the availability semantics and tenant-composite keys the accepted
sections use. The Go projections, consumers, workers and read models over
them are active behind the existing flags. These tables are not inert: their
nothing reads from them, so no surface reports a fabricated value — but neither
facts remain unavailable until their authoritative service sends them; no path
fabricates a value.

| Original requirement | Schema | Domain code | Status |
|---|---|---|---|
| 8.1 order_count distinct from completed | observed order count, order type and line facts | order ingress + commercial rebuild | COMPLETE |
| 8.1 category / item affinity | line facts and affinity projections | rebuildable business/branch rankings | COMPLETE |
| 8.1 / 8.3 service-recovery summary | support facts and recovery projection | authenticated support ingress + read block | COMPLETE |
| 8.3 order / RFQ / support refs on interactions and tasks | bounded related-ref columns | DTOs, revisions and history | COMPLETE |
| 8.3 automatic follow-up trigger engine | trigger configs/evaluations | idempotent system-authored tasks | COMPLETE |
| 8.4 customer value band | value definitions/projection | ordered thresholds and NO_BAND | COMPLETE |
| 8.5 corporate commercial projection | corporate commercial projection | order-type and spend projection | COMPLETE |
| 8.5 corporate RFQ projection and quote conversion | RFQ facts/projection | bounded outcomes + basis points | COMPLETE |
| 8.6 relationship signals (7) | `crm_relationship_signals` | deterministic, availability-aware signals | COMPLETE |

The CRM event-ingress extensions for
order-line, support and RFQ facts; the 40-field segment registry; the Customer
360 completion blocks; and their privacy, merge, concurrency and availability
coverage are included in this final-completion slice. External producer
absence is represented as `NOT_AVAILABLE`, not as a CRM-owned gap.

### Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean.
- Migration replay from `000001` through `000028` passes; `000028` rolls back
  and reapplies cleanly, leaving Sections 5-8I intact.
- The global drift guard (`internal/crmcontract`) passes at 196 routes with
  both new routes documented in OpenAPI and the route register. It caught them
  as undocumented the moment they were registered, which is what it is for.
- 10 real-Postgres loyalty tier tests pass, including the ledger-following,
  redemption-does-not-demote, concurrent-editor, tenant-scope and
  no-monetary-column cases.

One real defect was found and fixed by these tests before commit: a customer
with no loyalty account was told their state was NO_TIER but was not told which
rung they were climbing toward, so the record answered half the question a CRM
user opens it to ask.

## Section 9A — Food Catalogue Foundation — IMPLEMENTED / TESTED

Section 9A answers one question: **what does this vendor branch sell?** It is
the management-side authoritative catalogue and nothing else.

| Section | Scope | Status |
|---|---|---|
| 9A | Food catalogue foundation — menus, sections, items, variants, modifier groups, modifiers, relationships | **IMPLEMENTED / TESTED** |
| 9B | Pricing, availability, scheduling and operational profiles | **IMPLEMENTED / TESTED** |
| 9C | Discovery, integration and final Section 9 acceptance | **IMPLEMENTED / TESTED** |

### Section 9B — Pricing, Availability, Scheduling & Operational Profiles

### Section 9C — Discovery and integration

Section 9C is read-only service-facing composition. Catalogue/search, revision,
sellability and price-resolution routes are mounted only under
`/internal/vendor/v1/food`; they are never reachable through the public vendor
gateway and no frontend route is fabricated. Search is bounded and parameterized,
branch/capability scoped, and responses carry catalogue revision and generated-at
freshness markers. It reuses the Section 9B sellability and pricing authorities;
no migration or external service change was required. The global route surface is
262 operations (257 pre-9C plus five internal discovery routes).

### Sections 10/11 — production foundation and management surface

Migration `000033_vendor_production_foundation` establishes the shared capacity
engine for Cloud Kitchen and Pastry/Bakery. Migrations `000034` through
`000039` now adds worker lease/replay state and immutable change-decision history
after the separated IAM, slot-template fields, Cloud Kitchen packages/plans,
and first-class Pastry/Bakery closure schema. The current
management surface includes bounded slot generation, recurrence materialization,
production jobs, Cloud Kitchen lifecycle APIs, pastry lifecycle APIs, immutable
specification diff/change requests, quote revision/expiry/atomic acceptance,
milestones and conversion intents. The current Vendor route surface is 362
operations. Final domain certification is still pending targeted database
workflow and concurrency coverage.

Section 9B is implemented additively over the 9A catalogue. Migrations
`000031_vendor_food_operations` and `000032_vendor_section9b_permissions` add
timezone-aware schedules, operational availability, branch overrides,
append-only price history, effective price/availability resolution, preparation
and logistics profiles, the fail-closed inventory seam, and catalogue
revisions. Twenty-two authenticated management routes are registered; no
discovery or internal routes are introduced. The isolated real-Postgres food
regression and route/OpenAPI drift guard pass.

### What exists

`internal/features/food` implements the hierarchy — menu → section → item →
{variants, attached modifier groups → modifiers} — over eight tables created by
`000029_vendor_food_catalogue`, with three permissions seeded by
`000030_vendor_section9a_permissions`. Thirty-nine public management routes are
registered under `/api/v1/vendor/food`; **zero internal routes**, because
nothing outside this service produces a menu and Section 9C owns the
read/consumption contract.

Load-bearing decisions, each with a test that fails if it is reversed:

- **Ownership is a database fact.** Composite foreign keys mean a section
  addressed through the wrong menu, a variant mutated through the wrong item, a
  modifier reached through the wrong group and a group borrowed from another
  business are all unstorable, not merely refused by a service check.
- **Money is integer minor units** with a bounded range enforced by CHECK. The
  business's currency (Section 6) is read for the response and never copied onto
  a catalogue row, so nothing can disagree with it.
- **Ordering is deterministic by construction.** Create, archive and reorder all
  end in one `resequence` routine, so live rows are always contiguous from zero
  with archived rows behind them. A deferred unique `(parent, position)`
  constraint per collection means an atomic permutation is legal in flight and
  provably unique at commit.
- **Archive, never delete**, with one exception: the item/modifier-group
  relationship row, which carries no historical authority of its own.
- **Modifier groups are business-level reusable definitions.** A branch-scoped
  grant may read, attach, detach and reorder them on its own branch's items but
  may not edit the shared definitions, because a group edited from one branch
  changes every other branch's items.
- **Publication reads Section 6/7 rather than re-deciding it.** A DRAFT
  catalogue can be prepared while the business is still onboarding; ACTIVE
  requires the business, branch and branch capability to all be ACTIVE.
- **Media stays opaque.** References are proven against Media File Service's
  existing private ownership API before storage; with no media authority
  configured the write is refused rather than storing an unverified reference.

### What is deliberately absent

No schedules, sold-out state, branch price or preparation overrides, effective
price/availability resolver or price history — those are 9B, and the schema
takes them additively. No discovery, projection, search or internal consumption
API — that is 9C. No orders, cart, checkout, inventory, dispatch or wallet call:
a catalogue price is commercial configuration, not settlement.

The item and menu lifecycles cannot express `SOLD_OUT`; a contract test fails if
one is added, so 9B's availability engine cannot be half-invented here.

### Verification

- `go build ./...`, `go vet ./...` and `gofmt -l` clean; `git diff --check` clean.
- Migrations `000029`/`000030` apply on the Section 8 tip, replay on a fresh
  database, and roll back and reapply without touching Sections 5-8. The
  structural guard asserts the ownership keys, the deferred position
  constraints, the query indexes and the permission seed — and that the database
  itself refuses a non-food capability, a negative price, an impossible
  selection range and an archived row without `archived_at`.
- The Section 8I global drift guard (`internal/crmcontract`) passes at **235
  routes** with all 39 new routes documented in
  `section9a-food-catalogue.openapi.json` and `API_ROUTE_REGISTER.md`. It caught
  them as undocumented the moment they were registered, which is what it is for.
- `internal/section9acontract` pins the 39-operation surface in both directions
  and fails if the contract ever describes a floating-point price, restates
  currency, exposes an upload/download or byte field, adds a Section 9B or 9C
  path, or makes a nutritional or medical claim about a dietary code.
- Real-Postgres feature, security, IAM, nested-IDOR, concurrency, HTTP and scale
  tests pass in `internal/features/food`, including: cross-tenant and
  cross-branch refusal on every read and write path, the analyst read-only
  shape, the branch-manager scope, the role/permission matrix asserted against
  the real migration seed, duplicate/foreign/incomplete reorder refusal,
  simultaneous-edit and duplicate-attach races resolving to exactly one winner,
  outbox-and-state committing together with no orphan on rollback, and an
  idempotent create retry that replays rather than duplicating.

### Defect found and fixed during implementation

None in accepted Sections 5-8. One design fault was caught by the tests inside
this section before it could ship: archiving a menu section originally left its
items behind, reachable by id but attached to a section no longer in the
catalogue. Archival now cascades to the items in the same transaction, and
`TestSection_CreateListReorderArchive` fails if it stops doing so.

## Section 12A — Advanced Shop / Grocery / Market / Wholesale: IMPLEMENTED

Section 12A implements the Vendor management and commercial foundation only.
The fully enabled production router now contains **443 routes**, including 58
new authenticated `/shop` operations. Migration tip is
`000043_vendor_shop_permissions`.

Implemented scope:

- branch operating models with capability mapping for supermarket, grocery,
  foodstuff dealer, market trader and wholesaler;
- bounded hierarchical taxonomy with tenant-scoped parent keys, ordering,
  depth checks, cycle prevention and optimistic concurrency;
- product masters, variants/SKUs, handling/temperature/physical profiles,
  opaque ordered media, package relationships, identifiers and exact lookup;
- first-class measurement definitions/families, platform and tenant scopes,
  tenant-scoped aliases on both platform and vendor definitions, standard
  integer base units, Nigerian local units, product/variant and optional branch
  calibrations, exact versus calibrated-approximate
  conversions, rational fractions and price portions;
- branch listings, retail/wholesale coexistence, sell-unit selection,
  branch-only calibration assignment without promoting it to business truth,
  deterministic price precedence, MOQ/order multiples, non-overlapping
  quantity bands, overflow guards and append-only price versions;
- product-applicable fresh attributes for produce, tubers, plantain, fish,
  meat and vegetables, plus variable-measurement tolerances in basis points;
- monotonic scoped catalogue revisions, separated IAM grants, expected-version
  writes, shared idempotency, transactional audit and transactional outbox;
- repository-level Shop/Food/Cloud Kitchen/Pastry isolation and SQL ownership
  predicates for business, branch and nested resources, plus database category
  and packaging cycle guards;
- batched product, variant and fresh-option list hydration at the 2,000-product
  / 5,000-variant moderate fixture;
- dedicated strict command/query OpenAPI plus bidirectional Section 12A and
  global route parity.

Part A deliberately introduced none of the inventory authority subsequently
implemented by Section 12B. No Order, Wallet/payment or logistics execution
authority is present in either part.

## Section 12B — Shop inventory, availability and discovery: IMPLEMENTED

Section 12B completes the Vendor-owned Shop operational authority. Migration
tip is `000046_vendor_shop_inventory_permissions`; the fully enabled router has
**496 routes**: 42 authenticated inventory/bulk routes and 11 HMAC-v2 internal
discovery/reservation routes added to the 443-route Section 12A baseline.

Implemented scope:

- branch/variant stock bases, non-negative materialized balances, append-only
  movements and monotonic inventory/catalogue revisions;
- draft/post/cancel receipts, lot/batch/expiry evidence, FEFO/FIFO allocation,
  synchronous expired-lot exclusion and reconciliation;
- atomic multi-line reservations with confirm, commit, release, cancel and
  expiry transitions, controlled preorder/backorder policy and allocation;
- controlled adjustments, quarantine/release/disposal, inter-branch transfers
  with partial receipt and damaged/missing resolution, and stocktakes with
  variance posting;
- variable fulfilment measurements, reorder rules, complete alert generation
  and ACK/RESOLVE lifecycle;
- atomic and dry-run bulk catalogue, price, listing and inventory operations
  with leases, bounded retry, dead-letter state and row-level reports;
- repeatable-read discovery projections with retail/wholesale price precedence,
  MOQ/multiple, fresh/handling metadata, sellability, availability and both
  catalogue and inventory revisions;
- fail-closed IAM and branch scope, nested IDOR protection, HMAC caller
  allowlisting/nonces, idempotency fingerprints, transactional audit/outbox,
  concurrency hard gates and 10,000-product/20,000-variant scale coverage.

Authority boundary: Vendor owns catalogue, inventory and availability truth,
not customer Orders, checkout, payment/Wallet settlement, purchasing/suppliers,
or Dispatch/Taxi/Hauling execution. Internal reservation callers must provide
their own source reference; a reservation is not an Order or payment promise.
