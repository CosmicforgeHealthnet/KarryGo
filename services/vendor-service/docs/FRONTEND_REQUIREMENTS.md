# Frontend Requirements â€” Vendor Service Backend Contract Handoff

Plain-language backend contract handoff for the future frontend team (Vendor Flutter app, and the commerce surfaces added to the Customer app). No frontend code.

**Status: Backend work through Sections 12A, 12B and Section 20 production operations is implemented.** No frontend was built. See [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md), [SECTION10_11_HANDOFF.md](SECTION10_11_HANDOFF.md), and [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md).

**Section 7 contract correction:** future clients must recognize exactly `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`. Plain `PENDING` is not a VerificationStatus. `PENDING_REVIEW` is unsatisfied and activation-blocking when its requirement applies. The root-level `/internal/vendor/v1/verification-events` callback is service-only and must never be called or exposed by frontend code.

## âš ï¸ VENDOR AUTH OTP != LOGISTICS HANDOVER OTP/QR

These are two completely separate credential systems. The routes below (`/auth/*`) authenticate a Vendor staff member logging into the Vendor app. They have nothing to do with, and must never be used for, confirming a pickup or delivery custody handover between a logistics provider and a vendor/customer â€” that capability does not exist in the platform yet at all (see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md)). **The frontend team must never call a Vendor login/step-up OTP route to confirm delivery custody, and must never expect a delivery-handover credential to authenticate a Vendor staff session.**

## Section 5 â€” Authentication, Sessions and Staff IAM (full contract)

Full machine-readable contract: `docs/openapi/section5-auth-staff-iam.openapi.json`. Human-readable index: [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md).

**Auth model**: passwordless OTP (phone or email â€” see the phone-delivery caveat below). No username/password anywhere. `POST /auth/verify` returns `access_token` (short-lived, 15 min default) + `refresh_token` (long-lived, 30 days default, format `<opaque>` â€” treat as an opaque string, never parse it). Send `Authorization: Bearer <access_token>` on every protected route. When the access token expires, call `POST /auth/refresh` with the refresh token to get a new pair â€” the old refresh token is invalidated the instant a refresh succeeds (rotation), so never reuse it and never call refresh twice with the same token.

**Multi-vendor login**: if a login identity has active staff access at more than one vendor business, `POST /auth/verify` returns `{"requires_vendor_choice": true, "available_vendor_ids": [...]}` with **no tokens**. Show a vendor picker, then resubmit the exact same `POST /auth/verify` request with `vendor_id` added and the same `otp`/`challenge_id` â€” the code is still valid, it was not consumed by the first response.

**Session/permission staleness**: there is no client-side "am I still allowed to do X" cache â€” the backend re-resolves the caller's staff status and permissions on every single request from the database. If a staff member is suspended or their role changes mid-session, their very next API call (not just their next login) reflects it. The frontend should treat a sudden `403 forbidden` as "re-check what I can do," not as a bug.

**Step-up authentication**: for any operation the backend marks high-risk (see "High-risk operations" in [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md)), the frontend must first call `POST /auth/step-up/start` (with a `purpose` string, e.g. `"iam.high_risk_permission_change"`) while already logged in, prompt for the OTP that arrives, call `POST /auth/step-up/verify` to get a `grant_id`, then include that `grant_id` as `step_up_grant_id` in the actual high-risk request. The grant is single-use and expires in 5 minutes â€” if the high-risk request is rejected with a step-up-related `forbidden`, restart the step-up flow, don't retry the same grant.

**Staff/Role/Permission model**: permissions are opaque capability strings like `vendor.staff.manage` (see `GET /permissions` for the full catalogue with human-readable descriptions and an `is_high_risk` flag). **Never hardcode UI behavior on a role NAME** (e.g. "if role == Branch Manager, show X") â€” always check whether the specific permission the action needs is present in the staff member's effective set. There is no single "my effective permissions" endpoint yet â€” derive visibility from what actions succeed/fail, or (future) request one be added if a section needs it before the frontend build starts.

**Branch scoping**: a role assignment may be vendor-wide (`branch_id: null`) or scoped to one branch. A staff member's UI should reflect their actual scope â€” if all their assignments are branch-scoped, there is no "all branches" view for them; requesting data for a branch they aren't assigned to returns `403`/`404`, never partial data.

- **Routes required**: see [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md) â€” 21 total.
- **Authentication required**: none for `/auth/start|resend|verify|refresh|logout`; vendor staff bearer for everything else.
- **Permissions**: see the Register's Permission column; `GET /permissions` returns the live catalogue.
- **Request/response fields**: see the OpenAPI file for exact shapes per route.
- **Enum/state values**: staff `status` âˆˆ `{active, suspended}`. OTP challenge purpose âˆˆ `{login, step_up}` (backend-internal, never sent to the frontend). Assignment `branch_id` is `null` or a string.
- **Validation rules**: phone OR email required (not both) on auth routes; `otp`/`challenge_id` required on verify; `expected_version` required on version-guarded writes (suspend, reactivate, set-role-permissions).
- **Pagination**: none yet â€” `GET /staff`, `/roles`, `/permissions` return full lists (expected to stay small per vendor; add pagination if that assumption breaks in a later section).
- **Idempotency**: OTP verify/session creation is safe against retries via atomic server-side guards, not an `Idempotency-Key` header â€” do not add one client-side for auth routes. `POST /staff` (invite) is NOT idempotent â€” a duplicate submit creates a `409` conflict (already staff), which is safe to treat as success client-side.
- **`expectedVersion`**: required on `PUT /staff/:id/suspend`, `/reactivate`, `PUT /roles/:id/permissions`. Read the current `version` from the corresponding GET response before writing.
- **Realtime**: none yet â€” poll or re-fetch after a mutation.
- **Notification behavior**: OTP delivery is email-only in this session (see the phone caveat below); a start/resend call can fail with `service_unavailable` for phone identifiers â€” surface this clearly ("please use email to sign in").
- **Retry behavior**: rate-limited routes (`/auth/start`, `/resend`, `/verify`, `/refresh`, `/step-up/*`) return `429 rate_limited` with no `Retry-After` header yet â€” back off client-side (a few seconds) before retrying.
- **Error behavior**: standard envelope; auth failures deliberately return the SAME generic "Invalid or expired verification code" message for wrong-code, expired, locked, and wrong-challenge-id cases â€” do not attempt to distinguish these client-side beyond what the message says (this is intentional, to avoid account enumeration).
- **Feature flags**: none gate Section 5 â€” authentication is always on.
- **Backend dependencies**: `notification-service` (email OTP delivery â€” not yet allowlisted, see [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md) #3).
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

- **Envelope**: every response follows the platform-standard success/error envelope (`{"success": true, "data": {...}}` / `{"success": false, "error": {code, message, request_id, fields}}`) via `shared/go/httpx` middleware â€” no Vendor Service route will ever return a bare, un-enveloped JSON body.
- **Auth**: end-user routes use bearer tokens (customer via the shared-secret verification pattern every driver service already uses to accept `customer-service` tokens; vendor staff via `internal/features/auth`, not yet built). Internal service-to-service routes use `shared/go/serviceauth` HMAC, never a bearer token.
- **Idempotency**: any state-changing command (order creation, checkout, order accept/reject, purchase order submission) requires an `Idempotency-Key` header. A retried request with the same key and the same body is safely replayed, not re-executed. A retried request with the same key and a *different* body is refused as a conflict.
- **Optimistic concurrency**: any command mutating a versioned aggregate accepts `expectedVersion`; a stale version is refused with a conflict, never silently overwritten.
- **Pagination**: list endpoints use the platform's shared cursor/offset pagination convention (`shared/go/pagination`), not a bespoke per-feature scheme.
- **Money**: every amount is a kobo integer, never a float, matching the platform-wide rule.
- **Feature flags**: a route behind a disabled flag returns a clear, typed error (not a 404 that looks like a missing route, and not a silent no-op) once flag-gated routes exist â€” see [architecture/11_FEATURE_FLAG_AND_ROLLOUT_MODEL.md](architecture/11_FEATURE_FLAG_AND_ROLLOUT_MODEL.md).
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

**PENDING_EXTERNAL_DEPENDENCY â€” LOGISTICS OTP/QR.** Any frontend flow depending on a receiver-facing OTP/QR confirmation step cannot be specified yet â€” the credential does not exist anywhere in the platform (see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md)). This blocks only the final confirmed-handover step of platform-delivery order tracking; it does not block cart, checkout, order-status display, or vendor-side order management screens, none of which depend on it.

