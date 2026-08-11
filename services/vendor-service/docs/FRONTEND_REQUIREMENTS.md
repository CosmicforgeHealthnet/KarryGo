# Frontend Requirements — Vendor Service Backend Contract Handoff

Plain-language backend contract handoff for the future frontend team (Vendor Flutter app, and the commerce surfaces added to the Customer app). No frontend code.

**Status: Sections 5, 6, and 7 are implemented on the backend.** Later feature areas remain template-only — see [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) and [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md).

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
