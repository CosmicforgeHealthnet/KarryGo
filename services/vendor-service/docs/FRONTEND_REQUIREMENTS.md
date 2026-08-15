# Frontend Requirements — Vendor Service Backend Contract Handoff

Plain-language backend contract handoff for the future frontend team (Vendor Flutter app, and the commerce surfaces added to the Customer app). No frontend code.

**Status: Backend work through Sections 12A and 12B is implemented.** No frontend was built. See [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md), [SECTION10_11_HANDOFF.md](SECTION10_11_HANDOFF.md), and [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md).

**Section 7 contract correction:** future clients must recognize exactly `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`. Plain `PENDING` is not a VerificationStatus. `PENDING_REVIEW` is unsatisfied and activation-blocking when its requirement applies. The root-level `/internal/vendor/v1/verification-events` callback is service-only and must never be called or exposed by frontend code.

## ⚠️ VENDOR AUTH OTP != LOGISTICS HANDOVER OTP/QR

These are two completely separate credential systems. The routes below (`/auth/*`) authenticate a Vendor staff member logging into the Vendor app. They have nothing to do with, and must never be used for, confirming a pickup or delivery custody handover between a logistics provider and a vendor/customer — that capability does not exist in the platform yet at all (see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md)). **The frontend team must never call a Vendor login/step-up OTP route to confirm delivery custody, and must never expect a delivery-handover credential to authenticate a Vendor staff session.**

## Section 5 — Authentication, Sessions and Staff IAM (full contract)

Full machine-readable contract: `docs/openapi/section5-auth-staff-iam.openapi.json`. Human-readable index: [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md).

**Auth model**: passwordless OTP (phone or email — see the phone-delivery caveat below). No username/password anywhere. `POST /auth/verify` returns `access_token` (short-lived, 15 min default) + `refresh_token` (long-lived, 30 days default, format `<opaque>` — treat as an opaque string, never parse it). Send `Authorization: Bearer <access_token>` on every protected route. When the access token expires, call `POST /auth/refresh` with the refresh token to get a new pair — the old refresh token is invalidated the instant a refresh succeeds (rotation), so never reuse it and never call refresh twice with the same token.

**Multi-vendor login**: if a login identity has active staff access at more than one vendor business, `POST /auth/verify` returns `{"requires_vendor_choice": true, "available_vendor_ids": [...]}` with **no tokens**. Show a vendor picker, then resubmit the exact same `POST /auth/verify` request with `vendor_id` added and the same `otp`/`challenge_id` — the code is still valid, it was not consumed by the first response.

**Session/permission staleness**: there is no client-side "am I still allowed to do X" cache — the backend re-resolves the caller's staff status and permissions on every single request from the database. If a staff member is suspended or their role changes mid-session, their very next API call (not just their next login) reflects it. The frontend should treat a sudden `403 forbidden` as "re-check what I can do," not as a bug.

**Step-up authentication**: for any operation the backend marks high-risk (see "High-risk operations" in [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md)), the frontend must first call `POST /auth/step-up/start` (with a `purpose` string, e.g. `"iam.high_risk_permission_change"`) while already logged in, prompt for the OTP that arrives, call `POST /auth/step-up/verify` to get a `grant_id`, then include that `grant_id` as `step_up_grant_id` in the actual high-risk request. The grant is single-use and expires in 5 minutes — if the high-risk request is rejected with a step-up-related `forbidden`, restart the step-up flow, don't retry the same grant.

**Staff/Role/Permission model**: permissions are opaque capability strings like `vendor.staff.manage` (see `GET /permissions` for the full catalogue with human-readable descriptions and an `is_high_risk` flag). **Never hardcode UI behavior on a role NAME** (e.g. "if role == Branch Manager, show X") — always check whether the specific permission the action needs is present in the staff member's effective set. There is no single "my effective permissions" endpoint yet — derive visibility from what actions succeed/fail, or (future) request one be added if a section needs it before the frontend build starts.

**Branch scoping**: a role assignment may be vendor-wide (`branch_id: null`) or scoped to one branch. A staff member's UI should reflect their actual scope — if all their assignments are branch-scoped, there is no "all branches" view for them; requesting data for a branch they aren't assigned to returns `403`/`404`, never partial data.

- **Routes required**: see [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md) — 21 total.
- **Authentication required**: none for `/auth/start|resend|verify|refresh|logout`; vendor staff bearer for everything else.
- **Permissions**: see the Register's Permission column; `GET /permissions` returns the live catalogue.
- **Request/response fields**: see the OpenAPI file for exact shapes per route.
- **Enum/state values**: staff `status` ∈ `{active, suspended}`. OTP challenge purpose ∈ `{login, step_up}` (backend-internal, never sent to the frontend). Assignment `branch_id` is `null` or a string.
- **Validation rules**: phone OR email required (not both) on auth routes; `otp`/`challenge_id` required on verify; `expected_version` required on version-guarded writes (suspend, reactivate, set-role-permissions).
- **Pagination**: none yet — `GET /staff`, `/roles`, `/permissions` return full lists (expected to stay small per vendor; add pagination if that assumption breaks in a later section).
- **Idempotency**: OTP verify/session creation is safe against retries via atomic server-side guards, not an `Idempotency-Key` header — do not add one client-side for auth routes. `POST /staff` (invite) is NOT idempotent — a duplicate submit creates a `409` conflict (already staff), which is safe to treat as success client-side.
- **`expectedVersion`**: required on `PUT /staff/:id/suspend`, `/reactivate`, `PUT /roles/:id/permissions`. Read the current `version` from the corresponding GET response before writing.
- **Realtime**: none yet — poll or re-fetch after a mutation.
- **Notification behavior**: OTP delivery is email-only in this session (see the phone caveat below); a start/resend call can fail with `service_unavailable` for phone identifiers — surface this clearly ("please use email to sign in").
- **Retry behavior**: rate-limited routes (`/auth/start`, `/resend`, `/verify`, `/refresh`, `/step-up/*`) return `429 rate_limited` with no `Retry-After` header yet — back off client-side (a few seconds) before retrying.
- **Error behavior**: standard envelope; auth failures deliberately return the SAME generic "Invalid or expired verification code" message for wrong-code, expired, locked, and wrong-challenge-id cases — do not attempt to distinguish these client-side beyond what the message says (this is intentional, to avoid account enumeration).
- **Feature flags**: none gate Section 5 — authentication is always on.
- **Backend dependencies**: `notification-service` (email OTP delivery — not yet allowlisted, see [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md) #3).
- **Frontend assumptions that are forbidden**: do not assume phone-number login works end-to-end in production (email-only for now); do not cache permission decisions across requests; do not build a "remember me forever" flow beyond the documented refresh-token TTL; do not build any UI that reads or displays an OTP code from anywhere other than the user's own inbox (`debug_otp` in the API response exists only in non-production debug builds and must never be wired into a production build's UI).

## Per-feature template (fill in when a section implements the backing feature)

- **Routes required**
- **Authentication required** (customer bearer / vendor staff bearer / none)
- **Permissions**
- **Request fields**
- **Response fields**
- **Enum/state values**
- **Validation rules**
- **Pagination**
- **Filtering**
- **Sorting**
- **Media/upload behavior**
- **Idempotency requirements**
- **`expectedVersion`/version behavior**
- **Realtime events**
- **Notification behavior**
- **Retry behavior**
- **Error behavior**
- **Feature flags** gating this route
- **Backend dependencies**
- **Frontend assumptions**

## Platform-wide contract rules fixed by this architecture phase (apply to every future route)

- **Envelope**: every response follows the platform-standard success/error envelope (`{"success": true, "data": {...}}` / `{"success": false, "error": {code, message, request_id, fields}}`) via `shared/go/httpx` middleware — no Vendor Service route will ever return a bare, un-enveloped JSON body.
- **Auth**: end-user routes use bearer tokens (customer via the shared-secret verification pattern every driver service already uses to accept `customer-service` tokens; vendor staff via `internal/features/auth`, not yet built). Internal service-to-service routes use `shared/go/serviceauth` HMAC, never a bearer token.
- **Idempotency**: any state-changing command (order creation, checkout, order accept/reject, purchase order submission) requires an `Idempotency-Key` header. A retried request with the same key and the same body is safely replayed, not re-executed. A retried request with the same key and a *different* body is refused as a conflict.
- **Optimistic concurrency**: any command mutating a versioned aggregate accepts `expectedVersion`; a stale version is refused with a conflict, never silently overwritten.
- **Pagination**: list endpoints use the platform's shared cursor/offset pagination convention (`shared/go/pagination`), not a bespoke per-feature scheme.
- **Money**: every amount is a kobo integer, never a float, matching the platform-wide rule.
- **Feature flags**: a route behind a disabled flag returns a clear, typed error (not a 404 that looks like a missing route, and not a silent no-op) once flag-gated routes exist — see [architecture/11_FEATURE_FLAG_AND_ROLLOUT_MODEL.md](architecture/11_FEATURE_FLAG_AND_ROLLOUT_MODEL.md).
- **Realtime**: where a feature needs push updates (order status, fulfilment status), it follows the platform's existing websocket/notification-service pattern (already proven for customer and provider apps), not a bespoke transport.

