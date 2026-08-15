# API Route Register

## Section 7 registered routes

All public paths are relative to `/api/v1/vendor`, use the authenticated Vendor staff scope, and are described by `section7-onboarding-verification.openapi.json`. Every public mutation requires `Idempotency-Key`; the bank mutation additionally consumes a `finance.bank_change` step-up grant. The internal callback row is an absolute root-level path, not relative to the public base; it is HMAC-v2, allowlisted to `verification-compliance-service`, nonce protected, event-deduped, and source-version guarded.

**Section 7 contract correction:** the route-composition audit found `/api/v1/vendor/internal/vendor/v1/verification-events` accidental. The registered route is `POST /internal/vendor/v1/verification-events`, following the established `/internal/<domain>/v1` convention. Route count is unchanged. VerificationStatus uses `PENDING_REVIEW`, never plain `PENDING`.

| Service | Method | Path | Surface | Auth | Permission | Purpose | Idempotency / concurrency | Flag | Status |
|---|---|---|---|---|---|---|---|---|---|
| vendor-service | GET | /onboarding | Public | bearer | vendor.onboarding.read | Overview | read | none | VERIFIED |
| vendor-service | GET | /onboarding/progress | Public | bearer | vendor.onboarding.read | Progress | read | none | VERIFIED |
| vendor-service | GET | /onboarding/checklist | Public | bearer | vendor.onboarding.read | Data-driven requirements | read | none | VERIFIED |
| vendor-service | GET | /onboarding/blockers | Public | bearer | vendor.onboarding.read | Blockers | read | none | VERIFIED |
| vendor-service | GET | /onboarding/activation-readiness | Public | bearer | vendor.onboarding.read | Readiness | read | none | VERIFIED |
| vendor-service | POST | /onboarding/submit | Public | bearer | vendor.onboarding.manage | Submit onboarding | Idempotency-Key | onboarding writes | VERIFIED |
| vendor-service | POST | /onboarding/request-activation | Public | bearer | vendor.onboarding.manage | Request policy evaluation, never force activation | Idempotency-Key | onboarding writes | VERIFIED |
| vendor-service | GET | /onboarding/representative | Public | bearer | vendor.onboarding.read | Read representative | read | none | VERIFIED |
| vendor-service | PUT | /onboarding/representative | Public | bearer | vendor.onboarding.manage | Set/update representative | Idempotency-Key + expected_version; step-up after verification | onboarding writes | VERIFIED |
| vendor-service | GET | /onboarding/documents | Public | bearer | vendor.onboarding.read | List opaque Media refs | read | none | VERIFIED |
| vendor-service | POST | /onboarding/documents | Public | bearer | vendor.onboarding.manage | Attach Media ref after authority read-back | Idempotency-Key | onboarding writes | VERIFIED |
| vendor-service | PATCH | /onboarding/documents/:documentId | Public | bearer | vendor.onboarding.manage | Replace draft ref | Idempotency-Key + expected_version | onboarding writes | VERIFIED |
| vendor-service | DELETE | /onboarding/documents/:documentId | Public | bearer | vendor.onboarding.manage | Remove draft ref | Idempotency-Key + expected_version | onboarding writes | VERIFIED |
| vendor-service | GET | /onboarding/verifications | Public | bearer | vendor.verification.read | List projections | read | none | VERIFIED |
| vendor-service | GET | /onboarding/verifications/:dimension | Public | bearer | vendor.verification.read | Read dimension | read | none | VERIFIED |
| vendor-service | POST | /onboarding/verifications/:dimension/submit | Public | bearer | vendor.verification.submit | Create authority case | Idempotency-Key + durable client reference | onboarding + verification integration | VERIFIED |
| vendor-service | POST | /onboarding/verifications/:dimension/resubmit | Public | bearer | vendor.verification.submit | Remediation case | Idempotency-Key + durable client reference | onboarding + verification integration | VERIFIED |
| vendor-service | GET | /onboarding/bank-profile | Public | bearer | vendor.bank_profile.read | Masked Wallet projection | read | none | VERIFIED |
| vendor-service | PUT | /onboarding/bank-profile | Public | bearer | vendor.bank_profile.manage | Resolve/replace onboarding bank profile | Idempotency-Key + expected_version + single-use step-up | onboarding writes | VERIFIED |
| vendor-service | POST | /internal/vendor/v1/verification-events | Internal | HMAC v2 + nonce | verification-compliance-service only | Apply authoritative callback | event_id dedupe + source_version | verification integration | VERIFIED |

Section 7 adds 19 public Vendor routes and 1 internal Vendor route: 20 new, 73 cumulative Vendor operations.

### Section 7 external authority routes

| Service | Method | Full path | Auth/caller | Purpose | Status |
|---|---|---|---|---|---|
| verification-compliance-service | POST | /api/v1/verification-compliance/internal/vendor-cases/cases | HMAC v2 + nonce; vendor-service | Idempotent case creation | VERIFIED |
| verification-compliance-service | GET | /api/v1/verification-compliance/internal/vendor-cases/cases/by-client-reference/:clientReference | HMAC v2 + nonce; vendor/admin | Unknown-outcome read-back | VERIFIED |
| verification-compliance-service | GET | /api/v1/verification-compliance/internal/vendor-cases/cases/:caseRef | HMAC v2 + nonce; vendor/admin | Authoritative case read | VERIFIED |
| verification-compliance-service | POST | /api/v1/verification-compliance/internal/vendor-cases/cases/:caseRef/decisions | HMAC v2 + nonce; admin only | Source-versioned decision | VERIFIED; admin UI dependency remains |
| payment-wallet-service | POST | /api/v1/payment-wallet/internal/vendor-bank-profiles/resolve | HMAC v2; vendor-service only | Safe resolution preview | VERIFIED |
| payment-wallet-service | PUT | /api/v1/payment-wallet/internal/vendor-bank-profiles/:businessId | HMAC v2; vendor-service only | Idempotent resolve/register/default | VERIFIED |
| payment-wallet-service | GET | /api/v1/payment-wallet/internal/vendor-bank-profiles/:businessId | HMAC v2; vendor-service only | Masked authority read-back | VERIFIED |
| media-file-service | GET | /api/v1/media-files/internal/media/objects/:id | Existing HMAC v2 + nonce; Vendor deployment allowlist | Tenant/owner object validation | REUSED, no route change |

Human-readable master catalogue of every Vendor Commerce route, across every affected service. `docs/openapi/` is the machine-readable authority; this index is cross-checked by the Section 5 and Section 6 route-parity contract tests.

