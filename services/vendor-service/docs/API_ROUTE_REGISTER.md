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

**High-risk operations requiring a step-up grant** (in addition to the permission check): creating/editing a role or custom-role permission set that includes any `is_high_risk` permission (`vendor.finance.bank_change`, `vendor.owner.transfer`, `vendor.data.export_sensitive`), and assigning a role that carries one. Step-up purpose used: `iam.high_risk_permission_change`.

Add one row per route the moment it is registered in code - not before, and not batched at the end of a section. A route existing in code but missing here is a documentation-gate failure. `internal/section5contract` verifies the 21 Section 5 operations and `internal/section6contract` verifies all 32 new Section 6 operations, for 53 cumulative Vendor routes.
