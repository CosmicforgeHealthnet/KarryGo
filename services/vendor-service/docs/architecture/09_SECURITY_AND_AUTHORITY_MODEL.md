# 09 — Security and Authority Model

## Section 8I certification findings

Two authority defects were found by building the checks the earlier sections had
described but never mechanised.

**Internal ingress must not sit behind the public base.** Section 7 established
this rule for the verification callback and moved that route to the engine root.
`POST /internal/staff/provision-owner` — which creates the first Owner of a
vendor business, and is therefore one of the highest-authority operations this
service has — was still mounted under `/api/v1/vendor/`. It was HMAC-protected
throughout, so this was defence in depth rather than an open door, but the rule
exists precisely so that a gateway misconfiguration cannot become an authority
bypass. It now mounts at the root with the other internal routes, and a test
asserts no internal path is ever reachable under the public base.

**A remote call must not hold the privacy fence.** Section 8F's notification
handoff ran a synchronous HMAC call while holding a `FOR UPDATE` lock on the
customer profile row — the same lock the privacy and identity-merge handlers
take. A hanging notification-service could therefore block a redaction, which
turns an availability problem in one service into a compliance problem in
another. The handoff was restructured to release every lock before the network
call. What made that safe rather than merely faster is the stable
`NotificationRequestRef`: a retry after a crash re-sends with the same
idempotency key, so the remote side deduplicates and the customer is not
contacted twice.

The second fix also produced a useful negative result. Moving the contact-history
write out of the locked transaction alongside the network call broke the accepted
frequency-cap concurrency test — two deliveries for the same customer could both
pass the cap before either recorded a contact. The write was returned to the
reservation transaction, under the profile lock, and the test passes again. The
cap is now slightly conservative (a failed handoff still consumes a slot), which
is the correct direction for a rule that exists to protect the customer.

## Section 8H security model

**A score is not permission.** Stated first because everything else is subordinate to it. Consent, privacy, suppression, frequency and quiet hours are decided elsewhere and are not consulted to produce a score. There is no code path from a score to a send, and a real-Postgres test drives a high-churn customer who denied marketing consent through the segment engine and proves they are not selected.

**Business-wide intelligence is never shown to a branch actor.** A BUSINESS score summarises trade at branches a branch-scoped operator is not authorised to see, so they receive the BRANCH scope of a branch they hold or nothing — and they cannot order a business-wide rebuild either, because causing the figure is as good as reading it. An actor holding several branches must name one rather than being shown the first.

**Unavailable is not zero, enforced structurally.** A CHECK makes a non-CURRENT score row with a number in it unstorable, and the segment compilers read scores through a subquery requiring `state='CURRENT'` so a missing score yields SQL NULL and matches no numeric comparison. Absence can therefore never be mistaken for a low score, in the database or in a rule.

**No score can be written by hand.** There is no route, no permission and no column that lets a staff member declare a customer high-value or low-risk. A hand-set score would make every other score unfalsifiable, because a reader could no longer tell which numbers came from the model.

**Exports are the widest disclosure this service produces, and are gated accordingly.** Four separate permissions (read, create, download, cancel), because knowing an export exists is not reading it. Step-up on creation and again on download, purpose-bound and single-use. Tenant and branch scope applied before selection, so a branch actor cannot infer business-wide totals from a row count. Closed datasets and typed filters, so no client string reaches SQL. No PII, no free-text CRM notes, no payment or notification destination in any dataset. No stored URL: access is decided live, per request, and the token outlives neither the request nor the store's short window.

**Nothing partial is ever released.** `READY` has a database CHECK requiring a complete artifact, its checksum, its size, a completion instant and an expiry; promotion is a guarded update a concurrent cancellation wins; and a privacy re-certification between generation and publish refuses to release a file if any relationship stopped being exportable while it was being written.

## Section 8G security model

**Tenancy and nesting are enforced by the schema, not only by handlers.** Every child reaches its parent through a composite key — `(id, business_id)` for tenancy, `(id, business_id, corporate_customer_id)` for nested resources — so PostgreSQL refuses a membership naming another account's contact, or an interaction naming another vendor's site, even if application code were wrong. Every request derives its tenant and branch scope from the authenticated session; no request body can name a business.

