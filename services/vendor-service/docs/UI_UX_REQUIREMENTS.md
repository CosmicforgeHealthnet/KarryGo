# UI/UX Requirements — Vendor Service

Plain-language UI/UX requirements generated from Vendor Service's backend/product implementation, for the future Vendor Flutter app team. No frontend code, no Flutter/Dart in this file — only what a screen needs to show, do, and handle.

**Status: Sections 5, 6, and 7 are implemented on the backend and have documentation-only UI handoffs below.** Later feature areas remain template-only (see [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md)). Do not treat placeholder rows as implemented UI or design guidance.

**Section 7 contract correction:** render `PENDING_REVIEW` exactly; never shorten it to `PENDING`. It is not a success state and must keep the applicable business, branch, capability, brand, or branch-capability activation control blocked until the backend reports readiness. The internal verification callback is not a UI route.

## Per-screen template (fill this in when a section implements the backing feature)

- **Screen name**
- **Purpose**
- **User role** (vendor owner / vendor staff / customer / admin)
- **Sections** on the screen
- **Information displayed**
- **Actions** available
- **Forms / fields**, with validation rules mirrored from the backend usecase
- **Filters**
- **Tables / lists**, with pagination behavior
- **Permissions** gating each action
- **States**: loading, empty, error, success
- **Confirmation dialogs** and their exact copy intent
- **Warnings**
- **Disabled states** and why
- **Realtime updates** (which events, via which channel)
- **Important UX rules** specific to this screen

## Screen areas (placeholder — owning backend feature in parentheses)

| Area | Owning feature | Status |
|---|---|---|
| Vendor onboarding (business creation, capability selection, verification submission) | `features/vendors`, `capabilities`, `integrations/verification` | **IMPLEMENTED BACKEND — Sections 6/7 below** |
| Vendor authentication (login, step-up) | `features/auth` | **IMPLEMENTED — see "Section 5" below** |
| Vendor home / dashboard | (cross-feature) | NOT_STARTED |
| Businesses (session-scoped business settings) | `features/vendors` | **IMPLEMENTED BACKEND — Section 6 below** |
| Brands | `features/vendors` | **IMPLEMENTED BACKEND — Section 6 below** |
| Branches (location, hours, service radius) | `features/vendors` | **IMPLEMENTED BACKEND — Section 6 below** |
| Staff (invite, roles, permissions) | `features/staff` | **IMPLEMENTED — see "Section 5" below** |
| CRM (customer history, notes) | `features/crm` | NOT_STARTED |
| Restaurant/Food menu management | `features/food`, `catalog` | NOT_STARTED |
| Cloud Kitchen configuration | `features/cloud_kitchen` | NOT_STARTED |
| Pastry/Bakery catalogue and pre-order scheduling | `features/pastry` | NOT_STARTED |
| Shop/Grocery/Market/Wholesale catalogue | `features/shop`, `catalog` | NOT_STARTED |
| Inventory (stock levels, low-stock alerts) | `features/inventory` | NOT_STARTED |
| Suppliers | `features/purchasing` | NOT_STARTED |
| Purchasing (purchase orders) | `features/purchasing` | NOT_STARTED |
| Goods receiving | `features/purchasing` | NOT_STARTED |
| Recipes / ingredients | `features/recipes` | NOT_STARTED |
| Kitchen / production board | `features/production` | NOT_STARTED |
| Orders (incoming vendor orders, accept/reject) | `features/orders` | NOT_STARTED |
| Substitutions (propose/respond) | `features/substitutions` | NOT_STARTED |
| Delivery (vendor-owned rider dispatch, or platform-delivery status) | `features/delivery`, `fulfilment` | NOT_STARTED |
| Own riders (vendor-owned delivery staff) | `features/delivery`, `staff` | NOT_STARTED |
| Promotions | `features/promotions` | NOT_STARTED |
| Finance (payable summary, settlement status) | `features/finance_projection`, `integrations/wallet` | NOT_STARTED |
| Reports | `features/analytics` | NOT_STARTED |
| Settings | (cross-feature) | NOT_STARTED |
| Support | `integrations/support` | NOT_STARTED |