## Known dependency: Vendor admin review surface

The six-dimension Verification authority now exists and is integrated. The remaining operator dependency is `admin-backoffice-service`, which is still a scaffold: it has no Vendor review UI/work queue. Clients must display `PENDING_REVIEW` honestly and must not invent a Vendor-side approval control.

## Section 8A â€” CRM Foundation, Customer 360, Consent and Privacy

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
- **Query**: `limit` (1â€“100, default 50), `offset`, `sort` (`last_completed_order_at` | `completed_order_count` | `net_spend_minor` | `created_at`), `direction` (`asc`|`desc`), `subject_type`, `status`, `privacy_state` (`ACTIVE`|`RESTRICTED` only), `branch_ref`, `capability_type`, `commercial_projection_state`, `acquisition_source_type`, `min_completed_orders`, `max_completed_orders`, `min_net_spend_minor`, `max_net_spend_minor`, `last_order_after`, `last_order_before` (RFC3339).
- **There is no free-text search parameter**, and no filter over email/phone/address â€” CRM does not hold those fields. Do not build a search box that implies otherwise.
- Requesting `privacy_state=REDACTED` is a 422. Filtering by a branch the caller is not authorised for is a 403.
- **Response**: `{items: [{profile, commercial, marketing_eligibility}], pagination: {limit, offset, has_more}}`.

#### `GET /crm/customers/{profileId}` â€” Customer 360

- **Permission**: `vendor.crm.customer360.read`
- **Response blocks**: `profile`, `commercial`, `branches[]`, `capability`, `consent`, `privacy`, `identity_link`, `modules`.

#### `GET /crm/customers/{profileId}/consent`

- **Permission**: `vendor.crm.consent.read`. Read-only.

#### `GET /crm/customers/{profileId}/privacy`

- **Permission**: `vendor.crm.privacy.read`. Read-only.

#### `POST /crm/customers/{profileId}/archive` and `.../restore`

- **Permission**: `vendor.crm.customers.manage`. **Feature flag**: `VENDOR_CRM_WRITES_ENABLED`.
- **Concurrency**: `expected_version` query parameter or `If-Match` header is **required**. Omitting it is a 422; a wrong value is a 409 â€” reload and retry with the current `profile.version`.
- Returns 409 on a redacted or merged relationship. Do not offer these actions for those states.
- No `Idempotency-Key` is required: both are version-guarded state transitions, and re-issuing the same transition is already a no-op.

### Reading `commercial` correctly â€” the one thing most likely to be got wrong

`commercial.state` must be read **before** any figure.

- `NOT_AVAILABLE` means no authoritative commercial data has ever been observed. **Every numeric field is absent (`null`/omitted).** Render "No order history yet" â€” never "â‚¦0" and never "0 orders". Today this is the normal case, because no vendor commerce order publisher exists.
- `CURRENT` means the figures are trustworthy. `STALE` / `REBUILDING` / `ERROR` mean show the numbers with a freshness caveat.
- `commercial.stale` is a separate boolean for age within `CURRENT`.
- **All money is an integer count of minor units** (kobo). Divide by 100 for display; never parse as a float.
- `commercial.scope` is `BUSINESS` or `BRANCH`. When `BRANCH`, the figures cover **only the branches this user may see** â€” label them accordingly, never as the customer's total value. `last_order_ref` is omitted in that case because the most recent order may belong to a branch the user cannot see.
- `recency_days` and `order_frequency_per_year` are absent when there is nothing to derive them from. Do not substitute the profile creation date.

### Reading `consent` correctly

`consent.marketing` is keyed by channel and **always contains every channel** (`EMAIL`, `PUSH`, `IN_APP`, `SMS`). A missing key never means permission.

Each entry has `decision` (`ALLOW`|`DENY`|`UNKNOWN`), `eligible` (boolean), `reason`, `stale`, `observed_at`.

- **`eligible` is the only field that may gate a marketing action.** Do not re-derive it from `decision`: a customer can have a stored `ALLOW` and still be ineligible because of privacy suppression, restriction, staleness or archival.
- `UNKNOWN` is a real state meaning "we have no decision", not "no". Present it as "No consent on record â€” cannot contact", never as an opt-out the vendor may reverse.
- `consent.integration_available: false` means no consent authority is wired; every channel then reads `UNKNOWN`/ineligible. Show this as a system status, not as a property of the customer.
- Reasons: `CONSENT_ALLOWED`, `CONSENT_DENIED`, `CONSENT_UNKNOWN`, `CONSENT_STALE`, `CONSENT_SOURCE_UNAVAILABLE`, `PRIVACY_REDACTED`, `PRIVACY_RESTRICTED`, `PRIVACY_MARKETING_SUPPRESSED`, `PROFILE_MERGED`, `PROFILE_ARCHIVED`.

### Reading `privacy` correctly

`state` is `ACTIVE` | `RESTRICTED` | `REDACTED` | `MERGED`; plus `marketing_suppressed`, `redacted`, `redacted_at`, `integration_available`, `covered_domains[]`.

- `RESTRICTED`: still readable, not targetable. Disable marketing actions; keep operational history visible.
- `MERGED`: this relationship is no longer independent. Direct the user to `identity_link.merged_into_profile_ref`. Disable all actions.
- `REDACTED`: you will not normally encounter this â€” such relationships return 404 from every read. Handle 404 gracefully rather than assuming a bug.
- `marketing_suppressed: true` with `state: ACTIVE` is a valid combination: the relationship is intact but not targetable.

### Reading `modules`

With `VENDOR_CRM_SEGMENTATION_ENABLED=true`, Section 8B reports `tags` and `segments` as `AVAILABLE`. Section 8C similarly reports `interactions` and `tasks` as `AVAILABLE` only when `VENDOR_CRM_INTERACTIONS_ENABLED` and `VENDOR_CRM_TASKS_ENABLED` are enabled, and Section 8D reports `loyalty` as `AVAILABLE` only when `VENDOR_CRM_LOYALTY_ENABLED` is enabled. Disabled modules are omitted/unavailable; `campaigns`, `corporate`, and `intelligence` remain `NOT_IMPLEMENTED`.

### Feature flags and retries

- `VENDOR_CRM_ENABLED` off: the routes are **not registered**, so requests return **404**, not 503. A client must not interpret that as "customer not found" at the collection level.
- `VENDOR_CRM_WRITES_ENABLED` off (or the global commerce kill switch on): reads work, archive/restore return 503. Surface as "temporarily unavailable" and retry later; do not retry automatically in a tight loop.
- 409 from archive/restore is never retryable as-is â€” reload the profile and use the fresh `version`.

---

## Section 8B â€” Tags & Segmentation

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

**There is no SQL, no column name, no raw predicate and no free-text query anywhere in this document.** Anything not in the registry is rejected with `CRM_SEGMENT_FIELD_NOT_SUPPORTED` â€” the same code an unknown field gets, so a rejection cannot be used to probe the schema.

### Handling unavailable data

Every commercial field is guarded by whether that relationship has observed commercial data at all.

- A condition over data that was never observed is **undecidable**, and selects nobody. A negation of it also selects nobody.
- Preview returns `unavailable_fields[]` and a `warnings[]` entry when a rule uses such a field, and a second warning when a `NOT` wraps one.
- This is the difference between "we do not know what this customer spent" and "this customer spent nothing". Never present a zero for the former.
- With the consent integration off, consent fields are unavailable rather than reading `UNKNOWN`. A rule cannot be used to make everyone eligible by asserting the absence of a decision.

### Reading a segment's freshness

`evaluation_status` is `NOT_EVALUATED` | `QUEUED` | `RUNNING` | `CURRENT` | `STALE` | `FAILED`.

- `member_count` is meaningful for every value except `NOT_EVALUATED`. On `FAILED` it is the **last successful** audience, not zero â€” a failed rebuild never deletes the previous membership.
- `materialized_definition_version` vs `definition_version`: when they differ, the audience predates the current rule.
- `stale_since` and `last_evaluation_error_code` explain why.
- A rule edit sets `STALE` and enqueues a rebuild automatically; the previous audience stays readable throughout.

### Preview response

`definition_hash`, `evaluation_as_of`, `match_count`, `count_state` (`EXACT` | `AT_LEAST`), `scope`, `sample[]`, `rule_explanation`, `unavailable_fields[]`, `warnings[]`.