## Section 7 backend handoff (no frontend implemented)

The backend contract is under `/onboarding`. A future client should render progress, representative, document-reference metadata, six verification dimensions, remediation, masked bank profile, checklist, blockers and readiness. Upload bytes directly through `media-file-service`, then attach only its object id. Never offer Vendor users approve/verify controls.

Every mutation uses a fresh `Idempotency-Key`; representative, document, and bank mutations send `expected_version` (`0` on first creation). Bank replacement first obtains a `finance.bank_change` step-up grant and sends `step_up_grant_id`; never persist or log the full account number. No Flutter, mobile, web, or dashboard code was added.

All paths below are relative to `/api/v1/vendor`, require the Vendor bearer session, and use the standard success/error envelope. GET lists are bounded to the current Vendor aggregate and intentionally have no pagination/filter parameters.

| Method/path | Permission | Request / response and retry contract |
| --- | --- | --- |
| `GET /onboarding` | `vendor.onboarding.read` | Status/version, representative, projections, readiness |
| `GET /onboarding/progress` | `vendor.onboarding.read` | Same authoritative overview for progress rendering |
| `GET /onboarding/checklist` | `vendor.onboarding.read` | Data-driven requirements, satisfaction and blockers |
| `GET /onboarding/blockers` | `vendor.onboarding.read` | Current blocking requirements; safe to poll |
| `GET /onboarding/activation-readiness` | `vendor.onboarding.read` | `ready`, percentage, requirements, blockers |
| `POST /onboarding/submit` | `vendor.onboarding.manage` | Empty command; Idempotency-Key; marks submitted but never activates by request authority |
| `POST /onboarding/request-activation` | `vendor.onboarding.manage` | Empty command; Idempotency-Key; evaluates policy and returns readiness |
| `GET /onboarding/representative` | `vendor.onboarding.read` | Minimal representative detail and version |
| `PUT /onboarding/representative` | `vendor.onboarding.manage` | Fields/capacity/authorized/expected_version; replacement after Identity verification also needs `onboarding.representative_replace` step-up |
| `GET /onboarding/documents` | `vendor.onboarding.read` | Opaque references/metadata only; never bytes or signed URLs |
| `POST /onboarding/documents` | `vendor.onboarding.manage` | Scope, dimension, media_ref, type, `expected_version=0`; Media is validated before storage |
| `PATCH /onboarding/documents/:documentId` | `vendor.onboarding.manage` | Draft-only replacement with expected_version |
| `DELETE /onboarding/documents/:documentId` | `vendor.onboarding.manage` | JSON `expected_version`; draft-only removal |
| `GET /onboarding/verifications` | `vendor.verification.read` | All source-versioned projections |
| `GET /onboarding/verifications/:dimension` | `vendor.verification.read` | Exact dimension projection(s) |
| `POST /onboarding/verifications/:dimension/submit` | `vendor.verification.submit` | subject type/ref and document ids; returns authority state; configured remote ambiguity reconciles by client reference |
| `POST /onboarding/verifications/:dimension/resubmit` | `vendor.verification.submit` | Same contract for remediation; never accepts status/source fields |
| `GET /onboarding/bank-profile` | `vendor.bank_profile.read` | Masked Wallet projection only |
| `PUT /onboarding/bank-profile` | `vendor.bank_profile.manage` | Full account input is write-only; bank code, step-up grant, expected_version; returns masked projection |

Public clients must never call `POST /internal/vendor/v1/verification-events`. It is an HMAC-v2, allowlisted, nonce/event-deduped Verification callback and must never be exposed through a public gateway or contain frontend credentials.

Write behavior is gated by `VENDOR_SERVICE_ENABLED`, `VENDOR_ONBOARDING_WRITES_ENABLED`, `VENDOR_VERIFICATION_INTEGRATION_ENABLED` where applicable, and `COMMERCE_GLOBAL_KILL_SWITCH`. Reuse the same idempotency key only for an identical retry; a changed fingerprint or stale expected version returns 409. On a configured Verification timeout the backend may report an unknown outcome and reconcile; when the integration flag is disabled it records no submission. Treat 401 as session renewal, 403 as permission/step-up, 404 as tenant-scoped absence, 409 as reload/reconcile, 422 as field validation, and 503 as a disabled/unavailable dependency.

Verification statuses are exactly `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`. Dimensions are exactly `IDENTITY`, `BUSINESS`, `BANK`, `LOCATION`, `BRAND`, `FOOD_KITCHEN`. The frontend must never set either vocabulary as authority, must never substitute an ACTIVE command for verification, must never handle Verification HMAC credentials, and must upload document bytes through Media File Service before passing an opaque reference.

## Section 6 API handoff (no frontend implementation in this section)

Use `/api/v1/vendor` with the existing vendor bearer session and the Section 6 OpenAPI file. Implement business overview/edit, capability configuration, brand list/edit, branch list/edit, normalized hours, radius service area, explicit operations settings, and effective branch-capability views. Never send `vendor_id`, `owner_ref`, lifecycle status, or verification state as writable authority. All write retries must reuse their original `Idempotency-Key`; versioned writes must use the latest `expected_version` and treat HTTP 409 as a reload/reconcile state.

All routes below require vendor bearer auth and no step-up grant. GETs remain readable during rollout; every write requires `VENDOR_ONBOARDING_WRITES_ENABLED=true`, is blocked by `COMMERCE_GLOBAL_KILL_SWITCH=true`, and requires `Idempotency-Key`. `expected_version` is required for PATCH, pause/resume, hours, and update/upsert commands (use `0` for first service-area/settings/branch-capability configuration). The full request/response schemas and enums are in `openapi/section6-vendor-business.openapi.json`.

| Method | Path | Permission | Frontend result |
|---|---|---|---|
| GET | `/capability-types` | authenticated | Seven allowed types and family |
| GET / POST | `/businesses` | `vendor.business.read` / `.manage` | Paginated scope / new DRAFT business |
| GET / PATCH | `/businesses/:businessId` | `vendor.business.read` / `.manage` | Business / editable profile only |
| POST | `/businesses/:businessId/pause` | `vendor.business.manage` | ACTIVE to PAUSED |
| POST | `/businesses/:businessId/resume` | `vendor.business.manage` | PAUSED to ACTIVE only |
| GET / POST | `/businesses/:businessId/capabilities` | `vendor.capabilities.read` / `.manage` | Paginated list / DRAFT request |
| GET | `/businesses/:businessId/capabilities/:capabilityId` | `vendor.capabilities.read` | Capability projection |
| POST | `/businesses/:businessId/capabilities/:capabilityId/pause` | `vendor.capabilities.manage` | Pause one active capability |
| POST | `/businesses/:businessId/capabilities/:capabilityId/resume` | `vendor.capabilities.manage` | Resume one paused capability |
| GET / POST | `/businesses/:businessId/brands` | `vendor.brands.read` / `.manage` | Paginated list / NOT_STARTED brand |
| GET / PATCH | `/businesses/:businessId/brands/:brandId` | `vendor.brands.read` / `.manage` | Brand projection / allowed fields and opaque media refs |
| GET / POST | `/businesses/:businessId/branches` | `vendor.branches.read` / `.manage` | Paginated list / DRAFT branch |
| GET / PATCH | `/businesses/:businessId/branches/:branchId` | `vendor.branches.read` / `.manage` | Branch / profile-address-location update |
| POST | `/businesses/:businessId/branches/:branchId/pause` | `vendor.branches.manage` | Pause branch without rewriting children |
| POST | `/businesses/:businessId/branches/:branchId/resume` | `vendor.branches.manage` | Resume eligible paused branch |
| GET / PUT | `/businesses/:businessId/branches/:branchId/hours` | `vendor.branches.read` / `vendor.branch.hours.manage` | Normalized weekly intervals |
| GET / PUT | `/businesses/:businessId/branches/:branchId/service-area` | `vendor.branches.read` / `vendor.branch.settings.manage` | Versioned radius only |
| GET / PUT | `/businesses/:businessId/branches/:branchId/operations-settings` | `vendor.branches.read` / `vendor.branch.settings.manage` | Explicit schema-v1 settings |
| GET | `/businesses/:businessId/branches/:branchId/capabilities` | `vendor.capabilities.read` | Configured and effective states/reasons |
| PUT | `/businesses/:businessId/branches/:branchId/capabilities/:capabilityId` | `vendor.branch.capabilities.manage` | DRAFT configuration |
| POST | `/businesses/:businessId/branches/:branchId/capabilities/:capabilityId/pause` | `vendor.branch.capabilities.manage` | Pause one branch capability only |
| POST | `/businesses/:businessId/branches/:branchId/capabilities/:capabilityId/resume` | `vendor.branch.capabilities.manage` | Resume one eligible branch capability |