## Section 5 — Authentication, Sessions and Staff IAM (implemented)

Backing routes: see [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md). Reminder: this is Vendor-staff login, completely separate from the (not-yet-existing) logistics pickup/delivery handover OTP/QR — never merge these two concepts in any screen or copy.

### AUTH

- **Login identifier screen** — Purpose: collect phone or email. User role: anyone with a Vendor staff invite. Actions: submit → `POST /auth/start`. States: loading (submitting), error (invalid phone/email format, from the validation response's `fields`). No permissions gating (public).
- **OTP request state** — shown immediately after Start succeeds. Information: masked destination (e.g. "code sent to ...@example.com"), countdown until expiry (`expires_in` seconds). Important rule: phone numbers currently have no real delivery channel (see [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md)) — if Start fails with `service_unavailable` for a phone identifier, show a clear "please use your email instead" message, not a generic error.
- **OTP entry screen** — 6-digit code input. Actions: submit → `POST /auth/verify` with `challenge_id` + `otp` (+`vendor_id` if a prior attempt returned a vendor-choice response). Success state: navigate into the app with the returned tokens stored securely. Error state: generic "invalid or expired code" (the backend deliberately never distinguishes wrong/expired/locked to prevent enumeration — do not try to guess which one happened).
- **Resend state/countdown** — a "Resend code" action, disabled until the current code's TTL is close to expiry (avoid spamming `POST /auth/resend`, which is itself rate-limited and will return `429` if hit too often). On resend, the previous code stops working immediately — if the user already has the old code memorized/pasted, it will now fail; don't imply both codes are valid.
- **Expiry state** — if the OTP TTL fully elapses without a successful verify, show "This code has expired — request a new one," with a direct path back to resend/restart, not a bare error.
- **Incorrect OTP state** — inline error on the OTP field, decrement an implicit "attempts remaining" is NOT exposed by the API (the backend never reveals attempt counts, to avoid giving an attacker a countdown) — just show the generic invalid-code message each time.
- **Attempt-limit/lock state** — after enough wrong attempts the backend returns the SAME generic invalid-code error (it does not announce "you are now locked out," by design) — the practical UI behavior is: after N consecutive failures (a frontend-side heuristic, e.g. 3-5), proactively suggest "Try requesting a new code instead."
- **Rate-limit state** — `429 rate_limited` on start/resend/verify: show "Too many attempts, please wait a moment," no retry button for a few seconds (client-side cooldown; the API doesn't return an exact wait time yet).
- **Vendor-choice state** (multi-business staff only) — if verify returns `requires_vendor_choice: true` with `available_vendor_ids`, show a business picker (business names aren't available from this endpoint yet — Section 6 will need to supply a name-lookup; render raw IDs or a placeholder label for now) and resubmit verify with `vendor_id` set, reusing the same OTP the user already entered.
- **Login success** — store `access_token`/`refresh_token` securely (never in plain logs or analytics). Navigate to the vendor dashboard.
- **Session expiry** — when an API call returns `401 unauthorized` with a session-expired message, silently attempt `POST /auth/refresh` once; if that also fails, force a full re-login (clear stored tokens).
- **Forced reauthentication** — after logout-all or an admin-driven suspension, the next API call will 401/403; treat this identically to normal session expiry (attempt refresh, then force login) — the suspended case additionally surfaces "Your access has been suspended, contact your business owner" from a `403 forbidden`.
- **Suspended-account behavior** — a suspended staff member cannot complete login at all (Verify itself refuses) nor refresh an existing session — show the same "access suspended, contact your business owner" message in both places.
- **Logout** — `POST /auth/logout` with the current refresh token, then clear local tokens regardless of the API response (logout should always feel instant client-side).
- **Step-up authentication prompt** — before a high-risk action (see [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md)'s "High-risk operations"), show an explicit "Confirm this action" screen requesting a fresh OTP (reuses the same 6-digit entry UI as login, different copy: "We've sent a confirmation code to your email to confirm this sensitive action").
- **Step-up success/failure/expiry** — success: proceed with the original high-risk action, attaching the returned `grant_id`. Failure/expiry: return to the triggering screen with "Confirmation failed or expired, please try again" and restart step-up from scratch (a grant cannot be resumed).

### STAFF

- **Staff list** — Purpose: see everyone with access to the vendor business. Permission: `vendor.staff.read` (hide/disable this whole screen area if absent). Data: name/contact aren't returned by this API yet (only `id`, `status`, `version`, `created_at`) — Section 6 or a future staff-profile addition will need to supply display names; render the identifier or a placeholder for now, and flag this gap to product rather than inventing fake names. Filters: by status (active/suspended) — client-side, the API returns the full list. States: loading/empty ("No staff yet — invite your first team member")/error.
- **Staff detail** — one staff member's status, version, suspension reason (if any), and their role assignments (`GET /staff/:id/roles`).
- **Staff status** — a clear active/suspended badge.
- **Staff roles** — list of active assignments, each showing role name and branch scope (or "All branches" when `branch_id` is null).
- **Staff branch scope** — shown per-assignment, not per-staff-member (a staff member can hold different roles at different branches).
- **Suspend confirmation** — destructive-adjacent action, needs a confirmation dialog ("This will immediately sign this person out of all their active sessions. Continue?"), requires `vendor.staff.suspend`, requires the current `version` (fetch fresh before showing the dialog to minimize staleness), optional reason field.
- **Reactivate action** — explicit button, not automatic; same permission; note in the UI that previously-active sessions are NOT restored ("They'll need to sign in again").
- **Permissions display** — no single "list my effective permissions" endpoint exists yet; derive what to show/hide from the specific permission each action needs (see `GET /permissions` for the catalogue + descriptions to use as UI copy/tooltips).
- **Restricted-action behavior** — any action gated by a permission the current staff member lacks should be hidden or disabled (not shown-then-403'd) wherever the frontend can determine this ahead of time; a `403 forbidden` from the API is the fallback truth, not the primary UX signal.

### ROLES

- **Role list** — system role templates (11 defaults: Owner, General Manager, Branch Manager, Kitchen/Pastry, Shop/Inventory, Procurement, CRM/Support, Marketing, Finance, Vendor Rider, Analyst/Auditor) plus this vendor's own custom roles, visually distinguished (`is_system: true/false`). Permission: `vendor.roles.read`.
- **Role detail** — name, description, full permission-code list (cross-reference against `GET /permissions` for human-readable descriptions/category grouping — group the permission picker by `category` in any create/edit UI).
- **Create/edit role** (custom roles only — system role templates are never editable, hide edit affordances entirely when `is_system: true`) — permission: `vendor.roles.manage`. Only permissions the ACTING staff member already holds themselves are valid to pick — the API will refuse otherwise (403), so ideally pre-filter the picker to the actor's own effective set once that's obtainable, or at minimum surface the API's refusal clearly ("You can only grant permissions you hold yourself").
- **Assign/remove permissions** — `PUT /roles/:id/permissions` replaces the WHOLE set (not additive) — the UI must submit the complete intended permission list every time, and must pass `expected_version` from the most recently loaded role.
- **Role assignment** (to a staff member) — `POST /roles/:id/assign` with `staff_id` + optional `branch_id`. If assigning a role that includes any high-risk permission, the step-up flow triggers first (see AUTH above).
- **High-risk permission warning** — when a role being created/edited/assigned includes a permission flagged `is_high_risk: true` in the catalogue (bank-account change, ownership transfer, sensitive data export), show an explicit warning before submission: "This role includes a high-risk permission. You'll be asked to confirm your identity again."
- **Step-up requirement** — surfaced automatically by a `403` mentioning step-up is required; the UI's job is to catch this specific case and route into the step-up flow rather than showing a generic error.
- **Version-conflict/error behavior** — `409 conflict` on suspend/reactivate/set-permissions means someone else changed the record first; reload the current state and show "This was just updated by someone else — please review and try again," never silently retry with the stale version.

### PERMISSIONS

- **Read-only display where user cannot manage** — if the acting staff member lacks `vendor.roles.manage`/`vendor.roles.assign`, render role/permission screens as view-only (no edit/assign controls at all).
- **Disabled actions where permission is absent** — prefer hiding over disabling-with-no-explanation; where disabling is used, a tooltip naming the missing permission (from the catalogue's human-readable description) is more useful than a bare disabled button.
- **Branch-restricted behavior** — a Branch-Manager-type staff member managing staff/roles only ever sees/affects their own branch's scope; there is no UI affordance to "pick any branch" for someone whose own grants don't cover it (the backend refuses it anyway, but showing the option at all is misleading).
- **Owner-only/elevated actions** — `vendor.owner.transfer` can never be granted to any role but Owner (the backend refuses this outright at the API level) — do not build a UI that lets anyone attempt to assign it to a custom role; if a future "transfer ownership" feature is designed, it is a distinct, not-yet-built capability, not a role-permission checkbox.

## Customer-facing commerce screens (owned by the customer app, backed by Vendor Service)

| Area | Owning feature | Status |
|---|---|---|
| Vendor discovery / storefront browsing | `features/catalog`, `food`, `shop` | NOT_STARTED |
| Cart | `features/orders` | NOT_STARTED |
| Checkout (multi-vendor split, delivery method selection) | `features/orders`, `integrations/wallet` | NOT_STARTED |
| Substitution approval prompt | `features/substitutions` | NOT_STARTED |
| Order tracking (multi-vendor, multi-fulfilment-unit) | `features/fulfilment` | NOT_STARTED |
| **Pickup/delivery handover confirmation UI** | `integrations/handover` | **PENDING_EXTERNAL_DEPENDENCY** — the underlying OTP/QR credential does not exist anywhere in the platform yet (see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md)). No screen requirement can be written honestly until the credential shape is real; today's dispatch/hauling proof-of-delivery UI (photo/signature capture) is the closest existing pattern to reuse for evidence capture, but it is not the same thing as a receiver-facing OTP/QR entry screen. |

## Section 7 — Onboarding and activation UX specification only

This is a future-client handoff only. No Flutter, mobile, web, dashboard, navigation, component, or state-management implementation was added.

### Onboarding home

- **Purpose/roles/permission:** give Owner, General Manager, permitted Branch Manager, Finance, and Analyst/Auditor staff a truthful status view; backend permission is `vendor.onboarding.read`.
- **Data:** percentage, checklist, satisfied/missing items, blockers, next action, `PENDING_REVIEW`, `ACTION_REQUIRED`, readiness, and activation state.
- **Actions:** submit or request evaluation only with `vendor.onboarding.manage`. “Request activation” never promises or forces activation.
- **States:** skeleton/loading; empty `NOT_STARTED` with a start action; dependency error with safe retry; submitted success; disabled write control when flags/permissions block; confirmation before submission; realtime/poll refresh after verification events.

### Owner / authorized representative

- Show capacity, name/contact fields, ownership percentage when supplied, authorization, version, and read-only Identity verification status.
- Validate required names, email, phone, capacity, and 0–100 ownership percentage. Use `expected_version`; on 409 reload before resubmitting.
- Replacing an Identity-verified representative requires the backend-issued `onboarding.representative_replace` step-up grant. Explain that replacement invalidates Identity verification and requires resubmission.
- Keep representative PII out of generic notifications, analytics labels, logs, and list summaries. Loading/empty/error/success/disabled/validation/confirmation states are all required.

### Business verification and documents

- Render separate cards for `IDENTITY`, `BUSINESS`, `BANK`, `LOCATION`, `BRAND`, and `FOOD_KITCHEN`, with `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, or `SUSPENDED` exactly.
- Upload bytes through Media File Service. After upload completion, attach only the opaque Media object id plus document type/scope. Never display an object key, permanent URL, service credential, or document bytes from Vendor API data.
- Document rows show DRAFT/SUBMITTED/SUPERSEDED, dimension, subject scope, and version. Draft update/delete uses `expected_version`; submitted evidence is immutable and remediation creates a new submission.
- Rejection shows safe reason/remediation and a resubmit action. Expired/suspended states explain operational impact. Dependency-unavailable screens retain input locally only as appropriate and retry the same backend command with the same idempotency key.

### Bank

- Finance-authorized users select a bank, enter an account number, see the resolved account name, then confirm the masked account. Never persist the full account number or step-up grant client-side beyond the active command.
- First submission sends `expected_version=0`; replacement sends the loaded version and requires a fresh `finance.bank_change` step-up grant.
- Provide resolution loading, mismatch/validation, authority unavailable, step-up-required, version-conflict, success, and disabled states. The displayed record contains only bank code, masked account, resolved name, status and safe external/version metadata.

### Location, Brand, and Food/Kitchen

- Location is per branch. Show the branch name, evidence state, remediation, expiry and suspension. Location expiry/suspension gates only that branch and its effective operations.
- Brand is per brand when policy enables a Brand requirement. It remains read-only to Vendor users except for submitting evidence/remediation; no “approve brand” action exists.
- Food/Kitchen is per enabled Restaurant, Fast Food, or Cloud Kitchen branch-capability. Show which branch/capability is blocked. `PASTRY_BAKERY` is not silently treated as food-regulated unless a data-driven requirement is configured.
- For every card: loading, absent/not-started, pending, rejected, suspended, expired, verified, dependency-error, retry, and status-refresh behavior is required.

### Activation and admin-review awaiting state

- The activation view groups blockers by business, branch, and branch-capability. Eligible means the backend reports `ready=true`; activated means the authoritative business graph is ACTIVE.
- Food/Kitchen expiry gates only the affected food branch-capability; Location suspension gates the affected branch; Identity, Business, or Bank failure gates the business. No destructive delete occurs and history remains visible.
- `PENDING_REVIEW` must say that formal review occurs outside the Vendor app. `admin-backoffice-service` has no Vendor review product yet, so do not imply an SLA or provide an approval control.
- Activation success should refresh business/capability/branch effective state. A blocked result keeps the user on the checklist and focuses the first actionable blocker.

### Universal interaction rules

All writes use one generated `Idempotency-Key` retained across byte-equivalent retries; changed input gets a new key. All versioned writes retain and send `expected_version`. A frontend must never set verification status, `source_version`, business/capability ACTIVE, authority references, or internal HMAC credentials. Permission denial, disabled flags, kill switch, validation, conflict, dependency unavailable, and unknown-outcome reconciliation must be visually distinct.

## Section 6 management experience

- Business creation starts in `DRAFT`; activation, suspension, closure, and verification are operator-owned. Show lifecycle state as read-only. Vendor staff may only pause an `ACTIVE` record or resume a `PAUSED` record.
- Capability selection is multi-select from exactly Restaurant, Fast Food, Cloud Kitchen, Pastry/Bakery, Shop/Grocery, Market, and Wholesale. Never infer a single business type.
- Brand media controls upload through Media Service and persist only returned opaque references; do not accept pasted public URLs. Brand verification is read-only.
- Branch forms require structured address, coordinates, and IANA timezone. Hours support multiple intervals per weekday; overnight and overlapping intervals must be corrected client-side before submit.
- Service area is explicitly a radius preview, not delivery pricing or dispatch coverage. Operations settings expose only schema-v1 `accepting_new_orders` and `pickup_enabled` toggles.
- Branch capability rows show both configured state and effective state/reason because a paused parent gates operation without rewriting child state.
- Keep each returned `version`; send it as `expected_version`. Generate and retain an `Idempotency-Key` across retries of the same write.

## Known cross-cutting UX rules already fixed by architecture (apply to every screen above once built)

- Every list must have explicit loading/empty/error states — no bare blank screen.
- Every destructive or hard-to-reverse action (reject an order, remove a staff member, delete a catalogue item with active orders) needs a confirmation dialog.
- Every write action must be safe to retry (the backend is idempotency-key-protected — see [architecture/09_SECURITY_AND_AUTHORITY_MODEL.md](architecture/09_SECURITY_AND_AUTHORITY_MODEL.md)), so the UI should not need to invent its own double-submit protection beyond disabling the button while in flight.
- A vendor with multiple businesses/branches always needs an explicit, always-visible "acting as" indicator — never an implicit single-business assumption.