All routes below are mounted under `/api/v1/vendor` (`shared/go/serviceapp`'s `APIBase`). `GET /health`, `GET /ready`, `GET /api/v1/vendor/meta` also exist — platform-standard infrastructure every service gets for free, not listed here.

## Columns

Service | Method | Path | Public/Internal/Admin | Auth | Permission | Purpose | Main response | Idempotency required? | `expectedVersion` required? | Feature flag | Implementation section | OpenAPI file | Status

## Register

| Service | Method | Path | Public/Internal/Admin | Auth | Permission | Purpose | Main response | Idempotency? | expectedVersion? | Feature flag | Section | OpenAPI file | Status |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| vendor-service | POST | /auth/start | Public | none | — | Start passwordless OTP login | challenge_id, expires_in | No (challenge-id/resend semantics own idempotency) | No | none (core auth, always on) | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | POST | /auth/resend | Public | none | — | Resend the active OTP challenge | challenge_id, expires_in | No | No | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | POST | /auth/verify | Public | none | — | Verify OTP, issue tokens (or request vendor choice) | access_token, refresh_token, staff_id, vendor_id | No (atomic consume-once guard) | No | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | POST | /auth/refresh | Public | refresh token | — | Rotate a refresh session | access_token, refresh_token | No | No | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | POST | /auth/logout | Public | refresh token | — | Revoke a refresh session + its step-up grants | logged_out: true | No | No | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | POST | /auth/step-up/start | Authenticated | vendor staff bearer | — | Start step-up authentication for a high-risk operation | challenge_id, expires_in | No | No | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | POST | /auth/step-up/verify | Authenticated | vendor staff bearer | — | Verify step-up OTP, issue a single-use grant | grant_id, expires_in | No (atomic consume-once guard) | No | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | GET | /staff | Authenticated | vendor staff bearer | vendor.staff.read | List staff at the caller's vendor | staff[] | — | — | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | GET | /staff/:id | Authenticated | vendor staff bearer | vendor.staff.read | Read one staff member (vendor-scoped) | staff detail | — | — | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | POST | /staff | Authenticated | vendor staff bearer | vendor.staff.manage | Invite a new staff member | staff detail | No (409 on already-staff) | No | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | PUT | /staff/:id/suspend | Authenticated | vendor staff bearer | vendor.staff.suspend | Suspend a staff member, revoke their sessions | suspended: true | No | Yes | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | PUT | /staff/:id/reactivate | Authenticated | vendor staff bearer | vendor.staff.suspend | Reactivate a suspended staff member | reactivated: true | No | Yes | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | GET | /staff/:id/roles | Authenticated | vendor staff bearer | vendor.staff.read | List a staff member's active role assignments | assignments[] | — | — | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | GET | /permissions | Authenticated | vendor staff bearer | vendor.roles.read | List the global permission catalogue | permissions[] | — | — | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | GET | /roles | Authenticated | vendor staff bearer | vendor.roles.read | List system role templates + this vendor's custom roles | roles[] | — | — | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | GET | /roles/:id | Authenticated | vendor staff bearer | vendor.roles.read | Read one role + its permission codes | role detail | — | — | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | POST | /roles | Authenticated | vendor staff bearer | vendor.roles.manage | Create a vendor-scoped custom role | role detail | No | No | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | PUT | /roles/:id/permissions | Authenticated | vendor staff bearer | vendor.roles.manage | Replace a custom role's full permission set | updated: true | No | Yes | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | POST | /roles/:id/assign | Authenticated | vendor staff bearer | vendor.roles.assign | Assign a role to a staff member, optionally branch-scoped | assignment detail | No (409 on duplicate active assignment) | No | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | DELETE | /assignments/:id | Authenticated | vendor staff bearer | vendor.roles.assign | Revoke a staff role assignment | unassigned: true | No | No | none | 5 | section5-auth-staff-iam | VERIFIED |
| vendor-service | POST | /internal/staff/provision-owner | Internal | shared/go/serviceauth HMAC | — | Provision the first staff member (Owner) for a new vendor business | staff detail | Yes (idempotent by vendor_id+identifier) | No | none | 5 | section5-auth-staff-iam | VERIFIED |

### Section 6 registered routes

All rows use vendor staff bearer authentication, are business-scoped from the resolved session, and are documented by `section6-vendor-business.openapi.json`. Writes are gated by `VENDOR_ONBOARDING_WRITES_ENABLED` and `COMMERCE_GLOBAL_KILL_SWITCH`.

| Service | Method | Path | Surface | Auth | Permission | Purpose | Response | Idempotency? | expectedVersion? | Flag | Section | OpenAPI | Status |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| vendor-service | GET | /capability-types | Authenticated | bearer | authenticated | Exact capability catalogue | types/families | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses | Authenticated | bearer | vendor.business.manage | Create scoped business | DRAFT business | Yes | No | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses | Authenticated | bearer | vendor.business.read | List session-scoped businesses | businesses | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId | Authenticated | bearer | vendor.business.read | Get business | business | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | PATCH | /businesses/:businessId | Authenticated | bearer | vendor.business.manage | Update profile fields | business | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/pause | Authenticated | bearer | vendor.business.manage | Operational pause | business | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/resume | Authenticated | bearer | vendor.business.manage | Operational resume | business | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId/capabilities | Authenticated | bearer | vendor.capabilities.read | List capabilities | capabilities | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/capabilities | Authenticated | bearer | vendor.capabilities.manage | Request capability | DRAFT capability | Yes | No | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId/capabilities/:capabilityId | Authenticated | bearer | vendor.capabilities.read | Get capability | capability | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/capabilities/:capabilityId/pause | Authenticated | bearer | vendor.capabilities.manage | Pause capability | capability | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/capabilities/:capabilityId/resume | Authenticated | bearer | vendor.capabilities.manage | Resume capability | capability | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId/brands | Authenticated | bearer | vendor.brands.read | List brands | brands | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/brands | Authenticated | bearer | vendor.brands.manage | Create brand | NOT_STARTED brand | Yes | No | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId/brands/:brandId | Authenticated | bearer | vendor.brands.read | Get brand | brand | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | PATCH | /businesses/:businessId/brands/:brandId | Authenticated | bearer | vendor.brands.manage | Update brand | brand | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId/branches | Authenticated | bearer | vendor.branches.read | List branches | branches | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/branches | Authenticated | bearer | vendor.branches.manage | Create branch | DRAFT branch | Yes | No | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId/branches/:branchId | Authenticated | bearer | vendor.branches.read | Get branch | branch | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | PATCH | /businesses/:businessId/branches/:branchId | Authenticated | bearer | vendor.branches.manage | Update branch | branch | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/branches/:branchId/pause | Authenticated | bearer | vendor.branches.manage | Pause branch | branch | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/branches/:branchId/resume | Authenticated | bearer | vendor.branches.manage | Resume branch | branch | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId/branches/:branchId/hours | Authenticated | bearer | vendor.branches.read | Get normalized hours | intervals | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | PUT | /businesses/:businessId/branches/:branchId/hours | Authenticated | bearer | vendor.branch.hours.manage | Replace normalized hours | intervals | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId/branches/:branchId/service-area | Authenticated | bearer | vendor.branches.read | Get radius area | service area | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | PUT | /businesses/:businessId/branches/:branchId/service-area | Authenticated | bearer | vendor.branch.settings.manage | Put radius area | service area | Yes | Yes (0=create) | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId/branches/:branchId/operations-settings | Authenticated | bearer | vendor.branches.read | Get explicit settings | settings | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | PUT | /businesses/:businessId/branches/:branchId/operations-settings | Authenticated | bearer | vendor.branch.settings.manage | Put explicit settings | settings | Yes | Yes (0=create) | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | GET | /businesses/:businessId/branches/:branchId/capabilities | Authenticated | bearer | vendor.capabilities.read | Effective branch capabilities | configured/effective states | No | No | none | 6 | section6-vendor-business | VERIFIED |
| vendor-service | PUT | /businesses/:businessId/branches/:branchId/capabilities/:capabilityId | Authenticated | bearer | vendor.branch.capabilities.manage | Configure branch capability | DRAFT configuration | Yes | Yes (0=create) | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/branches/:branchId/capabilities/:capabilityId/pause | Authenticated | bearer | vendor.branch.capabilities.manage | Pause one branch capability | branch capability | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |
| vendor-service | POST | /businesses/:businessId/branches/:branchId/capabilities/:capabilityId/resume | Authenticated | bearer | vendor.branch.capabilities.manage | Resume one branch capability | branch capability | Yes | Yes | onboarding writes | 6 | section6-vendor-business | VERIFIED |

## Section 8A registered routes

Six vendor-facing operations plus one root-mounted internal event ingress, described by `section8a-crm-foundation.openapi.json`. Public paths are relative to `/api/v1/vendor`; the internal row is an absolute root-level path, deliberately **not** nested under the public base and **not** gateway-exposed, following the Section 7 callback correction.

**Authority boundary.** There is deliberately no vendor-facing route that creates a customer, sets consent, sets privacy state, or merges identities. Those are authority decisions and reach CRM only through the signed internal ingress. `internal/section8acontract` asserts this structurally, so adding such a route fails the build.

**Branch scope.** Every read is scope-aware: a vendor-wide grant sees business-level totals; a branch-scoped grant sees only relationships at authorised branches, with figures recomputed from those branches alone.

| Service | Method | Path | Surface | Auth | Permission | Purpose | Idempotency / concurrency | Flag | Status |
|---|---|---|---|---|---|---|---|---|---|
| vendor-service | GET | /crm/customers | Public | bearer | vendor.crm.customers.read | List relationships; redacted and merged-away excluded | read; limit/offset max 100, allowlisted sort | VENDOR_CRM_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/customers/:profileId | Public | bearer | vendor.crm.customer360.read | Customer 360; no canonical PII | read | VENDOR_CRM_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/customers/:profileId/consent | Public | bearer | vendor.crm.consent.read | Projected consent + marketing eligibility | read | VENDOR_CRM_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/customers/:profileId/privacy | Public | bearer | vendor.crm.privacy.read | Projected privacy state | read | VENDOR_CRM_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/customers/:profileId/archive | Public | bearer | vendor.crm.customers.manage | Archive an inactive relationship; refused when redacted or merged | expected_version (or If-Match) | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/customers/:profileId/restore | Public | bearer | vendor.crm.customers.manage | Restore an archived relationship; refused when redacted or merged | expected_version (or If-Match) | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /internal/vendor/v1/crm/events | Internal | HMAC v2 + nonce | per-event-type caller allowlist | Apply authoritative relationship/consent/privacy/identity/order events | (source_service,event_id) dedupe + payload fingerprint + source_version | per-integration flag | IMPLEMENTED |

Internal event types and their owning authority: `CUSTOMER_RELATIONSHIP_OBSERVED`, `CUSTOMER_IDENTITY_LINKED`, `CUSTOMER_IDENTITY_MERGED`, `CUSTOMER_CONSENT_CHANGED`, `CUSTOMER_PRIVACY_CHANGED` (customer-service); `CRM_ORDER_FACT_CHANGED` (the authoritative vendor order publisher, which does not exist yet and stays disabled behind `VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED`). A caller may never send an event type it does not own.

Section 8A adds **no** step-up requirement: the reads are ordinary, and archive/restore are low-risk lifecycle operations. Sensitive CRM export belongs to Section 8H and will require step-up then.

**High-risk operations requiring a step-up grant** (in addition to the permission check): creating/editing a role or custom-role permission set that includes any `is_high_risk` permission (`vendor.finance.bank_change`, `vendor.owner.transfer`, `vendor.data.export_sensitive`), and assigning a role that carries one. Step-up purpose used: `iam.high_risk_permission_change`.

## Section 8B registered routes

Twenty-one vendor-facing operations and **no** internal ones, described by `section8b-crm-tags-segmentation.openapi.json`. All paths are relative to `/api/v1/vendor`. Section 8B consumes no external authority, so it introduces no signed callback: every input it acts on already belongs to Vendor Service.

**Authority boundary.** Section 8B produces audiences and nothing else. There is no route that sends a message, schedules a campaign, or grants marketing permission, and no permission that could later be read as one. Segment membership is never proof of consent — whatever section eventually does the sending must re-check consent and privacy at send time. `internal/section8bcontract` asserts this structurally, so a campaign-shaped route or a `send` permission fails the build.

**Rule safety.** Dynamic membership comes from a validated JSON rule language. A rule can only name a field published by `GET /crm/segment-fields`; field keys, operators and values are checked against a closed registry and reach the database only as bind parameters. There is no field, operator, sort or filter anywhere in this surface that accepts SQL, a column name or a raw predicate.

**Branch scope.** A `BUSINESS`-scope tag or segment requires a vendor-wide grant. A branch-scoped caller cannot create one, see one in a list or by id, or preview at business scope — the member count is itself a disclosure. A branch-scoped rule reads that branch's slice of every commercial fact rather than the business total. Customer 360 applies the narrower classification rule: branch callers receive only their permitted branches' tags and segments, never BUSINESS or other-branch classification.

| Service | Method | Path | Surface | Auth | Permission | Purpose | Idempotency / concurrency | Flag | Status |
|---|---|---|---|---|---|---|---|---|---|
| vendor-service | GET | /crm/segment-fields | Public | bearer | vendor.crm.segments.read | Publish the closed field/operator registry and complexity limits | read | VENDOR_CRM_SEGMENTATION_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/tags | Public | bearer | vendor.crm.tags.read | List tags with scope-filtered member counts | read; limit/offset max 200 | VENDOR_CRM_SEGMENTATION_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/tags | Public | bearer | vendor.crm.tags.manage | Define a tag; reserved `sys:`/`system:`/`karrygo:` prefixes refused | Idempotency-Key | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/tags/bulk-assign | Public | bearer | vendor.crm.tags.manage | Assign one tag to many relationships, all or nothing | Idempotency-Key; max 500 refs | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/tags/bulk-remove | Public | bearer | vendor.crm.tags.manage | Remove one tag from many relationships | Idempotency-Key; max 500 refs | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/tags/:tagId | Public | bearer | vendor.crm.tags.read | Read one tag | read | VENDOR_CRM_SEGMENTATION_ENABLED | IMPLEMENTED |
| vendor-service | PATCH | /crm/tags/:tagId | Public | bearer | vendor.crm.tags.manage | Rename/re-describe; scope is immutable | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/tags/:tagId/archive | Public | bearer | vendor.crm.tags.manage | Retire a tag; removes memberships, keeps history | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/segments | Public | bearer | vendor.crm.segments.read | List segments; branch callers never see BUSINESS-scope ones | read; limit/offset max 200 | VENDOR_CRM_SEGMENTATION_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/segments | Public | bearer | vendor.crm.segments.manage | Define a STATIC or DYNAMIC segment in DRAFT | Idempotency-Key | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/segments/preview | Public | bearer | vendor.crm.segments.read | Evaluate an unsaved rule; persists nothing | Idempotency-Key; sample max 100, count budget | VENDOR_CRM_SEGMENTATION_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/segments/:segmentId | Public | bearer | vendor.crm.segments.read | Read one segment with its current definition and freshness | read | VENDOR_CRM_SEGMENTATION_ENABLED | IMPLEMENTED |
| vendor-service | PATCH | /crm/segments/:segmentId | Public | bearer | vendor.crm.segments.manage | Rename or replace the rule; mints a new immutable version | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/segments/:segmentId/activate | Public | bearer | vendor.crm.segments.manage | DRAFT to ACTIVE | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/segments/:segmentId/archive | Public | bearer | vendor.crm.segments.manage | Retire a segment; terminal, drops membership | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/segments/:segmentId/materialize | Public | bearer | vendor.crm.segments.manage | Enqueue a rebuild; returns the in-flight run if one exists | Idempotency-Key; one queued run per definition | VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/segments/:segmentId/members | Public | bearer | vendor.crm.segments.members.read | Page the current generation only, with its freshness | read; limit/offset max 200 | VENDOR_CRM_SEGMENTATION_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/segments/:segmentId/members/bulk-add | Public | bearer | vendor.crm.segments.manage | Add explicit members (STATIC only) | Idempotency-Key; max 500 refs | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/segments/:segmentId/members/bulk-remove | Public | bearer | vendor.crm.segments.manage | Remove explicit members (STATIC only) | Idempotency-Key; max 500 refs | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/segments/:segmentId/exclusions/bulk-add | Public | bearer | vendor.crm.segments.manage | Exclude relationships from a DYNAMIC segment; survives rebuilds | Idempotency-Key; max 500 refs | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/segments/:segmentId/exclusions/bulk-remove | Public | bearer | vendor.crm.segments.manage | Lift manual exclusions | Idempotency-Key; max 500 refs | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |

**Permissions.** Five, not one per endpoint: `vendor.crm.tags.read`, `vendor.crm.tags.manage`, `vendor.crm.segments.read`, `vendor.crm.segments.manage`, `vendor.crm.segments.members.read`. Bulk tagging is the same authority as tagging, and previewing or materializing is the same authority as reading or managing a definition; splitting those apart would only create ways for a role to be half-granted. Listing a segment's actual members is separate, because naming the customers in an audience is a strictly larger disclosure than reading the audience's definition and size.

## Section 8C — interactions and tasks

| Service | Method | Exact path | Exposure | Auth | Permission | Purpose | Command contract | Flag | Status |
|---|---|---|---|---|---|---|---|---|---|
| vendor-service | GET | /crm/interactions | Public | bearer | vendor.crm.interactions.read | Filter and page the permitted relationship timeline | read | VENDOR_CRM_INTERACTIONS_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/interactions | Public | bearer | vendor.crm.interactions.create | Record a CRM interaction | Idempotency-Key | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/interactions/{interactionId} | Public | bearer | vendor.crm.interactions.read | Read one permitted interaction | read | VENDOR_CRM_INTERACTIONS_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/interactions/{interactionId}/correct | Public | bearer | vendor.crm.interactions.manage | Correct through append-only revision history | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/interactions/{interactionId}/revisions | Public | bearer | vendor.crm.interactions.read | Read immutable interaction revisions | read | VENDOR_CRM_INTERACTIONS_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/customers/{profileId}/interactions | Public | bearer | vendor.crm.interactions.read | Page one customer's permitted timeline | read | VENDOR_CRM_INTERACTIONS_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/tasks | Public | bearer | vendor.crm.tasks.read | Filter and page permitted CRM tasks | read | VENDOR_CRM_TASKS_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/tasks | Public | bearer | vendor.crm.tasks.create | Create a CRM follow-up task | Idempotency-Key | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/tasks/{taskId} | Public | bearer | vendor.crm.tasks.read | Read one permitted task | read | VENDOR_CRM_TASKS_ENABLED | IMPLEMENTED |
| vendor-service | PATCH | /crm/tasks/{taskId} | Public | bearer | vendor.crm.tasks.manage | Update mutable task details | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/tasks/{taskId}/start | Public | bearer | vendor.crm.tasks.manage | Transition OPEN to IN_PROGRESS | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/tasks/{taskId}/complete | Public | bearer | vendor.crm.tasks.manage | Complete an open/in-progress task | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/tasks/{taskId}/cancel | Public | bearer | vendor.crm.tasks.manage | Cancel an open/in-progress task | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/tasks/{taskId}/reassign | Public | bearer | vendor.crm.tasks.manage | Assign/unassign an eligible staff member | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/tasks/{taskId}/history | Public | bearer | vendor.crm.tasks.read | Read immutable task activity | read | VENDOR_CRM_TASKS_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/customers/{profileId}/tasks | Public | bearer | vendor.crm.tasks.read | Page one customer's permitted tasks | read | VENDOR_CRM_TASKS_ENABLED | IMPLEMENTED |

Section 8C adds exactly six permissions: `vendor.crm.interactions.read`, `vendor.crm.interactions.create`, `vendor.crm.interactions.manage`, `vendor.crm.tasks.read`, `vendor.crm.tasks.create`, and `vendor.crm.tasks.manage`. It adds no step-up grant, external callback, bulk operation, notification route, support-case route, or internal route.

**Step-up.** Section 8B adds none. Nothing here sends a message or exports customer data, so the friction would buy nothing. Campaign execution (8E/8F) and sensitive export (8H) will need their own.

## Section 8D — loyalty points

| Service | Method | Exact path | Exposure | Auth | Permission | Purpose | Command contract | Flag | Status |
|---|---|---|---|---|---|---|---|---|---|
| vendor-service | GET | /crm/loyalty/program | Public | bearer | vendor.crm.loyalty.read | Read the loyalty programme; unwritten defaults report version 0 | read (never creates a row) | VENDOR_CRM_LOYALTY_ENABLED | IMPLEMENTED |
| vendor-service | PATCH | /crm/loyalty/program | Public | bearer | vendor.crm.loyalty.manage | Configure status, points naming, expiry policy, redemption granularity, adjustment limit | Idempotency-Key + expected_version | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/customers/{profileId}/loyalty | Public | bearer | vendor.crm.loyalty.read | Read one customer's balance; NOT_ENROLLED when they hold no account | read (never creates a row) | VENDOR_CRM_LOYALTY_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/customers/{profileId}/loyalty/entries | Public | bearer | vendor.crm.loyalty.read | Page the immutable points ledger, newest first | read; fixed ordering, closed entry_type filter | VENDOR_CRM_LOYALTY_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/customers/{profileId}/loyalty/adjustments | Public | bearer | vendor.crm.loyalty.adjust | Manually credit or debit points with a mandatory reason | Idempotency-Key; bounded by manual_adjustment_limit | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/loyalty/entries/{entryId}/reverse | Public | bearer | vendor.crm.loyalty.reverse | Post a compensating REVERSAL; the original is never edited | Idempotency-Key; full reversal only, once per entry | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/loyalty/tiers | Public | bearer | vendor.crm.loyalty.read | Read the tier ladder; an unconfigured business reports an empty set at version 0 | read (never creates a row) | VENDOR_CRM_LOYALTY_ENABLED | IMPLEMENTED |
| vendor-service | PUT | /crm/loyalty/tiers | Public | bearer | vendor.crm.loyalty.tiers.manage | Atomically replace the whole tier ladder; ranks derived from thresholds | Idempotency-Key + expected_version; max 8 tiers, distinct thresholds | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |

Section 8D adds exactly four permissions: `vendor.crm.loyalty.read`, `vendor.crm.loyalty.manage`, `vendor.crm.loyalty.adjust`, and `vendor.crm.loyalty.reverse`. Adjust and reverse are flagged high-risk and are granted only to Owner, General Manager and CRM/Support; Branch Manager, Marketing, Analyst/Auditor and Finance receive read only; kitchen, inventory, procurement and rider receive nothing.

**No earn or redeem endpoint, and no internal route.** The earning and redemption engines exist as service-internal domain primitives (`CreditPoints`, `DebitPoints`) awaiting a trusted producer. Publishing either as a vendor-callable endpoint would let any authorised staff member name their own points total — a far larger authority than the bounded, reason-carrying manual adjustment above. No internal HTTP route is registered because no producer contract exists to receive on yet; the Vendor Order domain is Section 9 work.

**Step-up.** Section 8D adds none, and does not reuse the Section 7 mechanism. The abuse risk on a manual adjustment is bounded by the programme's own configurable limit, by a dedicated high-risk permission, by a mandatory audited reason and by full reversibility — none of which is true of the bank-detail changes step-up exists to guard.

## Section 8E — campaign intelligence and the audience engine

| Service | Method | Exact path | Exposure | Auth | Permission | Purpose | Command contract | Flag | Status |
|---|---|---|---|---|---|---|---|---|---|
| vendor-service | GET | /crm/campaigns | Public | bearer | vendor.crm.campaigns.read | Page campaigns visible to the caller's scope | read; fixed updated_at DESC ordering, closed filters | VENDOR_CRM_CAMPAIGNS_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/campaigns | Public | bearer | vendor.crm.campaigns.manage | Define a campaign in DRAFT with immutable definition version 1 | Idempotency-Key | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/campaigns/{campaignId} | Public | bearer | vendor.crm.campaigns.read | Read one campaign and its current definition | read | VENDOR_CRM_CAMPAIGNS_ENABLED | IMPLEMENTED |
| vendor-service | PATCH | /crm/campaigns/{campaignId} | Public | bearer | vendor.crm.campaigns.manage | Replace the definition; mints a new immutable version when anything changed | Idempotency-Key + expected_version; scope immutable | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/campaigns/{campaignId}/archive | Public | bearer | vendor.crm.campaigns.manage | Retire a campaign; ARCHIVED is terminal | Idempotency-Key + expected_version; re-archive is a no-op | VENDOR_CRM_WRITES_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/campaigns/{campaignId}/preview | Public | bearer | vendor.crm.campaigns.preview | Evaluate the current audience; persists nothing | read/evaluation; no Idempotency-Key because nothing is written | VENDOR_CRM_CAMPAIGNS_ENABLED | IMPLEMENTED |
| vendor-service | POST | /crm/campaigns/{campaignId}/prepare | Public | bearer | vendor.crm.campaigns.prepare | Enqueue a durable audience freeze; returns 202 with the BUILDING snapshot | Idempotency-Key; a run already in flight is returned, not duplicated | VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/campaigns/{campaignId}/snapshots | Public | bearer | vendor.crm.campaigns.read | Page snapshot history, newest first | read | VENDOR_CRM_CAMPAIGNS_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/campaigns/{campaignId}/snapshots/{snapshotId} | Public | bearer | vendor.crm.campaigns.read | Read one snapshot with per-channel eligibility figures | read; figures derived from the stored evidence | VENDOR_CRM_CAMPAIGNS_ENABLED | IMPLEMENTED |
| vendor-service | GET | /crm/campaigns/{campaignId}/snapshots/{snapshotId}/targets | Public | bearer | vendor.crm.campaigns.targets.read | Page the relationships inside a prepared audience | read; no contact data, closed cohort/channel filters | VENDOR_CRM_CAMPAIGNS_ENABLED | IMPLEMENTED |

Section 8E adds exactly five permissions: `vendor.crm.campaigns.read`, `vendor.crm.campaigns.manage`, `vendor.crm.campaigns.preview`, `vendor.crm.campaigns.prepare`, and `vendor.crm.campaigns.targets.read`. Listing targets is flagged high-risk and separated from reading a campaign for the same reason Section 8B separates segment member reads from segment reads: the customer-by-customer list is a strictly larger disclosure than the definition that produced it. Owner, General Manager, Marketing and Branch Manager receive all five; CRM/Support receives read, manage and preview but not the target list; Analyst/Auditor receives read and preview only; finance, kitchen, inventory, procurement and rider receive nothing.

**No send route, no schedule route, and no internal route.** Section 8E prepares audiences; Section 8F will execute them. There is no `vendor.crm.campaigns.send` permission, no scheduling endpoint, no notification handoff and no internal ingress, because no producer contract exists to receive on and delivery authority belongs to notification-service. Every prepared channel carries `send_time_recheck_required=true`, which a database check makes impossible to clear: PREPARED is never permission to contact anybody.

**Step-up.** Section 8E adds none. Planning a campaign neither sends a message nor exports customer data, so the friction would buy nothing; Section 8F's actual delivery and Section 8H's sensitive export will each need their own decision.

Add one row per route the moment it is registered in code. A route existing in code but missing here is a documentation-gate failure. Contract packages verify 21 Section 5, 32 Section 6, 20 Section 7, 7 Section 8A, 21 Section 8B, 16 Section 8C, 6 Section 8D, and 10 Section 8E operations — **133 cumulative Vendor routes**. `cmd`'s HTTP contract tests also verify the production registration helpers, while each Section contract enforces bidirectional OpenAPI parity.

## Section 8F — campaign execution

| Method | Exact path | Exposure | Permission | Contract |
|---|---|---|---|---|
| POST | /crm/campaigns/{campaignId}/runs | Public | vendor.crm.campaigns.execute | Idempotency-Key + expected_version; create send-now/scheduled run |
| GET | /crm/campaigns/{campaignId}/runs | Public | vendor.crm.campaigns.runs.read | Page runs |
| GET | /crm/campaigns/{campaignId}/runs/{runId} | Public | vendor.crm.campaigns.runs.read | Read run and counts |
| POST | /crm/campaigns/{campaignId}/runs/{runId}/cancel | Public | vendor.crm.campaigns.runs.manage | Idempotency-Key + expected_version |
| POST | /crm/campaigns/{campaignId}/runs/{runId}/pause | Public | vendor.crm.campaigns.runs.manage | Idempotency-Key + expected_version |
| POST | /crm/campaigns/{campaignId}/runs/{runId}/resume | Public | vendor.crm.campaigns.runs.manage | Idempotency-Key + expected_version |
| GET | /crm/campaigns/{campaignId}/runs/{runId}/deliveries | Public | vendor.crm.campaigns.deliveries.read | Page privacy-safe deliveries |
| GET | /crm/campaigns/{campaignId}/runs/{runId}/deliveries/{deliveryId} | Public | vendor.crm.campaigns.deliveries.read | No destination PII |
| GET | /crm/campaigns/{campaignId}/runs/{runId}/deliveries/{deliveryId}/history | Public | vendor.crm.campaigns.deliveries.read | Page append-only transitions |
| POST | /internal/vendor/v1/crm/campaign-delivery-events | Internal root | exact HMAC caller: notification-service | nonce + event_id dedupe + source_version |

Section 8F adds nine public operations and one internal operation: **143 cumulative Vendor routes**. Historical reads remain available with execution disabled; mutations and workers fail/stand down safely. Execution/run management are high-risk capabilities with idempotency, optimistic concurrency, audit, rate limits, and kill switches. No generic CRM per-request step-up consumer exists, so no new ad hoc step-up mechanism was invented.

## Section 8G — corporate customer CRM

Base path for every row below: `/crm/corporate-customers`. All are public (authenticated vendor surface), bearer auth, tenant-scoped from the session, and gated by `VENDOR_CRM_ENABLED` + `VENDOR_CRM_CORPORATE_ENABLED`; mutations additionally require `VENDOR_CRM_WRITES_ENABLED`.

| Method | Exact path | Permission | Purpose | Command contract |
|---|---|---|---|---|
| GET | /crm/corporate-customers | vendor.crm.corporate.read | Page accounts visible to the caller's branch scope | read; closed filters, allowlisted sort |
| POST | /crm/corporate-customers | vendor.crm.corporate.manage | Open a relationship in PROSPECT; allocates the vendor-local CORP- reference | Idempotency-Key |
| GET | /crm/corporate-customers/{corporateId} | vendor.crm.corporate.read | Read one account, its actionable summary and five most recent interactions | read |
| PATCH | /crm/corporate-customers/{corporateId} | vendor.crm.corporate.manage | Edit the account; status is not editable here | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/activate | vendor.crm.corporate.manage | PROSPECT/ON_HOLD/INACTIVE to ACTIVE | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/hold | vendor.crm.corporate.manage | ACTIVE to ON_HOLD | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/deactivate | vendor.crm.corporate.manage | End the working relationship, keep the record | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/archive | vendor.crm.corporate.manage | Terminal; cancels the account's open tasks in the same transaction | Idempotency-Key + expected_version |
| GET | /crm/corporate-customers/{corporateId}/branches | vendor.crm.corporate.read | Read scope mode and the exact branch set | read |
| PUT | /crm/corporate-customers/{corporateId}/branches | vendor.crm.corporate.manage | Replace the branch set wholesale | Idempotency-Key + expected_version; cannot widen past the caller's own scope |
| GET | /crm/corporate-customers/{corporateId}/contacts | vendor.crm.corporate.read | Page contacts (references + non-identifying label only) | read |
| POST | /crm/corporate-customers/{corporateId}/contacts | vendor.crm.corporate.contacts.manage | Link an existing Section 8A relationship as a contact | Idempotency-Key; redacted relationships refused |
| GET | /crm/corporate-customers/{corporateId}/contacts/{contactId} | vendor.crm.corporate.read | Read one contact | read |
| PATCH | /crm/corporate-customers/{corporateId}/contacts/{contactId} | vendor.crm.corporate.contacts.manage | Change role/status/primary; promotion demotes the previous primary | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/contacts/{contactId}/archive | vendor.crm.corporate.contacts.manage | End the role; deactivates the contact's memberships | Idempotency-Key + expected_version |
| GET | /crm/corporate-customers/{corporateId}/memberships | vendor.crm.corporate.read | Page memberships | read |
| POST | /crm/corporate-customers/{corporateId}/memberships | vendor.crm.corporate.contacts.manage | Create a membership; contact/site/cost centre must be of the same account | Idempotency-Key |
| PATCH | /crm/corporate-customers/{corporateId}/memberships/{membershipId} | vendor.crm.corporate.contacts.manage | Edit a membership | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/memberships/{membershipId}/archive | vendor.crm.corporate.contacts.manage | End a membership | Idempotency-Key + expected_version |
| GET | /crm/corporate-customers/{corporateId}/locations | vendor.crm.corporate.read | Page the organization's own sites | read |
| POST | /crm/corporate-customers/{corporateId}/locations | vendor.crm.corporate.locations.manage | Create a site; IANA timezone and bounded address components | Idempotency-Key |
| GET | /crm/corporate-customers/{corporateId}/locations/{locationId} | vendor.crm.corporate.read | Read one site | read |
| PATCH | /crm/corporate-customers/{corporateId}/locations/{locationId} | vendor.crm.corporate.locations.manage | Edit a site | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/locations/{locationId}/archive | vendor.crm.corporate.locations.manage | Archive a site; the row is kept because others may name it | Idempotency-Key + expected_version |
| GET | /crm/corporate-customers/{corporateId}/cost-centers | vendor.crm.corporate.read | Page cost-centre labels | read |
| POST | /crm/corporate-customers/{corporateId}/cost-centers | vendor.crm.corporate.cost_centers.manage | Create a cost-centre label | Idempotency-Key |
| GET | /crm/corporate-customers/{corporateId}/cost-centers/{costCenterId} | vendor.crm.corporate.read | Read one cost centre | read |
| PATCH | /crm/corporate-customers/{corporateId}/cost-centers/{costCenterId} | vendor.crm.corporate.cost_centers.manage | Edit a cost centre | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/cost-centers/{costCenterId}/archive | vendor.crm.corporate.cost_centers.manage | Archive a cost centre | Idempotency-Key + expected_version |
| GET | /crm/corporate-customers/{corporateId}/interactions | vendor.crm.interactions.read **+** vendor.crm.corporate.read | Page the account timeline, newest first | read |
| POST | /crm/corporate-customers/{corporateId}/interactions | vendor.crm.interactions.create **+** vendor.crm.corporate.read | Record an interaction against the account | Idempotency-Key |
| GET | /crm/corporate-customers/{corporateId}/interactions/{interactionId} | vendor.crm.interactions.read **+** vendor.crm.corporate.read | Read one interaction | read |
| POST | /crm/corporate-customers/{corporateId}/interactions/{interactionId}/correct | vendor.crm.interactions.manage **+** vendor.crm.corporate.read | Section 8C immutable correction; supersedes rather than edits | Idempotency-Key |
| GET | /crm/corporate-customers/{corporateId}/tasks | vendor.crm.tasks.read **+** vendor.crm.corporate.read | Page account tasks | read |
| POST | /crm/corporate-customers/{corporateId}/tasks | vendor.crm.tasks.create **+** vendor.crm.corporate.read | Open follow-up work; archived accounts accept none | Idempotency-Key |
| GET | /crm/corporate-customers/{corporateId}/tasks/{taskId} | vendor.crm.tasks.read **+** vendor.crm.corporate.read | Read one task | read |
| PATCH | /crm/corporate-customers/{corporateId}/tasks/{taskId} | vendor.crm.tasks.manage **+** vendor.crm.corporate.read | Edit an open task | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/tasks/{taskId}/start | vendor.crm.tasks.manage **+** vendor.crm.corporate.read | OPEN to IN_PROGRESS | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/tasks/{taskId}/complete | vendor.crm.tasks.manage **+** vendor.crm.corporate.read | Close as done | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/tasks/{taskId}/cancel | vendor.crm.tasks.manage **+** vendor.crm.corporate.read | Close without doing it | Idempotency-Key + expected_version |
| POST | /crm/corporate-customers/{corporateId}/tasks/{taskId}/reassign | vendor.crm.tasks.manage **+** vendor.crm.corporate.read | Move to another active, scoped staff member | Idempotency-Key + expected_version |

Section 8G adds exactly five permissions: `vendor.crm.corporate.read`, `vendor.crm.corporate.manage`, `vendor.crm.corporate.contacts.manage`, `vendor.crm.corporate.locations.manage`, and `vendor.crm.corporate.cost_centers.manage`. Contacts, locations and cost centres are separated because they are different disclosures and different remits: linking a person to an organization is not the same act as maintaining that organization's site list. Owner, General Manager and Branch Manager receive all five; CRM/Support receives read and contacts; Analyst/Auditor receives read only; kitchen and rider receive nothing.

**Corporate activity reuses Section 8C's permissions rather than reissuing them under a corporate name.** A corporate copy of `vendor.crm.tasks.manage` would be a second place for the same question to be answered differently. Each corporate activity route therefore requires the Section 8C grant **and** `vendor.crm.corporate.read`, so a caller who may log interactions does not thereby learn which organizations this vendor works with.

**No internal route, and no commerce, finance or approval route.** Corporate contact privacy and identity merge arrive through the existing Section 8A event ingress and its `PrivacyHandler` port, so there is nothing for a corporate-events endpoint to receive. There is no corporate order, invoice, credit, budget, contract, quotation, wallet or approval endpoint, permission, schema, column or state anywhere in this section; three separate tests (migration, OpenAPI contract, live router) reject one structurally rather than by convention.

**Step-up.** Section 8G adds none. Recording who a vendor deals with neither moves money nor exports customer data; corporate ordering and Section 8H's sensitive export will each need their own decision.

Contract packages verify 21 Section 5, 32 Section 6, 20 Section 7, 7 Section 8A, 21 Section 8B, 16 Section 8C, 6 Section 8D, 10 Section 8E, 10 Section 8F, and 41 Section 8G operations — **184 cumulative Vendor routes** (41 new public, 0 new internal).

## Section 8H — CRM intelligence and secure exports

All rows are public (authenticated vendor surface), bearer auth, tenant-scoped from the session. Intelligence routes are gated by `VENDOR_CRM_ENABLED` + `VENDOR_CRM_INTELLIGENCE_ENABLED`; export routes by `VENDOR_CRM_ENABLED` + `VENDOR_CRM_EXPORTS_ENABLED`. Commands additionally require `VENDOR_CRM_WRITES_ENABLED`.

| Method | Exact path | Permission | Purpose | Command contract |
|---|---|---|---|---|
| GET | /crm/intelligence/overview | vendor.crm.intelligence.read | Per-model availability, freshness and coverage counts | read; `branch_ref` selects a branch the caller holds |
| GET | /crm/intelligence/rfm | vendor.crm.intelligence.read | Quintile histograms plus unavailable reasons | read |
| GET | /crm/intelligence/churn | vendor.crm.intelligence.read | Risk-band histogram plus unavailable reasons | read |
| GET | /crm/customers/{profileId}/intelligence | vendor.crm.intelligence.read | One relationship's scores, availability first | read; redacted/merged relationships report NOT_AVAILABLE |
| POST | /crm/intelligence/rebuild | vendor.crm.intelligence.rebuild | Enqueue a durable rebuild; returns 202 | Idempotency-Key; a run already in flight is returned, not duplicated |
| GET | /crm/exports | vendor.crm.exports.read | Page export jobs visible to the caller | read; branch actors never see a business-wide job |
| POST | /crm/exports | vendor.crm.exports.create | Queue a durable export; returns 202 | Idempotency-Key + **step-up** (`crm.export.create`) |
| GET | /crm/exports/{exportId} | vendor.crm.exports.read | Read one job's status, row count, checksum and expiry | read |
| POST | /crm/exports/{exportId}/cancel | vendor.crm.exports.cancel | Stop a job and revoke its artifact | Idempotency-Key + expected_version |
| POST | /crm/exports/{exportId}/download | vendor.crm.exports.download | Release short-lived private access to a completed artifact | Idempotency-Key + **step-up** (`crm.export.download`) |

Section 8H adds exactly six permissions: `vendor.crm.intelligence.read`, `vendor.crm.intelligence.rebuild`, `vendor.crm.exports.read`, `vendor.crm.exports.create`, `vendor.crm.exports.download`, and `vendor.crm.exports.cancel`. Owner and General Manager receive all six; Branch Manager receives all but rebuild, scoped to their own branches; Marketing, CRM/Support and Analyst/Auditor receive intelligence read plus export read/create/download; Finance receives export read only, because an export is the widest customer-data grant this service issues; kitchen, inventory, procurement and rider receive nothing.

**There is no permission to write a score, and no route that does.** No `POST /score`, no `PATCH /churn-score`, no VIP flag. Intelligence is derived, and a hand-set score would make every other score unfalsifiable. Rebuild is separated from read because recomputing a scope is expensive operational work, not a refresh.

**Export authorities are separated because they are different disclosures.** Knowing an export exists is not reading it, and asking for one is not the same act as releasing its bytes. Download is a POST rather than a GET precisely because it changes state: it spends a step-up grant, writes an audit record and mints a token, none of which should be cacheable, linkable or repeatable.

**No internal route.** Nothing outside this service produces a Vendor CRM score or consumes a Vendor CRM export, so there is no ingress for one to arrive on.

**Step-up.** Section 8H is the first CRM section to consume the Section 5 elevation. Export creation and export download each require their own fresh, purpose-bound, single-use grant; a grant taken for one cannot be spent on the other, and a spent grant cannot be replayed. Intelligence reads and rebuilds do not require step-up: recomputing a heuristic neither moves money nor releases customer data.

Contract packages verify 21 Section 5, 32 Section 6, 20 Section 7, 7 Section 8A, 21 Section 8B, 16 Section 8C, 6 Section 8D, 10 Section 8E, 10 Section 8F, 41 Section 8G, 10 Section 8H, and 2 Section 8 completion operations — **196 cumulative Vendor routes** (2 new public, 0 new internal in the completion).

## Section 8I — final route certification

Section 8I adds **zero** public routes and **zero** internal routes. The final
Vendor surface is **196 routes**, and that figure is no longer maintained by
hand: `internal/crmcontract` builds the production router from the same
`Register*` helpers `cmd/main.go` calls and fails the build if the count moves,
if a live route is missing from any OpenAPI specification or from this register,
or if a specification advertises an operation that does not exist.

One correction was made to an existing route's **mounting**, not to the surface.
`POST /internal/staff/provision-owner` was registered under the public vendor
base (`/api/v1/vendor/internal/staff/provision-owner`) while this register, the
Section 5 OpenAPI and `internal/section5contract` all specified the engine root.
The documentation was right and the wiring was wrong; the wiring now mounts it at
`/internal/staff/provision-owner`, alongside the Section 7 verification callback
and the CRM event ingress. Every internal-authority route is now unreachable
through the path the public gateway forwards, asserted by
`TestInternalRoutesAreNeverMountedUnderThePublicBase`.

### Final internal route inventory

| Method | Path | Caller gate |
|---|---|---|
| POST | /internal/staff/provision-owner | HMAC service auth |
| POST | /internal/vendor/v1/verification-events | HMAC + exact caller + nonce + event dedupe + source version |
| POST | /internal/vendor/v1/crm/events | HMAC + caller allowlist + nonce |
| POST | /internal/vendor/v1/crm/campaign-delivery-events | HMAC + caller allowlist + nonce |

### OpenAPI specification index

| Section | Specification | Operations |
|---|---|---|
| 5 | `section5-auth-staff-iam.openapi.json` | 21 |
| 6 | `section6-vendor-business.openapi.json` | 32 |
| 7 | `section7-onboarding-kyb.openapi.json` | 20 |
| 8A | `section8a-crm-foundation.openapi.json` | 7 |
| 8B | `section8b-crm-tags-segmentation.openapi.json` | 21 |
| 8C | `section8c-crm-interactions-tasks.openapi.json` | 16 |
| 8D | `section8d-crm-loyalty.openapi.json` | 6 |
| 8E | `section8e-crm-campaign-intelligence.openapi.json` | 10 |
| 8F | `section8f-campaign-execution.openapi.json` | 10 |
| 8G | `section8g-crm-corporate-customers.openapi.json` | 41 |
| 8H | `section8h-crm-intelligence-exports.openapi.json` | 10 |
| 8 completion | `section8-completion-crm.openapi.json` | 2 |
| | **Total** | **196** |

## Section 9A — Food catalogue foundation

All public paths below are relative to `/api/v1/vendor`, sit behind the
authenticated Vendor staff session, and are described by
`section9a-food-catalogue.openapi.json`. Every mutation accepts the shared
`Idempotency-Key` header and every mutation of an existing row requires
`expected_version`. **Section 9A registers zero internal routes**: nothing
outside this service produces a menu, and Section 9C — not this section — owns
the read/consumption contract other services will use.

Registration is gated by `COMMERCE_FOOD_ENABLED`; mutations additionally require
`VENDOR_FOOD_CATALOGUE_WRITES_ENABLED` and are withheld by
`COMMERCE_GLOBAL_KILL_SWITCH`. With the feature off, none of these routes is
registered at all, so a disabled catalogue has no attack surface rather than
routes that answer 503 and thereby confirm it exists.

| Service | Method | Path | Surface | Auth | Permission | Purpose | Idempotency / concurrency | Flag | Status |
|---|---|---|---|---|---|---|---|---|---|
| vendor-service | POST | /api/v1/vendor/food/menus | Public | bearer | vendor.food.catalogue.manage | Create a DRAFT branch menu for one food capability | Idempotency-Key | food catalogue writes | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/menus | Public | bearer | vendor.food.catalogue.read | List menus in the caller's branch scope | read, bounded | food catalogue | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/menus/{menuId} | Public | bearer | vendor.food.catalogue.read | Read one menu | read | food catalogue | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/food/menus/{menuId} | Public | bearer | vendor.food.catalogue.manage | Edit menu name/description | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/menus/{menuId}/activate | Public | bearer | vendor.food.catalogue.publish | Publish, enforcing Section 6/7 business, branch and capability state | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/menus/{menuId}/pause | Public | bearer | vendor.food.catalogue.publish | Administratively withhold a menu | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/menus/{menuId}/archive | Public | bearer | vendor.food.catalogue.publish | Retire a menu; terminal, never reactivated | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/menus/{menuId}/sections | Public | bearer | vendor.food.catalogue.manage | Append a section | Idempotency-Key | food catalogue writes | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/menus/{menuId}/sections | Public | bearer | vendor.food.catalogue.read | List sections in deterministic order | read | food catalogue | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/menus/{menuId}/sections/reorder | Public | bearer | vendor.food.catalogue.manage | Atomic complete reorder | Idempotency-Key + transactional | food catalogue writes | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/food/menus/{menuId}/sections/{sectionId} | Public | bearer | vendor.food.catalogue.manage | Edit a section through its own menu | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/archive | Public | bearer | vendor.food.catalogue.manage | Retire a section and archive its items | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items | Public | bearer | vendor.food.catalogue.manage | Create a DRAFT item at integer minor-unit price | Idempotency-Key | food catalogue writes | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items | Public | bearer | vendor.food.catalogue.read | List a section's items with media resolved in one query | read, bounded | food catalogue | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items/reorder | Public | bearer | vendor.food.catalogue.manage | Atomic complete reorder within a section | Idempotency-Key + transactional | food catalogue writes | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId} | Public | bearer | vendor.food.catalogue.read | Read one item | read | food catalogue | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/food/items/{itemId} | Public | bearer | vendor.food.catalogue.manage (+ publish for ACTIVE) | Edit item text, price, metadata and lifecycle | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/items/{itemId}/archive | Public | bearer | vendor.food.catalogue.manage | Retire an item and its variants | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId}/media | Public | bearer | vendor.food.catalogue.read | List opaque Media refs | read | food catalogue | VERIFIED |
| vendor-service | PUT | /api/v1/vendor/food/items/{itemId}/media | Public | bearer | vendor.food.catalogue.manage | Replace the ordered Media ref set after authority read-back | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/items/{itemId}/variants | Public | bearer | vendor.food.catalogue.manage | Create a size/portion variant | Idempotency-Key | food catalogue writes | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId}/variants | Public | bearer | vendor.food.catalogue.read | List variants | read | food catalogue | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/items/{itemId}/variants/reorder | Public | bearer | vendor.food.catalogue.manage | Atomic complete reorder within an item | Idempotency-Key + transactional | food catalogue writes | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/food/items/{itemId}/variants/{variantId} | Public | bearer | vendor.food.catalogue.manage | Edit a variant through its own item | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/items/{itemId}/variants/{variantId}/archive | Public | bearer | vendor.food.catalogue.manage | Retire a variant | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId}/modifier-groups | Public | bearer | vendor.food.catalogue.read | List attached groups with definitions joined | read | food catalogue | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/items/{itemId}/modifier-groups/reorder | Public | bearer | vendor.food.catalogue.manage | Atomic complete reorder of attachments | Idempotency-Key + transactional | food catalogue writes | VERIFIED |
| vendor-service | PUT | /api/v1/vendor/food/items/{itemId}/modifier-groups/{groupId} | Public | bearer | vendor.food.catalogue.manage | Attach a shared group to one item | Idempotency-Key + DB uniqueness | food catalogue writes | VERIFIED |
| vendor-service | DELETE | /api/v1/vendor/food/items/{itemId}/modifier-groups/{groupId} | Public | bearer | vendor.food.catalogue.manage | Detach the relationship only | Idempotency-Key | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/modifier-groups | Public | bearer | vendor.food.catalogue.manage (business-wide) | Create a reusable choice group | Idempotency-Key | food catalogue writes | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/modifier-groups | Public | bearer | vendor.food.catalogue.read | List the business's groups | read, bounded | food catalogue | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/modifier-groups/{groupId} | Public | bearer | vendor.food.catalogue.read | Read one group | read | food catalogue | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/food/modifier-groups/{groupId} | Public | bearer | vendor.food.catalogue.manage (business-wide) | Edit a shared group | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/modifier-groups/{groupId}/archive | Public | bearer | vendor.food.catalogue.manage (business-wide) | Retire a group, its modifiers and every attachment | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/modifier-groups/{groupId}/modifiers | Public | bearer | vendor.food.catalogue.manage (business-wide) | Create a non-negative priced choice | Idempotency-Key | food catalogue writes | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/modifier-groups/{groupId}/modifiers | Public | bearer | vendor.food.catalogue.read | List a group's modifiers | read | food catalogue | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/modifier-groups/{groupId}/modifiers/reorder | Public | bearer | vendor.food.catalogue.manage (business-wide) | Atomic complete reorder within a group | Idempotency-Key + transactional | food catalogue writes | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/food/modifier-groups/{groupId}/modifiers/{modifierId} | Public | bearer | vendor.food.catalogue.manage (business-wide) | Edit a modifier through its own group | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/modifier-groups/{groupId}/modifiers/{modifierId}/archive | Public | bearer | vendor.food.catalogue.manage (business-wide) | Retire one choice | Idempotency-Key + expected_version | food catalogue writes | VERIFIED |

