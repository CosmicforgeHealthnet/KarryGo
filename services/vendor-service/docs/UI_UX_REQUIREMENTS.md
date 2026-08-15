# UI/UX Requirements — Vendor Service

Plain-language UI/UX requirements generated from Vendor Service's backend/product implementation, for the future Vendor Flutter app team. No frontend code, no Flutter/Dart in this file — only what a screen needs to show, do, and handle.

**Status: Backend work through Sections 10 and 11 is implemented; no UI was built.** Implemented backend areas have documentation-only handoffs below or in [SECTION10_11_HANDOFF.md](SECTION10_11_HANDOFF.md). Do not treat backend readiness as implemented UI.

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
| CRM (customer history, notes) | `features/crm` | **IMPLEMENTED BACKEND — Section 8** |
| Restaurant/Food menu management | `features/food`, `catalog` | **IMPLEMENTED BACKEND — Section 9** |
| Cloud Kitchen configuration | `features/cloud_kitchen` | **IMPLEMENTED BACKEND — Section 10** |
| Pastry/Bakery catalogue and pre-order scheduling | `features/pastry` | **IMPLEMENTED BACKEND — Section 11** |
| Shop/Grocery/Market/Wholesale catalogue | `features/shop`, `catalog` | **IMPLEMENTED BACKEND — Sections 12A/12B** |
| Inventory (stock levels, low-stock alerts) | `features/shop` | **IMPLEMENTED BACKEND — Section 12B** |
| Suppliers | `features/purchasing` | NOT_STARTED |
| Purchasing (purchase orders) | `features/purchasing` | NOT_STARTED |
| Goods receiving | `features/purchasing` | NOT_STARTED |
| Recipes / ingredients | `features/recipes` | NOT_STARTED |
| Kitchen / production board | `features/production` | **IMPLEMENTED BACKEND — Sections 10/11** |
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

## Section 8A — CRM Foundation, Customer 360, Consent and Privacy

Backend only in this section; no frontend source was written. This records the UX the implemented contract supports, and — just as importantly — the UX it deliberately does not.

### The mental model to design against

A CRM record here is a **commercial relationship with this business**, not a person's profile. Karry Go's customer authority owns the person. That single fact drives most of what follows: there is no contact card, no email address, no phone number, and no way for a vendor to add a customer by typing an identifier.

Designers should expect, and design for, a CRM that is **empty in a new deployment** and stays empty until commerce integration exists. The empty state is the default state, not an edge case.

### Customer list

Per row: relationship reference, subject type (Customer / Guest / External), relationship status, privacy state, last order date, completed order count, spend, branch and capability context, and a marketing-eligibility indicator.

- **There is no name, email or phone to show.** A display label exists in the contract but is always absent today. Design the row around the relationship (recency, value, branch), not around identity. If a label is needed, the row should degrade to the relationship reference without looking broken.
- **No free-text search.** Filters only: subject type, status, privacy state, branch, capability, commercial-projection state, order-count range, spend range, last-order date range, acquisition source. Do not design a search bar that implies name/email/phone lookup — the data does not exist to answer it.
- Pagination is limit/offset, max 100 per page, with a stable sort.
- Redacted and merged-away relationships never appear.

### Empty, loading and error states