`AT_LEAST` means the synchronous count budget was reached. It is a genuine lower bound, not an estimate â€” render it as "at least N".

Sample rows carry `profile_ref`, `display_label` (usually null), `commercial_projection_state`, `matched_conditions[]` and `unavailable_conditions[]`. The condition arrays are node paths (`root.children[0]`) for highlighting the builder. **There is no contact data in any Section 8B response.**

### Bulk operations

- Cap 500 per request; over it returns 422 with `CRM_BULK_OPERATION_TOO_LARGE` and `details.max_batch`.
- All or nothing: any invalid, out-of-scope, redacted or merged-away reference rejects the whole batch and writes nothing.
- Failures report a single not-found rather than a per-reference report â€” deliberately, so the endpoint cannot enumerate relationships across tenants or branches.
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

`available_points` is the spendable number and already excludes anything past its expiry. `pending_expiry_points` is the portion already dead but not yet swept â€” display it as an explanation, never add it to the available figure. `next_expiry_at` and `points_expiring_next` describe the soonest surviving expiry date and its size.

### Points history

Default page 50, maximum 200, `limit`/`offset` pagination with `has_more`. Ordering is fixed (newest first) and there is no sort parameter. Filters are `entry_type` (one of the eight ledger types), `created_from` and `created_to`; an unrecognised `entry_type` is a 422, not an empty page.

`reversed_by_entry_ref` on a row means it has since been reversed; `reverses_entry_ref` on a `REVERSAL` row points back at its original. Use both to render corrections as a linked pair. Command keys and producer source references are deliberately absent from the payload.

### Commands

Both commands require a stable `Idempotency-Key` header. The same key with the same body replays the stored response; the same key with a different body is a 409 and must not be retried as-is. A missing key is a 400.

`POST .../loyalty/adjustments` takes `direction` (`CREDIT` or `DEBIT`), a positive `points`, and a mandatory `reason` (1-500 plain-text characters). It returns the posted entry **and** the refreshed summary, so a client does not need a follow-up read. The reason is written to the audit log only and appears in no response payload, ledger row, or emitted event.

`POST /crm/loyalty/entries/{entryId}/reverse` takes only a mandatory `reason`. Reversal is always full; there is no amount field.

### Error codes

Codes arrive in `error.details.code`. The ones a client must handle specifically: `LOYALTY_VERSION_CONFLICT` (reload the programme), `LOYALTY_IDEMPOTENCY_CONFLICT` (do not retry with the same key), `LOYALTY_INSUFFICIENT_POINTS`, `LOYALTY_ADJUSTMENT_LIMIT_EXCEEDED`, `LOYALTY_INVALID_EXPIRY_POLICY`, `LOYALTY_ENTRY_NOT_REVERSIBLE`, `LOYALTY_ENTRY_ALREADY_REVERSED` (refresh the row â€” someone else reversed it), `LOYALTY_REVERSAL_POINTS_ALREADY_CONSUMED` (offer a manual debit instead), `LOYALTY_PROGRAM_DISABLED`, `LOYALTY_PROGRAM_PAUSED`, `LOYALTY_ACCOUNT_NOT_FOUND`, `LOYALTY_ACCOUNT_NOT_ACTIONABLE`, `LOYALTY_CUSTOMER_REDACTED`, `LOYALTY_CUSTOMER_MERGED`, and `LOYALTY_INVALID_POINTS`.

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

Only `ALLOW` yields `snapshot_eligibility_state: ELIGIBLE_AT_SNAPSHOT`. `DENY`, `UNKNOWN`, a missing projection and a lapsed `ALLOW` all yield `INELIGIBLE_CONSENT`, with `reason_code` one of `CONSENT_DENIED`, `CONSENT_UNKNOWN`, `CONSENT_STALE` or `CONSENT_SOURCE_UNAVAILABLE`. In a deployment with no consent authority integrated â€” the platform's state today â€” **every** channel returns `CONSENT_SOURCE_UNAVAILABLE`. A client must render that as "consent data is unavailable, so nobody is contactable", never as an empty result or a loading failure.

### Idempotency and concurrency

`POST /crm/campaigns`, `PATCH /crm/campaigns/{id}`, `POST .../archive` and `POST .../prepare` all require a stable `Idempotency-Key`; the same key with the same body replays the original response, and the same key with a different body is a 409. `POST .../preview` deliberately does **not** take a key, because it persists nothing.

`PATCH` requires `expected_version` and a stale value is `CRM_CAMPAIGN_VERSION_CONFLICT` â€” reload, do not retry. Archive accepts an optional `expected_version`; archiving an already-archived campaign is a successful no-op.

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

Every command requires one stable `Idempotency-Key`; replaying it returns the original response, and reusing it with a different body is `CRM_IDEMPOTENCY_CONFLICT`. Every update, transition and archive additionally requires the `expected_version` you last read â€” `CRM_CORPORATE_CUSTOMER_VERSION_CONFLICT` means someone else changed the record, so re-read before retrying rather than looping.

Nested resources are addressed under their account and validated against it server-side: a contact, site or cost centre from another account of the same vendor is refused with `CRM_CORPORATE_NESTED_SCOPE_INVALID`, not silently ignored. `corporate_ref`, `contact_ref`, `customer_profile_ref` and the rest are opaque; do not parse them or derive identity from them.

Paginate every list with `limit`/`offset` and `has_more`. `sort` is allowlisted (`updated_at`, `created_at`, `organization_name`, `relationship_started_at`) and an unrecognised value falls back to `updated_at` rather than erroring. `search` is matched literally against the organization name â€” `%` and `_` are not wildcards.

Contact responses carry `customer_profile_ref`, an optional non-identifying `display_label`, `privacy_suppressed` and `actionable`. There is no contact destination anywhere in this API; do not attempt to resolve one from another service on the strength of a corporate link.

## Section 8H intelligence and export API

Use the 10 `/crm/intelligence...` and `/crm/exports...` operations documented in `API_ROUTE_REGISTER.md` and authoritative in `openapi/section8h-crm-intelligence-exports.openapi.json`. There are no internal routes and deliberately **no** endpoint that writes a score: no `POST /score`, no `PATCH /churn-score`, no VIP flag. Intelligence is derived, and a client that needs to change one is asking for a different feature.

`risk_score` is an integer 0-100 heuristic, not a probability â€” do not format it as a percentage, do not multiply it, and do not derive a confidence from it. RFM dimensions are 1-5 quintiles of the scope's scored cohort and are not comparable across businesses.

**Branch on state, never on the presence of a number.** `rfm_state` and `churn_state` always answer. When a state is `NOT_AVAILABLE` the numeric fields are absent, and the correct rendering is "not available" plus the reason code â€” never `0`, never `â€”` where a real zero could appear, and never a default band. `STALE` means the numbers are real but their inputs have moved; show them with `source_as_of`.

Every command requires one stable `Idempotency-Key`; replaying it returns the original response, and reusing it with a different body is `CRM_IDEMPOTENCY_CONFLICT`. `POST /crm/intelligence/rebuild` returns 202 and is idempotent by construction: a run already queued or running for the same scope and model is returned rather than duplicated.

**Export creation and download each require their own fresh step-up grant** (`crm.export.create` and `crm.export.download`). A grant is single-use and purpose-bound: one taken for download cannot create an export, and a spent one cannot be replayed. `CRM_EXPORT_STEP_UP_REQUIRED` means obtain a new grant, not retry.

Poll `GET /crm/exports/{exportId}` for status. `POST /crm/exports/{exportId}/download` is a POST because it changes state â€” it spends a grant, writes an audit record and mints a token. The response carries `download_path`, `download_token` and `expires_at`; send the token in a header, never in a query string, and never persist it. There is no URL, no `artifact_ref` and no permanent link in any response.

Paginate lists with `limit`/`offset` and `has_more`. `row_limit` defaults to 100000 and cannot exceed 250000; an export that would exceed it fails with `CRM_EXPORT_TOO_LARGE` rather than returning a short file. No exported dataset contains an email address, phone number, postal address, date of birth, government identifier, payment credential, notification destination or free-text CRM note â€” do not build a UI that expects one.

## Section 8I â€” final frontend contract note

The CRM backend is complete across Sections 8Aâ€“8H and the API surface is frozen
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

## Section 8 completion â€” loyalty tiers contract