Section 9A adds exactly three permissions: `vendor.food.catalogue.read`,
`vendor.food.catalogue.manage` and `vendor.food.catalogue.publish`. Owner,
General Manager and Branch Manager receive all three (a Branch Manager's reach
is still decided per request from their assignment, so publishing applies only
to their own branches); Kitchen/Pastry receives read and manage but not publish,
because keeping the menu accurate is the job and deciding the branch is open for
it is not; Analyst/Auditor receives read only. Marketing, CRM/Support, Finance,
Procurement, Shop/Inventory and Vendor Rider receive nothing. **None of the three
is high-risk and none requires step-up** — editing a menu is not a bank change,
and marking it so would dilute step-up where it genuinely matters.

**Modifier groups are business-level definitions, not per-branch copies.** A
branch-scoped grant may read them and attach, detach and reorder them on its own
branch's items, but may not create, edit or archive the shared definitions,
because a group edited from one branch changes every other branch's items. The
rows marked *(business-wide)* above are the ones that require a vendor-wide grant.

**Media stays opaque.** There is no upload route in this section. `PUT
/food/items/{itemId}/media` accepts references Media File Service has already
issued and proves each one against that service's existing private ownership API
before storing it; with no media credentials configured the write is refused
rather than storing an unverified reference.

Contract packages verify 21 Section 5, 32 Section 6, 20 Section 7, 7 Section 8A,
21 Section 8B, 16 Section 8C, 6 Section 8D, 10 Section 8E, 10 Section 8F, 41
Section 8G, 10 Section 8H, 2 Section 8 completion and 39 Section 9A operations —
**283 cumulative Vendor routes** (20 new public management routes added for the
shared Sections 10/11 production surface).
The Section 8I global guard in `internal/crmcontract` is unchanged and still
holds the whole surface against every specification and this register in both
directions.