**Branch scope cannot be widened from inside it.** A branch-scoped operator sees business-wide accounts plus branch-set accounts covering a branch they hold. Replacing an account's branch set cannot extend it past the caller's own scope and cannot remove the caller's own branch, so scope is never used to escape scope.

**Permissions are separated by disclosure, not by table.** Five permissions: read, account management, contacts, locations and cost centres. Linking a person to an organization is a different act from maintaining that organization's site list, so they are different grants. Corporate activity reuses Section 8C's permissions and additionally requires `vendor.crm.corporate.read`: a caller who may log interactions does not thereby learn which organizations this vendor works with.

**No PII, checked twice.** No Section 8G table or response holds an email address, phone number, government ID or device token; a schema test and a wire-level test each reject one. The only address in the section is the customer organization's own site record. A redacted relationship keeps its corporate link for continuity but is marked non-actionable with its label withheld — the link survives, the person is not addressable through it.

**Step-up.** Section 8G requires none. Recording who a vendor deals with neither moves money nor exports customer data, so the friction would buy nothing. Corporate ordering and Section 8H's sensitive export will each need their own decision.

## Section 8E security model

Tenant and branch scope are resolved server-side from the session on every campaign operation and are never read from the path or the body. A campaign outside the caller's tenant or branch is reported not-found rather than forbidden, so probing identifiers discloses nothing.

Five permissions, and the split that matters is the fifth: `vendor.crm.campaigns.targets.read` is separate from `.read` because listing the customers inside an audience is a strictly larger disclosure than reading the definition that produced it. It is flagged high-risk and is not granted to CRM/Support or Analyst by default. There is deliberately no `vendor.crm.campaigns.send`: this section cannot send, and a permission naming an authority the service does not hold would be granted long before the guard rails around it existed.

Branch isolation is enforced where the disclosure happens. A branch-scoped actor cannot see, preview or prepare a `BUSINESS`-scope campaign, because the audience size alone describes the whole business; and a `BRANCH` campaign may only use a `BRANCH` segment for its own branch, which is what stops a branch manager reaching business-wide customer data through a lawful-looking campaign.

Privacy is engine policy, not a campaign setting. There is no `include_redacted`, no `ignore_privacy`, and no definition field that could disable it — the only suppression a vendor controls is service recovery, which does not touch privacy. Redacted, restricted, marketing-suppressed, merged and archived relationships never become targets at all; they exist only as aggregate counts, so they are absent from the one table Section 8F will read.

No Section 8E table or response holds an email address, phone number, postal address, government ID or device token, and both a schema test and a wire-level test reject one. Step-up authentication is deliberately not required: planning a campaign neither sends a message nor exports customer data, so the friction would buy nothing. Section 8F's delivery and Section 8H's sensitive export will each need their own decision.

## Section 8C security model

Every query and mutation is tenant fenced and filtered through the established business/branch scope resolver. Branch actors cannot see business-only or other-branch rows, counts, history, search hits, or Customer 360 timestamps. Assignees must be active same-tenant staff with a role assignment covering the task scope. Corrections and task mutations use expected versions, commands use idempotency keys, and privacy/merge races serialize on the customer profile row. Plain-text validation rejects markup, data payloads, and unsafe control content; free text is excluded from audit, outbox, and structured logging.

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

## Section 8A — CRM authority and scope

**Permissions** (all added in `000011`, none high-risk): `vendor.crm.customers.read`, `vendor.crm.customers.manage`, `vendor.crm.customer360.read`, `vendor.crm.consent.read`, `vendor.crm.privacy.read`.

There is deliberately **no** `vendor.crm.consent.override`, `vendor.crm.privacy.override` or `vendor.crm.identity.force_merge`. No role, including Owner, can be granted authority the platform does not delegate to vendors, and a migration test asserts none is ever introduced.

**Branch scope.** CRM resolves an actor's scope per permission rather than asking a yes/no question: a vendor-wide grant sees business-level totals, while a branch-scoped grant sees only relationships that exist at its authorised branches. The restriction is applied inside SQL, through an `EXISTS` join over `crm_customer_branch_relations` — loading business-wide rows and filtering them in Go would leak existence and totals through counts and pagination. Branch-scoped commercial figures are recomputed from the visible branches alone, and `last_order_ref` is withheld because the most recent order may belong to an unauthorised branch.