### Routes

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/api/v1/vendor/crm/loyalty/tiers` | `vendor.crm.loyalty.read` | Empty set at version 0 when unconfigured â€” not a 404 |
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
`version` is 0 when no ladder has ever been written â€” send it back on the write.

### PUT request

```
{ "tiers": [ { "tier_key", "display_name", "minimum_lifetime_points" } ],
  "expected_version": 3 }
```

Do **not** send `rank`; it is derived. Omit `expected_version` on a first write.
`tier_key` is uppercased server-side and must match `^[A-Z][A-Z0-9_]{1,39}$`.

Failures all carry `CRM_LOYALTY_TIER_CONFIG_INVALID`:
- **422** â€” duplicate key, duplicate threshold, more than 8 rungs, negative
  threshold, bad key format, empty display name.
- **409** â€” `expected_version` no longer matches.

Every refusal is total: a rejected write leaves the previous ladder untouched.

### `CustomerLoyaltyTier` (inside Customer 360 as `loyalty_tier`)

```
{ "state": "NOT_AVAILABLE" | "NO_TIER" | "CURRENT",
  "tier_key", "display_name", "rank",
  "lifetime_points", "next_tier_key", "points_to_next_tier",
  "configured_tier_count", "definition_set_version" }
```

**Availability semantics â€” the part that matters:**

- `NOT_AVAILABLE` is **not zero and not the bottom tier.** It means no ladder is
  configured for this business, or this relationship's loyalty account is
  redacted or merged away. No tier fields are populated.
- `NO_TIER` means a ladder exists and this customer has not reached its first
  rung. `next_tier_key` and `points_to_next_tier` describe that first rung.
- `CURRENT` means they hold `tier_key`. `next_tier_key` is absent on the top
  rung â€” do not treat its absence as an error.
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

## Section 9A â€” food catalogue API contract for clients

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
not exist at all (404), rather than answering 503 â€” a client should treat their
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

`POST .../reorder` takes `{"ordered_ids": [...]}` â€” **every live id in that
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
`FOOD_MEDIA_UNAVAILABLE` â€” the write is refused rather than storing something
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

## Section 13 â€” Catalogue, Taxonomy and Search (backend contract)

**No frontend was built.** This is the contract handoff.

### âš ï¸ Most of Section 13 is NOT callable from frontend code

Of Section 13's 60 routes, **50 are root-mounted internal routes** authenticated
with serviceauth HMAC-v2. They require a shared secret, a signature over the
request, a timestamp and a single-use nonce.

**Frontend code must never construct an HMAC-v2 signature.** Doing so would mean
shipping a platform-wide service secret to a device, which would hand any user
who unpacks the app the ability to publish taxonomy for every vendor on the
marketplace. If the customer app needs discovery, it calls a backend service
that is on the allowlist, and that service calls Vendor Service.

**The 10 routes below are the entire vendor-facing Section 13 surface.**

### Vendor taxonomy â€” `/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/...`

Bearer token (Section 5 session). `namespaceCode` is `FOOD` or `SHOP` â€” always
explicit, never inferred.

| Method | Path suffix | Permission | Purpose |
|---|---|---|---|
| GET | `/published-version` | `vendor.taxonomy.read` | The published canonical version |
| GET | `/facets` | `vendor.taxonomy.read` | Published facets |
| GET | `/nodes` | `vendor.taxonomy.read` | Published nodes; `?facet_id=` narrows |
| GET | `/nodes/{nodeId}` | `vendor.taxonomy.read` | One node plus its aliases/synonyms |
| GET | `/dietary-claims` | `vendor.taxonomy.read` | Claim catalogue with verification policies |
| GET | `/classifications` | `vendor.taxonomy.read` | Mappings + tags + claims for one owned resource |
| GET | `/mappings` | `vendor.taxonomy.read` | Mappings for one owned resource |
| GET | `/tag-assignments` | `vendor.taxonomy.read` | Semantic tags for one owned resource |
| GET | `/dietary-claim-assignments` | `vendor.taxonomy.read` | Claims + verification state |
| PUT | `/mappings` | `vendor.taxonomy.mapping.manage` | Replace facet mappings |

### Addressing a resource

The five source-addressed reads take two query parameters:

- `source_type` â€” one of `FOOD_ITEM`, `SHOP_PRODUCT`, `SHOP_VARIANT`
- `source_ref` â€” the identifier of the caller's own catalogue resource

`source_type` decides the namespace. `FOOD_ITEM` is only valid under `FOOD`;
`SHOP_PRODUCT` and `SHOP_VARIANT` only under `SHOP`. Getting this wrong returns
404, not a helpful correction.

### The scope rule the client must understand

**Do not send `business_ref` or `branch_ref`. Ever.** They are not optional
fields the client may omit â€” they are not accepted at all, and a body containing
either is rejected with **400**, not silently ignored.

The server resolves both from the authenticated session and from the owning
catalogue row. This is why a request for another vendor's `source_ref` returns
**404 rather than 403**: a distinguishable "forbidden" would let a caller
enumerate another vendor's catalogue by watching status codes. Do not present
404 here as "something went wrong" â€” for a resource the vendor genuinely owns it
means the reference is wrong or the staff member's branch scope does not cover
it.

### `PUT /mappings` request shape

```json
{
  "facet_code": "CUISINE",
  "source_type": "FOOD_ITEM",
  "source_ref": "fi_...",
  "mappings": [
    { "node_code": "YORUBA", "mapping_type": "PRIMARY", "confidence_basis": "MANUAL" }
  ]
}
```

`Idempotency-Key` is **required**. Replaying the same key returns the original
result rather than applying the command twice â€” safe to retry on a dropped
connection.

The command **replaces** every mapping the resource holds inside that facet.
Sending `"mappings": []` removes them all. Present this to the user as "these
are the categories for this item", not as incremental add/remove.

### Response envelope

Standard platform envelope on every route:

```json
{ "success": true, "data": { } }
```

```json
{ "success": false, "error": { "code": "...", "message": "...", "request_id": "...", "fields": [] } }
```

### Error handling

| Status | Meaning | What the client should do |
|---|---|---|
| 400 | Invalid body, unknown field, bad `source_type` | Fix the request; do not retry unchanged |
| 401 | No/expired session | Re-authenticate |
| 403 | Permission absent | Show read-only state with a reason |
| 404 | No such record **in this namespace and scope** | Wrong reference, wrong namespace, or outside branch scope |
| 409 | Revision conflict or invalid state transition | Reload and re-present |
| 503 | Feature flag off / rollout paused | "Temporarily unavailable", offer retry. **Not** a permission problem |

Note 503 carefully: writes are gated by a rollout flag independently of
permissions, and a disabled rollout must never be shown to a vendor as "you do
not have permission".

### Pagination, facets and suggestions

Not part of the vendor surface. They belong to the internal search API, which
frontend code does not call. When a customer discovery surface is built, its
contract will come from whichever backend service fronts search â€” bounded page
size (max 50), opaque cursor, bounded facet families (max 8), bounded
suggestions (max 10).

### What the vendor surface cannot do

No route here creates or publishes a taxonomy version, edits a facet or node,
manages vocabulary, opens a governance case, changes ranking, or rebuilds a
projection. Those are platform capabilities on the internal surface. Do not
design a Vendor app screen that assumes any of them will become reachable with a
vendor token â€” no vendor role grants them, and none is planned to.

### Dietary claims: display honestly

A claim carries `status`, `verification_policy`, `risk_level`, optional
`evidence_ref`, `verified_at` and `expires_at`.

Show the verification policy **before** the vendor declares a claim, so the
evidence requirement is known in advance rather than discovered on rejection.
Never render a `DECLARED` claim as verified, and warn before an expiring claim
lapses â€” a lapsed dietary claim on food is a safety matter, not a data-quality
one.


## Section 14 â€” Generic Inventory frontend contract

Machine-readable contract:
`docs/openapi/section14-vendor-inventory.openapi.json`. 34 routes: 27 public
vendor routes and 7 internal routes.

### The 7 internal routes are not callable from frontend code

`/internal/vendor/v1/inventory/...` requires an HMAC v2 service signature, a
caller allowlist entry and a single-use nonce. A browser or mobile client
cannot hold those credentials and must never try. They exist for trusted
services such as checkout.

### Base and envelopes

Public routes hang off `/api/v1/vendor` and use bearer authentication. Success
and error envelopes are the platform standard already documented above:

```json
{ "success": true, "data": {} }
```

```json
{ "success": false, "error": { "code": "...", "message": "...", "request_id": "...", "fields": [] } }
```

### Scope: the rule the client must understand

Every branch-bound route takes `branchId` in the path. The server derives the
Vendor tenant from the session and never from the body. A staff member scoped
to one branch who requests another receives a refusal that is deliberately
indistinguishable from "does not exist" â€” the client must not interpret it as
an empty result set and must not retry against other branch IDs.

### Idempotency

Every mutating route accepts the shared `Idempotency-Key` header. Reservations
additionally carry their own durable idempotency on
`(source_type, source_ref)`: replaying an identical request returns the current
reservation state rather than creating a second hold, while reusing the same
`source_ref` with different lines returns `409`. A client retrying after a
network timeout should reuse the same `source_ref`, not generate a new one.

### Quantities are integers

All quantities are int64. `available_quantity` is server-derived, is never
negative, and already excludes quarantined and expired stock. Do not compute
availability client-side from on-hand minus reserved; with lot-tracked items
that arithmetic is wrong.

### Conflict semantics

`409` is a normal, expected outcome on this surface, not an exception:

- committing or releasing a reservation that has already settled,
- posting a stocktake whose expected-quantity snapshot has gone stale,
- receiving a transfer that was already received,
- quarantining more stock than physically exists,
- creating a transfer whose source and destination items do not share a stock
  identity.

Each needs a specific recovery path in the UI. A generic "something went wrong"
toast will strand the operator, particularly on the stale-stocktake case, which
is common in a busy branch.

### Reading freshness

`GET /inventory/branches/{branchId}/revision` returns a monotonic revision.
Cache against it and re-read when it moves rather than polling balances.

### What this contract does not offer

No supplier, purchase-order, recipe, production, order, checkout, payment or
delivery data is reachable through these routes. Reorder rules hold an opaque
`supplier_ref` string and nothing more.


## Section 15 - Purchasing frontend contract

Contract: `docs/openapi/section15-vendor-purchasing.openapi.json`. 56 routes,
all authenticated under `/api/v1/vendor/purchasing`. Section 15 has no internal
HMAC surface, so there is nothing here a client must avoid calling.

### Scope
Every route derives the tenant from the session. Branch filters are
intersected with the caller's scope; a branch the caller does not hold returns
a refusal that is deliberately indistinguishable from "no such branch". Do not
retry across branch ids.

### Idempotency and concurrency
Mutations accept `Idempotency-Key`; the same key with the same payload replays,
with a different payload conflicts. Mutations on a versioned aggregate carry
`expected_version` (or `If-Match`); a stale value returns 409 rather than
overwriting a concurrent change. Reuse the same key when retrying a timeout.

### Quantities and money
All quantities and amounts are int64; money is in minor units. Purchase-unit
conversion is exact - never compute it client-side with floats.

### Conflict semantics
409 is a normal outcome here, not an exception:
- confirming a goods receipt that was already posted,
- reversing a receipt whose stock has been reserved or consumed,
- over-delivery beyond the branch's tolerance,
- approving with a stale version after the order changed,
- submitting a cost variance decision twice.

Each needs its own recovery path.

### Supplier performance: handle nulls honestly
Every rate is `{value, metric_status, sample_count}`. When `metric_status` is
`INSUFFICIENT_HISTORY`, `value` is null. Coalescing it to zero, or rendering it
as a full bar, will misreport a supplier's record. Show "not enough history".

### What this contract does not offer
No wallet, payment, remittance or settlement data. Receiving stock moves no
money.

<!-- SECTION 16B RECIPES -->
## Section 16B frontend contract

Recipe quantities and yields use exact numerator/denominator values.

Frontend clients must not convert recipe quantities through floating-point arithmetic.

ACTIVE and RETIRED versions are immutable historical definitions.

Validation issues must be surfaced before activation.

No frontend implementation is included.

<!-- SECTION16C_IMPLEMENTATION -->
## Section 16C frontend integration requirements

Backend routes are available for a future production board.

Frontend commands must preserve:

- `Idempotency-Key`;
- latest `If-Match` version;
- authenticated Vendor context;
- branch scope;
- exact numerator/denominator quantities.

A `PRODUCTION_INVENTORY_TOP_UP_REQUIRED` response must lead to an explicit
additional-reservation workflow. The frontend must never silently convert it
into an Inventory adjustment or negative stock.

## Section 16D - operational cost backend support

The backend supports these cost surfaces. No frontend code is implemented here.

- Ingredient and packaging cost policy management, including a business default
  and an optional per-branch override, with version/`If-Match` concurrency.
- Manual configured operational cost, where the active policy selects it.
- Recipe estimated COGS for the ACTIVE version or any explicit version, with a
  per-component breakdown, packaging contribution, expected-waste contribution,
  cost per serving and cost per output unit.
- Immutable recipe cost snapshots plus recipe and recipe-version cost history,
  so a UI can show v1/v2/v3 COGS without old figures moving.
- Production expected cost, actual cost, and expected-versus-actual variance
  per batch and per component.
- Highest-cost ingredients and packaging over recent snapshots.

A cost view must be treated as separately permissioned: a user may legitimately
be able to open a recipe and run production while having no access to any cost
field on the same screen. Cost values are integer minor units with an explicit
currency; a component may be present with no cost and an explicit reason, and
that must render as unknown rather than as zero.

## Section 17B - Commerce checkout frontend contract

Base `/api/v1/commerce`, Customer Service bearer token. Contract:
`docs/openapi/section17b-commerce-checkout.openapi.json`.

**Checkout is a validation step, not a purchase.** Nothing on this surface
quotes logistics, charges a card, holds money or creates an order. What it
does do is hold real physical stock for Shop lines, on a clock, so a client
that opens a checkout and walks away is costing a vendor sellable inventory
until the TTL elapses.

### The three calls

| Call | Headers | Notes |
|---|---|---|
| `POST /checkout-sessions` | `If-Match: <cart version>`, `Idempotency-Key` | Body is `{"cart_id": "..."}` and nothing else; unknown fields are rejected. |
| `GET /checkout-sessions/{sessionId}` | - | Settles and reports EXPIRED if the TTL has passed. |
| `POST /checkout-sessions/{sessionId}/cancel` | `Idempotency-Key` | Releases every hold and returns the cart to ACTIVE. |

`If-Match` carries the cart version the user is actually looking at. A
mismatch is 409 and means the cart moved: reload it, show the user what
changed, and let them re-confirm. Do not retry with the new version
automatically.

### Read `status`, not the HTTP code

`POST` answers **201 for both outcomes**, because a refusal is a resource with
per-line reasons rather than an error envelope:

- `VALIDATED` - proceed. Every required group holds stock until `expires_at`.
- `FAILED` - do not proceed. `failure_code` says why at the session level and
  every line carries `validation_status` plus a `reason_code`; render those,
  because they are the only thing that tells the user which line to fix.
- `EXPIRED` / `CANCELLED` - the hold is gone; start again from the cart.

Errors (4xx) mean the cart itself is not checkout-ready: not ACTIVE, empty, no
delivery address, or a stale `If-Match`.

### Show the clock

`expires_at` and `reservation_ttl_seconds` are real. Display the remaining
time, and treat expiry as a state the user will hit rather than an edge case.
Poll `GET` (or re-read on resume) rather than assuming a session is still
valid; a session past its TTL is never reported as valid.

### The cart is frozen while a session is live

Starting checkout moves the cart to `CHECKOUT_IN_PROGRESS`, and every Section
17A cart command then returns 409. Offer "change my order" as **cancel the
checkout, then edit**, not as a cart write that will fail. Cancelling is
cheap, always available, and returns the stock immediately.

### Price and catalogue changes

`price_changed` on the session, and `price_changed` / `catalogue_changed` per
line, mean the authoritative answer differs from what the user last saw. The
line stays sellable and keeps its hold; its `validation_status` is `CHANGED`.
Both figures are present - `snapshot_unit_price_minor` is what they saw and
`unit_price_minor` is what it costs now - so show the difference and get an
explicit acknowledgement. Section 17B deliberately does not resolve this for
you; the final immutable quote belongs to Section 17D.

### Money fields

`quoted_items_total_minor` is the items total if fulfilled exactly as ordered.
`max_authorized_items_total_minor` is the most the items could legitimately
cost once a variable-weight line is weighed, given the sell unit's own
overfill tolerance; for a cart with no variable-weight line the two are equal.

**Neither is a payable amount.** Both are items-only: no delivery fee, no
service fee, no tax. Presenting either as "total to pay" would be wrong, and
would be wrong again when Section 17C adds logistics pricing. Label them for
what they are, or hold them back until there is a quote.

### Variable-weight lines

A `VARIABLE_MEASURE` line carries `expected_measure_quantity`,
`requested_measure_ref` and the two tolerance figures in basis points. The
honest presentation is "about 1kg, you will be charged for the actual weight,
up to N% more" - the tolerance is exactly what `measure_overfill_bps`
expresses, and `max_authorized_total_minor` is that ceiling in money.

### Groups

A Shop checkout may span several vendors and branches; each group is one
`business_id` / `branch_id` pair with its own lines and its own totals, and
each holds its stock independently. A Food checkout always has exactly one
group. Group by vendor in the UI - the user is buying from several shops at
once, and the later fulfilment will reflect that.

`reservation_status` is operational detail, not something a customer needs to
see. `NOT_REQUIRED` on a Food group is normal and means nothing is wrong.


## Section 17D - Immutable checkout quote frontend contract

Base path `/api/v1/commerce`, Customer Service bearer required:

| Method/path | Body | Purpose |
|---|---|---|
| `POST /checkout/{sessionId}/quotes` | `{"fulfilment_plan_id":"..."}` | Create the immutable commercial quote |
| `GET /quotes/{quoteId}` | none | Read historical quote evidence without recomputation |
| `POST /quotes/{quoteId}/approve` | `{"quote_fingerprint":"..."}` | Approve the exact displayed agreement |

Both POSTs require a fresh `Idempotency-Key`. Request bodies are strict: never
send customer/vendor/branch ids, prices, fees, discounts, subsidy allocation
or totals. Render `price_disclosure_snapshot`, `funding_breakdown`,
`vendor_branch_groups`, `lines` and `logistics_quotes`; the backend explicitly
labels prices `VENDOR_ONLINE_PRICE` and never promises physical-store parity.

For exact-price quotes, `quoted_total_minor == max_authorized_total_minor`.
Variable-measure quotes may have a larger maximum, but it is the bounded cap
the customer is approving; show both figures before approval. Approve exactly
the `quote_fingerprint` returned by create/read. On `409` for expiry,
fingerprint mismatch or changed checkout/fulfilment evidence, create a new
quote and ask the customer to approve it again. Never silently substitute a
new total. `APPROVED` is approval evidence; `EXPIRED` is unusable even though
the historical row remains readable.

There is no payment or order call in this section. Do not treat quote approval
as payment success, authorization, capture, ParentOrder creation or mission
dispatch.


<!-- SECTION17A_CUSTOMER_CART -->
## Section 17C - Fulfilment planning and smart hybrid delivery frontend contract

Two customer calls and two vendor-staff calls.

### Customer

`POST /api/v1/commerce/checkout/{sessionId}/fulfilment-plan` builds the plan;
`GET` on the same path reads it back. **Neither takes a body, and the POST
deliberately has no request schema at all.** There is no fee, currency,
business id, branch id or customer id to send: the fee comes from the logistics
authority, the tenant comes from the checkout, and the customer comes from the
bearer token. A client that sends one is not rejected - it is simply ignored -
so do not build a UI that expects to influence any of them.

A `200` may carry `status: "UNSERVICEABLE"`. That is not an error state to
retry blindly: it means at least one vendor group cannot be delivered, and
`groups[]` names which and why. Render the refused groups individually - the
customer needs to know that the pharmacy can be delivered and the frozen-food
vendor cannot, not that "checkout failed".

Reason codes a client should have copy for:

| Code | Meaning for the customer |
|---|---|
| `NO_PROVIDER_AVAILABLE` | Nobody is available for this route right now. Retrying later may work. |
| `OUT_OF_VENDOR_SERVICE_AREA` | This vendor does not deliver to that address. |
| `BELOW_VENDOR_MINIMUM_ORDER` | Add more from this vendor, or the order cannot be delivered by them. |
| `PICKUP_BRANCH_NOT_ACCEPTING_ORDERS` | This vendor has paused orders. |
| `EXCEEDS_ALL_VEHICLE_CAPACITY` / `LOAD_EXCEEDS_CAPACITY` | The order is too large for any available vehicle. Splitting it may help. |
| `NO_VEHICLE_WITH_REQUIRED_HANDLING` | Nothing available can carry these goods safely (cold chain is the usual cause). |
| `DELIVERY_ADDRESS_REQUIRED` | No delivery address is set on the cart. |
| `LOGISTICS_AUTHORITY_UNAVAILABLE` | A temporary fault, not a refusal. Retrying is appropriate. |

**Show the funding split, not just the total.** Each mission and each group
carries `funding` with `gross_minor`, `customer_minor`, `vendor_funded_minor`
and `platform_funded_minor`. A free delivery is `customer_minor: 0` with a
non-zero `gross_minor` and `vendor_funded_minor` - display it as "Free delivery
(paid by the vendor)" rather than hiding the cost, because the cost is real and
the customer's receipt will reconcile against it later.

`expires_at` is never later than the earliest authority quote in the plan.
Treat an expired plan as stale and re-plan; do not present its fee.

`missions[].group_ids` is what makes a consolidated delivery explainable: one
fee covering two vendors is legitimate, and the UI should say which two.

### Vendor staff

`GET`/`PUT /api/v1/vendor/delivery/branches/{branchId}/policy`, branch-scoped,
`If-Match` required on the PUT (`0` to create).

Two fields need careful UI copy:

- `max_delivery_radius_meters` can only NARROW the branch service area. If a
  vendor sets it wider than their configured service area, nothing changes.
  Say so in the form rather than letting them believe they extended their
  reach.
- `free_delivery_threshold_minor` is a **vendor-funded** promotion. The vendor
  absorbs the delivery cost above that threshold; it is not waived. Label it
  accordingly, because the vendor's payable will reflect it.

The form has no beneficiary field and must not add one: who receives the
delivery fee is derived from who performs the delivery.

For HYBRID, display the selected owner and bounded trigger (`DISTANCE_OVERFLOW`,
`TIME_OVERFLOW`, `CAPACITY_OVERFLOW`, `VEHICLE_OVERFLOW`, `SLA_OVERFLOW`, or
`OWN_RIDER_FIRST`). Show the accepted customer delivery-price guarantee beside
the actual logistics evidence; a fallback must never silently raise the
customer contribution. Before custody, fallback is a replan of the same
VendorOrder/parcel. After custody, show transfer/return-required state until
authoritative handover evidence exists.

## Section 17A - Customer Cart Frontend Contract

Base path: `/api/v1/commerce`.

The contract exposes 12 customer-cart operations covering active-cart
creation/read, Shop grouping, add/update/remove/clear item operations,
delivery-address set/remove, receiver set/remove and cart abandonment.

Authentication is Customer Service bearer authentication. Frontend requests must
not send customer authority. Commands use Idempotency-Key. Mutations against an
existing cart use If-Match optimistic concurrency.

Food/Shop catalogue authority, effective price evidence and catalogue revisions
are resolved server-side.

## Section 17F - Customer commercial changes

Use `POST /orders/{orderId}/commercial-changes` with only the new authoritative
checkout session and fulfilment plan references. Read the returned replacement
quote from the change resource and send its exact `quote_fingerprint` to the
approve route. Do not send customer identity, totals, deltas, refund amounts,
vendor allocations or Wallet references. Treat a Wallet timeout as pending and
poll the change status; do not show the new address/service state as applied
until the server reports `APPLIED`.

## Section 18 - Order operations

Vendor staff clients use `If-Match` for every order action and must render the
food queue, Shop picking outcomes, packing/readiness and delivery authority.
Substitution proposals show the frozen replacement snapshot, price delta and
expiry; customer responses use the authenticated customer session and are
safe to retry. Never show a platform-delivered order as vendor-confirmed
pickup/delivery, and treat Wallet refund states as pending until confirmed.

Shop picking additions: use `GET /vendor/commerce/orders/{vendorOrderId}/picking-queue`
for the immutable ordered line snapshot. Render `FULL_AVAILABLE`,
`PARTIAL_AVAILABLE`, or `OUT_OF_STOCK`, every structured quality preference and
its `MET`/`PARTIAL`/`NOT_AVAILABLE`/`SUBSTITUTE` result, the remaining quantity,
and the frozen substitution policy. Variable-weight/count lines must show the
estimate, hard maximum, actual quantity, exact final amount and financial state;
packing is disabled while a case, quality result, inventory finalization,
reapproval, or Wallet operation is unresolved.
## Section 21 commerce

Group ordering must display the organizer-paid model and never collect split
participant payment. Participant links are scoped/expiring and selections are
frozen when the organizer closes the group. Scheduled orders display desired
delivery separately from production ready-by and logistics activation. RFQ
screens display immutable quote revisions and the accepted CheckoutQuote
reference. Advanced RFQ screens expose counter-offer revisions, payment policy,
deposit status, cancellation/refund policy and authoritative conversion status.
Corporate recurring feeding screens expose pause/resume/cancel controls and
future occurrence/proposal evidence without implying early driver assignment.

## Promotions and delivery funding

The Vendor management routes are listed in `API_ROUTE_REGISTER.md`; campaign
create/update/status/coupon commands require the authenticated staff scope and
current `If-Match` where documented. The client sends rules, never a computed
discount, funding amount, customer identity or Platform approval. Coupon codes
are accepted only by the existing CheckoutQuote create body as `coupon_code`;
the final quote is authoritative and may reject an exhausted, paused, stale or
foreign campaign.

Render `promotion_snapshot` and `delivery_funding_snapshot` from the quote/
order response as immutable receipt evidence. Show `gross_minor`,
`customer_minor`, `vendor_funded_minor` and `platform_funded_minor` separately.
An automatic HYBRID fallback does not authorize a higher customer charge; a
customer-requested higher service level continues through the existing
commercial-change/reapproval flow. Wallet funding operations are internal and
must never appear as a frontend mutation.

## Vendor Finance

Use the six `/api/v1/vendor/finance` routes documented in
`openapi/vendor-finance.openapi.json`. Amounts are integer minor units; format
them for display but never calculate available, pending or withdrawable funds
client-side. Wallet's response is authoritative. Show pending earnings,
available payable, transaction/adjustment history, masked bank accounts, and
withdrawal status/read-back without exposing ledger account IDs.

Adding or changing a bank account continues to use the existing onboarding
bank flow with purpose `finance.bank_change`. Before a withdrawal, run the
existing step-up flow with purpose `finance.withdrawal`, then send its one-time
`grant_id` as `step_up_grant_id` plus a stable `Idempotency-Key`. After an
ambiguous response, read the withdrawal list/status; never infer failure or
submit a differently keyed duplicate.

## Commerce settlement and refund projection

No frontend was implemented. Future customer/Vendor clients must render the
server projection, not infer money movement from an order event:

- `REFUND_PENDING` means Wallet movement is unresolved or a released-party liability is pending.
- `UNKNOWN` means the authority call was ambiguous and recovery/read-back is in progress.
- `PARTIALLY_REFUNDED` and `REFUNDED` are shown only after Wallet read-back.
- Item-unavailable, cheaper replacement, lower measured amount and cheaper replan copy must describe the exact affected difference, never the gross order.
- A customer-caused/replannable delivery failure shows no automatic refund. An ambiguous or prepared-Food case links Support review and must not be labelled refunded.

Clients send no refund amount, beneficiary, funding source, provider identity,
or allocation id. Customer-initiated address/service changes continue through
the replacement quote and explicit approval flow before any additional hold.

## Fulfilment group, parcel and mission model

Backend only. No frontend was implemented.

### Readiness

| Surface | Readiness | Note |
|---|---|---|
| Delivery plan + plan history | READY | `GET /fulfilment/groups/{groupId}`, `.../plans` |
| Packages (parcels) + manifest | READY | list, read, pack, amend before handover |
| Package custody timeline | READY | `GET /fulfilment/parcels/{parcelId}/custody` |
| Pickup readiness | READY | `PUT /fulfilment/units/{unitId}/readiness` |
| Own delivery missions + rider assignment + execution | READY | `/fulfilment/vendor-missions/...` |
| Platform delivery status | READY (read-only projection) | inside the group response |
| Return initiation + return status | READY | `POST /fulfilment/parcels/{parcelId}/returns` |
| Handover OTP/QR action on a vendor mission | PARTIAL | the mission carries a `handover_ref`; the credential itself is issued and verified by its owning authority, not by this service |
| Provider transfer display | PARTIAL | modelled and safe to render, but no logistics authority emits a transfer event yet |
| **Taxi commerce delivery** | **DEFERRED** | `driver-taxi-service` has no commerce mission ingress. Do not build a Taxi option; selecting it is refused server-side |

### Vocabulary for the interface

Use business language, not table names.

| Show | Means |
|---|---|
| Delivery plan | the plan version currently in force |
| Plan changed | a new plan version was appended; the previous one is still readable |
| Packages | parcels, each with its own reference the customer can be told |
| Pickup status | the unit's readiness |
| Own delivery | a VendorMission — the vendor's own rider |
| KarryGo delivery | a platform mission projection |
| Delivery status | the projected status, with the time it was last reported |
| Return | the same package coming back — never a new package |

### Rules the interface must respect

- **Every mutation needs `If-Match`** with the version last read, except packing
  and return initiation, which need `Idempotency-Key`. Retry with the same key
  rather than issuing a second request.
- **A package reference never changes.** If the interface shows a new reference
  after a replan or a return, something is wrong on the client side.
- **Platform delivery is read-only.** There is no control anywhere that sets a
  KarryGo mission's status; that belongs to the executing service.
- **A manifest is not editable after handover.** The API answers
  `FULFILMENT_PARCEL_SEALED`; present it as "already collected", not as an error
  the user can retry.
- **An order may have several deliveries.** Never present a multi-mission group
  as a single delivery — say how many and show each one's progress.
- **In a shared order, a vendor sees only its own units and packages.** The API
  filters this; the interface must not attempt to fetch more.

---

## Delivery Requirement and Vehicle Selection

No frontend was implemented. Contract notes for whoever builds it.

### Where the data comes from

No new endpoint. `requirement_decisions` is returned on each plan version by
the plan reads that already exist:

- `GET /api/v1/vendor/fulfilment/groups/{groupId}` (current plan)
- `GET /api/v1/vendor/fulfilment/groups/{groupId}/plans` (full history)

### Shape

Each decision carries:

| Field | Use |
|---|---|
| `minimum_physical_class` | what the load actually needs; may be `CAR` |
| `selected_vehicle_class` / `selected_service` | what will run it; never `CAR` |
| `executable` | false means nothing can currently run it |
| `primary_reason` | one controlled code — the headline |
| `supporting_reasons` | ordered controlled codes — the detail |
| `candidates` | every class considered, each with its own outcome |
| `measurement_state` | `ESTIMATED` or `CONFIRMED` |
| `measurement_source` | catalogue, vendor, packing scale or derived |
| `policy_version` | the rules in force when it was decided |

### Rules for the client

1. **Never re-derive a vehicle class in the client.** The server owns safety
   thresholds. A client that computes its own class will eventually disagree
   with the delivery that actually ran, and it will be the customer who finds
   out.

2. **Map reason codes to copy; never render the code.** The vocabulary is
   closed and stable, so a lookup table is safe. An unmapped code should fall
   back to generic copy rather than leaking `PACKAGE_DIMENSION_EXCEEDED` into
   a customer's screen.

3. **`minimum_physical_class` and `selected_vehicle_class` differing is normal
   and worth showing to operators.** It usually means the right vehicle exists
   but cannot be run — today, the disabled taxi integration. Do not treat it
   as an error.

4. **Treat `measurement_state` as load-bearing.** An estimate presented as a
   measurement is how a vendor is blamed for a figure they never gave.

5. **A plan version change is not a price change.** Section 24 governs price;
   an escalation carries no charge, and the UI must not imply one.

6. **Do not expose thresholds or `policy_version` to customers or vendors.**
   They are operator and support vocabulary.


## Multi-vendor consolidation and basket optimization

### Planning with a preference

`POST /api/v1/commerce/checkout/{sessionId}/fulfilment-plan` takes an optional
`{"fulfilment_preference": "FASTEST"|"BALANCED"|"SAVER"}`.

1. **Send nothing to get the default.** An absent body means BALANCED. An
   unrecognised value is a 422, not a silent default.
2. **Re-plan when the customer changes preference.** The mode is part of the
   plan and part of the quote fingerprint, so a plan built for SAVER cannot be
   approved as FASTEST.
3. **Read `candidates` for the explanation.** `status` is `SELECTED`,
   `REJECTED` or `INELIGIBLE`, and the three are genuinely different:
   `INELIGIBLE` means a constraint refused it and no price would have changed
   the outcome.
4. **`MULTI_STOP_UNSUPPORTED` is expected, not an error.** It is today's
   runtime limitation and it will appear regularly. Surface it as an
   explanation of the split, never as a failure.
5. **Do not show `policy_version`, thresholds or fingerprints to customers.**
   They are operator and support vocabulary.

### Basket optimization

1. **Everything except acceptance is free of consequence.** Calling
   `/basket/optimize` or `/basket/cart/optimize` repeatedly costs nothing and
   changes nothing. Do not build confirmation dialogs around them.
2. **A proposal has an expiry.** Show it, and re-run rather than letting a
   stale Accept fail. `409` on acceptance means it expired or the cart moved -
   both are recoverable by re-optimizing, and neither should read as an error
   the customer caused.
3. **Acceptance is idempotent.** `replayed: true` means this had already
   happened and the same `cart_version` is being returned. Treat it as success.
4. **`requote_required` is always true.** After acceptance, run the normal
   checkout quote. The proposal's totals were an estimate; the quote is the
   money.
5. **Budget is `final_delivered_total_minor`.** Never evaluate
   `merchandise_total_minor` against the customer's budget - the API does not,
   and a UI that did would tell them something the backend disagrees with.
6. **Handle `no_complete_basket` separately from `fits_budget: false`.** One
   means "nothing available", the other means "too expensive". Rendering both
   as one state loses the difference that tells the customer what to do.
7. **`quality_variant_unavailable_reason` is normal.** The catalogue carries no
   ordinal quality signal for most goods, so the QUALITY_BALANCED variant is
   usually absent. Do not treat it as a gap to fill client-side.
8. **`search_truncated` should be surfaced to operators, not customers.** It
   means the bounded search stopped early. Customers do not need it; the team
   tuning the bounds does.
9. **`quoted_available_quantity` is not a reservation.** Nothing is held.
   Checkout revalidates, and a UI promising availability from this field will
   be wrong sometimes.

### Recurring baskets

1. **`expected_version` is required on every edit**, as a query parameter or an
   `If-Match` header. A `409` means somebody else changed it - re-read and
   re-present rather than retrying.
2. **`timezone` is required and must be an IANA name.** Send the customer's
   own zone, not the device's UTC offset.
3. **An auto-approving policy requires `ceiling_minor`.** The API refuses the
   combination rather than silently downgrading, so the form must collect both
   together.
4. **`day_of_month` is capped at 28.** Offer 1-28 only, and explain it: a
   monthly basket on the 31st would skip February.
5. **Generation states are not order states.** `PROPOSED` and
   `AWAITING_APPROVAL` need the customer; `AUTO_APPROVED` means the occurrence
   was inside their own configured bound. **None of them means anything has
   been bought or charged.**
6. **Show `comparison` on every occurrence**, including when
   `same_vendor_plan_available` is false - "your usual shops cannot supply this
   today" is information, and an empty comparison silently omits it.
7. **`material_changes` drives the approval prompt.** A vendor change and a
   product change are different sentences to a customer, and the field
   distinguishes them.

## Dispatch multi-pickup delivery

The customer UI continues to consume the Vendor fulfilment/order projection;
it must not call Dispatch's internal Commerce API directly. A selected
multi-pickup mission needs an ordered pickup timeline, one Vendor/branch and
manifest summary per logical stop, parcel count, delivery mode, promised-by
time and bounded runtime reason. Co-located Vendors may share one map pin but
must remain separate pickup rows.

Show `NO_PROVIDER_QUEUED` as matching in progress, not as delivery failure.
Show `SLA_EXHAUSTED`, receiver unavailable, rescheduled, return required,
transfer required and returned as distinct states. Never infer a refund or
settlement from them; those appear only after the existing Vendor/Wallet policy
projection decides them.

Pickup handover uses the provider/Vendor stop credential; delivery handover
reuses the established Dispatch customer verification-code flow. Do not show
an OTP before Dispatch has issued it, do not merge codes across stops, and do
not treat a replayed response as a second pickup.

## Vendor Rider Mode

The Vendor-owned Rider contract is documented in
`docs/handoffs/VENDOR_RIDER_MODE.md`. It is a restricted Vendor-staff surface,
not a marketplace-provider surface. Secure VendorMission pickup, receiver
completion and return remain dependency-gated until a Vendor-compatible
handover authority is approved and wired; the UI must not treat Dispatch's own
mission credential as interchangeable.

## Dispatch evidence, exceptions, returns and transfer

Customer and Vendor frontends consume Vendor's existing order/fulfilment
projection and must not call Dispatch provider/internal routes. Provider-app
work is a separate handoff: exact stop manifest confirmation, private media
upload, controlled-camera UX, exception reason/recovery selection,
product-class return presentation and the two-provider transfer challenge.

Clients cannot set custodian, provider replacement, financial allocation,
refund, additional fee or completion. `409` policy/state failures require a
read-back, not a blind retry. Replayed idempotent responses are success. Media
references remain opaque and private; evidence bytes never travel in mission
JSON.

## Realtime notifications and confidence ETA

The customer order ETA response preserves the existing `allocations` array and
adds `missions`. Render each mission independently as `earliest_at–latest_at`;
never collapse a split delivery into one precise time. Show `confidence` as a
controlled LOW/MEDIUM/HIGH label, retain the accepted promise separately, and
use `delay_state` for risk/delay messaging. On reconnect or any source-version
gap, read back `GET /api/v1/commerce/orders/{orderId}/eta`; realtime payloads
are hints, not client-side authority.

Vendor order detail may expose its scoped current ETA, provider-arrival range
and delay state. The UI must not calculate ETA from coordinates, replace an
authoritative route, select notification recipients/templates, or infer
delivery from a push. Vendor Rider mission updates are realtime notification
intents only; the existing Section 31 secure-handover dependency remains
visible and fail-closed. No frontend is implemented by Section 36.
## Section 39 Analytics Dashboard Handoff

Future UI may consume Analytics Service, not Vendor Service, for operational
dashboard cards:

- sales, order count and AOV with explicit window and `data_as_of`
- inventory, stock-out, turnover, waste/expiry and purchase-cost trends
- supplier on-time, short-delivery and damage metrics where source data exists
- preparation/picking, acceptance, cancellation and refund-reason metrics
- own-rider versus platform fulfilment performance
- Cloud Kitchen/Pastry utilization and RFQ conversion
- CRM cohorts: NEW, REPEAT, LAPSED, HIGH_VALUE
- reliability score with component values, status, trend, window and
  calculation version
- demand/stock-out intelligence clearly labeled as estimates
- price benchmarks only when aggregate sample sufficiency is met

The UI must state that operational analytics are decision-support data, not
audited accounting statements. Reliability is not a star rating, not
verification status and not popularity.

Order Guarantee must not be shown as active. Its backend status is
`FUTURE_DISABLED`.

## Section 40 Security, Privacy And Compliance Handoff

Frontend work is documentation-only for Section 40. Render only fields returned
by the backend; do not infer hidden customer, receiver, provider, bank,
verification or support-evidence data from opaque references.

- Authorization failures should be shown as controlled unavailable/forbidden
  states without revealing whether another tenant's object exists.
- Rate-limit responses require cooldown/retry UX; do not expose account
  existence through different copy. Search, quote creation, support/report
  creation and expensive Analytics reads may return HTTP 429.
- Media refs are not public URLs. Request signed/time-bound access through the
  media-file-service flow. A ref can still be rejected for evidence attachment
  when it is not bound to the exact mission/Parcel; prompt for a fresh upload
  rather than retrying the foreign ref.
- Price benchmark cards must handle suppressed low-sample cohorts and must not
  try to reconstruct competitor identities or exact prices client-side.
- Restricted/prohibited goods return a controlled backend policy rejection.
  Pharmacy/medicine Vendor Commerce is backend-excluded. Do not add a UI
  override, hidden category, import bypass or “admin enable” path.
- OTP/handover screens must respect server retry, cooldown, expiry and replay
  states; do not locally generate or persist challenge secrets.