List responses use `data.items` plus `data.pagination {limit, offset, has_more}`. Supported query parameters are `limit` (1-100), `offset`, allowlisted `status`, `capability_type`, `brand_id`, `search` (maximum 100 characters), `sort`, and `direction=asc|desc` where applicable. Treat 400 as malformed/unknown input, 401 as sign-in required, 403 as permission/scope denial, 404 as absent from this business scope, 409 as lifecycle/version/idempotency conflict, 422 as validation feedback, and 503 as a rollout/kill-switch gate. Reuse the same idempotency key only for a byte-equivalent retry; a changed request with the same key returns 409.

## Known dependency: pickup/delivery handover confirmation

**PENDING_EXTERNAL_DEPENDENCY — LOGISTICS OTP/QR.** Any frontend flow depending on a receiver-facing OTP/QR confirmation step cannot be specified yet — the credential does not exist anywhere in the platform (see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md)). This blocks only the final confirmed-handover step of platform-delivery order tracking; it does not block cart, checkout, order-status display, or vendor-side order management screens, none of which depend on it.

## Known dependency: Vendor admin review surface

The six-dimension Verification authority now exists and is integrated. The remaining operator dependency is `admin-backoffice-service`, which is still a scaffold: it has no Vendor review UI/work queue. Clients must display `PENDING_REVIEW` honestly and must not invent a Vendor-side approval control.

## Section 8A — CRM Foundation, Customer 360, Consent and Privacy

### Hard rules for any CRM client

These are not style preferences; the backend enforces each one and will reject or ignore attempts to work around them.

- **Do not submit a customer-service customer id to create a CRM customer.** There is no create-customer route. Relationships come into existence only from an authenticated authority event. A vendor with no integrated commerce authority will legitimately see an empty CRM.
- **Do not attempt to set consent.** No route exists. Consent is a read-only projection of an external authority. A UI must never present a control that appears to grant marketing permission on a customer's behalf.
- **Do not attempt to set privacy state, or to un-redact.** No route exists.
- **Do not attempt to force a merge.** No route exists, and the backend performs no name/phone/email/device similarity matching.
- **Do not expect canonical customer PII.** CRM returns no email, phone, address or date of birth, and `display_label` is currently always `null`. If human-readable identity is needed, it must come from the customer authority through an approved contract that does not exist yet.

### Routes

All paths are relative to `/api/v1/vendor`. All require a Vendor staff bearer token. Errors use the standard envelope; validation failures are **422**, permission denials **403**, unknown/out-of-scope/redacted relationships **404**, version and state conflicts **409**, and disabled writes **503**.

#### `GET /crm/customers`

- **Permission**: `vendor.crm.customers.read`
- **Query**: `limit` (1–100, default 50), `offset`, `sort` (`last_completed_order_at` | `completed_order_count` | `net_spend_minor` | `created_at`), `direction` (`asc`|`desc`), `subject_type`, `status`, `privacy_state` (`ACTIVE`|`RESTRICTED` only), `branch_ref`, `capability_type`, `commercial_projection_state`, `acquisition_source_type`, `min_completed_orders`, `max_completed_orders`, `min_net_spend_minor`, `max_net_spend_minor`, `last_order_after`, `last_order_before` (RFC3339).
- **There is no free-text search parameter**, and no filter over email/phone/address — CRM does not hold those fields. Do not build a search box that implies otherwise.
- Requesting `privacy_state=REDACTED` is a 422. Filtering by a branch the caller is not authorised for is a 403.
- **Response**: `{items: [{profile, commercial, marketing_eligibility}], pagination: {limit, offset, has_more}}`.

#### `GET /crm/customers/{profileId}` — Customer 360

- **Permission**: `vendor.crm.customer360.read`
- **Response blocks**: `profile`, `commercial`, `branches[]`, `capability`, `consent`, `privacy`, `identity_link`, `modules`.

#### `GET /crm/customers/{profileId}/consent`

- **Permission**: `vendor.crm.consent.read`. Read-only.

#### `GET /crm/customers/{profileId}/privacy`

- **Permission**: `vendor.crm.privacy.read`. Read-only.

#### `POST /crm/customers/{profileId}/archive` and `.../restore`

- **Permission**: `vendor.crm.customers.manage`. **Feature flag**: `VENDOR_CRM_WRITES_ENABLED`.
- **Concurrency**: `expected_version` query parameter or `If-Match` header is **required**. Omitting it is a 422; a wrong value is a 409 — reload and retry with the current `profile.version`.
- Returns 409 on a redacted or merged relationship. Do not offer these actions for those states.
- No `Idempotency-Key` is required: both are version-guarded state transitions, and re-issuing the same transition is already a no-op.

### Reading `commercial` correctly — the one thing most likely to be got wrong

`commercial.state` must be read **before** any figure.

- `NOT_AVAILABLE` means no authoritative commercial data has ever been observed. **Every numeric field is absent (`null`/omitted).** Render "No order history yet" — never "₦0" and never "0 orders". Today this is the normal case, because no vendor commerce order publisher exists.
- `CURRENT` means the figures are trustworthy. `STALE` / `REBUILDING` / `ERROR` mean show the numbers with a freshness caveat.
- `commercial.stale` is a separate boolean for age within `CURRENT`.
- **All money is an integer count of minor units** (kobo). Divide by 100 for display; never parse as a float.
- `commercial.scope` is `BUSINESS` or `BRANCH`. When `BRANCH`, the figures cover **only the branches this user may see** — label them accordingly, never as the customer's total value. `last_order_ref` is omitted in that case because the most recent order may belong to a branch the user cannot see.
- `recency_days` and `order_frequency_per_year` are absent when there is nothing to derive them from. Do not substitute the profile creation date.

### Reading `consent` correctly

`consent.marketing` is keyed by channel and **always contains every channel** (`EMAIL`, `PUSH`, `IN_APP`, `SMS`). A missing key never means permission.

Each entry has `decision` (`ALLOW`|`DENY`|`UNKNOWN`), `eligible` (boolean), `reason`, `stale`, `observed_at`.

- **`eligible` is the only field that may gate a marketing action.** Do not re-derive it from `decision`: a customer can have a stored `ALLOW` and still be ineligible because of privacy suppression, restriction, staleness or archival.
- `UNKNOWN` is a real state meaning "we have no decision", not "no". Present it as "No consent on record — cannot contact", never as an opt-out the vendor may reverse.
- `consent.integration_available: false` means no consent authority is wired; every channel then reads `UNKNOWN`/ineligible. Show this as a system status, not as a property of the customer.
- Reasons: `CONSENT_ALLOWED`, `CONSENT_DENIED`, `CONSENT_UNKNOWN`, `CONSENT_STALE`, `CONSENT_SOURCE_UNAVAILABLE`, `PRIVACY_REDACTED`, `PRIVACY_RESTRICTED`, `PRIVACY_MARKETING_SUPPRESSED`, `PROFILE_MERGED`, `PROFILE_ARCHIVED`.

### Reading `privacy` correctly

`state` is `ACTIVE` | `RESTRICTED` | `REDACTED` | `MERGED`; plus `marketing_suppressed`, `redacted`, `redacted_at`, `integration_available`, `covered_domains[]`.

- `RESTRICTED`: still readable, not targetable. Disable marketing actions; keep operational history visible.
- `MERGED`: this relationship is no longer independent. Direct the user to `identity_link.merged_into_profile_ref`. Disable all actions.
- `REDACTED`: you will not normally encounter this — such relationships return 404 from every read. Handle 404 gracefully rather than assuming a bug.
- `marketing_suppressed: true` with `state: ACTIVE` is a valid combination: the relationship is intact but not targetable.

### Reading `modules`

With `VENDOR_CRM_SEGMENTATION_ENABLED=true`, Section 8B reports `tags` and `segments` as `AVAILABLE`. Section 8C similarly reports `interactions` and `tasks` as `AVAILABLE` only when `VENDOR_CRM_INTERACTIONS_ENABLED` and `VENDOR_CRM_TASKS_ENABLED` are enabled, and Section 8D reports `loyalty` as `AVAILABLE` only when `VENDOR_CRM_LOYALTY_ENABLED` is enabled. Disabled modules are omitted/unavailable; `campaigns`, `corporate`, and `intelligence` remain `NOT_IMPLEMENTED`.

### Feature flags and retries

- `VENDOR_CRM_ENABLED` off: the routes are **not registered**, so requests return **404**, not 503. A client must not interpret that as "customer not found" at the collection level.
- `VENDOR_CRM_WRITES_ENABLED` off (or the global commerce kill switch on): reads work, archive/restore return 503. Surface as "temporarily unavailable" and retry later; do not retry automatically in a tight loop.
- 409 from archive/restore is never retryable as-is — reload the profile and use the fresh `version`.