### OpenAPI specification index (post-9C)

| Section | Specification | Operations |
|---|---|---|
| 5 | `section5-auth-staff-iam.openapi.json` | 21 |
| 6 | `section6-vendor-business.openapi.json` | 32 |
| 7 | `section7-onboarding-kyb.openapi.json` | 20 |
| 8A | `section8a-crm-foundation.openapi.json` | 7 |
| 8B | `section8b-crm-tags-segmentation.openapi.json` | 21 |
| 8C | `section8c-crm-interactions-tasks.openapi.json` | 16 |
| 8D | `section8d-crm-loyalty.openapi.json` | 6 |
| 8E | `section8e-crm-campaign-intelligence.openapi.json` | 10 |
| 8F | `section8f-campaign-execution.openapi.json` | 10 |
| 8G | `section8g-crm-corporate-customers.openapi.json` | 41 |
| 8H | `section8h-crm-intelligence-exports.openapi.json` | 10 |
| 8 completion | `section8-completion-crm.openapi.json` | 2 |
| 9A | `section9a-food-catalogue.openapi.json` | 39 |
| 9B | `section9b-food-operations.openapi.json` | 22 |
| 9C | `section9c-food-discovery.openapi.json` | 5 |
| 10/11 | `sections10-11-production.openapi.json` | 21 |
| | **Total** | **283** |