**Indistinguishable denials.** Wrong tenant, unauthorised branch and redacted all return not-found, so relationship identifiers cannot be probed.

**Step-up.** Section 8A adds none: the reads are ordinary and archive/restore are low-risk. Sensitive CRM export belongs to Section 8H.

**Privacy over consent.** Eligibility precedence is redaction > merge > marketing suppression > restriction > archival > consent unavailable > DENY > UNKNOWN/stale > ALLOW. The decision function accepts no override parameter, so fail-closed is a property of the type rather than a convention callers must remember.

**Anti-resurrection.** A redacted subject's opaque reference is retained solely so a late event can be recognised and refused; it is never used for marketing, presentation, enrichment or matching. Hashing it was explicitly rejected — a hash is not anonymisation, and it would make suppression unreliable. This holds in both orderings: a late event after a redaction, and a redaction that arrives before any relationship exists.

## Section 8B — the segmentation rule language

The rule language is the only place in Vendor Service where a user's input shapes a database query, so it is treated as the security boundary it is.

**Closed registry, not a parser.** A condition can reference only a field defined in `NewSegmentFieldRegistry`, and each field's SQL is a hand-written Go function. Field keys, operators and values never become SQL text: identifiers are never derived from input, and values are always bind parameters. There is no free-text filter, no user-chosen sort column, no JSONPath, no user-supplied regex and no escape hatch anywhere in the surface. Adding a field is a reviewed code change.

**Forbidden fields are absent, not filtered.** Canonical email, phone, address, date of birth, credentials, subject references, privacy subject references, wallet balances, bank data, verification documents, support evidence and raw event payloads have no key in the registry. A test asserts their absence directly rather than asserting that a filter catches them — a filter can be bypassed; a field that does not exist cannot.

**Rejections disclose nothing.** An unknown field and a forbidden field return the same code, so a rejection cannot be used to map the schema. Database errors are translated before they reach a client, because their text names tables and columns.

**Complexity is bounded.** Depth, node count, OR branches, IN values, NOT depth and tag references all have configured ceilings, and exceeding one is a stable validation error rather than a query that takes the materializer down.

**Privacy is enforced outside the rule.** Redacted and merged-away relationships are excluded by the engine before any vendor condition is applied, and privacy state is not a targetable field — a vendor cannot write a rule that selects redacted customers, nor one that overrides consent. Eligibility is readable but never assertable: there is no `consent.override` field, and there could not be one, because eligibility is derived rather than stored.

**Unknown data cannot be mistaken for a value.** A condition over an unobserved projection compiles to SQL `NULL` and the rule is evaluated `IS TRUE`, so it matches nobody — and a `NOT` over it also matches nobody rather than selecting every customer with no data.

**Branch scope reaches into the rule.** A branch-scoped segment compiles its commercial conditions against that branch's relation row rather than the business aggregate, cannot reference another branch or another branch's private tag, and cannot be previewed at business scope. Every count is branch-filtered: the member count is the disclosure, not just the sample.

**Step-up.** Section 8B adds none. It builds audiences and sends nothing, so the friction would buy nothing. Campaign execution and sensitive export will need their own.

## Section 12 Shop inventory security boundary

Section 12 uses nine distinct Shop inventory authorities for read, receive,
reserve, adjust, transfer, stocktake, quarantine, policy and bulk operations.
Migration seeds grant only operationally appropriate roles; marketing, finance,
kitchen and pastry roles do not acquire inventory mutation merely through their
names. Every service method resolves tenant and branch scope before returning
or locking a nested resource, including reservation transitions whose id alone
would otherwise permit branch escape.

Public writes use the shared PostgreSQL idempotency store with body fingerprint
conflict detection. Inventory revisions and row locks serialize balances,
reservations, quarantines, transfers, expiry and stocktakes; bounded quantities
and non-negative database checks fail closed. Audit/outbox evidence commits in
the same transaction as each Section 12 mutation.

Internal discovery/reservation routes use HMAC v2 with exact method/path/body
binding, approved caller identities and database-consumed nonces. An empty
allowlist denies every caller. Wrong-tenant, wrong-branch and wrong-parent ids
remain indistinguishable from missing records. The discovery read uses one
repeatable-read, read-only PostgreSQL snapshot so catalogue, effective price,
sellability and inventory revisions cannot be assembled from different states.