---

## Section 8B — Tags & Segmentation

21 public routes, no internal ones. All paths relative to `/api/v1/vendor`. Full schemas in `docs/openapi/section8b-crm-tags-segmentation.openapi.json`.

### Common contract

| Concern | Contract |
|---|---|
| Auth | Vendor staff bearer token on every route |
| Tenancy | Derived from the session. `business_id` is never accepted from a body, path or query |
| Pagination | `limit` (1-200, default 50) + `offset`; response carries `pagination.has_more` |
| Idempotency | **Every** POST and PATCH requires `Idempotency-Key`. Same key + same body replays the stored response; same key + different body is 409 |
| Concurrency | `expected_version` in the body on PATCH/archive/activate; stale version is 409 |
| Errors | Standard envelope. A stable machine code is in `error.details.code`; rule rejections also carry `error.details.path` and a `fields[].field` of `rule.<path>` |
| Search | `search` is a parameterised substring match on the name. There is no sort parameter and no filter expression syntax |

### Routes

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/crm/segment-fields` | `vendor.crm.segments.read` | The field/operator registry and complexity limits. **Fetch this before rendering a builder** |
| GET | `/crm/tags` | `vendor.crm.tags.read` | Filters: `scope_type`, `branch_ref`, `status`, `search`. Defaults to ACTIVE only |
| POST | `/crm/tags` | `vendor.crm.tags.manage` | 201. Body: `name`, `description?`, `scope{type,branch_ref?}` |
| POST | `/crm/tags/bulk-assign` | `vendor.crm.tags.manage` | Body: `tag_ref`, `profile_refs[]` (max 500) |
| POST | `/crm/tags/bulk-remove` | `vendor.crm.tags.manage` | Same body shape |
| GET | `/crm/tags/{tagId}` | `vendor.crm.tags.read` | |
| PATCH | `/crm/tags/{tagId}` | `vendor.crm.tags.manage` | `name?`, `description?`, `expected_version` |
| POST | `/crm/tags/{tagId}/archive` | `vendor.crm.tags.manage` | `expected_version?` |
| GET | `/crm/segments` | `vendor.crm.segments.read` | Filters: `segment_type`, `scope_type`, `branch_ref`, `status`, `evaluation_status`, `search` |
| POST | `/crm/segments` | `vendor.crm.segments.manage` | 201. Created in `DRAFT` |
| POST | `/crm/segments/preview` | `vendor.crm.segments.read` | Persists nothing |
| GET | `/crm/segments/{segmentId}` | `vendor.crm.segments.read` | Includes the current rule and its explanation |
| PATCH | `/crm/segments/{segmentId}` | `vendor.crm.segments.manage` | `name?`, `description?`, `rule?`, `change_reason?`, `expected_version` |
| POST | `/crm/segments/{segmentId}/activate` | `vendor.crm.segments.manage` | DRAFT to ACTIVE only |
| POST | `/crm/segments/{segmentId}/archive` | `vendor.crm.segments.manage` | Terminal; drops membership |
| POST | `/crm/segments/{segmentId}/materialize` | `vendor.crm.segments.manage` | **202**, asynchronous |
| GET | `/crm/segments/{segmentId}/members` | `vendor.crm.segments.members.read` | Current generation only |
| POST | `/crm/segments/{segmentId}/members/bulk-add` | `vendor.crm.segments.manage` | STATIC only; 409 `CRM_SEGMENT_STATIC_ONLY` otherwise |
| POST | `/crm/segments/{segmentId}/members/bulk-remove` | `vendor.crm.segments.manage` | STATIC only |
| POST | `/crm/segments/{segmentId}/exclusions/bulk-add` | `vendor.crm.segments.manage` | DYNAMIC only; 409 `CRM_SEGMENT_DYNAMIC_ONLY` otherwise |
| POST | `/crm/segments/{segmentId}/exclusions/bulk-remove` | `vendor.crm.segments.manage` | DYNAMIC only |

### Building a rule

A rule document is:

```json
{
  "schema_version": 1,
  "root": { "op": "AND", "children": [ ... ] },
  "exclusion": { ... }
}
```

A node is **either** a group (`op` + `children`) **or** a condition (`field` + `operator` + `value`). Sending both is a 422. `AND`/`OR` take one or more children; `NOT` takes exactly one.

A condition's `field` must be a key from `GET /crm/segment-fields`; its `operator` must be one that field publishes; its `value` must match the field's type:

| Field type | Value shape | Operators |
|---|---|---|
| `BOOLEAN` | `true` / `false` | `EQ`, `NEQ` |
| `INTEGER` | whole number | `EQ`, `NEQ`, `GT`, `GTE`, `LT`, `LTE`, `BETWEEN` (`[min,max]`) |
| `MONEY_MINOR` | `{"currency":"NGN","minor":100000}` | same as INTEGER |
| `ENUM` | string from `values`, or an array for `IN`/`NOT_IN` | `EQ`, `NEQ`, `IN`, `NOT_IN` (+ `EXISTS`/`NOT_EXISTS` where published) |
| `TIMESTAMP` | RFC3339 string | `BEFORE`, `AFTER`, `WITHIN_LAST` (`{"amount":30,"unit":"DAY"}`), `EXISTS`, `NOT_EXISTS` |
| `TAG_REFERENCE` | `{"tag_id":"crmtag_..."}` | `HAS_TAG`, `NOT_HAS_TAG` |

`EXISTS` and `NOT_EXISTS` take **no** `value`; sending one is a 422.

**There is no SQL, no column name, no raw predicate and no free-text query anywhere in this document.** Anything not in the registry is rejected with `CRM_SEGMENT_FIELD_NOT_SUPPORTED` — the same code an unknown field gets, so a rejection cannot be used to probe the schema.

### Handling unavailable data

Every commercial field is guarded by whether that relationship has observed commercial data at all.

- A condition over data that was never observed is **undecidable**, and selects nobody. A negation of it also selects nobody.
- Preview returns `unavailable_fields[]` and a `warnings[]` entry when a rule uses such a field, and a second warning when a `NOT` wraps one.
- This is the difference between "we do not know what this customer spent" and "this customer spent nothing". Never present a zero for the former.
- With the consent integration off, consent fields are unavailable rather than reading `UNKNOWN`. A rule cannot be used to make everyone eligible by asserting the absence of a decision.

### Reading a segment's freshness

`evaluation_status` is `NOT_EVALUATED` | `QUEUED` | `RUNNING` | `CURRENT` | `STALE` | `FAILED`.

- `member_count` is meaningful for every value except `NOT_EVALUATED`. On `FAILED` it is the **last successful** audience, not zero — a failed rebuild never deletes the previous membership.
- `materialized_definition_version` vs `definition_version`: when they differ, the audience predates the current rule.
- `stale_since` and `last_evaluation_error_code` explain why.
- A rule edit sets `STALE` and enqueues a rebuild automatically; the previous audience stays readable throughout.

### Preview response

`definition_hash`, `evaluation_as_of`, `match_count`, `count_state` (`EXACT` | `AT_LEAST`), `scope`, `sample[]`, `rule_explanation`, `unavailable_fields[]`, `warnings[]`.

`AT_LEAST` means the synchronous count budget was reached. It is a genuine lower bound, not an estimate — render it as "at least N".

Sample rows carry `profile_ref`, `display_label` (usually null), `commercial_projection_state`, `matched_conditions[]` and `unavailable_conditions[]`. The condition arrays are node paths (`root.children[0]`) for highlighting the builder. **There is no contact data in any Section 8B response.**

### Bulk operations

- Cap 500 per request; over it returns 422 with `CRM_BULK_OPERATION_TOO_LARGE` and `details.max_batch`.
- All or nothing: any invalid, out-of-scope, redacted or merged-away reference rejects the whole batch and writes nothing.
- Failures report a single not-found rather than a per-reference report — deliberately, so the endpoint cannot enumerate relationships across tenants or branches.
- `applied` counts rows actually changed; `skipped` counts references already in the requested state. Re-running a bulk operation is safe and reports `applied: 0`.

### Error codes (`error.details.code`)

`CRM_TAG_NOT_FOUND`, `CRM_TAG_ALREADY_EXISTS`, `CRM_TAG_ARCHIVED`, `CRM_TAG_SCOPE_DENIED`, `CRM_BULK_OPERATION_TOO_LARGE`, `CRM_SEGMENT_NOT_FOUND`, `CRM_SEGMENT_ALREADY_EXISTS`, `CRM_SEGMENT_INVALID_STATE`, `CRM_SEGMENT_RULE_REQUIRED`, `CRM_SEGMENT_RULE_NOT_ALLOWED`, `CRM_SEGMENT_RULE_INVALID`, `CRM_SEGMENT_FIELD_NOT_SUPPORTED`, `CRM_SEGMENT_OPERATOR_NOT_SUPPORTED`, `CRM_SEGMENT_VALUE_INVALID`, `CRM_SEGMENT_TOO_COMPLEX`, `CRM_SEGMENT_SCOPE_DENIED`, `CRM_SEGMENT_EVALUATION_FAILED`, `CRM_SEGMENT_STATIC_ONLY`, `CRM_SEGMENT_DYNAMIC_ONLY`, `CRM_PROFILE_REDACTED`, `CRM_PROFILE_MERGED`, `CRM_VERSION_CONFLICT`, `CRM_IDEMPOTENCY_CONFLICT`, `CRM_CROSS_TENANT_REFERENCE`, `CRM_SEGMENTATION_DISABLED`.

`CRM_SEGMENT_TOO_COMPLEX` carries the breached limit in `details` (e.g. `max_nodes`, `actual_depth`).

### Branch scope

- A branch-scoped caller cannot create, read or preview a `BUSINESS`-scope object; attempts return `CRM_TAG_SCOPE_DENIED` / `CRM_SEGMENT_SCOPE_DENIED`, and lists simply omit them.
- In Customer 360, a branch-scoped caller receives only tags and segment classifications owned by their permitted branches. BUSINESS tags, business-wide segments, other-branch labels, and global classification counts/metadata are omitted. This narrower disclosure rule does not change the main tag-management API.
- Tag member counts and segment member lists are filtered to the caller's branches.
- A branch-scoped rule reads that branch's slice of every commercial fact. "Two orders" means two orders **at that branch**, not two across the business.
- A branch-scoped rule cannot reference another branch or another branch's private tag; both return `CRM_SEGMENT_SCOPE_DENIED`.

### Feature flags and retries

- `VENDOR_CRM_ENABLED` or `VENDOR_CRM_SEGMENTATION_ENABLED` off: the routes are **not registered**, so requests return **404**, not 503. A client must not read that as "segment not found".
- `VENDOR_CRM_WRITES_ENABLED` off (or the global commerce kill switch on): reads and previews work; every command returns 503 with `CRM_SEGMENTATION_DISABLED` or the shared unavailable envelope.
- `VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED` off: definitions and previews still work, but `materialize` returns 503 and no segment will report `CURRENT` that has not already been built. Do not present a frozen audience as live.
- 409 from a version conflict is never retryable as-is: reload the object and use the fresh `version`.
- 202 from `materialize` means queued, not done. Poll the segment's `evaluation_status`; do not re-POST in a loop, as repeated requests return the same in-flight run.

## Section 8C API handoff

The exact 16 operations and schemas are authoritative in `openapi/section8c-crm-interactions-tasks.openapi.json`. Every POST/PATCH requires `Idempotency-Key`; corrections and all task mutations also require the latest `expected_version`. Send JSON with no unknown properties. On a 409, reload and reconcile; do not retry a stale version with a new key.

Write commands use the established Redis limiter per business/staff: 60 creates per minute and 120 correction/task mutations per minute. A 429 is retryable after waiting; a limiter outage fails closed with the established 503 response. Reuse the same idempotency key and payload when retrying.

Interaction inputs use `NOTE`, `PHONE_CALL`, `IN_PERSON`, `EMAIL`, `SMS`, `IN_APP`, `SOCIAL`, `SALES_FOLLOW_UP`, `SERVICE_FOLLOW_UP`, `FEEDBACK`, `ACCOUNT_REVIEW`, or `OTHER`; direction is `INBOUND`, `OUTBOUND`, `INTERNAL`, or `NOT_APPLICABLE`; visibility is `BUSINESS` or `BRANCH`. `EMAIL`, `SMS`, and `IN_APP` describe a recorded method only and never mean a notification was sent. Notes are plain text, not HTML/Markdown, with a 200-character subject and 5,000-character body ceiling.

Tasks use status `OPEN`, `IN_PROGRESS`, `COMPLETED`, or `CANCELLED` and priority `LOW`, `NORMAL`, `HIGH`, or `URGENT`. State changes use the action routes, never PATCH. `is_overdue` is derived for open/in-progress tasks whose UTC `due_at` has passed. The server validates assignee tenant, active status, and business/branch eligibility; clients must treat rejection as authoritative.

Customer 360 returns at most five recent interactions and five open tasks. Use `GET /crm/customers/{profileId}/interactions` and `/tasks` for full pagination. Branch-filtered responses intentionally omit inaccessible content and counts. Ordinary APIs return no activity content for a redacted profile and do not make a merged source independently actionable.

Permissions are `vendor.crm.interactions.read`, `.create`, `.manage`, and `vendor.crm.tasks.read`, `.create`, `.manage`. Owner, General Manager, CRM & Support, Marketing, and Branch Manager receive full capability within their established scope; Analyst/Auditor are read-only; operational roles receive none. No frontend may request or display canonical customer email, phone, address, government ID, payment data, credentials, audit internals, or outbox identifiers from this surface.

## Section 8D API handoff

The exact 6 operations and schemas are authoritative in `openapi/section8d-crm-loyalty.openapi.json`. There are no internal routes, and there is deliberately **no** `POST /earn` and **no** `POST /redeem`: the earning and redemption engines are service-internal primitives awaiting a trusted producer, so a client that needs to award points for an order is waiting on Section 9, not on a flag.

### Routes

| Method | Path | Permission |
|---|---|---|
| GET | `/crm/loyalty/program` | `vendor.crm.loyalty.read` |
| PATCH | `/crm/loyalty/program` | `vendor.crm.loyalty.manage` |
| GET | `/crm/customers/{profileId}/loyalty` | `vendor.crm.loyalty.read` |
| GET | `/crm/customers/{profileId}/loyalty/entries` | `vendor.crm.loyalty.read` |
| POST | `/crm/customers/{profileId}/loyalty/adjustments` | `vendor.crm.loyalty.adjust` |
| POST | `/crm/loyalty/entries/{entryId}/reverse` | `vendor.crm.loyalty.reverse` |

### Points are integers

Every points field on every request and response is a whole number (`int64`). Never parse one into a float, never format one with decimal places, and never divide one to display a "value". `points_name_singular` and `points_name_plural` are the vendor's display nouns and are the only string fields with "points" in the name.

### Programme configuration

`GET /crm/loyalty/program` returns `configured: false` and `version: 0` for a business that has never set loyalty up; the payload is still complete, so render it as the current state rather than an empty form. `PATCH` is a partial update: send only the fields being changed plus `expected_version`, which must be `0` on the first write and the current `version` thereafter.

Clearing an expiry window requires `expiry_policy: "NEVER"` together with `clear_expiry_days: true`. Sending `NEVER` while leaving an existing `expiry_days` in place is rejected with `LOYALTY_INVALID_EXPIRY_POLICY`, as is `FIXED_DAYS` with no window.

### Customer balance

`enrollment` is `NOT_ENROLLED` or `ENROLLED` and must be read before any figure: an unenrolled customer legitimately has no account, and rendering a zero balance would state something the server has not claimed. Reading this endpoint never creates an account.

`available_points` is the spendable number and already excludes anything past its expiry. `pending_expiry_points` is the portion already dead but not yet swept — display it as an explanation, never add it to the available figure. `next_expiry_at` and `points_expiring_next` describe the soonest surviving expiry date and its size.

### Points history

Default page 50, maximum 200, `limit`/`offset` pagination with `has_more`. Ordering is fixed (newest first) and there is no sort parameter. Filters are `entry_type` (one of the eight ledger types), `created_from` and `created_to`; an unrecognised `entry_type` is a 422, not an empty page.

`reversed_by_entry_ref` on a row means it has since been reversed; `reverses_entry_ref` on a `REVERSAL` row points back at its original. Use both to render corrections as a linked pair. Command keys and producer source references are deliberately absent from the payload.

### Commands

Both commands require a stable `Idempotency-Key` header. The same key with the same body replays the stored response; the same key with a different body is a 409 and must not be retried as-is. A missing key is a 400.

`POST .../loyalty/adjustments` takes `direction` (`CREDIT` or `DEBIT`), a positive `points`, and a mandatory `reason` (1-500 plain-text characters). It returns the posted entry **and** the refreshed summary, so a client does not need a follow-up read. The reason is written to the audit log only and appears in no response payload, ledger row, or emitted event.

`POST /crm/loyalty/entries/{entryId}/reverse` takes only a mandatory `reason`. Reversal is always full; there is no amount field.

### Error codes

Codes arrive in `error.details.code`. The ones a client must handle specifically: `LOYALTY_VERSION_CONFLICT` (reload the programme), `LOYALTY_IDEMPOTENCY_CONFLICT` (do not retry with the same key), `LOYALTY_INSUFFICIENT_POINTS`, `LOYALTY_ADJUSTMENT_LIMIT_EXCEEDED`, `LOYALTY_INVALID_EXPIRY_POLICY`, `LOYALTY_ENTRY_NOT_REVERSIBLE`, `LOYALTY_ENTRY_ALREADY_REVERSED` (refresh the row — someone else reversed it), `LOYALTY_REVERSAL_POINTS_ALREADY_CONSUMED` (offer a manual debit instead), `LOYALTY_PROGRAM_DISABLED`, `LOYALTY_PROGRAM_PAUSED`, `LOYALTY_ACCOUNT_NOT_FOUND`, `LOYALTY_ACCOUNT_NOT_ACTIONABLE`, `LOYALTY_CUSTOMER_REDACTED`, `LOYALTY_CUSTOMER_MERGED`, and `LOYALTY_INVALID_POINTS`.

### Flags and failure modes

- `VENDOR_CRM_LOYALTY_ENABLED` off: the routes are **not registered**, so requests return **404**, not 503. Treat that as "loyalty is not turned on for this deployment", not as "customer not found".
- `VENDOR_CRM_WRITES_ENABLED` off, or the global commerce kill switch on: reads work, commands return 503. Retry later; do not loop.
- Write commands share the established Redis limiter per business and staff member; a 429 is retryable after waiting, with the same key and payload.

## Section 8E API handoff

The exact 10 operations and schemas are authoritative in `openapi/section8e-crm-campaign-intelligence.openapi.json`. There are no internal routes, and there is deliberately **no** send, schedule, deliver or dispatch endpoint: Section 8E prepares audiences and Section 8F will execute them. A client that needs to actually contact a customer is waiting on Section 8F, not on a flag.

### Routes

| Method | Path | Permission |
|---|---|---|
| GET | `/crm/campaigns` | `vendor.crm.campaigns.read` |
| POST | `/crm/campaigns` | `vendor.crm.campaigns.manage` |
| GET | `/crm/campaigns/{campaignId}` | `vendor.crm.campaigns.read` |
| PATCH | `/crm/campaigns/{campaignId}` | `vendor.crm.campaigns.manage` |
| POST | `/crm/campaigns/{campaignId}/archive` | `vendor.crm.campaigns.manage` |
| POST | `/crm/campaigns/{campaignId}/preview` | `vendor.crm.campaigns.preview` |
| POST | `/crm/campaigns/{campaignId}/prepare` | `vendor.crm.campaigns.prepare` |
| GET | `/crm/campaigns/{campaignId}/snapshots` | `vendor.crm.campaigns.read` |
| GET | `/crm/campaigns/{campaignId}/snapshots/{snapshotId}` | `vendor.crm.campaigns.read` |
| GET | `/crm/campaigns/{campaignId}/snapshots/{snapshotId}/targets` | `vendor.crm.campaigns.targets.read` |

### Preparing is not permission to send

This is the contract detail most likely to be misread, so it is stated in the payloads rather than only in prose. Every target channel returns `send_time_recheck_required: true`, every snapshot returns `send_time_consent_recheck_required`, `send_time_frequency_recheck_required` and `send_time_quiet_hours_recheck_required` all true, and the target page restates `send_time_recheck_required` on every page so a client paging a large audience cannot lose it between requests. These are pinned to `true` in the schema and by a database check; a client must not treat them as a variable to branch on.

`snapshot_consent_decision`, `snapshot_consent_eligible`, `consent_as_of` and `consent_source_version` are **historical evidence of what was observed at preparation time**. They are never rewritten when a customer later changes their mind. Do not cache them as current consent, and do not present them to an operator as the customer's present-day preference.

### Consent fails closed

Only `ALLOW` yields `snapshot_eligibility_state: ELIGIBLE_AT_SNAPSHOT`. `DENY`, `UNKNOWN`, a missing projection and a lapsed `ALLOW` all yield `INELIGIBLE_CONSENT`, with `reason_code` one of `CONSENT_DENIED`, `CONSENT_UNKNOWN`, `CONSENT_STALE` or `CONSENT_SOURCE_UNAVAILABLE`. In a deployment with no consent authority integrated — the platform's state today — **every** channel returns `CONSENT_SOURCE_UNAVAILABLE`. A client must render that as "consent data is unavailable, so nobody is contactable", never as an empty result or a loading failure.

### Idempotency and concurrency

`POST /crm/campaigns`, `PATCH /crm/campaigns/{id}`, `POST .../archive` and `POST .../prepare` all require a stable `Idempotency-Key`; the same key with the same body replays the original response, and the same key with a different body is a 409. `POST .../preview` deliberately does **not** take a key, because it persists nothing.

`PATCH` requires `expected_version` and a stale value is `CRM_CAMPAIGN_VERSION_CONFLICT` — reload, do not retry. Archive accepts an optional `expected_version`; archiving an already-archived campaign is a successful no-op.

### Counts reconcile

`source_count - segment_exclusion_count - privacy_suppressed_count - service_recovery_suppressed_count = target_count`, and `control_count + treatment_count = target_count`, on both the preview and every snapshot. `consent_suppressed_count` is the number of targets for whom **no** campaign channel was eligible; it sits outside the funnel because it does not remove anybody from the audience. A client may rely on the funnel arithmetic.

`count_state` is `EXACT`. Privacy- and service-recovery-suppressed relationships never appear as target rows at all, so a client that only reads the target list will never see one.

### Preparation is asynchronous

`POST .../prepare` returns **202** with a snapshot in `BUILDING`. Poll `GET .../snapshots/{snapshotId}` until `CURRENT` (usable) or `FAILED`. A second prepare while one is in flight returns the same run rather than starting another; if a concurrent request lost the race the response is 409 `CRM_CAMPAIGN_PREPARATION_IN_PROGRESS`, which the client should resolve by reading the in-flight snapshot rather than retrying.

`FAILED` with `dead_lettered: true` will not retry itself. `error_code: CRM_CAMPAIGN_AUDIENCE_NOT_CURRENT` means the source segment's membership moved on; the fix is to prepare again, which pins the new generation.

A failed preparation never disturbs the previous audience: the campaign's `current_snapshot_ref` still points at the last snapshot that succeeded, and its targets are intact.

### Audience source

`source_segment_ref` must be a Section 8B segment of the same business, visible at the campaign's scope, and not archived. A dynamic segment must be `CURRENT`; anything else is refused with `CRM_CAMPAIGN_AUDIENCE_NOT_CURRENT` and the remedy is to rebuild the segment first. There is no way to supply a rule, a query or a customer list to this API.

### Errors

`CRM_CAMPAIGN_NOT_FOUND`, `CRM_CAMPAIGN_SCOPE_DENIED`, `CRM_CAMPAIGN_VERSION_CONFLICT`, `CRM_CAMPAIGN_INVALID_STATE`, `CRM_CAMPAIGN_AUDIENCE_NOT_FOUND`, `CRM_CAMPAIGN_AUDIENCE_NOT_CURRENT`, `CRM_CAMPAIGN_AUDIENCE_SCOPE_INVALID`, `CRM_CAMPAIGN_INVALID_CHANNEL`, `CRM_CAMPAIGN_INVALID_EXPERIMENT`, `CRM_CAMPAIGN_INVALID_FREQUENCY_POLICY`, `CRM_CAMPAIGN_INVALID_QUIET_HOURS`, `CRM_CAMPAIGN_PREPARATION_IN_PROGRESS`, `CRM_CAMPAIGN_PREPARATION_FAILED`, `CRM_CAMPAIGN_ALREADY_EXISTS`, `CRM_CAMPAIGN_SNAPSHOT_NOT_FOUND`, `CRM_CAMPAIGNS_DISABLED`, and the shared `CRM_IDEMPOTENCY_CONFLICT`. Codes travel in `error.details.code`; branch on the code, not the message.

A campaign or snapshot outside the caller's tenant or branch returns **404**, never 403. The two cases are deliberately indistinguishable, so a client must not infer existence from the status.

### Flags and failure modes

- `VENDOR_CRM_CAMPAIGNS_ENABLED` off: routes are **not registered**, so requests return **404**, not 503. Treat that as "campaigns are not turned on for this deployment".
- `VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED` off: reads, previews and existing snapshots keep working; only `POST .../prepare` returns 503. Never render an already-prepared audience as empty in this state.
- `VENDOR_CRM_WRITES_ENABLED` off, or the global commerce kill switch on: reads and previews work, commands return 503. Retry later; do not loop.
- Commands share the established Redis limiter per business and staff member; a 429 is retryable after waiting, with the same key and payload.

No frontend may request or display canonical customer email, phone, address, government ID, payment data, credentials, audit internals, or outbox identifiers from this surface. The campaign API returns none of them, and none may be joined in from elsewhere on a campaign screen.

## Section 8F execution API

Use the nine `/crm/campaigns/{campaignId}/runs...` operations documented in `API_ROUTE_REGISTER.md`. Create, pause, resume, and cancel require one stable `Idempotency-Key`; create also requires the campaign `expected_version`, while state mutations require the run `expected_version`. Reuse the same key and byte-equivalent payload after an ambiguous network failure.

Send `scheduled_for` only for `SCHEDULED`, in RFC3339 with an explicit offset. Paginate runs, deliveries, and history with `limit`/`offset` and `has_more`. Treat `HANDED_OFF`, `ACCEPTED`, and `DELIVERED` as different states. A cancel response only cancels work not yet handed off. All returned identifiers are opaque references; do not derive identity from them or attempt to obtain contact destinations elsewhere.

## Section 8G corporate CRM API

Use the 41 `/crm/corporate-customers...` operations documented in `API_ROUTE_REGISTER.md` and authoritative in `openapi/section8g-crm-corporate-customers.openapi.json`. There are no internal routes and deliberately **no** order, invoice, credit, budget, contract, quotation, wallet or approval endpoint: a client that needs corporate commerce is waiting on a section that does not exist yet, not on a flag.

Every command requires one stable `Idempotency-Key`; replaying it returns the original response, and reusing it with a different body is `CRM_IDEMPOTENCY_CONFLICT`. Every update, transition and archive additionally requires the `expected_version` you last read — `CRM_CORPORATE_CUSTOMER_VERSION_CONFLICT` means someone else changed the record, so re-read before retrying rather than looping.

Nested resources are addressed under their account and validated against it server-side: a contact, site or cost centre from another account of the same vendor is refused with `CRM_CORPORATE_NESTED_SCOPE_INVALID`, not silently ignored. `corporate_ref`, `contact_ref`, `customer_profile_ref` and the rest are opaque; do not parse them or derive identity from them.

Paginate every list with `limit`/`offset` and `has_more`. `sort` is allowlisted (`updated_at`, `created_at`, `organization_name`, `relationship_started_at`) and an unrecognised value falls back to `updated_at` rather than erroring. `search` is matched literally against the organization name — `%` and `_` are not wildcards.

Contact responses carry `customer_profile_ref`, an optional non-identifying `display_label`, `privacy_suppressed` and `actionable`. There is no contact destination anywhere in this API; do not attempt to resolve one from another service on the strength of a corporate link.

## Section 8H intelligence and export API

Use the 10 `/crm/intelligence...` and `/crm/exports...` operations documented in `API_ROUTE_REGISTER.md` and authoritative in `openapi/section8h-crm-intelligence-exports.openapi.json`. There are no internal routes and deliberately **no** endpoint that writes a score: no `POST /score`, no `PATCH /churn-score`, no VIP flag. Intelligence is derived, and a client that needs to change one is asking for a different feature.

`risk_score` is an integer 0-100 heuristic, not a probability — do not format it as a percentage, do not multiply it, and do not derive a confidence from it. RFM dimensions are 1-5 quintiles of the scope's scored cohort and are not comparable across businesses.

**Branch on state, never on the presence of a number.** `rfm_state` and `churn_state` always answer. When a state is `NOT_AVAILABLE` the numeric fields are absent, and the correct rendering is "not available" plus the reason code — never `0`, never `—` where a real zero could appear, and never a default band. `STALE` means the numbers are real but their inputs have moved; show them with `source_as_of`.

Every command requires one stable `Idempotency-Key`; replaying it returns the original response, and reusing it with a different body is `CRM_IDEMPOTENCY_CONFLICT`. `POST /crm/intelligence/rebuild` returns 202 and is idempotent by construction: a run already queued or running for the same scope and model is returned rather than duplicated.

**Export creation and download each require their own fresh step-up grant** (`crm.export.create` and `crm.export.download`). A grant is single-use and purpose-bound: one taken for download cannot create an export, and a spent one cannot be replayed. `CRM_EXPORT_STEP_UP_REQUIRED` means obtain a new grant, not retry.

Poll `GET /crm/exports/{exportId}` for status. `POST /crm/exports/{exportId}/download` is a POST because it changes state — it spends a grant, writes an audit record and mints a token. The response carries `download_path`, `download_token` and `expires_at`; send the token in a header, never in a query string, and never persist it. There is no URL, no `artifact_ref` and no permanent link in any response.

Paginate lists with `limit`/`offset` and `has_more`. `row_limit` defaults to 100000 and cannot exceed 250000; an export that would exceed it fails with `CRM_EXPORT_TOO_LARGE` rather than returning a short file. No exported dataset contains an email address, phone number, postal address, date of birth, government identifier, payment credential, notification destination or free-text CRM note — do not build a UI that expects one.

## Section 8I — final frontend contract note

The CRM backend is complete across Sections 8A–8H and the API surface is frozen
at 194 routes. Section 8I changed one path a client may already have been
calling:

- `POST /internal/staff/provision-owner` is an **internal, HMAC-signed** route
  and is no longer reachable under `/api/v1/vendor/`. It never was a frontend
  route; if any tooling called it through the public base, it must move to the
  engine root and sign with service credentials.

Two contracts govern every CRM screen and are worth restating because they are
easy to get wrong:

- **Branch on state, not on the presence of a value.** `NOT_AVAILABLE`,
  `UNKNOWN`, `STALE` and an absent projection are all distinct from zero and
  from false. Every module that can be unavailable says so in a dedicated field.
- **Never derive permission from CRM data.** No score, tag, segment or balance
  authorises contact. The only path to a customer is the campaign flow, which
  re-checks consent and privacy at handoff and will suppress with a safe reason
  code that the UI should surface verbatim.


---

## Section 8 completion — loyalty tiers contract

### Routes

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/api/v1/vendor/crm/loyalty/tiers` | `vendor.crm.loyalty.read` | Empty set at version 0 when unconfigured — not a 404 |
| PUT | `/api/v1/vendor/crm/loyalty/tiers` | `vendor.crm.loyalty.tiers.manage` | Atomic whole-ladder replacement; `Idempotency-Key` required |