## Section 9B - Food operations, pricing and availability

These 22 authenticated vendor-management routes extend the Section 9A food
catalogue. They are documented by `section9b-food-operations.openapi.json` and
share the catalogue feature flag. They are management-only: there are no
discovery or internal service routes in Section 9B.

| Service | Method | Path | Surface | Auth | Permission | Purpose | Idempotency / concurrency | Flag | Status |
|---|---|---|---|---|---|---|---|---|---|
| vendor-service | POST | /api/v1/vendor/food/menus/{menuId}/schedules | Public | bearer | vendor.food.operations.manage | Create a timezone-aware menu schedule | Idempotency-Key | food operations | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/menus/{menuId}/schedules | Public | bearer | vendor.food.catalogue.read | List menu schedules | read | food operations | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/menus/{menuId}/schedules/{scheduleId} | Public | bearer | vendor.food.catalogue.read | Read one schedule | read | food operations | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/food/menus/{menuId}/schedules/{scheduleId} | Public | bearer | vendor.food.operations.manage | Update a schedule with expected_version | Idempotency-Key + expected_version | food operations | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/menus/{menuId}/schedules/{scheduleId}/archive | Public | bearer | vendor.food.operations.manage | Archive a schedule | Idempotency-Key + expected_version | food operations | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId}/availability | Public | bearer | vendor.food.catalogue.read | Read manual availability state | read | food operations | VERIFIED |
| vendor-service | PUT | /api/v1/vendor/food/items/{itemId}/availability | Public | bearer | vendor.food.operations.manage | Set manual availability state | Idempotency-Key + expected_version | food operations | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId}/availability/effective | Public | bearer | vendor.food.catalogue.read | Resolve effective availability | read | food operations | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId}/availability-source | Public | bearer | vendor.food.catalogue.read | Read availability-source configuration | read | food operations | VERIFIED |
| vendor-service | PUT | /api/v1/vendor/food/items/{itemId}/availability-source | Public | bearer | vendor.food.operations.manage | Configure manual or inventory-linked source | Idempotency-Key + expected_version | food operations | VERIFIED |
| vendor-service | DELETE | /api/v1/vendor/food/items/{itemId}/availability-source | Public | bearer | vendor.food.operations.manage | Clear availability-source configuration | Idempotency-Key + expected_version | food operations | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId}/branch-override | Public | bearer | vendor.food.catalogue.read | Read branch price/preparation override | read | food operations | VERIFIED |
| vendor-service | PUT | /api/v1/vendor/food/items/{itemId}/branch-override | Public | bearer | vendor.food.operations.manage (+ pricing.manage for price) | Set branch override | Idempotency-Key + expected_version | food operations | VERIFIED |
| vendor-service | DELETE | /api/v1/vendor/food/items/{itemId}/branch-override | Public | bearer | vendor.food.operations.manage (+ pricing.manage for price) | Clear branch override | Idempotency-Key + expected_version | food operations | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId}/pricing/history | Public | bearer | vendor.food.catalogue.read | List immutable price history | read, bounded | food operations | VERIFIED |
| vendor-service | POST | /api/v1/vendor/food/items/{itemId}/pricing/resolve | Public | bearer | vendor.food.catalogue.read | Resolve effective price from persisted versions | read | food operations | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId}/prep-profile | Public | bearer | vendor.food.catalogue.read | Read effective preparation profile | read | food operations | VERIFIED |
| vendor-service | PUT | /api/v1/vendor/food/items/{itemId}/prep-profile | Public | bearer | vendor.food.operations.manage | Set preparation profile | Idempotency-Key + expected_version | food operations | VERIFIED |
| vendor-service | DELETE | /api/v1/vendor/food/items/{itemId}/prep-profile | Public | bearer | vendor.food.operations.manage | Clear preparation profile | Idempotency-Key + expected_version | food operations | VERIFIED |
| vendor-service | GET | /api/v1/vendor/food/items/{itemId}/logistics-profile | Public | bearer | vendor.food.catalogue.read | Read logistics profile | read | food operations | VERIFIED |
| vendor-service | PUT | /api/v1/vendor/food/items/{itemId}/logistics-profile | Public | bearer | vendor.food.operations.manage | Set logistics profile | Idempotency-Key + expected_version | food operations | VERIFIED |
| vendor-service | DELETE | /api/v1/vendor/food/items/{itemId}/logistics-profile | Public | bearer | vendor.food.operations.manage | Clear logistics profile | Idempotency-Key + expected_version | food operations | VERIFIED |