- **Empty because nothing is integrated** (today's normal): "Customer relationships appear here once orders start flowing." Do not show a "Add your first customer" call to action — there is no such action.
- **Empty because filters exclude everything**: distinguish clearly from the above.
- **Forbidden (403)**: the user lacks the CRM permission, or asked about a branch outside their scope. Say so plainly without revealing whether anything exists there.
- **Not found (404)**: covers "no such relationship", "outside your scope" and "redacted" identically, by design. The UI must not try to distinguish them or speculate.

### Customer 360

Sections: relationship summary, commercial summary, branch and capability context, data freshness, consent summary, privacy summary, merge status, and availability markers for future modules.

**The single most important UX rule in this section**: never render "unknown" as zero.

When `commercial.state` is `NOT_AVAILABLE`, show "No order history recorded" — not "₦0", not "0 orders", not an empty chart implying no spending. A vendor who reads "₦0 spent" about a loyal customer will make a bad commercial decision based on a number the system never actually knew. Reserve real zeros for genuinely observed zeros.

Show freshness explicitly where the backend provides it: `observed_at`, `stale`, and the projection state. A stale figure should look stale.

All money arrives as integer minor units. Format for display; never treat as a float.

### Branch-scoped view

A branch manager sees only relationships that exist at their authorised branches, and the figures they see cover **only those branches**. The backend enforces this; the UI must not undermine it by mislabelling.

Label branch-scoped figures unambiguously — "Spend at Ikeja branch", not "Total spend". A customer who spent ₦100,000 at branch A and ₦300,000 at branch B must show ₦100,000 to the branch A manager, framed as that branch's figure. Never present it as the customer's lifetime value, and never hint that other branches exist.

`commercial.scope` (`BUSINESS` / `BRANCH`) is provided precisely so the label can be driven by data rather than by guesswork.

### Consent

Read-only, per channel (Email, Push, In-app, SMS), showing decision, eligibility, reason, observed time and staleness.

- **The interface must never let staff set, override or "fix" consent.** No toggle, no "mark as opted in", no bulk action. If a vendor believes a decision is wrong, the resolution is with the customer authority, not here.
- Present three distinct states, never two. `UNKNOWN` is "no decision on record — cannot contact", visually distinct from `DENY` ("customer declined"). Collapsing them into a single "not opted in" toggle is exactly the design error that leads to someone contacting a customer they had no permission to contact.
- Show the reason, not just the outcome. "Cannot email: no consent on record" is actionable; a grey icon is not.
- When the consent integration is unavailable, say so as a **system** state ("Consent service not connected") rather than attributing it to the customer.
- SMS consent may read `ALLOW` even though the platform has no SMS sender. Do not offer an SMS action on that basis.

### Privacy

Show state and its consequences, with affected actions visibly disabled and explained.

- **Restricted**: visible, not targetable. Marketing controls disabled with a reason.
- **Marketing suppressed**: relationship intact, targeting off. This can co-occur with an `ACTIVE` state and even a stored `ALLOW` — the UI must follow `eligible`, not `decision`.
- **Merged**: show that this relationship has been merged and link to the surviving one. All actions disabled.
- **Redacted**: normally unreachable (reads 404). If encountered, show a neutral "This relationship is no longer available" — never the previous data, and never an option to restore.
- Nothing in the privacy view may be presented as vendor-editable. There is no override, at any permission level, including Owner.

### Archive and restore

The only vendor-facing writes. Archive is a tidiness action for dormant relationships and must never be presented as deletion, erasure or a privacy remedy — the copy should avoid "delete" and "remove" entirely.

Both require the current record version; a stale version returns 409, so the UI should reload and re-present rather than silently retrying. Both are refused on redacted and merged relationships, so those actions should not be offered there at all.

### Future modules

Tags and segments report `AVAILABLE` when Section 8B is enabled; interactions and tasks report `AVAILABLE` when their Section 8C flags are enabled; loyalty reports `AVAILABLE` when `VENDOR_CRM_LOYALTY_ENABLED` is on. Their scope-filtered empty lists then genuinely mean "none visible to this actor." Campaigns, corporate, and intelligence remain `NOT_IMPLEMENTED`. The same applies to category and item affinity.

### Role-based visibility

- Owner / General Manager / CRM & Support: full Section 8A surface.
- Marketing: customer list, Customer 360 and consent eligibility. **No privacy view, no archive/restore, and structurally no access to contact data.**
- Branch Manager: read-only, branch-scoped as described above.
- Analyst / Auditor: read-only including privacy state.
- Finance, Kitchen/Pastry, Shop/Inventory, Procurement, Vendor Rider: no CRM access by default — CRM should not appear in their navigation.

---

## Section 8B — Tags & Segmentation

Documentation only. No frontend code exists for this section; what follows is the contract a future builder must respect.

### The one rule that shapes every screen here

A segment is an **audience**, not permission to contact it. Nothing in Section 8B sends anything, and no screen built on it may imply otherwise. A "Message this segment" button does not belong in any of these views until campaign execution exists and re-checks consent at send time.

### Tags

**Tag list.** Name, scope (Business / Branch), member count, status. Business and branch tags should be visually distinguishable; a branch tag is that branch's private vocabulary and reads confusingly next to a company-wide one without a marker.

- Member counts are **scope-filtered**. A branch manager's "VIP: 34" is 34 at their branches, not 34 in the company. Label the count with its scope, or a branch manager will quote it as a company figure in a meeting.
- Archived tags are hidden from ordinary selectors. Offer a filter to see them; do not mix them into a tag picker.

**Create / edit.** Name, optional description, scope selector.

- Names are compared case- and whitespace-insensitively. Warn **before** submitting when the entered name collapses onto an existing one ("VIP already exists"), rather than surfacing a 409 after the fact — duplicate near-identical tags are the single most common way a CRM taxonomy rots.
- `sys:`, `system:` and `karrygo:` are reserved. Explain why rather than showing a bare rejection: those prefixes are how a future platform-computed trait will be distinguishable from a vendor's own label.
- Scope is **immutable after creation**. Do not render a scope control on the edit screen; moving a tag between branches would silently reinterpret every existing membership.
- A branch-scoped user must not be offered the Business scope option at all.

**Assigning tags.** From a customer's Customer 360, and in bulk from a filtered customer list.

- Bulk operations are **all or nothing**, capped at 500 relationships. Show the cap before the user selects 900 rows, not after they press Apply.
- The response reports `requested` / `applied` / `skipped`. `skipped` means "already tagged", not "failed" — word it that way.
- A failed bulk operation reports a single not-found rather than naming which references were bad. That is deliberate (it would otherwise enumerate other tenants' relationships), so the UI must not promise a per-row error report.

**Archiving.** Removes current memberships and keeps the tag's history. Say both things in the confirmation: users reasonably expect archiving to be reversible and it is not, for membership.

### Segments

**Segment list.** Name, type (Static / Dynamic), scope, status, member count, and — non-negotiably — **freshness**.

A member count without its `evaluation_status` is the defining failure mode of this screen: a stale audience presented as a live one is how a vendor targets the wrong people. Render:

- `CURRENT` — membership matches the current definition.
- `STALE` — a real, usable audience, but out of date. Show `stale_since` and offer Rebuild.
- `QUEUED` / `RUNNING` — a rebuild is in flight; the shown membership is still the previous one.
- `FAILED` — the last rebuild failed. **The member count is still real** (it is the last successful audience). Say "Last rebuild failed — showing the audience from <time>", never blank the count.
- `NOT_EVALUATED` — never built. Show no count at all rather than zero.

When `materialized_definition_version` differs from `definition_version`, the audience was built from an older rule. Say so explicitly.

**Segment builder.** The core screen.

- The builder **fetches `GET /crm/segment-fields` and renders only what it returns.** It must not carry a hardcoded field list: fields appear and disappear with deployment configuration, and a hardcoded list will offer fields that match nobody.
- Field choice drives operator choice, which drives the value editor. Never offer an operator a field does not publish.
- Fields with `available: false` should be shown as unavailable with an explanation ("No order data is connected yet"), not hidden — hiding them makes the absence look like a missing feature rather than a missing integration.
- Nested AND / OR / NOT groups, within the published limits (`max_depth`, `max_nodes`, `max_or_children`, `max_in_values`, `max_not_depth`, `max_tag_references`). Enforce these client-side so the user is stopped while building rather than on submit.
- Money values are always an integer count of minor units plus an explicit currency. Present a currency-aware money input; never a float.
- There is **no free-text query box, no "advanced SQL" mode and no raw filter escape hatch** anywhere in this builder. If a user needs something the registry cannot express, the answer is a new registered field, not a text box.

**Preview.** Runs before saving and persists nothing.

- Show `match_count` with `count_state`. `AT_LEAST` means the count budget was reached — render it as "at least 50,000", never as an exact figure and never as an estimate.
- The sample carries a relationship reference, an optional safe display label and a projection state. There is no email, phone or address to show, because CRM does not hold any. Do not design a preview table with contact columns.
- `matched_conditions` and `unavailable_conditions` are node paths. Highlight the corresponding builder nodes — that is what turns a count into an explanation.
- `warnings` and `unavailable_fields` must be surfaced prominently. "Some conditions use data we have not observed, so they match nobody" is the difference between a user understanding an empty audience and filing a bug.
- Preview needs only the read permission. An analyst who cannot create segments can still explore.

**Static membership.** Add/remove customers explicitly, in bulk, capped at 500.

- Only offered for `STATIC` segments. For a dynamic segment there must be **no** "add member" control at all: the server refuses it, and offering it would promise something that would silently disappear at the next rebuild.

**Exclusions.** Only for `DYNAMIC` segments.

- Present as "never include this customer in this audience", a standing decision that survives rule edits and rebuilds. It is not the same control as removing a static member and should not look like one.
- The effect is immediate, not at the next rebuild — reflect the new count straight away.

**Members list.** Paginated, scope-filtered, showing the current generation only.

- Show the membership freshness block alongside the list, for the same reason as the segment list.
- Rows are relationship references. Again: no contact data exists to display.

### Customer 360 additions

Tags and Segments now report `AVAILABLE` in the module map and carry real content.

- Tags: chips, with branch tags marked. A business-scoped viewer sees permitted business and branch classifications. A branch-scoped viewer sees only tags owned by their permitted branches — **business-wide and other-branch classifications are not disclosed**, because the label itself reveals how a wider scope classified the customer.
- Segments: the audiences this customer currently belongs to, each with its freshness. A branch-scoped viewer sees branch segments only; showing "Top 1% nationally" to a branch manager would leak a business-wide ranking they cannot otherwise see.

### Privacy in the segmentation UI

- There is **no** "include redacted customers" control, at any permission level. Redacted and merged-away relationships are excluded by the engine before any vendor rule runs.
- There is **no** "ignore consent" control. Consent eligibility is a readable field in the builder; it is not assertable.
- Privacy state is not a targetable field and must not appear in the field picker.
- A customer's redaction removes them from every tag and audience immediately. If a user has a segment open when that happens, the count changing is correct behaviour, not a glitch.

### Role-based visibility

- Owner / General Manager / CRM & Support / Marketing: full tag and segment surface, including member lists. Marketing gets audience building because that is the role's core work; it still gets no campaign execution, because that does not exist yet.
- Branch Manager: tags and segments, branch-scoped. The Business scope option must not be offered, and business-scope objects will not appear in their lists at all.
- Analyst / Auditor: read and preview only. No create, edit, archive, materialize or membership controls.
- Finance, Kitchen/Pastry, Shop/Inventory, Procurement, Vendor Rider: no access — segmentation should not appear in their navigation.

## Section 8C interaction and task UI contract

This is a documentation handoff only; Section 8C contains no frontend implementation.

The existing Customer 360 screen should gain two bounded cards when the corresponding modules report `AVAILABLE`: recent interactions (at most five plus `has_more`) and open tasks (at most five plus a scope-safe exact count). A "view all" action should open the dedicated paginated customer interaction or task endpoint. Never infer hidden totals from timestamps, pagination, or an empty response.

The interaction experience needs a timeline, add form, detail view, and revision history. The form must use the server's closed type/direction/outcome lists; collect a business or permitted branch visibility; limit subject to 200 and plain-text note to 5,000 characters; and warn staff not to enter unnecessary sensitive information. Corrections require a reason and the latest `version`; the UI must reload on a 409 rather than silently overwrite history.

The task experience needs a list or board with status, priority, assignee, branch, due-date, overdue, customer, and date filters. It should show `OPEN`, `IN_PROGRESS`, `COMPLETED`, and `CANCELLED` only; overdue is an indicator, never a fifth state. Start, complete, cancel, edit, and reassign actions send the current `expected_version` and a stable `Idempotency-Key`. The assignee picker must show only server-approved active staff for the current scope; an inactive assignee remains visibly inactive until an authorized user reassigns the task.

Business actors with the appropriate permission can see business-level and permitted branch activity. Branch actors see only their authorized branch activity: no business-only row, other-branch row, hidden count, or hidden latest timestamp. Redacted and merged-source profiles must not expose ordinary activity content or actionable controls. Feature-disabled routes return 404; write-disabled commands return the established unavailable response.

## Section 8D loyalty UI contract

This is a documentation handoff only; Section 8D contains no frontend implementation.

### Programme configuration

One screen per business, behind `vendor.crm.loyalty.manage`. It edits status (`DISABLED` / `ACTIVE` / `PAUSED`), the points display nouns (singular and plural, 40 characters each — a vendor may call them "stars" or "beans"), the expiry policy, the redemption minimum and increment, and the manual adjustment limit. Every label in the product must use the vendor's own configured noun, never a hardcoded "points".

The expiry control is a single choice between "never expire" and "expire after N days", because the two are not independently valid: `NEVER` with a window and `FIXED_DAYS` without one are both rejected as `LOYALTY_INVALID_EXPIRY_POLICY`. When the operator switches to a shorter window, the UI must state plainly that **points customers already hold keep their original expiry** and only future points use the new one. Presenting this as "all points now expire after N days" would be a lie the backend deliberately does not implement.

A business that has never configured loyalty is shown the defaults with `configured: false` and `version: 0`. The save action sends that `version` as `expected_version`; a 409 means someone else changed the programme (or a first credit materialised it) and the screen must reload rather than retry.

Status needs honest copy, because the three states gate different things: `DISABLED` stops earning, redemption **and manual adjustment**, but reversal still works; `PAUSED` stops earning and redemption while adjustment and reversal still work; `ACTIVE` allows everything. A "pause" affordance should say that existing balances are preserved — nothing is confiscated by pausing.

### Customer 360 loyalty card

Shown when the `loyalty` module reports `AVAILABLE` and the actor holds `vendor.crm.loyalty.read`. It presents the enrolment state first, because `NOT_ENROLLED` and a zero balance are different facts: an unenrolled customer has never held points and must not be rendered as "0 points". Then the available balance, lifetime credited and debited totals, and the next expiry date with how many points fall on it.

`pending_expiry_points` is non-zero when points are past their expiry but the background sweep has not yet recorded it. Those points are already excluded from `available_points`, so the card must never add them back; surface them as an explanation ("N points have expired and are being processed") so a balance that is about to drop is not a surprise.

A merged-away or redacted customer reports `NOT_ENROLLED` with no figures. The card should render an explanatory state, not an empty balance, and should offer no actions.

### Points history

A paginated ledger, newest first, default 50 and maximum 200 per page. Ordering is fixed server-side; do not offer a sort control, because there is none. The only filters are entry type and a created-at range.

Each row shows the type, the signed delta, the resulting balance, and the timestamp. Types need plain-language labels: earned, redeemed, expired, manually added, manually removed, reversed, and transferred in/out from an account merge. A row carrying `reversed_by_entry_ref` must be visibly struck through or badged as reversed and linked to its reversal; a `REVERSAL` row links back through `reverses_entry_ref`. Without that, a history containing a mistake and its correction reads as two unexplained movements.

### Manual adjustment

Behind `vendor.crm.loyalty.adjust`, which is deliberately not granted to Marketing, Branch Manager, Analyst or Finance — the action should not appear in their navigation at all. The form collects a direction (credit or debit), a **positive** whole number of points, and a mandatory reason of up to 500 plain-text characters.

The points field must reject decimals and negatives in the client as well as the server: direction carries the sign, and a negative amount is a validation failure, not a clever way to debit. The client should show the programme's `manual_adjustment_limit` before submission rather than letting the operator discover it as a 409 (`LOYALTY_ADJUSTMENT_LIMIT_EXCEEDED`).

The reason field needs copy explaining where it goes: it is written to the audit log and is visible to auditors, and it is deliberately **not** carried on the ledger row or any downstream event. Staff should be told not to enter customer contact details or case narratives there.

A manual credit may enrol a customer who has no account. A manual debit may not, and may not exceed the available balance — both surface as errors and the form should not offer a debit at all on a `NOT_ENROLLED` customer.

### Reversal

Behind `vendor.crm.loyalty.reverse`. Offered only on `EARN`, `REDEEM`, `ADJUSTMENT_CREDIT` and `ADJUSTMENT_DEBIT` rows that are not already reversed; every other row must not show the action, because the server refuses it (`LOYALTY_ENTRY_NOT_REVERSIBLE`).

Reversal is always full — there is no partial-amount field — and requires a mandatory reason handled exactly like the adjustment reason. The confirmation copy should state the exact points and resulting balance.

Two refusals need specific, non-alarming copy rather than a generic error. `LOYALTY_REVERSAL_POINTS_ALREADY_CONSUMED` means the customer has already spent or expired those particular points, so the credit cannot be cleanly taken back; the recovery is a manual debit with an explanation, and the UI should offer that path. `LOYALTY_ENTRY_ALREADY_REVERSED` means someone else got there first, and the row should refresh to show the existing reversal.

When a debit reversal restores points whose expiry has since passed, the history will show the reversal immediately followed by an expiry of the same amount. That is correct and must be explained in the UI rather than looking like the points were given and then taken away by a bug.

### Permissions and states

- Owner / General Manager: full loyalty surface.
- CRM & Support: read, adjust and reverse — but **not** programme configuration.
- Marketing / Branch Manager / Analyst & Auditor / Finance: read only. Branch Manager reads stay branch-scoped, and a customer outside their branches is simply absent, not denied.
- Kitchen/Pastry, Shop/Inventory, Procurement, Vendor Rider: no access — loyalty should not appear in their navigation.

### What the UI must never imply

Points are not money. No screen may show a currency symbol beside a points figure, offer a cash value, suggest withdrawal or transfer to a wallet, or place loyalty alongside wallet balances in a way that reads as a second account. There is no rewards catalogue, no tier ladder, no voucher and no cashback in Section 8D, and the UI should not scaffold placeholders for them.

## Section 8E campaign intelligence UI contract

This is a documentation handoff only; Section 8E contains no frontend implementation.

The single most important thing this UI must convey is that **preparing a campaign is not sending one, and a prepared audience is not permission to contact anybody**. Every screen below has to survive a user who assumes otherwise. There is no send button to build, because there is no send endpoint; a "Ready to send" affordance anywhere in this surface would be describing a capability the backend deliberately does not have.

### Campaign list

Behind `vendor.crm.campaigns.read`. Filters are status, scope, branch, objective, channel and a created-date range, plus a name search; ordering is fixed at most-recently-updated first and is not user-selectable. The five statuses are `DRAFT`, `PREPARING`, `PREPARED`, `PREPARATION_FAILED` and `ARCHIVED`. There is no "scheduled", "sending" or "sent" state to render, and inventing a badge for one would misdescribe what the platform can do today.

`PREPARATION_FAILED` deserves careful copy: it can legitimately coexist with a usable prepared audience, because a failed rebuild never empties the one that already worked. The honest phrasing is "the latest rebuild failed; the previous audience is still available", not "this campaign is broken".

A branch-scoped user sees only their own branch's campaigns. A business-wide campaign is not shown as locked or greyed out — it is absent, because its name and audience size describe the whole business.

### Create and edit

Behind `vendor.crm.campaigns.manage`. The form collects a name, an objective, a scope, one to four channels, a source audience segment, up to ten exclusion segments, an optional frequency policy, an optional quiet-hours policy, a service-recovery toggle and an optional experiment.

Scope must be chosen at creation and then locked, with copy explaining why: changing it would reinterpret every audience already prepared from the campaign. A branch campaign's segment picker must offer only that branch's segments; offering a business-wide one and letting the server refuse it would be a worse experience than not offering it.

The objective list is `GENERAL_MARKETING`, `RETENTION`, `REACTIVATION`, `LOYALTY` and `ANNOUNCEMENT`. All five are marketing objectives and all five require marketing consent — including `ANNOUNCEMENT`. The UI must not imply that an announcement bypasses consent, because it does not.

Editing a prepared campaign returns it to `DRAFT`. Warn before saving: the prepared audience stays available as history, but the new definition has never been prepared and the user will need to prepare again.

Service-recovery suppression defaults on. Turning it off is a deliberate act and should read as one — the plain-language consequence is "this campaign may reach customers with an unresolved complaint open".

### Experiment configuration

Allocations are integer basis points and must total exactly 10000. The editor should show percentages for humans while sending basis points, refuse to save at anything other than 10000, and say by how much the total is off. At most one control cohort, capped at 50%; one to five treatment variants; unique keys.

Explain the holdout honestly: a control member is selected into the audience and satisfies every safety rule, and is simply not sent the campaign when Section 8F exists. It is not an exemption from consent or privacy.

### Audience preview

Behind `vendor.crm.campaigns.preview`. Preview writes nothing and changes no state, so it can be re-run freely.

Present the funnel in the order the engine applies it, with the reconciliation visible: source count, minus segment exclusions, minus privacy suppressions, minus service-recovery suppressions, equals the target count; and control plus treatment equals target. If those do not add up on screen, the screen is wrong.

Per channel, show how many targets were eligible and how many were not, with the ineligibility reasons broken out. When no consent authority is integrated the honest answer is "nobody is currently contactable on any channel, because consent data is unavailable" — not an empty state that reads like a data-loading failure.

A stale source segment must be surfaced prominently: the preview counts nobody and the fix is to rebuild the segment, not to retry the preview. The `source_segment_evaluation_status` is what tells you.

The frequency policy always reports `NOT_AVAILABLE`. The copy should say the cap is stored and will be enforced at send time, and that it cannot be evaluated yet because nothing has been delivered — never "0 of 3 contacts used".

Quiet hours may be tested against a specific instant the user picks. That is a "what if we sent at this time" question and nothing more; it schedules nothing, and the UI must not present it as picking a send time.

The sample is bounded and carries a relationship reference, a safe display label and a reason code. There is no email address, phone number or device token to show, and no column should be reserved for one.

### Prepare and snapshots

Behind `vendor.crm.campaigns.prepare`. Preparation is asynchronous: the action returns a `BUILDING` snapshot and the UI polls until it reports `CURRENT` or `FAILED`. Pressing the button twice returns the same run — the UI should reflect that rather than showing two.

The snapshot summary presents the same reconciled funnel as the preview plus the frozen source segment version and generation, and must display the three send-time recheck obligations rather than hiding them as implementation detail. A snapshot is evidence of a past decision; its counts never change afterwards.

`FAILED` with `dead_lettered: true` means the run gave up and will not retry. `CRM_CAMPAIGN_AUDIENCE_NOT_CURRENT` means the source segment moved on and the fix is to prepare again, not to wait.

### Target list

Behind `vendor.crm.campaigns.targets.read`, which is a separate and larger disclosure than reading the campaign — the UI should treat entering this view as a deliberate step rather than a tab that opens by default.

Rows carry an opaque relationship reference, an optional safe display label, a cohort, the current actionability flag and the per-channel snapshot consent evidence. There is no contact detail anywhere in the response and none may be displayed, inferred or joined in from elsewhere on this screen.

`currently_actionable: false` needs its reason shown (`PRIVACY_REDACTED`, `PRIVACY_RESTRICTED`, `PRIVACY_MARKETING_SUPPRESSED`, `IDENTITY_MERGED`) or, when `service_recovery_active` is true, an explanation that the customer has an open complaint. Crucially, `currently_actionable: true` still does not mean "will be sent" — every channel row carries `send_time_recheck_required` and the UI must say so on the page, not only in a tooltip.

### Flags and failure modes

- `VENDOR_CRM_CAMPAIGNS_ENABLED` off: routes are not registered and requests return 404. That means "campaigns are not turned on for this deployment", not "campaign not found".
- `VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED` off: definitions, previews and already-prepared snapshots all keep working; only new preparations are refused with 503. An existing audience must never render as empty because preparation is frozen.
- `VENDOR_CRM_WRITES_ENABLED` off, or the global commerce kill switch on: reads and previews work, commands return 503.
- A branch user reaching a campaign outside their scope receives 404, not 403. The UI should render that as "not found" and must not disclose that the campaign exists elsewhere.

There is no rewards catalogue in Section 8E. Section 8F now defines backend contracts for campaign execution; this is documentation only and no frontend was implemented.

## Section 8F campaign execution UI contract

- A run composer offers Send now or Schedule, an RFC3339 schedule time, an approved template key or bounded plain-text title/body, and a final audience/version confirmation. Every create command keeps one Idempotency-Key and sends the displayed campaign `expected_version`.
- Run pages display `SCHEDULED`, `QUEUED`, `RUNNING`, `PAUSED`, `COMPLETED`, `COMPLETED_WITH_FAILURES`, `CANCELLED`, or `FAILED`, plus total, handed-off, delivered, suppressed, and failed counts. `HANDED_OFF` must be labelled “accepted by Notification”, never “delivered”.
- Pause, resume, and cancel use the run's current `version`. Cancellation cannot recall a delivery already handed off; the confirmation copy must say so.
- Delivery views paginate and show channel, cohort/variant, state, retry count, safe suppression reason, and transition history. They never show an email address, phone, token, or destination.
- Surface quiet-hours deferrals, frequency-cap suppression, current-consent/privacy blocks, control-group holdouts, channel-unavailable suppression, and retry/final failures as distinct safe reasons.
- SMS must not be offered while Notification Service has no production SMS channel.

## Section 8G corporate customer UI contract

- A corporate account page shows the organization, its `CORP-` reference, lifecycle status, scope (business-wide or a named branch set), account manager, and the actionable summary counts. Counts exclude archived children and privacy-suppressed contacts, so they describe what an operator can act on rather than everything the tables hold — label them accordingly.
- Lifecycle actions are `Activate`, `Put on hold`, `Deactivate` and `Archive`. Each sends the displayed `version` as `expected_version` with one Idempotency-Key. Archiving must warn that it cancels the account's open tasks and closes the account to new activity; nothing is deleted.
- **Contacts are people the vendor already knows.** The add-contact flow picks an existing customer relationship; it must never offer fields for an email address, phone number or postal address, because Section 8G stores none and returns none. Show the display label, and when it is absent show the contact as non-actionable rather than inventing a placeholder name.
- A contact whose relationship was redacted stays visible for continuity with `privacy_suppressed: true` and `actionable: false`. It must be rendered as unavailable rather than removed, and every action on it must be disabled — `actionable` is the only signal to branch on.
- Promoting a primary contact silently demotes the previous one server-side. Say so in the confirmation, so the change does not look like a bug.
- **Cost centres are labels.** Do not present a budget, limit, currency, balance or spend column, and do not imply that a cost centre authorises anything. The same applies to membership roles: `APPROVER` describes how a person participates, and approves nothing.
- Locations are the customer organization's own sites and are unrelated to the vendor's branches. Keep the two vocabularies visually distinct; a user who confuses them will scope an account wrongly.
- Archive, not delete, is the only removal on contacts, memberships, locations and cost centres. Copy must say the record is kept so past activity stays attributable.
- The account timeline and task list are the Section 8C surfaces with a corporate subject. Reuse the same components, and expect the same vocabularies, bounds and immutable-correction behaviour: a correction supersedes the original rather than editing it.
- Corporate activity needs the Section 8C permission **and** corporate read. A user who can log interactions but lacks corporate read must not see the corporate section at all, rather than seeing it and failing on click.

There is no corporate ordering, invoicing, credit, budget, contract or approval UI in Section 8G, because there is no backend for one. Do not design a placeholder for it.

## Section 8H CRM intelligence and export UI contract

- **Lead every intelligence view with availability.** Show `profile_count`, `available_count` and `unavailable_count` before any histogram. A business with no order data must read as "1,200 relationships, 0 scored, 1,200 unavailable — no commercial data observed", never as "0 customers". Surface the `unavailable_reasons` breakdown; it is what makes an empty dashboard actionable instead of mysterious.
- **Never render a score as a percentage or a probability.** Churn risk is an integer rank in 0-100 with a band. Copy must say "risk score" or "risk band", never "% likely to churn", "chance of leaving" or "prediction". RFM dimensions are quintiles of *this scope's scored cohort* and must be labelled as relative — a 5 means "top fifth of the customers we could score here", not "excellent".
- **A score is not permission, and the UI must not imply it is.** A high-risk list is a follow-up queue, not an audience. Any path from an intelligence view to a campaign must go through the ordinary segment and campaign flows, where consent, privacy, frequency and quiet hours are enforced. Never offer "message these customers" directly from a score.
- **Distinguish stale from absent.** A `STALE` score is real and readable, shown with its `source_as_of`; an unavailable one has no numbers at all. Do not grey them out identically, and never show a stale score without its as-of instant.
- **Show the model identity.** Every score view carries `model_key`, `model_version` and `source_as_of`. When a rebuild is running, keep showing the last good figures marked stale rather than blanking the page.
- **Rebuild is an operational action.** Confirm it, show that a run already in flight is returned rather than duplicated, and make clear it is scope-wide work rather than a refresh of the row in front of the user.
- **There is no way to edit a score, and there must be no UI that suggests otherwise** — no "mark as VIP", no "override risk", no manual RFM entry. Intelligence is derived.
- **Branch users see branch intelligence only.** Never show a business-wide figure to a branch-scoped operator, and when a user holds several branches make them choose one rather than silently picking the first.
- **Export request flow.** Dataset from a fixed list, format CSV or JSONL, an optional typed filter, and a row limit with the ceiling stated up front. Show the exact field list the dataset will emit before the user commits. Never offer a column picker, a sort, a search box or anything resembling a query.
- **Export progress and expiry.** Poll the job; show `QUEUED`/`RUNNING`/`READY`/`FAILED`/`CANCELLED`/`EXPIRED`, the row count, the checksum and — prominently — the expiry. An expired export must be visibly gone, not a button that fails on click.
- **Download requires its own step-up.** Explain why before prompting, and make clear the resulting link is short-lived and single-use. Never store, bookmark, email or re-display a download token; if the user needs the file again they ask again, and that is by design.
- **Failed and cancelled exports must show the reason code**, particularly `CRM_EXPORT_TOO_LARGE` (narrow the filter) and `CRM_EXPORT_STORAGE_UNAVAILABLE` (retry later, not a data problem).

There is no AI, recommendation, next-best-action or automated-campaign UI in Section 8H, because there is no backend for one. Do not design a placeholder for it.

## Section 8I — final UI/UX contract note

Section 8I adds no surface, so every UI contract above stands unchanged. Two
points now apply across all of them, and a client that ignores either will show
something untrue.

- **Distinguish "we do not know" from "there is none", everywhere.** Commercial
  figures, consent decisions, RFM and churn scores and campaign eligibility can
  all be legitimately unavailable on a deployment whose external authorities are
  not yet connected, and each reports that state explicitly. Rendering an
  unavailable value as `0`, as an empty list, or as the safest band turns a
  missing integration into a business fact the operator will act on.
- **Nothing in CRM is a permission to contact anybody.** A tag, a segment
  membership, a loyalty balance, a corporate relationship and an intelligence
  score are all descriptions. Consent, privacy state, suppression, frequency and
  quiet hours decide reachability, and they are re-checked immediately before
  any handoff. A UI that offers "message these customers" from a score or a
  segment view must route through the campaign flow where those checks live.


---

## Section 8 completion — loyalty tiers

**Status: backend complete, no frontend built.** This section describes what a
future Vendor UI should present. It is a contract, not an implementation.

Customer 360 may also expose affinity, service-recovery, customer-value,
relationship-signal and corporate-context blocks. Each block carries explicit
availability; `NOT_AVAILABLE` must never be rendered as zero, false or an empty
success state. Producer-owned facts remain unavailable until their authority
delivers them.

### Tier configuration screen

One screen, editing the whole ladder as a set. Do NOT build per-rung add/edit/
delete controls that save independently: the API replaces the ladder atomically
because a ladder is only valid as a set, and a UI that saved one rung at a time
would be fighting that.

- The ladder is at most **8 rungs**, shown in ascending threshold order.
- **Rank is not editable.** It is derived from the thresholds. Show it as a
  read-only ordinal that reorders live as the operator changes a threshold, so
  the rule is visible rather than surprising.
- **Thresholds must be distinct.** Validate client-side and show which two rungs
  collide, because the server refuses the whole write and a generic error would
  leave the operator hunting.
- The threshold field must be labelled in terms of **lifetime points earned**,
  never "balance" or "points available". These are different numbers and the
  distinction is the whole feature.
- Save requires `expected_version` from the read. On a 409, do not silently
  reload: tell the operator someone else changed the ladder and show them what
  it is now, because their intended change may no longer make sense.
- Removing all rungs is legitimate (an empty array) and turns tier recognition
  off. Confirm it explicitly — every customer's status disappears.

### Customer 360 — tier standing

Rendered inside the loyalty block, not as its own card. Three states, and they
must look different:

- `CURRENT` — show the tier name, and the progress to the next rung when
  `next_tier_key` is present. On the top rung there is no next tier; do not
  render an empty or 100% progress bar.
- `NO_TIER` — a ladder exists and this customer has not reached its first rung.
  Show what the first rung is and what it takes. Do **not** render this as a
  greyed-out bottom tier; they are not in the bottom tier, they are not on the
  ladder.
- `NOT_AVAILABLE` — this business has configured no ladder, or the relationship's
  loyalty account is redacted or merged away. Show nothing about tiers at all.
  Do not show "Bronze", "None", "-" or a zero: the platform must not invent a
  status the vendor never defined.

`lifetime_points` is absent for an unenrolled customer. Render its absence as
absence, not as 0.

### What must NOT appear anywhere in the tier UI

No discount, no entitlement, no reward, no points multiplier, no monetary
figure. Tiers confer recognition only, the backend has no field for any of it,
and a UI that implied otherwise would be promising customers something the
system cannot deliver.

## Section 9A — food catalogue management (backend-derived requirements)

Backend only. Nothing below was implemented; it records what the Section 9A API
obliges a vendor console to do, and — more usefully — what it forbids it from
pretending.

### Menu management

A menu belongs to **one branch and one food capability** and neither can be
changed after creation. The create form must therefore ask for both up front,
and the branch picker must offer only branches the signed-in user's scope
reaches and that hold the chosen capability — a branch without it is refused
with `FOOD_CAPABILITY_NOT_SUPPORTED`, which is a poor first experience if the
picker offered it.

Only `RESTAURANT` and `FAST_FOOD` exist here. Do not show the other five KarryGo
capabilities in this surface.

Status actions are `activate`, `pause` and `archive`, and each is a separate
command carrying `expected_version`:

- **Activate** can fail with `FOOD_MENU_NOT_PUBLISHABLE` because the business,
  the branch or the branch capability is not ACTIVE. The message names which.
  This is a legitimate state during onboarding, not an error to hide: the
  console should let the user keep editing a DRAFT menu and explain what has to
  happen before it can go live.
- **Archive is terminal.** The confirmation must say so plainly — an archived
  menu is never reactivated, and archiving it also archives its sections and
  their items. Do not present it as a soft toggle next to pause.
- **Pause is administrative.** It withholds the whole menu. It is not
  "temporarily sold out", it is not a schedule, and the console must not label
  it as either; time-based availability arrives in Section 9B.

### Section and item ordering

Sections, items, variants, modifiers and an item's attached modifier groups are
each explicitly ordered, and every reorder endpoint takes **the complete list of
that collection's live ids, exactly once, in the new order**. A drag-and-drop
implementation should therefore submit the whole visible order after the drop
rather than a single moved id. A partial list, a duplicate or an id from another
collection is refused with `FOOD_INVALID_REORDER`.

The server returns the canonical order after the write. Render from that
response rather than from local state, because a concurrent editor may have
changed the collection; positions are always contiguous from zero.

Archived rows sort behind live ones and are hidden unless the caller asks for
`include_archived=true`. An "show archived" toggle is worth having on section,
item, variant and modifier lists.

### Item editor

- **Prices are integer minor units.** The field the user types in is a
  currency amount; what goes on the wire is `base_price_minor` as a whole
  number. Do the conversion once, at the edge, and never with floating-point
  arithmetic. The response reports `currency` — display that, do not assume one.
  Zero is a valid price. Negative and out-of-range values are refused.
- **Status** is DRAFT / ACTIVE / PAUSED in the editor. Publishing an item
  (setting ACTIVE) needs the publish permission, so a Kitchen user's editor must
  show the field as read-only rather than letting them submit and be refused.
  ARCHIVED is not settable here — it has its own action.
- **Dietary and allergen codes** are free-form within a bounded shape
  (`A-Z`, `0-9`, `_`, 2-40 characters, up to 16 and 24 respectively). Offer a
  suggestion list, allow a vendor's own code, and normalise on entry so the user
  sees what will be stored. The UI must not describe these as verified,
  certified or safe: they are the vendor's own declaration and the API says so.
- **Media** is a picker over references the vendor already uploaded through
  Media File Service. There is no upload endpoint in this section. The whole
  ordered set is replaced in one `PUT`, with at most twelve entries, no
  duplicates and exactly one cover; if the user marks none, the first becomes
  the cover.

### Variant editor

A variant either inherits the item's base price or overrides it. Model this as a
two-state control, not an empty number field: the response's
`inherits_base_price` says which, and returning to inheritance requires sending
`clear_price_override: true` rather than clearing the input.

### Modifier group editor

`min_select` and `max_select` are the whole rule. "Required" is derived from
`min_select > 0` — show it as a derived label, never as an independent checkbox
the user can set to disagree with the numbers. `max_select` must be at least
`min_select` and at least 1; both are capped at 50.

**Modifier groups are shared across the whole business.** The editor must say so
before the user saves: renaming "Add extras" or repricing "Extra cheese" changes
every item that uses it, across every branch. A branch-scoped user gets 403 on
create/edit/archive here and should see a read-only view with an explanation,
not a form that fails on submit. Attaching and detaching a group on their own
branch's item remains available to them.

Detaching a group from an item removes only the link; the group survives for
every other item. Archiving a group detaches it everywhere — the confirmation
should say how many items are affected.

### Concurrency conflict handling

Every mutation of an existing row carries `expected_version`, and a stale value
returns 409 with `FOOD_STALE_VERSION`. The console must not silently retry with
a refreshed version — that is how one editor's change disappears. Show the
conflict, reload the current state, and let the user decide.

Every mutation also accepts `Idempotency-Key`. Generate one per user-initiated
command and reuse it on network retry, so a double-tapped "Create menu" produces
one menu. Reusing a key with a different body is a 409 by design.

### Permissions

| Role | Read | Manage | Publish |
|---|---|---|---|
| Owner, General Manager | yes | yes | yes |
| Branch Manager | yes | yes (own branches; not shared modifier groups) | yes (own branches) |
| Kitchen / Pastry | yes | yes | no |
| Analyst / Auditor | yes | no | no |
| Everyone else | no | no | no |

Hide or disable what a user cannot do rather than letting the API refuse it, but
never rely on that alone: the server decides, and it reports an out-of-scope
menu as **not found** rather than forbidden so ids cannot be probed.

### What this surface must not claim

It is a management console, not a storefront. A menu marked ACTIVE here means
"published for management purposes"; whether an item can actually be sold right
now — schedules, sold-out state, effective price at a branch — is Section 9B and
is an authenticated Section 9B management answer. Do not build a
customer-facing preview that implies discovery. Section 9C is an internal
service-to-service consumption contract; no storefront or frontend route is
implemented here.
## Sections 10/11 boundary

Operational screens may expose capacity, package, plan, pastry, quote and
production-job state through the Vendor management surface. Do not present a
capacity reservation or conversion intent as a paid Order, delivery promise,
stock balance or customer ETA.

## Section 12A Shop management UX handoff

Future Vendor UI should provide separate workspaces for category trees,
products/media, variants/SKUs/barcodes, packaging hierarchy, branch operating
models/listings, retail and wholesale pricing, quantity tiers, fresh attributes
and handling metadata. Permission boundaries must remain visible: catalogue,
measurements, listings, retail pricing, wholesale pricing and fresh management
are distinct grants.

The measure selector must group standard, local/container, package, grouped
count, load and price-portion units. Never display a universal kg equivalent
for Paint, Derica, Mudu or another local container. Show product/branch
calibrations as approximate with their tolerance and source; show exact
conversions separately. Fractional units are numerator/denominator choices
(for example half Paint or quarter basket), not invented `HALF_*` unit codes.
Tenant aliases may be added to platform measures without changing another
business's vocabulary. A branch calibration is selected only on that branch's
listing sell unit and must be labelled as branch-specific.

Price editors use integer minor units through the API but present localized NGN
amounts. Explain precedence before an override is saved: branch sell-unit,
branch listing, sell-unit, then variant base; wholesale quantity bands take
precedence. Tier editors must prevent overlaps and clearly show inclusive band
boundaries, open-ended final bands, MOQ and order multiples.

Variable-weight/count and price-portion screens configure expected quantity,
under/over tolerance and whether actual measurement is required. Stock,
availability and fulfilled quantities belong in the separate Section 12B
inventory workspace described below.
Likewise, handling/temperature metadata is declarative and must not be rendered
as a delivery assignment or ETA.

Every mutation sends a new `Idempotency-Key` and the loaded `expected_version`.
On a stale-version 409, reload and offer comparison; never silently overwrite.
This is a management console only—no Shop storefront or customer discovery is
implemented by Section 12A.

## Section 12B Shop inventory UX handoff

The Vendor console should expose distinct workspaces for balances/movements,
receiving and lots, reservations, transfers, stocktakes, quarantines, inventory
policy, reorder rules, alerts and bulk operations. Keep catalogue edits separate
from operational stock commands even when both refer to one listing.

Balance screens show on-hand, reserved, quarantined, in-transit, backorder and
derived available quantities together with the inventory revision. Movement
history is append-only. Adjustment screens require a controlled reason and
loaded revision; a 409 must reload and compare rather than silently retry.

Receiving screens support draft review before posting, lot/batch and expiry
evidence, and cancellation only when reversal remains safe. Transfer screens
must distinguish draft, approved, in-transit, partially received and resolved
states, and separately capture received, damaged and missing quantities.
Stocktakes require an explicit scope and show expected/count/variance before
posting. Quarantine actions must never look like ordinary stock deletion.

Reservation and availability views distinguish physical allocation from
backorder, display expiry when present, and never label a reservation as an
Order, payment, checkout or delivery promise. Variable-measure fulfilment must
show requested and actual measure, tolerance result, measurer and timestamp.

Alert cards cover low/out of stock, expiring/expired lots, quarantined stock,
backorders, reservation pressure, transfer discrepancies and stocktake
variance. ACK and RESOLVE are separate auditable actions. Bulk screens show dry
run results, row errors, progress, retry/dead-letter status and revision
conflicts; never present a failed or partial import as successful.

Customer storefront UI is outside this service, but a future consumer may use
the internal discovery projection. It must render effective retail/wholesale
price, MOQ/order multiple, fresh and handling metadata, sellability reason and
availability from the same response revision, and refresh on either catalogue
or inventory revision change.