Both are gated by `VENDOR_CRM_LOYALTY_ENABLED`; the write additionally by
`VENDOR_CRM_WRITES_ENABLED`. When loyalty is disabled the routes are not
registered at all, so a client sees 404 rather than an empty ladder.

### `LoyaltyTierSet` (GET response `data`)

```
{ "tiers": [ { "id", "tier_key", "display_name",
               "minimum_lifetime_points", "rank", "status",
               "version", "updated_at" } ],
  "version": 0, "updated_at": null }
```

`tiers` is ascending by `minimum_lifetime_points`; `rank` 1 is the lowest rung.
`version` is 0 when no ladder has ever been written — send it back on the write.

### PUT request

```
{ "tiers": [ { "tier_key", "display_name", "minimum_lifetime_points" } ],
  "expected_version": 3 }
```

Do **not** send `rank`; it is derived. Omit `expected_version` on a first write.
`tier_key` is uppercased server-side and must match `^[A-Z][A-Z0-9_]{1,39}$`.

Failures all carry `CRM_LOYALTY_TIER_CONFIG_INVALID`:
- **422** — duplicate key, duplicate threshold, more than 8 rungs, negative
  threshold, bad key format, empty display name.
- **409** — `expected_version` no longer matches.

Every refusal is total: a rejected write leaves the previous ladder untouched.