## Section 9C - Internal food discovery

These five read-only routes are mounted at the engine root under
`/internal/vendor/v1/food`, never under the public `/api/v1/vendor` gateway
base. They require HMAC v2, an allowlisted service caller and a unique nonce.
They reuse the Section 9B sellability, pricing and catalogue-revision
authorities; no customer-facing frontend route is created.

| Service | Method | Full path | Surface | Auth | Caller | Purpose | Status |
|---|---|---|---|---|---|---|---|
| vendor-service | GET | /internal/vendor/v1/food/branches/{branchId}/catalogue | Internal | HMAC v2 + nonce | customer-service, api-gateway, order-service, discovery-service | Effective branch catalogue snapshot | VERIFIED |
| vendor-service | GET | /internal/vendor/v1/food/branches/{branchId}/search | Internal | HMAC v2 + nonce | customer-service, api-gateway, order-service, discovery-service | Bounded effective catalogue search | VERIFIED |
| vendor-service | GET | /internal/vendor/v1/food/branches/{branchId}/revision | Internal | HMAC v2 + nonce | customer-service, api-gateway, order-service, discovery-service | Catalogue revision/freshness marker | VERIFIED |
| vendor-service | GET | /internal/vendor/v1/food/branches/{branchId}/items/{itemId}/sellability | Internal | HMAC v2 + nonce | customer-service, api-gateway, order-service, discovery-service | Effective sellability | VERIFIED |
| vendor-service | POST | /internal/vendor/v1/food/branches/{branchId}/pricing/resolve | Internal | HMAC v2 + nonce | customer-service, api-gateway, order-service, discovery-service | Effective price resolution | VERIFIED |

