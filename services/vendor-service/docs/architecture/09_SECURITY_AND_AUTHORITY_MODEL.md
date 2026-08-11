# 09 — Security and Authority Model

Updated after Section 5 (Authentication, Sessions and Staff IAM) implementation — this now describes the actual built model, not a plan. See [IMPLEMENTATION_STATUS.md](../IMPLEMENTATION_STATUS.md) for test evidence.

## Section 7 authority and privacy

Verification decisions belong only to `verification-compliance-service`; Vendor accepts them through HMAC-v2, allowlist, database nonce, event-id and source-version gates and independently polls authority until terminal. Bank resolution/storage belongs to Wallet; Vendor stores only a safe projection. File bytes/object validity belongs to Media; Vendor stores only an opaque id after tenant-scoped read-back. Bank replacement is serialized per business, checks `expected_version`, and consumes a `finance.bank_change` step-up grant before Wallet mutation. Section 7 audit writes share the domain/outbox transaction.

## Section 6 authority

The authenticated `StaffContext.VendorID` is the only business scope; body/path values never grant access. `VendorBusiness.id` reconciles to that provisional Section 5 scope and `owner_ref` is derived from `StaffID`; owner transfer is not exposed. Permissions are explicit and deny-by-default, including branch targets. Every SQL read/write includes `business_id`, same-business foreign keys prevent cross-brand/capability attachment, and production staff role assignment validates actual branch ownership. Vendor actors cannot activate, suspend, close, reject, approve, or verify; their only lifecycle command is operational `ACTIVE` to `PAUSED` or `PAUSED` to `ACTIVE` with optimistic concurrency.

## Inbound: who may call Vendor Service

- **Vendor staff (Vendor app)**: bearer access tokens issued by `POST /auth/verify`/`/auth/refresh`, signed via `shared/go/auth.TokenSigner` — the exact same shared JWT-like signer every other service uses, not a competing standard. Claims are deliberately minimal (`Subject`=staff id, `Role`="vendor_staff", `Service`="vendor", `SessionID`) — see "Token model" below for why.
- **Internal service-to-service callers**: `shared/go/serviceauth`, allowlisted via `VENDOR_SERVICE_SECRETS=caller=secret,...`. Section 7 adds `POST /internal/vendor/v1/verification-events`, restricted to `verification-compliance-service` and protected by a database-consumed nonce plus event dedupe.
- **Admin/operator actions**: still routed through `admin-backoffice-service` once it exists — unchanged from the Foundation phase's plan; no admin-specific auth path was added to Vendor Service.

## Token model — implemented

Access tokens carry only `{Subject, Role, Service, SessionID, Type, IssuedAt, ExpiresAt}` (`shared/go/auth.Claims`, unmodified). Full scope — vendor id, branch scope, effective permissions — is **never embedded in the JWT**. Instead, `vendorauthhttp.RequireAuth` (the one middleware every protected route passes through) re-resolves the complete context from the database on every single request:

1. Verify the JWT signature/expiry/role/service (`shared/go/auth.TokenSigner.Verify`).
2. Load the referenced session by `SessionID` (`vendor_sessions`); reject if revoked or expired, or if its `staff_id` doesn't match the token's `Subject`.
3. Load current staff status by id; reject with `403` if not `active`.
4. Set `authctx.StaffContext{StaffID, VendorID, SessionID}` (a shared, non-feature-owned package — see [03](03_VENDOR_SERVICE_MODULE_MAP.md)) into the request context for every downstream handler to read.

**This was a deliberate choice over extending the shared `Claims` struct** (which would have meant editing a type every other service in the platform also depends on) **and over embedding a permission-version claim** (which would only invalidate staleness at the next token refresh, not the next request). The cost is a database read on every authenticated request; the benefit, confirmed by `TestAuth_SuspendedStaffCannotRefresh` and the HTTP contract tests, is that suspension and permission changes take effect immediately — not at token expiry, not at next login.

**Never trust vendor_id/branch_id from a request body** — every handler reads scope from `authctx.StaffContext`, resolved server-side from the session, never from client input. Confirmed by `TestIAM_TenantIsolation_VendorACannotAccessVendorBStaff`.

## Outbound: how Vendor Service calls other services

Section 7 uses concrete HMAC-v2 clients for Verification, Wallet and Media, all with bounded HTTP timeouts and authoritative read-back on unknown mutation outcomes. Section 5 notification delivery remains HMAC-signed as `vendor-service`; its destination allowlist is still a deployment dependency.

## Replay protection

Unchanged: `shared/go/serviceauth.Verifier` still does not track consumed nonces at the shared-package level. Vendor Service's one inbound internal route (`provision-owner`) does not yet have its own `noncestore` — not built in Section 5, since that route is idempotent by design (same `vendor_id`+identifier returns the existing owner rather than erroring), which makes a bare replay harmless for this specific route today. A future internal route without that natural idempotency should add the `noncestore` pattern before going live.

## Idempotency

Two layers now exist, deliberately different in shape because they solve different problems:

- **`internal/platform/idempotency`** (Foundation phase, HTTP `Idempotency-Key` header pattern) — not used by any Section 5 route. OTP verify and session creation are safe against retries through their own atomic, narrower guards instead (see below), which are a better fit than a generic key-replay cache for a flow whose "retry" semantics are inherently one-shot (a second verify with the same code must fail, not replay the first success).
- **Atomic state-machine guards** (`internal/features/auth/repositories`): `OTPChallengeRepository.Consume` and `StepUpRepository.Consume` are both single `UPDATE ... WHERE status = 'active'` statements — `RowsAffected() == 0` means someone else (or an expiry/lock) already resolved it, so the caller can never "succeed twice." Proven under real concurrency by `TestAuth_ConcurrentVerifySucceedsOnlyOnce` (10 concurrent verifies, exactly 1 success) and the analogous step-up/assignment tests.
- **`AssignmentRepository.Create`** relies on a Postgres partial unique index (`(staff_id, role_id, COALESCE(branch_id,'')) WHERE revoked_at IS NULL`) so a duplicate-assignment race is refused at the database layer, not just checked-then-inserted in application code — proven by `TestIAM_DuplicateAssignmentPrevented`.

## Role/permission model — implemented

`internal/features/staff` owns roles/permissions/assignments. **Enforcement lives in the usecase layer, not the HTTP layer** — a deliberate divergence from the Foundation phase's plan (which, based on fleet-partner-service's precedent, expected HTTP-layer RBAC checks). The reason: Section 5's authorization decisions are not simple "does this role have this permission" lookups — they also require delegation checks (does the ACTOR hold the permission being granted, at a covering scope) and step-up-grant consumption, both of which need direct repository access and are naturally usecase-layer concerns. Keeping them in the usecase (`staffusecases.StaffService`/`RoleService`, both calling `AuthorizationEvaluator.Authorize` first) means the same authorization logic protects the internal `ProvisionOwner` call path too, not only HTTP requests.

`AuthorizationEvaluator.Authorize(ctx, staffID, permissionCode, Target{BranchID})` is deny-by-default: a vendor-wide grant (`branch_id IS NULL` on the assignment) covers any target; a branch-scoped grant only covers a target with the exact same branch id — never a vendor-wide target, never a different branch. This is what makes "a Branch Manager cannot gain another branch merely by submitting its branch ID" true at the data-model level, not just by convention — proven by `TestIAM_BranchScopedActorCannotEscapeBranch`.

**Role names never authorize anything** — `staffusecases.requirePermission` always takes a permission-code string, never a role name; `TestIAM_RoleNameAloneDoesNotAuthorize_OnlyPermissionDoes` proves a staff member holding a named role (Analyst/Auditor) is still refused an action their role's actual permission set doesn't cover.

**Privilege escalation is prevented structurally**, not just by convention: `delegationCheck` (shared by `StaffService.InviteStaff` and `RoleService.AssignRole`/`CreateCustomRole`/`SetRolePermissions`) requires every permission in the target role to be one the ACTING staff member already holds themselves, at a scope covering the target branch — and `vendor.owner.transfer` is hardcoded (`staffmodels.OwnerOnlyPermissionCodes`) as never delegatable to any role, system or custom, regardless of what the actor holds. Proven by `TestIAM_PrivilegeEscalation_ActorCannotDelegatePermissionTheyLack`.

**High-risk permission changes require step-up**: `RoleService.requireStepUp` consumes a step-up grant (purpose `iam.high_risk_permission_change`) via the narrow `StepUpConsumer` port before applying any role/assignment change whose resulting permission set includes an `is_high_risk` catalogue entry — even for the Owner, who is authorized to delegate every permission but must still step up to use that authority for a high-risk grant. Proven by `TestIAM_HighRiskRoleAssignmentRequiresStepUp`.

## Audit trail — implemented

`internal/audit.PostgresRecorder` persists to `vendor_audit_log` (migration `000004`). Every IAM mutation (`staff.provisioned_owner`, `.invited`, `.suspended`, `.reactivated`, `role.created`, `.permissions_changed`, `.assigned`, `.unassigned`) calls it with actor, action, aggregate, vendor/branch scope, before/after state, and reason where applicable — never raw OTP, tokens, or step-up secrets (nothing in the codebase constructs an `audit.Entry` containing one; confirmed by code review, not just convention). Not yet wrapped in the same Postgres transaction as its triggering mutation (a append-after-commit call, not append-in-transaction) — a known, disclosed gap: an audit write failing after a successful mutation would leave the mutation applied without its audit row. Acceptable for this phase (the mutation itself is always the source of truth; audit is a secondary trail) but worth closing with a shared transaction helper if a future section needs stronger audit guarantees.

## IDOR / cross-tenant isolation — implemented and tested

Every staff/role/assignment repository query is scoped by `vendor_id` in the SQL `WHERE` clause itself — `StaffRepository.GetByIDForVendor`, `RoleRepository.ListVisibleToVendor`/`GetVisibleToVendor` never "load by id, then check vendor_id in Go." A row belonging to another vendor produces the identical "not found" outcome as a genuinely nonexistent row, so a caller can never distinguish "wrong tenant" from "doesn't exist" through response content. Proven directly by `TestIAM_TenantIsolation_VendorACannotAccessVendorBStaff` (cross-vendor read, suspend, role-list-leak, and role-assignment attempts all refused).

`internal/architecture_test.go` remains the static guardrail against a different mistake class (cross-feature coupling, not cross-tenant leakage) — unchanged from the Foundation phase.