### `CustomerLoyaltyTier` (inside Customer 360 as `loyalty_tier`)

```
{ "state": "NOT_AVAILABLE" | "NO_TIER" | "CURRENT",
  "tier_key", "display_name", "rank",
  "lifetime_points", "next_tier_key", "points_to_next_tier",
  "configured_tier_count", "definition_set_version" }
```

**Availability semantics — the part that matters:**

- `NOT_AVAILABLE` is **not zero and not the bottom tier.** It means no ladder is
  configured for this business, or this relationship's loyalty account is
  redacted or merged away. No tier fields are populated.
- `NO_TIER` means a ladder exists and this customer has not reached its first
  rung. `next_tier_key` and `points_to_next_tier` describe that first rung.
- `CURRENT` means they hold `tier_key`. `next_tier_key` is absent on the top
  rung — do not treat its absence as an error.
- `lifetime_points` is **lifetime credited points net of reversed credit**, not
  the spendable balance shown in the loyalty block. They will differ whenever a
  customer has redeemed, and that is correct: redeeming does not demote. It also
  differs from the account's own lifetime counter, which by design never goes
  down: a reversed credit is excluded here, because a credit posted in error
  must not confer permanent status.
- There is no endpoint to set a customer's tier, and there will not be one.
  Standing is derived from the ledger.