## Sections 10/11 - Shared production management

All state-changing Sections 10/11 routes require `Idempotency-Key` and use the
shared Vendor replay/fingerprint contract. Versioned bodies require a positive
expected version; production-job transitions use a positive `If-Match`.

| Service | Method | Path | Surface | Auth | Permission | Purpose | Status |
|---|---|---|---|---|---|---|---|
| vendor-service | GET | /api/v1/vendor/production/pools | Public | bearer | vendor.production.read | List capacity pools | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/pools | Public | bearer | vendor.production.manage | Create capacity pool | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/pools/{poolId} | Public | bearer | vendor.production.read | Read capacity pool | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/production/pools/{poolId} | Public | bearer | vendor.production.manage | Update capacity pool | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/pools/{poolId}/archive | Public | bearer | vendor.production.manage | Archive capacity pool | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/reservations | Public | bearer | vendor.production.operate | Create hold/confirmed reservation | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/reservations/{reservationId} | Public | bearer | vendor.production.read | Read reservation | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/reservations/{reservationId}/confirm | Public | bearer | vendor.production.operate | Confirm reservation | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/reservations/{reservationId}/release | Public | bearer | vendor.production.operate | Release reservation | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/capacity/preview | Public | bearer | vendor.production.read | Preview capacity | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/capacity/utilization | Public | bearer | vendor.production.read | Capacity utilization | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/reservations/{reservationId}/reschedule | Public | bearer | vendor.production.operate | Reschedule reservation | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/lines | Public | bearer | vendor.production.read | List production lines | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/lines | Public | bearer | vendor.production.manage | Create production line | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/lines/{lineId}/archive | Public | bearer | vendor.production.manage | Archive production line | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/rules | Public | bearer | vendor.production.read | List capacity rules | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/rules | Public | bearer | vendor.production.manage | Create capacity rule | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/rules/{ruleId}/archive | Public | bearer | vendor.production.manage | Archive capacity rule | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/blackouts | Public | bearer | vendor.production.read | List blackouts | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/blackouts | Public | bearer | vendor.production.manage | Create blackout | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/blackouts/{blackoutId}/cancel | Public | bearer | vendor.production.manage | Cancel blackout | VERIFIED |

## Sections 10/11 - Cloud Kitchen and Pastry/Bakery

| Service | Method | Path | Surface | Auth | Permission | Purpose | Status |
|---|---|---|---|---|---|---|---|
| vendor-service | GET | /api/v1/vendor/cloud-kitchen/settings/{branchId} | Public | bearer | vendor.production.read | Read branch Cloud Kitchen settings | VERIFIED |
| vendor-service | PUT | /api/v1/vendor/cloud-kitchen/settings/{branchId} | Public | bearer | vendor.cloud_kitchen.manage | Update Cloud Kitchen settings | VERIFIED |
| vendor-service | GET | /api/v1/vendor/cloud-kitchen/bulk-packages | Public | bearer | vendor.production.read | List bulk packages | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/bulk-packages | Public | bearer | vendor.cloud_kitchen.manage | Create bulk package | VERIFIED |
| vendor-service | GET | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId} | Public | bearer | vendor.production.read | Read bulk package | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId} | Public | bearer | vendor.cloud_kitchen.manage | Update bulk package | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId}/archive | Public | bearer | vendor.cloud_kitchen.manage | Archive bulk package | VERIFIED |
| vendor-service | GET | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId}/components | Public | bearer | vendor.production.read | List package composition | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId}/components | Public | bearer | vendor.cloud_kitchen.manage | Add package component | VERIFIED |
| vendor-service | GET | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId}/bands | Public | bearer | vendor.production.read | List quantity bands | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId}/bands | Public | bearer | vendor.cloud_kitchen.manage | Add quantity band and price history | VERIFIED |
| vendor-service | GET | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId}/capacity-preview | Public | bearer | vendor.production.read | Preview matched band | VERIFIED |
| vendor-service | GET | /api/v1/vendor/cloud-kitchen/corporate-plans | Public | bearer | vendor.production.read | List corporate plans | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans | Public | bearer | vendor.cloud_kitchen.manage | Create recurring plan | VERIFIED |
| vendor-service | GET | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId} | Public | bearer | vendor.production.read | Read recurring plan | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/activate | Public | bearer | vendor.cloud_kitchen.manage | Activate recurring plan | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/suspend | Public | bearer | vendor.cloud_kitchen.manage | Suspend recurring plan | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/resume | Public | bearer | vendor.cloud_kitchen.manage | Resume recurring plan | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/archive | Public | bearer | vendor.cloud_kitchen.manage | Archive recurring plan | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/{status} | Public | bearer | vendor.cloud_kitchen.manage | Activate, suspend or archive plan | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/exceptions | Public | bearer | vendor.cloud_kitchen.manage | Add occurrence exception | VERIFIED |
| vendor-service | GET | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/occurrences | Public | bearer | vendor.production.read | List materialized occurrences | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/occurrences/{occurrenceId}/retry | Public | bearer | vendor.cloud_kitchen.manage | Retry capacity-conflict occurrence | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/occurrences/{occurrenceId}/reschedule | Public | bearer | vendor.cloud_kitchen.manage | Reschedule occurrence | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/products | Public | bearer | vendor.pastry.read | List pastry products | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/products | Public | bearer | vendor.pastry.manage | Create pastry product | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/option-groups | Public | bearer | vendor.pastry.read | List reusable option groups | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/option-groups | Public | bearer | vendor.pastry.manage | Create option group | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/option-groups/{groupId}/options | Public | bearer | vendor.pastry.manage | Add option | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/option-groups/{groupId}/options | Public | bearer | vendor.pastry.read | List reusable group options | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/products/{productId}/option-groups | Public | bearer | vendor.pastry.manage | Attach option group | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/custom-requests | Public | bearer | vendor.pastry.manage | Create custom cake request | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/custom-requests/{requestId}/specifications | Public | bearer | vendor.pastry.manage | Append immutable specification revision | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/custom-requests/{requestId}/quotes | Public | bearer | vendor.pastry.quote.manage | Create quote revision | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/quotes/{quoteId}/approve | Public | bearer | vendor.pastry.quote.approve | Approve quote | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/quotes/{quoteId}/accept | Public | bearer | vendor.pastry.quote.manage | Accept quote and create conversion intent | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/custom-requests/{requestId}/milestones | Public | bearer | vendor.pastry.read | Read milestones | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/products/{productId} | Public | bearer | vendor.pastry.read | Read pastry product | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/pastry/products/{productId} | Public | bearer | vendor.pastry.manage | Update pastry product | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/products/{productId}/archive | Public | bearer | vendor.pastry.manage | Archive pastry product | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/option-groups/{groupId} | Public | bearer | vendor.pastry.read | Read option group | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/pastry/option-groups/{groupId} | Public | bearer | vendor.pastry.manage | Update option group | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/option-groups/{groupId}/archive | Public | bearer | vendor.pastry.manage | Archive option group | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/pastry/options/{optionId} | Public | bearer | vendor.pastry.manage | Update pastry option | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/options/{optionId}/archive | Public | bearer | vendor.pastry.manage | Archive pastry option | VERIFIED |
| vendor-service | DELETE | /api/v1/vendor/pastry/products/{productId}/option-groups/{groupId} | Public | bearer | vendor.pastry.manage | Detach option group | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/custom-requests | Public | bearer | vendor.pastry.read | List custom requests | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/custom-requests/{requestId} | Public | bearer | vendor.pastry.read | Read custom request | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/pastry/custom-requests/{requestId} | Public | bearer | vendor.pastry.manage | Update custom request while mutable | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/custom-requests/{requestId}/submit | Public | bearer | vendor.pastry.manage | Submit custom request | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/custom-requests/{requestId}/cancel | Public | bearer | vendor.pastry.manage | Cancel custom request | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/custom-requests/{requestId}/specifications | Public | bearer | vendor.pastry.read | List specification revisions | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/custom-requests/{requestId}/changes | Public | bearer | vendor.pastry.read | List change requests | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/custom-requests/{requestId}/changes | Public | bearer | vendor.pastry.quote.manage | Request specification change | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/custom-requests/{requestId}/quotes | Public | bearer | vendor.pastry.quote.manage | List quote revisions | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/quotes/{quoteId} | Public | bearer | vendor.pastry.quote.manage | Read quote | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/quotes/{quoteId}/revise | Public | bearer | vendor.pastry.quote.manage | Revise quote | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/quotes/{quoteId}/decline | Public | bearer | vendor.pastry.quote.manage | Decline quote | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/custom-requests/{requestId}/milestones/{milestoneType}/{state} | Public | bearer | vendor.pastry.manage | Transition milestone | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/custom-requests/{requestId}/specifications/diff | Public | bearer | vendor.pastry.read | Compare specifications | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/custom-requests/{requestId}/change-eligibility | Public | bearer | vendor.pastry.read | Evaluate change eligibility | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/custom-requests/{requestId}/cancellation-eligibility | Public | bearer | vendor.pastry.read | Evaluate cancellation eligibility | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/changes/{changeId}/decision | Public | bearer | vendor.pastry.manage | Approve or decline change request | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/changes/{changeId}/history | Public | bearer | vendor.pastry.read | Read change decision history | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/slots | Public | bearer | vendor.production.read | List production slots | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/slots | Public | bearer | vendor.production.manage | Create production slot | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/slots/{slotId}/close | Public | bearer | vendor.production.manage | Close production slot | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/jobs | Public | bearer | vendor.production.read | List production jobs | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/jobs | Public | bearer | vendor.production.operate | Create production job | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/jobs/{jobId} | Public | bearer | vendor.production.read | Read production job | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/jobs/{jobId}/start | Public | bearer | vendor.production.operate | Start production job | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/jobs/{jobId}/ready | Public | bearer | vendor.production.operate | Mark production job ready | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/jobs/{jobId}/complete | Public | bearer | vendor.production.operate | Complete production job | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/jobs/{jobId}/{status} | Public | bearer | vendor.production.operate | Transition production job | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/slot-templates | Public | bearer | vendor.production.read | List slot templates | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/slot-templates | Public | bearer | vendor.production.manage | Create slot template | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/slot-templates/{templateId} | Public | bearer | vendor.production.read | Read slot template | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/production/slot-templates/{templateId} | Public | bearer | vendor.production.manage | Update slot template | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/slot-templates/{templateId}/archive | Public | bearer | vendor.production.manage | Archive slot template | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/slot-templates/{templateId}/generate | Public | bearer | vendor.production.manage | Generate bounded production slots | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/lines/{lineId} | Public | bearer | vendor.production.read | Read production line | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/production/lines/{lineId} | Public | bearer | vendor.production.manage | Update production line | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/rules/{ruleId} | Public | bearer | vendor.production.read | Read capacity rule | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/production/rules/{ruleId} | Public | bearer | vendor.production.manage | Update capacity rule | VERIFIED |
| vendor-service | DELETE | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId}/components/{componentId} | Public | bearer | vendor.cloud_kitchen.manage | Remove package component and snapshot definition | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId}/bands/{bandId} | Public | bearer | vendor.cloud_kitchen.manage | Revise quantity band and append price history | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/bulk-packages/{packageId}/bands/{bandId}/archive | Public | bearer | vendor.cloud_kitchen.manage | Archive quantity band | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/occurrences/{occurrenceId}/replay | Public | bearer | vendor.cloud_kitchen.manage | Replay a dead-letter occurrence | VERIFIED |
| vendor-service | POST | /api/v1/vendor/cloud-kitchen/corporate-plans/{planId}/occurrences/{occurrenceId}/cancel | Public | bearer | vendor.cloud_kitchen.manage | Cancel reserved occurrence safely | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/products/{productId}/capacity-preview | Public | bearer | vendor.pastry.read | Preview standard pastry batch capacity | VERIFIED |
| vendor-service | POST | /api/v1/vendor/pastry/products/{productId}/production-batches | Public | bearer | vendor.pastry.manage | Reserve capacity and create standard pastry batch | VERIFIED |
| vendor-service | GET | /api/v1/vendor/pastry/custom-requests/{requestId}/milestones/{milestoneType}/history | Public | bearer | vendor.pastry.read | Read milestone transition history | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/slots/{slotId} | Public | bearer | vendor.production.read | Read production slot | VERIFIED |
| vendor-service | PATCH | /api/v1/vendor/production/slots/{slotId} | Public | bearer | vendor.production.manage | Update production slot | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/slots/{slotId}/open | Public | bearer | vendor.production.manage | Open production slot | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/slots/{slotId}/blackout | Public | bearer | vendor.production.manage | Black out production slot | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/slots/{slotId}/archive | Public | bearer | vendor.production.manage | Archive production slot | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/jobs/{jobId}/events | Public | bearer | vendor.production.read | Read production job history | VERIFIED |
| vendor-service | GET | /api/v1/vendor/production/alerts | Public | bearer | vendor.production.read | List durable production alerts | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/alerts/{alertId}/acknowledge | Public | bearer | vendor.production.operate | Acknowledge production alert | VERIFIED |
| vendor-service | POST | /api/v1/vendor/production/alerts/{alertId}/resolve | Public | bearer | vendor.production.operate | Resolve production alert | VERIFIED |