### Completion projections

The Section 8 completion projections (category/item affinity, service
recovery, customer value band, relationship signals, corporate commercial and
RFQ summaries, automatic follow-up trigger metadata) are available in Customer
360 and the internal producer-ingress paths described in
`IMPLEMENTATION_STATUS.md`. Producer-owned facts may still report
`NOT_AVAILABLE` until their authoritative service sends them; clients must
render that state distinctly from zero or false. No frontend source is included
in this repository.

## Section 9A — food catalogue API contract for clients

Backend only; no frontend or mobile source was changed. This records the exact
contract a client integrates against. The authoritative document is
`docs/openapi/section9a-food-catalogue.openapi.json`.

### Base and auth

All 39 routes are under `/api/v1/vendor/food`, behind the ordinary vendor staff
bearer session. **There is no internal route and no customer-facing route in
this section**; a management GET is not discovery, and a customer app must not
be pointed at these paths. Section 9C's internal service contract is not a
frontend API; a future public consumer remains outside this implementation.

Registration is gated by `COMMERCE_FOOD_ENABLED`. When it is off the routes do
not exist at all (404), rather than answering 503 — a client should treat their
absence as "not enabled for this deployment".

### Envelopes

Success: `{"success": true, "data": {...}}`.
Failure: `{"success": false, "error": {"code", "message", "details": {"code": "FOOD_*"}, "fields": [...]}}`.