## Section 12A — Shop catalogue and commercial management

All routes below are authenticated Vendor management APIs. Mutations require the shared `Idempotency-Key`; there are no inventory, availability, stock, reservation, Order, Wallet, customer-discovery, or logistics-execution routes in Part A.

| Method | Path |
|---|---|
| GET | /api/v1/vendor/shop/catalogue-revision |
| POST / GET | /api/v1/vendor/shop/categories |
| GET / PATCH | /api/v1/vendor/shop/categories/{categoryId} |
| POST | /api/v1/vendor/shop/categories/{categoryId}/archive |
| POST | /api/v1/vendor/shop/categories/{categoryId}/restore |
| POST / GET | /api/v1/vendor/shop/measurements |
| GET / PATCH | /api/v1/vendor/shop/measurements/{measureId} |
| POST | /api/v1/vendor/shop/measurements/{measureId}/archive |
| GET / POST | /api/v1/vendor/shop/measurements/{measureId}/aliases |
| DELETE | /api/v1/vendor/shop/measurements/{measureId}/aliases/{aliasId} |
| POST / GET | /api/v1/vendor/shop/products |
| GET / PATCH | /api/v1/vendor/shop/products/{productId} |
| POST | /api/v1/vendor/shop/products/{productId}/archive |
| POST / GET | /api/v1/vendor/shop/products/{productId}/variants |
| PATCH | /api/v1/vendor/shop/products/{productId}/variants/{variantId} |
| POST | /api/v1/vendor/shop/products/{productId}/variants/{variantId}/archive |
| POST / GET | /api/v1/vendor/shop/products/{productId}/measure-calibrations |
| PATCH | /api/v1/vendor/shop/products/{productId}/measure-calibrations/{calibrationId} |
| POST | /api/v1/vendor/shop/products/{productId}/measure-calibrations/{calibrationId}/archive |
| POST / GET | /api/v1/vendor/shop/products/{productId}/sell-units |
| PATCH | /api/v1/vendor/shop/products/{productId}/sell-units/{sellUnitId} |
| POST | /api/v1/vendor/shop/products/{productId}/sell-units/{sellUnitId}/archive |
| POST / GET | /api/v1/vendor/shop/products/{productId}/packaging |
| POST | /api/v1/vendor/shop/products/{productId}/packaging/{relationId}/archive |
| PUT / GET | /api/v1/vendor/shop/products/{productId}/fresh-attributes |
| POST | /api/v1/vendor/shop/products/{productId}/fresh-attributes/{attributeId}/archive |
| GET | /api/v1/vendor/shop/variants/{variantId} |
| POST / GET | /api/v1/vendor/shop/variants/{variantId}/identifiers |
| POST | /api/v1/vendor/shop/variants/{variantId}/identifiers/{identifierId}/archive |
| GET | /api/v1/vendor/shop/identifier-lookup |
| GET / PUT | /api/v1/vendor/shop/branches/{branchId}/operating-models |
| POST | /api/v1/vendor/shop/branches/{branchId}/operating-models/archive |
| POST / GET | /api/v1/vendor/shop/branches/{branchId}/listings |
| GET / PATCH | /api/v1/vendor/shop/listings/{listingId} |
| POST | /api/v1/vendor/shop/listings/{listingId}/archive |
| PUT | /api/v1/vendor/shop/listings/{listingId}/sell-units/{listingUnitId} (selection, branch calibration, retail/wholesale overrides) |
| POST / GET | /api/v1/vendor/shop/listings/{listingId}/quantity-breaks |
| PATCH | /api/v1/vendor/shop/listings/{listingId}/quantity-breaks/{breakId} |
| POST | /api/v1/vendor/shop/listings/{listingId}/quantity-breaks/{breakId}/archive |
| GET | /api/v1/vendor/shop/listings/{listingId}/resolved-price |
| GET | /api/v1/vendor/shop/price-history |

## Section 12B — Vendor-owned inventory and effective Shop discovery

The management surface has 42 routes. Every mutation uses the shared Vendor idempotency contract. The 11 internal routes are root-mounted under `/internal/vendor/v1`, require HMAC v2, a single-use nonce, and an explicit caller allowlist.

| Method | Path |
|---|---|
| PUT | /api/v1/vendor/shop/branches/{branchId}/inventory/stock-bases |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/balances |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/balances/{stockBasisId} |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/movements |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/revision |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/variants/{variantId}/availability |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/variants/{variantId}/sellability |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/receipts |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/transfers |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/stocktakes |
| POST | /api/v1/vendor/shop/branches/{branchId}/inventory/stocktakes |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/quarantines |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/reorder-rules |
| GET | /api/v1/vendor/shop/branches/{branchId}/inventory/alerts |
| POST | /api/v1/vendor/shop/inventory/receipts |
| GET | /api/v1/vendor/shop/inventory/receipts/{receiptId} |
| POST | /api/v1/vendor/shop/inventory/receipts/{receiptId}/post |
| POST | /api/v1/vendor/shop/inventory/receipts/{receiptId}/cancel |
| POST | /api/v1/vendor/shop/inventory/reservations |
| GET | /api/v1/vendor/shop/inventory/reservations/{reservationId} |
| POST | /api/v1/vendor/shop/inventory/reservations/{reservationId}/confirm |
| POST | /api/v1/vendor/shop/inventory/reservations/{reservationId}/commit |
| POST | /api/v1/vendor/shop/inventory/reservations/{reservationId}/release |
| POST | /api/v1/vendor/shop/inventory/adjustments |
| POST | /api/v1/vendor/shop/inventory/quarantines |
| POST | /api/v1/vendor/shop/inventory/quarantines/{quarantineId}/resolve |
| POST | /api/v1/vendor/shop/inventory/transfers |
| GET | /api/v1/vendor/shop/inventory/transfers/{transferId} |
| POST | /api/v1/vendor/shop/inventory/transfers/{transferId}/dispatch |
| POST | /api/v1/vendor/shop/inventory/transfers/{transferId}/receive |
| POST | /api/v1/vendor/shop/inventory/transfers/{transferId}/cancel |
| GET | /api/v1/vendor/shop/inventory/stocktakes/{stocktakeId} |
| PUT | /api/v1/vendor/shop/inventory/stocktakes/{stocktakeId}/lines/{stockBasisId} |
| POST | /api/v1/vendor/shop/inventory/stocktakes/{stocktakeId}/post |
| PUT | /api/v1/vendor/shop/listings/{listingId}/inventory-policy |
| POST | /api/v1/vendor/shop/inventory/fulfilment-measurements |
| PUT | /api/v1/vendor/shop/inventory/stock-bases/{stockBasisId}/reorder-rule |
| POST | /api/v1/vendor/shop/inventory/alerts/{alertId}/acknowledge |
| POST | /api/v1/vendor/shop/inventory/alerts/{alertId}/resolve |
| POST | /api/v1/vendor/shop/bulk-operations |
| GET | /api/v1/vendor/shop/bulk-operations/{jobId} |
| POST | /api/v1/vendor/shop/bulk-operations/{jobId}/apply |
| GET | /internal/vendor/v1/shop/branches/{branchId}/catalogue |
| GET | /internal/vendor/v1/shop/branches/{branchId}/search |
| GET | /internal/vendor/v1/shop/branches/{branchId}/products/{productId} |
| GET | /internal/vendor/v1/shop/branches/{branchId}/variants/{variantId}/availability |
| GET | /internal/vendor/v1/shop/branches/{branchId}/variants/{variantId}/sellability |
| POST | /internal/vendor/v1/shop/branches/{branchId}/pricing/resolve |
| GET | /internal/vendor/v1/shop/branches/{branchId}/revision |
| POST | /internal/vendor/v1/shop/branches/{branchId}/inventory/reservations |
| POST | /internal/vendor/v1/shop/branches/{branchId}/inventory/reservations/{reservationId}/confirm |
| POST | /internal/vendor/v1/shop/branches/{branchId}/inventory/reservations/{reservationId}/commit |
| POST | /internal/vendor/v1/shop/branches/{branchId}/inventory/reservations/{reservationId}/release |