Branch on `error.details.code`, not on prose. The Section 9A codes are
`FOOD_CAPABILITY_NOT_SUPPORTED`, `FOOD_MENU_NOT_FOUND`, `FOOD_MENU_NOT_EDITABLE`,
`FOOD_MENU_NOT_PUBLISHABLE`, `FOOD_SECTION_NOT_FOUND`, `FOOD_ITEM_NOT_FOUND`,
`FOOD_VARIANT_NOT_FOUND`, `FOOD_MODIFIER_GROUP_NOT_FOUND`,
`FOOD_MODIFIER_NOT_FOUND`, `FOOD_STALE_VERSION`, `FOOD_INVALID_REORDER`,
`FOOD_DUPLICATE_RELATIONSHIP`, `FOOD_ENTITY_ARCHIVED`, `FOOD_DUPLICATE_NAME`,
`FOOD_MEDIA_UNAVAILABLE` and `FOOD_IDEMPOTENCY_CONFLICT`.

A resource outside the caller's business or branch scope returns **404, not
403**. That is deliberate and clients should not treat it as a bug.

### Money representation

`base_price_minor`, `price_override_minor` and `price_delta_minor` are **integer
minor units** of the business's currency, reported once per item as `currency`
(ISO-4217). Parse them as 64-bit integers. Never use a float, and never divide
before display in a way that round-trips through one. Valid range is
`0 .. 99_999_999_999`; a modifier delta may be zero but never negative.

### Concurrency

Every mutation of an existing row requires `expected_version` in the body, taken
from the last read. A stale value returns 409 `FOOD_STALE_VERSION`. Media
replacement also bumps the item's own version, so a client holding an item
loaded before a media change must reload before editing it.

### Idempotency

Send `Idempotency-Key` on every POST/PATCH/PUT/DELETE. The same key with the
same body replays the stored response; the same key with a different body is
409; a failed command releases the key so a genuine retry can run. This is the
same store Sections 6-8 use.

### Pagination

`GET /food/menus`, `GET /food/menus/{menuId}/sections/{sectionId}/items` and
`GET /food/modifier-groups` accept `limit` (1-100, default 50) and `offset`, and
return `pagination: {limit, offset, has_more}`. The other list endpoints are
naturally bounded collections of one parent's children.

All list endpoints accept `include_archived=true`; archived rows always sort
behind live ones.

### Ordering contract

`POST .../reorder` takes `{"ordered_ids": [...]}` — **every live id in that
collection, exactly once, in the new order**. The attachment reorder addresses
groups by `modifier_group_id`. Duplicates, unknown ids, ids belonging to another
parent and partial lists all return 422 `FOOD_INVALID_REORDER`. On success the
canonical order is returned; render from it.

### Media

`PUT /food/items/{itemId}/media` replaces the whole ordered set:
`{"media": [{"media_ref": "...", "is_primary": true}], "expected_version": N}`.
Maximum 12, no duplicates, at most one `is_primary` (if none is given the first
entry becomes the cover), and an empty array clears the set.

References must already exist in Media File Service and be owned by this
business, upload-complete, active and not unsafe or quarantined; anything else
is refused. A reference the media service cannot confirm returns 503
`FOOD_MEDIA_UNAVAILABLE` — the write is refused rather than storing something
unverified. **There is no upload endpoint here**: obtain the reference from
Media File Service first.

### Vocabularies

- Capability: `RESTAURANT`, `FAST_FOOD`.
- Menu and item status: `DRAFT`, `ACTIVE`, `PAUSED`, `ARCHIVED`.
- Section, variant, modifier group and modifier status: `ACTIVE`, `ARCHIVED`.
- Dietary and allergen codes: `^[A-Z0-9_]{2,40}$`, up to 16 and 24 respectively,
  normalised to upper case, de-duplicated and returned sorted.

Section 9B now owns the separate operational management contract for
`SOLD_OUT`/`PAUSED`, schedule windows, effective price/availability and
preparation/logistics profiles. Section 9C composes those authorities only for
allowlisted internal callers; clients must not synthesize those answers locally.

### Permissions

`vendor.food.catalogue.read`, `vendor.food.catalogue.manage` and
`vendor.food.catalogue.publish`. Publish is required for menu
activate/pause/archive and for setting an item to ACTIVE. Creating, editing and
archiving **modifier groups and modifiers additionally requires a business-wide
grant**, because those definitions are shared across every branch; a
branch-scoped caller receives 403 with an explanatory message and can still
attach, detach and reorder groups on their own branch's items.
## Sections 10/11 boundary

Cloud Kitchen and Pastry/Bakery management APIs are authenticated Vendor
operations surfaces only. No consumer frontend, checkout, payment, Order,
Inventory or delivery UI is introduced by these sections; any future consumer
experience must consume an accepted downstream contract.

## Section 12A Shop frontend contract

Backend routes under `/api/v1/vendor/shop` support a future authenticated
Vendor console for category management, products, variants/SKUs, barcodes,
measure definitions and aliases, local-unit calibration, fractional sell
units, packaging, branch operating models/listings, retail/wholesale prices,
quantity tiers, fresh attributes and handling profiles.

Clients must use the server-provided ids, versions, currency and controlled
vocabularies. Money is sent as integer minor units; physical quantities use
integer grams, millilitres, millimetres/count or bounded fractions/tolerances.
Do not infer package ratios from names, treat approximate calibration as exact,
or assume Paint/Derica/Mudu has one universal weight.

Aliases added to a platform measure are still tenant-specific. A calibration
with `branch_id` must be selected through the listing sell-unit command's
`calibration_id`; never copy it onto the business-wide sell unit. Disabled
listing sell units must remain unavailable even when a base or override price
exists.

All non-GET requests require `Idempotency-Key`. Mutable aggregates require
`expected_version`; a conflict is a user-visible concurrent edit, not a retry
with overwritten data. Respect per-operation permission visibility, but the API
remains the authority for tenant, branch and nested-resource scope.

Do not build stock counts, availability badges, receiving, reservations,
transfers, stocktakes, preorder/backorder execution or Shop customer discovery
from the Section 12A contract. Those capabilities are supplied by the separate
Section 12B contracts below. Checkout, payment and delivery execution remain
outside Vendor Service.

## Section 12B Shop inventory frontend contract

Authenticated Vendor management routes are under
`/api/v1/vendor/shop/branches/:branchId/inventory` plus the documented bulk,
receipt, reservation, transfer, stocktake, quarantine, reorder-rule and alert
resources. Use
`docs/openapi/section12b-shop-inventory.openapi.json` as the command/query
contract and generate a fresh `Idempotency-Key` per user command, reusing it
only for an identical network retry. Send loaded expected versions/revisions
where declared; treat 409 as reload/reconcile.

Never compute availability by subtracting cached fields in the client. Display
the server's balance and sellability projection, including catalogue and
inventory revisions, and refresh when either changes. Quantities are bounded
int64 base-unit integers; money remains integer minor units. Expired lots are
not sellable even before the reconciliation worker runs.

Internal discovery/reservation routes are rooted at
`/internal/vendor/v1/shop`, use HMAC-v2 caller credentials and nonce replay
protection, and must never be called from a browser or mobile client. A trusted
backend consumer may use
`docs/openapi/section12-shop-discovery.openapi.json`; frontend code receives
only that consumer's accepted public contract. Never embed service secrets or
construct internal signatures in client code.

Required screens and states are detailed in `UI_UX_REQUIREMENTS.md`. At minimum,
cover loading, empty, permission-disabled, validation, idempotent retry,
revision conflict, dependency unavailable, partial transfer, dry-run bulk row
errors, dead-letter bulk failure, and successful ACK/RESOLVE transitions.
