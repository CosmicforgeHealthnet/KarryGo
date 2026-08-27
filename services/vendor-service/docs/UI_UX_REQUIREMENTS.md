# UI/UX Requirements â€” Vendor Service

Plain-language UI/UX requirements generated from Vendor Service's backend/product implementation, for the future Vendor Flutter app team. No frontend code, no Flutter/Dart in this file â€” only what a screen needs to show, do, and handle.

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

## Screen areas (placeholder â€” owning backend feature in parentheses)

| Area | Owning feature | Status |
|---|---|---|
| Vendor onboarding (business creation, capability selection, verification submission) | `features/vendors`, `capabilities`, `integrations/verification` | **IMPLEMENTED BACKEND â€” Sections 6/7 below** |
| Vendor authentication (login, step-up) | `features/auth` | **IMPLEMENTED â€” see "Section 5" below** |
| Vendor home / dashboard | (cross-feature) | NOT_STARTED |
| Businesses (session-scoped business settings) | `features/vendors` | **IMPLEMENTED BACKEND â€” Section 6 below** |
| Brands | `features/vendors` | **IMPLEMENTED BACKEND â€” Section 6 below** |
| Branches (location, hours, service radius) | `features/vendors` | **IMPLEMENTED BACKEND â€” Section 6 below** |
| Staff (invite, roles, permissions) | `features/staff` | **IMPLEMENTED â€” see "Section 5" below** |
| CRM (customer history, notes) | `features/crm` | **IMPLEMENTED BACKEND â€” Section 8** |
| Restaurant/Food menu management | `features/food`, `catalog` | **IMPLEMENTED BACKEND â€” Section 9** |
| Cloud Kitchen configuration | `features/cloud_kitchen` | **IMPLEMENTED BACKEND â€” Section 10** |
| Pastry/Bakery catalogue and pre-order scheduling | `features/pastry` | **IMPLEMENTED BACKEND â€” Section 11** |
| Shop/Grocery/Market/Wholesale catalogue | `features/shop`, `catalog` | **IMPLEMENTED BACKEND â€” Sections 12A/12B** |
| Inventory (stock levels, low-stock alerts) | `features/shop` | **IMPLEMENTED BACKEND â€” Section 12B** |
| Suppliers | `features/purchasing` | NOT_STARTED |
| Purchasing (purchase orders) | `features/purchasing` | NOT_STARTED |
| Goods receiving | `features/purchasing` | NOT_STARTED |
| Recipes / ingredients | `features/recipes` | NOT_STARTED |
| Kitchen / production board | `features/production` | **IMPLEMENTED BACKEND â€” Sections 10/11/20** |
| Orders (incoming vendor orders, accept/reject) | `features/orders` | NOT_STARTED |
| Substitutions (propose/respond) | `features/substitutions` | NOT_STARTED |
| Delivery (PLATFORM/VENDOR/HYBRID policy, fallback and custody evidence) | `features/delivery`, `fulfilment` | **IMPLEMENTED BACKEND — delivery policy and smart hybrid Prompt 1** |
| Own riders (vendor-owned execution state) | `features/delivery`, `staff` | **IMPLEMENTED BACKEND — execution state only; no fleet-management product** |
| Promotions | `features/promotions` | **IMPLEMENTED BACKEND — management and CheckoutQuote application; no frontend built** |
| Finance (payable summary, settlement status) | `features/finance_projection`, `integrations/wallet` | NOT_STARTED |
| Reports | `features/analytics` | NOT_STARTED |
| Settings | (cross-feature) | NOT_STARTED |
| Support | `integrations/support` | NOT_STARTED |

## Section 5 â€” Authentication, Sessions and Staff IAM (implemented)

Backing routes: see [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md). Reminder: this is Vendor-staff login, completely separate from the (not-yet-existing) logistics pickup/delivery handover OTP/QR â€” never merge these two concepts in any screen or copy.

### AUTH

- **Login identifier screen** â€” Purpose: collect phone or email. User role: anyone with a Vendor staff invite. Actions: submit â†’ `POST /auth/start`. States: loading (submitting), error (invalid phone/email format, from the validation response's `fields`). No permissions gating (public).
- **OTP request state** â€” shown immediately after Start succeeds. Information: masked destination (e.g. "code sent to ...@example.com"), countdown until expiry (`expires_in` seconds). Important rule: phone numbers currently have no real delivery channel (see [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md)) â€” if Start fails with `service_unavailable` for a phone identifier, show a clear "please use your email instead" message, not a generic error.
- **OTP entry screen** â€” 6-digit code input. Actions: submit â†’ `POST /auth/verify` with `challenge_id` + `otp` (+`vendor_id` if a prior attempt returned a vendor-choice response). Success state: navigate into the app with the returned tokens stored securely. Error state: generic "invalid or expired code" (the backend deliberately never distinguishes wrong/expired/locked to prevent enumeration â€” do not try to guess which one happened).
- **Resend state/countdown** â€” a "Resend code" action, disabled until the current code's TTL is close to expiry (avoid spamming `POST /auth/resend`, which is itself rate-limited and will return `429` if hit too often). On resend, the previous code stops working immediately â€” if the user already has the old code memorized/pasted, it will now fail; don't imply both codes are valid.
- **Expiry state** â€” if the OTP TTL fully elapses without a successful verify, show "This code has expired â€” request a new one," with a direct path back to resend/restart, not a bare error.
- **Incorrect OTP state** â€” inline error on the OTP field, decrement an implicit "attempts remaining" is NOT exposed by the API (the backend never reveals attempt counts, to avoid giving an attacker a countdown) â€” just show the generic invalid-code message each time.
- **Attempt-limit/lock state** â€” after enough wrong attempts the backend returns the SAME generic invalid-code error (it does not announce "you are now locked out," by design) â€” the practical UI behavior is: after N consecutive failures (a frontend-side heuristic, e.g. 3-5), proactively suggest "Try requesting a new code instead."
- **Rate-limit state** â€” `429 rate_limited` on start/resend/verify: show "Too many attempts, please wait a moment," no retry button for a few seconds (client-side cooldown; the API doesn't return an exact wait time yet).
- **Vendor-choice state** (multi-business staff only) â€” if verify returns `requires_vendor_choice: true` with `available_vendor_ids`, show a business picker (business names aren't available from this endpoint yet â€” Section 6 will need to supply a name-lookup; render raw IDs or a placeholder label for now) and resubmit verify with `vendor_id` set, reusing the same OTP the user already entered.
- **Login success** â€” store `access_token`/`refresh_token` securely (never in plain logs or analytics). Navigate to the vendor dashboard.
- **Session expiry** â€” when an API call returns `401 unauthorized` with a session-expired message, silently attempt `POST /auth/refresh` once; if that also fails, force a full re-login (clear stored tokens).
- **Forced reauthentication** â€” after logout-all or an admin-driven suspension, the next API call will 401/403; treat this identically to normal session expiry (attempt refresh, then force login) â€” the suspended case additionally surfaces "Your access has been suspended, contact your business owner" from a `403 forbidden`.
- **Suspended-account behavior** â€” a suspended staff member cannot complete login at all (Verify itself refuses) nor refresh an existing session â€” show the same "access suspended, contact your business owner" message in both places.
- **Logout** â€” `POST /auth/logout` with the current refresh token, then clear local tokens regardless of the API response (logout should always feel instant client-side).
- **Step-up authentication prompt** â€” before a high-risk action (see [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md)'s "High-risk operations"), show an explicit "Confirm this action" screen requesting a fresh OTP (reuses the same 6-digit entry UI as login, different copy: "We've sent a confirmation code to your email to confirm this sensitive action").
- **Step-up success/failure/expiry** â€” success: proceed with the original high-risk action, attaching the returned `grant_id`. Failure/expiry: return to the triggering screen with "Confirmation failed or expired, please try again" and restart step-up from scratch (a grant cannot be resumed).

### STAFF

- **Staff list** â€” Purpose: see everyone with access to the vendor business. Permission: `vendor.staff.read` (hide/disable this whole screen area if absent). Data: name/contact aren't returned by this API yet (only `id`, `status`, `version`, `created_at`) â€” Section 6 or a future staff-profile addition will need to supply display names; render the identifier or a placeholder for now, and flag this gap to product rather than inventing fake names. Filters: by status (active/suspended) â€” client-side, the API returns the full list. States: loading/empty ("No staff yet â€” invite your first team member")/error.
- **Staff detail** â€” one staff member's status, version, suspension reason (if any), and their role assignments (`GET /staff/:id/roles`).
- **Staff status** â€” a clear active/suspended badge.
- **Staff roles** â€” list of active assignments, each showing role name and branch scope (or "All branches" when `branch_id` is null).
- **Staff branch scope** â€” shown per-assignment, not per-staff-member (a staff member can hold different roles at different branches).
- **Suspend confirmation** â€” destructive-adjacent action, needs a confirmation dialog ("This will immediately sign this person out of all their active sessions. Continue?"), requires `vendor.staff.suspend`, requires the current `version` (fetch fresh before showing the dialog to minimize staleness), optional reason field.
- **Reactivate action** â€” explicit button, not automatic; same permission; note in the UI that previously-active sessions are NOT restored ("They'll need to sign in again").
- **Permissions display** â€” no single "list my effective permissions" endpoint exists yet; derive what to show/hide from the specific permission each action needs (see `GET /permissions` for the catalogue + descriptions to use as UI copy/tooltips).
- **Restricted-action behavior** â€” any action gated by a permission the current staff member lacks should be hidden or disabled (not shown-then-403'd) wherever the frontend can determine this ahead of time; a `403 forbidden` from the API is the fallback truth, not the primary UX signal.

### ROLES

- **Role list** â€” system role templates (11 defaults: Owner, General Manager, Branch Manager, Kitchen/Pastry, Shop/Inventory, Procurement, CRM/Support, Marketing, Finance, Vendor Rider, Analyst/Auditor) plus this vendor's own custom roles, visually distinguished (`is_system: true/false`). Permission: `vendor.roles.read`.
- **Role detail** â€” name, description, full permission-code list (cross-reference against `GET /permissions` for human-readable descriptions/category grouping â€” group the permission picker by `category` in any create/edit UI).
- **Create/edit role** (custom roles only â€” system role templates are never editable, hide edit affordances entirely when `is_system: true`) â€” permission: `vendor.roles.manage`. Only permissions the ACTING staff member already holds themselves are valid to pick â€” the API will refuse otherwise (403), so ideally pre-filter the picker to the actor's own effective set once that's obtainable, or at minimum surface the API's refusal clearly ("You can only grant permissions you hold yourself").
- **Assign/remove permissions** â€” `PUT /roles/:id/permissions` replaces the WHOLE set (not additive) â€” the UI must submit the complete intended permission list every time, and must pass `expected_version` from the most recently loaded role.
- **Role assignment** (to a staff member) â€” `POST /roles/:id/assign` with `staff_id` + optional `branch_id`. If assigning a role that includes any high-risk permission, the step-up flow triggers first (see AUTH above).
- **High-risk permission warning** â€” when a role being created/edited/assigned includes a permission flagged `is_high_risk: true` in the catalogue (bank-account change, ownership transfer, sensitive data export), show an explicit warning before submission: "This role includes a high-risk permission. You'll be asked to confirm your identity again."
- **Step-up requirement** â€” surfaced automatically by a `403` mentioning step-up is required; the UI's job is to catch this specific case and route into the step-up flow rather than showing a generic error.
- **Version-conflict/error behavior** â€” `409 conflict` on suspend/reactivate/set-permissions means someone else changed the record first; reload the current state and show "This was just updated by someone else â€” please review and try again," never silently retry with the stale version.

### PERMISSIONS

- **Read-only display where user cannot manage** â€” if the acting staff member lacks `vendor.roles.manage`/`vendor.roles.assign`, render role/permission screens as view-only (no edit/assign controls at all).
- **Disabled actions where permission is absent** â€” prefer hiding over disabling-with-no-explanation; where disabling is used, a tooltip naming the missing permission (from the catalogue's human-readable description) is more useful than a bare disabled button.
- **Branch-restricted behavior** â€” a Branch-Manager-type staff member managing staff/roles only ever sees/affects their own branch's scope; there is no UI affordance to "pick any branch" for someone whose own grants don't cover it (the backend refuses it anyway, but showing the option at all is misleading).
- **Owner-only/elevated actions** â€” `vendor.owner.transfer` can never be granted to any role but Owner (the backend refuses this outright at the API level) â€” do not build a UI that lets anyone attempt to assign it to a custom role; if a future "transfer ownership" feature is designed, it is a distinct, not-yet-built capability, not a role-permission checkbox.

## Customer-facing commerce screens (owned by the customer app, backed by Vendor Service)

| Area | Owning feature | Status |
|---|---|---|
| Vendor discovery / storefront browsing | `features/catalog`, `food`, `shop` | NOT_STARTED |
| Cart | `features/orders` | NOT_STARTED |
| Checkout (multi-vendor split, delivery method selection) | `features/orders`, `integrations/wallet` | NOT_STARTED |
| Substitution approval prompt | `features/substitutions` | NOT_STARTED |
| Order tracking (multi-vendor, multi-fulfilment-unit) | `features/fulfilment` | NOT_STARTED |
| **Pickup/delivery handover confirmation UI** | `integrations/handover` | **PENDING_EXTERNAL_DEPENDENCY** â€” the underlying OTP/QR credential does not exist anywhere in the platform yet (see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md)). No screen requirement can be written honestly until the credential shape is real; today's dispatch/hauling proof-of-delivery UI (photo/signature capture) is the closest existing pattern to reuse for evidence capture, but it is not the same thing as a receiver-facing OTP/QR entry screen. |

## Section 7 â€” Onboarding and activation UX specification only

This is a future-client handoff only. No Flutter, mobile, web, dashboard, navigation, component, or state-management implementation was added.

### Onboarding home

- **Purpose/roles/permission:** give Owner, General Manager, permitted Branch Manager, Finance, and Analyst/Auditor staff a truthful status view; backend permission is `vendor.onboarding.read`.
- **Data:** percentage, checklist, satisfied/missing items, blockers, next action, `PENDING_REVIEW`, `ACTION_REQUIRED`, readiness, and activation state.
- **Actions:** submit or request evaluation only with `vendor.onboarding.manage`. â€œRequest activationâ€ never promises or forces activation.
- **States:** skeleton/loading; empty `NOT_STARTED` with a start action; dependency error with safe retry; submitted success; disabled write control when flags/permissions block; confirmation before submission; realtime/poll refresh after verification events.

### Owner / authorized representative

- Show capacity, name/contact fields, ownership percentage when supplied, authorization, version, and read-only Identity verification status.
- Validate required names, email, phone, capacity, and 0â€“100 ownership percentage. Use `expected_version`; on 409 reload before resubmitting.
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
- Brand is per brand when policy enables a Brand requirement. It remains read-only to Vendor users except for submitting evidence/remediation; no â€œapprove brandâ€ action exists.
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

- Every list must have explicit loading/empty/error states â€” no bare blank screen.
- Every destructive or hard-to-reverse action (reject an order, remove a staff member, delete a catalogue item with active orders) needs a confirmation dialog.
- Every write action must be safe to retry (the backend is idempotency-key-protected â€” see [architecture/09_SECURITY_AND_AUTHORITY_MODEL.md](architecture/09_SECURITY_AND_AUTHORITY_MODEL.md)), so the UI should not need to invent its own double-submit protection beyond disabling the button while in flight.
- A vendor with multiple businesses/branches always needs an explicit, always-visible "acting as" indicator â€” never an implicit single-business assumption.

## Section 8A â€” CRM Foundation, Customer 360, Consent and Privacy

Backend only in this section; no frontend source was written. This records the UX the implemented contract supports, and â€” just as importantly â€” the UX it deliberately does not.

### The mental model to design against

A CRM record here is a **commercial relationship with this business**, not a person's profile. Karry Go's customer authority owns the person. That single fact drives most of what follows: there is no contact card, no email address, no phone number, and no way for a vendor to add a customer by typing an identifier.

Designers should expect, and design for, a CRM that is **empty in a new deployment** and stays empty until commerce integration exists. The empty state is the default state, not an edge case.

### Customer list

Per row: relationship reference, subject type (Customer / Guest / External), relationship status, privacy state, last order date, completed order count, spend, branch and capability context, and a marketing-eligibility indicator.

- **There is no name, email or phone to show.** A display label exists in the contract but is always absent today. Design the row around the relationship (recency, value, branch), not around identity. If a label is needed, the row should degrade to the relationship reference without looking broken.
- **No free-text search.** Filters only: subject type, status, privacy state, branch, capability, commercial-projection state, order-count range, spend range, last-order date range, acquisition source. Do not design a search bar that implies name/email/phone lookup â€” the data does not exist to answer it.
- Pagination is limit/offset, max 100 per page, with a stable sort.
- Redacted and merged-away relationships never appear.

### Empty, loading and error states

- **Empty because nothing is integrated** (today's normal): "Customer relationships appear here once orders start flowing." Do not show a "Add your first customer" call to action â€” there is no such action.
- **Empty because filters exclude everything**: distinguish clearly from the above.
- **Forbidden (403)**: the user lacks the CRM permission, or asked about a branch outside their scope. Say so plainly without revealing whether anything exists there.
- **Not found (404)**: covers "no such relationship", "outside your scope" and "redacted" identically, by design. The UI must not try to distinguish them or speculate.

### Customer 360

Sections: relationship summary, commercial summary, branch and capability context, data freshness, consent summary, privacy summary, merge status, and availability markers for future modules.

**The single most important UX rule in this section**: never render "unknown" as zero.

When `commercial.state` is `NOT_AVAILABLE`, show "No order history recorded" â€” not "â‚¦0", not "0 orders", not an empty chart implying no spending. A vendor who reads "â‚¦0 spent" about a loyal customer will make a bad commercial decision based on a number the system never actually knew. Reserve real zeros for genuinely observed zeros.

Show freshness explicitly where the backend provides it: `observed_at`, `stale`, and the projection state. A stale figure should look stale.

All money arrives as integer minor units. Format for display; never treat as a float.

### Branch-scoped view

A branch manager sees only relationships that exist at their authorised branches, and the figures they see cover **only those branches**. The backend enforces this; the UI must not undermine it by mislabelling.

Label branch-scoped figures unambiguously â€” "Spend at Ikeja branch", not "Total spend". A customer who spent â‚¦100,000 at branch A and â‚¦300,000 at branch B must show â‚¦100,000 to the branch A manager, framed as that branch's figure. Never present it as the customer's lifetime value, and never hint that other branches exist.

`commercial.scope` (`BUSINESS` / `BRANCH`) is provided precisely so the label can be driven by data rather than by guesswork.

### Consent

Read-only, per channel (Email, Push, In-app, SMS), showing decision, eligibility, reason, observed time and staleness.

- **The interface must never let staff set, override or "fix" consent.** No toggle, no "mark as opted in", no bulk action. If a vendor believes a decision is wrong, the resolution is with the customer authority, not here.
- Present three distinct states, never two. `UNKNOWN` is "no decision on record â€” cannot contact", visually distinct from `DENY` ("customer declined"). Collapsing them into a single "not opted in" toggle is exactly the design error that leads to someone contacting a customer they had no permission to contact.
- Show the reason, not just the outcome. "Cannot email: no consent on record" is actionable; a grey icon is not.
- When the consent integration is unavailable, say so as a **system** state ("Consent service not connected") rather than attributing it to the customer.
- SMS consent may read `ALLOW` even though the platform has no SMS sender. Do not offer an SMS action on that basis.

### Privacy

Show state and its consequences, with affected actions visibly disabled and explained.

- **Restricted**: visible, not targetable. Marketing controls disabled with a reason.
- **Marketing suppressed**: relationship intact, targeting off. This can co-occur with an `ACTIVE` state and even a stored `ALLOW` â€” the UI must follow `eligible`, not `decision`.
- **Merged**: show that this relationship has been merged and link to the surviving one. All actions disabled.
- **Redacted**: normally unreachable (reads 404). If encountered, show a neutral "This relationship is no longer available" â€” never the previous data, and never an option to restore.
- Nothing in the privacy view may be presented as vendor-editable. There is no override, at any permission level, including Owner.

### Archive and restore

The only vendor-facing writes. Archive is a tidiness action for dormant relationships and must never be presented as deletion, erasure or a privacy remedy â€” the copy should avoid "delete" and "remove" entirely.

Both require the current record version; a stale version returns 409, so the UI should reload and re-present rather than silently retrying. Both are refused on redacted and merged relationships, so those actions should not be offered there at all.

### Future modules

Tags and segments report `AVAILABLE` when Section 8B is enabled; interactions and tasks report `AVAILABLE` when their Section 8C flags are enabled; loyalty reports `AVAILABLE` when `VENDOR_CRM_LOYALTY_ENABLED` is on. Their scope-filtered empty lists then genuinely mean "none visible to this actor." Campaigns, corporate, and intelligence remain `NOT_IMPLEMENTED`. The same applies to category and item affinity.

### Role-based visibility

- Owner / General Manager / CRM & Support: full Section 8A surface.
- Marketing: customer list, Customer 360 and consent eligibility. **No privacy view, no archive/restore, and structurally no access to contact data.**
- Branch Manager: read-only, branch-scoped as described above.
- Analyst / Auditor: read-only including privacy state.
- Finance, Kitchen/Pastry, Shop/Inventory, Procurement, Vendor Rider: no CRM access by default â€” CRM should not appear in their navigation.

---

## Section 8B â€” Tags & Segmentation

Documentation only. No frontend code exists for this section; what follows is the contract a future builder must respect.

### The one rule that shapes every screen here

A segment is an **audience**, not permission to contact it. Nothing in Section 8B sends anything, and no screen built on it may imply otherwise. A "Message this segment" button does not belong in any of these views until campaign execution exists and re-checks consent at send time.

### Tags

**Tag list.** Name, scope (Business / Branch), member count, status. Business and branch tags should be visually distinguishable; a branch tag is that branch's private vocabulary and reads confusingly next to a company-wide one without a marker.

- Member counts are **scope-filtered**. A branch manager's "VIP: 34" is 34 at their branches, not 34 in the company. Label the count with its scope, or a branch manager will quote it as a company figure in a meeting.
- Archived tags are hidden from ordinary selectors. Offer a filter to see them; do not mix them into a tag picker.

**Create / edit.** Name, optional description, scope selector.

- Names are compared case- and whitespace-insensitively. Warn **before** submitting when the entered name collapses onto an existing one ("VIP already exists"), rather than surfacing a 409 after the fact â€” duplicate near-identical tags are the single most common way a CRM taxonomy rots.
- `sys:`, `system:` and `karrygo:` are reserved. Explain why rather than showing a bare rejection: those prefixes are how a future platform-computed trait will be distinguishable from a vendor's own label.
- Scope is **immutable after creation**. Do not render a scope control on the edit screen; moving a tag between branches would silently reinterpret every existing membership.
- A branch-scoped user must not be offered the Business scope option at all.

**Assigning tags.** From a customer's Customer 360, and in bulk from a filtered customer list.

- Bulk operations are **all or nothing**, capped at 500 relationships. Show the cap before the user selects 900 rows, not after they press Apply.
- The response reports `requested` / `applied` / `skipped`. `skipped` means "already tagged", not "failed" â€” word it that way.
- A failed bulk operation reports a single not-found rather than naming which references were bad. That is deliberate (it would otherwise enumerate other tenants' relationships), so the UI must not promise a per-row error report.

**Archiving.** Removes current memberships and keeps the tag's history. Say both things in the confirmation: users reasonably expect archiving to be reversible and it is not, for membership.

### Segments

**Segment list.** Name, type (Static / Dynamic), scope, status, member count, and â€” non-negotiably â€” **freshness**.

A member count without its `evaluation_status` is the defining failure mode of this screen: a stale audience presented as a live one is how a vendor targets the wrong people. Render:

- `CURRENT` â€” membership matches the current definition.
- `STALE` â€” a real, usable audience, but out of date. Show `stale_since` and offer Rebuild.
- `QUEUED` / `RUNNING` â€” a rebuild is in flight; the shown membership is still the previous one.
- `FAILED` â€” the last rebuild failed. **The member count is still real** (it is the last successful audience). Say "Last rebuild failed â€” showing the audience from <time>", never blank the count.
- `NOT_EVALUATED` â€” never built. Show no count at all rather than zero.

When `materialized_definition_version` differs from `definition_version`, the audience was built from an older rule. Say so explicitly.

**Segment builder.** The core screen.

- The builder **fetches `GET /crm/segment-fields` and renders only what it returns.** It must not carry a hardcoded field list: fields appear and disappear with deployment configuration, and a hardcoded list will offer fields that match nobody.
- Field choice drives operator choice, which drives the value editor. Never offer an operator a field does not publish.
- Fields with `available: false` should be shown as unavailable with an explanation ("No order data is connected yet"), not hidden â€” hiding them makes the absence look like a missing feature rather than a missing integration.
- Nested AND / OR / NOT groups, within the published limits (`max_depth`, `max_nodes`, `max_or_children`, `max_in_values`, `max_not_depth`, `max_tag_references`). Enforce these client-side so the user is stopped while building rather than on submit.
- Money values are always an integer count of minor units plus an explicit currency. Present a currency-aware money input; never a float.
- There is **no free-text query box, no "advanced SQL" mode and no raw filter escape hatch** anywhere in this builder. If a user needs something the registry cannot express, the answer is a new registered field, not a text box.

**Preview.** Runs before saving and persists nothing.

- Show `match_count` with `count_state`. `AT_LEAST` means the count budget was reached â€” render it as "at least 50,000", never as an exact figure and never as an estimate.
- The sample carries a relationship reference, an optional safe display label and a projection state. There is no email, phone or address to show, because CRM does not hold any. Do not design a preview table with contact columns.
- `matched_conditions` and `unavailable_conditions` are node paths. Highlight the corresponding builder nodes â€” that is what turns a count into an explanation.
- `warnings` and `unavailable_fields` must be surfaced prominently. "Some conditions use data we have not observed, so they match nobody" is the difference between a user understanding an empty audience and filing a bug.
- Preview needs only the read permission. An analyst who cannot create segments can still explore.

**Static membership.** Add/remove customers explicitly, in bulk, capped at 500.

- Only offered for `STATIC` segments. For a dynamic segment there must be **no** "add member" control at all: the server refuses it, and offering it would promise something that would silently disappear at the next rebuild.

**Exclusions.** Only for `DYNAMIC` segments.

- Present as "never include this customer in this audience", a standing decision that survives rule edits and rebuilds. It is not the same control as removing a static member and should not look like one.
- The effect is immediate, not at the next rebuild â€” reflect the new count straight away.

**Members list.** Paginated, scope-filtered, showing the current generation only.

- Show the membership freshness block alongside the list, for the same reason as the segment list.
- Rows are relationship references. Again: no contact data exists to display.

### Customer 360 additions

Tags and Segments now report `AVAILABLE` in the module map and carry real content.

- Tags: chips, with branch tags marked. A business-scoped viewer sees permitted business and branch classifications. A branch-scoped viewer sees only tags owned by their permitted branches â€” **business-wide and other-branch classifications are not disclosed**, because the label itself reveals how a wider scope classified the customer.
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
- Finance, Kitchen/Pastry, Shop/Inventory, Procurement, Vendor Rider: no access â€” segmentation should not appear in their navigation.

## Section 8C interaction and task UI contract

This is a documentation handoff only; Section 8C contains no frontend implementation.

The existing Customer 360 screen should gain two bounded cards when the corresponding modules report `AVAILABLE`: recent interactions (at most five plus `has_more`) and open tasks (at most five plus a scope-safe exact count). A "view all" action should open the dedicated paginated customer interaction or task endpoint. Never infer hidden totals from timestamps, pagination, or an empty response.

The interaction experience needs a timeline, add form, detail view, and revision history. The form must use the server's closed type/direction/outcome lists; collect a business or permitted branch visibility; limit subject to 200 and plain-text note to 5,000 characters; and warn staff not to enter unnecessary sensitive information. Corrections require a reason and the latest `version`; the UI must reload on a 409 rather than silently overwrite history.

The task experience needs a list or board with status, priority, assignee, branch, due-date, overdue, customer, and date filters. It should show `OPEN`, `IN_PROGRESS`, `COMPLETED`, and `CANCELLED` only; overdue is an indicator, never a fifth state. Start, complete, cancel, edit, and reassign actions send the current `expected_version` and a stable `Idempotency-Key`. The assignee picker must show only server-approved active staff for the current scope; an inactive assignee remains visibly inactive until an authorized user reassigns the task.

Business actors with the appropriate permission can see business-level and permitted branch activity. Branch actors see only their authorized branch activity: no business-only row, other-branch row, hidden count, or hidden latest timestamp. Redacted and merged-source profiles must not expose ordinary activity content or actionable controls. Feature-disabled routes return 404; write-disabled commands return the established unavailable response.

## Section 8D loyalty UI contract

This is a documentation handoff only; Section 8D contains no frontend implementation.

### Programme configuration

One screen per business, behind `vendor.crm.loyalty.manage`. It edits status (`DISABLED` / `ACTIVE` / `PAUSED`), the points display nouns (singular and plural, 40 characters each â€” a vendor may call them "stars" or "beans"), the expiry policy, the redemption minimum and increment, and the manual adjustment limit. Every label in the product must use the vendor's own configured noun, never a hardcoded "points".

The expiry control is a single choice between "never expire" and "expire after N days", because the two are not independently valid: `NEVER` with a window and `FIXED_DAYS` without one are both rejected as `LOYALTY_INVALID_EXPIRY_POLICY`. When the operator switches to a shorter window, the UI must state plainly that **points customers already hold keep their original expiry** and only future points use the new one. Presenting this as "all points now expire after N days" would be a lie the backend deliberately does not implement.

A business that has never configured loyalty is shown the defaults with `configured: false` and `version: 0`. The save action sends that `version` as `expected_version`; a 409 means someone else changed the programme (or a first credit materialised it) and the screen must reload rather than retry.

Status needs honest copy, because the three states gate different things: `DISABLED` stops earning, redemption **and manual adjustment**, but reversal still works; `PAUSED` stops earning and redemption while adjustment and reversal still work; `ACTIVE` allows everything. A "pause" affordance should say that existing balances are preserved â€” nothing is confiscated by pausing.

### Customer 360 loyalty card

Shown when the `loyalty` module reports `AVAILABLE` and the actor holds `vendor.crm.loyalty.read`. It presents the enrolment state first, because `NOT_ENROLLED` and a zero balance are different facts: an unenrolled customer has never held points and must not be rendered as "0 points". Then the available balance, lifetime credited and debited totals, and the next expiry date with how many points fall on it.

`pending_expiry_points` is non-zero when points are past their expiry but the background sweep has not yet recorded it. Those points are already excluded from `available_points`, so the card must never add them back; surface them as an explanation ("N points have expired and are being processed") so a balance that is about to drop is not a surprise.

A merged-away or redacted customer reports `NOT_ENROLLED` with no figures. The card should render an explanatory state, not an empty balance, and should offer no actions.

### Points history

A paginated ledger, newest first, default 50 and maximum 200 per page. Ordering is fixed server-side; do not offer a sort control, because there is none. The only filters are entry type and a created-at range.

Each row shows the type, the signed delta, the resulting balance, and the timestamp. Types need plain-language labels: earned, redeemed, expired, manually added, manually removed, reversed, and transferred in/out from an account merge. A row carrying `reversed_by_entry_ref` must be visibly struck through or badged as reversed and linked to its reversal; a `REVERSAL` row links back through `reverses_entry_ref`. Without that, a history containing a mistake and its correction reads as two unexplained movements.

### Manual adjustment

Behind `vendor.crm.loyalty.adjust`, which is deliberately not granted to Marketing, Branch Manager, Analyst or Finance â€” the action should not appear in their navigation at all. The form collects a direction (credit or debit), a **positive** whole number of points, and a mandatory reason of up to 500 plain-text characters.

The points field must reject decimals and negatives in the client as well as the server: direction carries the sign, and a negative amount is a validation failure, not a clever way to debit. The client should show the programme's `manual_adjustment_limit` before submission rather than letting the operator discover it as a 409 (`LOYALTY_ADJUSTMENT_LIMIT_EXCEEDED`).

The reason field needs copy explaining where it goes: it is written to the audit log and is visible to auditors, and it is deliberately **not** carried on the ledger row or any downstream event. Staff should be told not to enter customer contact details or case narratives there.

A manual credit may enrol a customer who has no account. A manual debit may not, and may not exceed the available balance â€” both surface as errors and the form should not offer a debit at all on a `NOT_ENROLLED` customer.

### Reversal

Behind `vendor.crm.loyalty.reverse`. Offered only on `EARN`, `REDEEM`, `ADJUSTMENT_CREDIT` and `ADJUSTMENT_DEBIT` rows that are not already reversed; every other row must not show the action, because the server refuses it (`LOYALTY_ENTRY_NOT_REVERSIBLE`).

Reversal is always full â€” there is no partial-amount field â€” and requires a mandatory reason handled exactly like the adjustment reason. The confirmation copy should state the exact points and resulting balance.

Two refusals need specific, non-alarming copy rather than a generic error. `LOYALTY_REVERSAL_POINTS_ALREADY_CONSUMED` means the customer has already spent or expired those particular points, so the credit cannot be cleanly taken back; the recovery is a manual debit with an explanation, and the UI should offer that path. `LOYALTY_ENTRY_ALREADY_REVERSED` means someone else got there first, and the row should refresh to show the existing reversal.

When a debit reversal restores points whose expiry has since passed, the history will show the reversal immediately followed by an expiry of the same amount. That is correct and must be explained in the UI rather than looking like the points were given and then taken away by a bug.

### Permissions and states

- Owner / General Manager: full loyalty surface.
- CRM & Support: read, adjust and reverse â€” but **not** programme configuration.
- Marketing / Branch Manager / Analyst & Auditor / Finance: read only. Branch Manager reads stay branch-scoped, and a customer outside their branches is simply absent, not denied.
- Kitchen/Pastry, Shop/Inventory, Procurement, Vendor Rider: no access â€” loyalty should not appear in their navigation.

### What the UI must never imply

Points are not money. No screen may show a currency symbol beside a points figure, offer a cash value, suggest withdrawal or transfer to a wallet, or place loyalty alongside wallet balances in a way that reads as a second account. There is no rewards catalogue, no tier ladder, no voucher and no cashback in Section 8D, and the UI should not scaffold placeholders for them.

## Section 8E campaign intelligence UI contract

This is a documentation handoff only; Section 8E contains no frontend implementation.

The single most important thing this UI must convey is that **preparing a campaign is not sending one, and a prepared audience is not permission to contact anybody**. Every screen below has to survive a user who assumes otherwise. There is no send button to build, because there is no send endpoint; a "Ready to send" affordance anywhere in this surface would be describing a capability the backend deliberately does not have.

### Campaign list

Behind `vendor.crm.campaigns.read`. Filters are status, scope, branch, objective, channel and a created-date range, plus a name search; ordering is fixed at most-recently-updated first and is not user-selectable. The five statuses are `DRAFT`, `PREPARING`, `PREPARED`, `PREPARATION_FAILED` and `ARCHIVED`. There is no "scheduled", "sending" or "sent" state to render, and inventing a badge for one would misdescribe what the platform can do today.

`PREPARATION_FAILED` deserves careful copy: it can legitimately coexist with a usable prepared audience, because a failed rebuild never empties the one that already worked. The honest phrasing is "the latest rebuild failed; the previous audience is still available", not "this campaign is broken".

A branch-scoped user sees only their own branch's campaigns. A business-wide campaign is not shown as locked or greyed out â€” it is absent, because its name and audience size describe the whole business.

### Create and edit

Behind `vendor.crm.campaigns.manage`. The form collects a name, an objective, a scope, one to four channels, a source audience segment, up to ten exclusion segments, an optional frequency policy, an optional quiet-hours policy, a service-recovery toggle and an optional experiment.

Scope must be chosen at creation and then locked, with copy explaining why: changing it would reinterpret every audience already prepared from the campaign. A branch campaign's segment picker must offer only that branch's segments; offering a business-wide one and letting the server refuse it would be a worse experience than not offering it.

The objective list is `GENERAL_MARKETING`, `RETENTION`, `REACTIVATION`, `LOYALTY` and `ANNOUNCEMENT`. All five are marketing objectives and all five require marketing consent â€” including `ANNOUNCEMENT`. The UI must not imply that an announcement bypasses consent, because it does not.

Editing a prepared campaign returns it to `DRAFT`. Warn before saving: the prepared audience stays available as history, but the new definition has never been prepared and the user will need to prepare again.

Service-recovery suppression defaults on. Turning it off is a deliberate act and should read as one â€” the plain-language consequence is "this campaign may reach customers with an unresolved complaint open".

### Experiment configuration

Allocations are integer basis points and must total exactly 10000. The editor should show percentages for humans while sending basis points, refuse to save at anything other than 10000, and say by how much the total is off. At most one control cohort, capped at 50%; one to five treatment variants; unique keys.

Explain the holdout honestly: a control member is selected into the audience and satisfies every safety rule, and is simply not sent the campaign when Section 8F exists. It is not an exemption from consent or privacy.

### Audience preview

Behind `vendor.crm.campaigns.preview`. Preview writes nothing and changes no state, so it can be re-run freely.

Present the funnel in the order the engine applies it, with the reconciliation visible: source count, minus segment exclusions, minus privacy suppressions, minus service-recovery suppressions, equals the target count; and control plus treatment equals target. If those do not add up on screen, the screen is wrong.

Per channel, show how many targets were eligible and how many were not, with the ineligibility reasons broken out. When no consent authority is integrated the honest answer is "nobody is currently contactable on any channel, because consent data is unavailable" â€” not an empty state that reads like a data-loading failure.

A stale source segment must be surfaced prominently: the preview counts nobody and the fix is to rebuild the segment, not to retry the preview. The `source_segment_evaluation_status` is what tells you.

The frequency policy always reports `NOT_AVAILABLE`. The copy should say the cap is stored and will be enforced at send time, and that it cannot be evaluated yet because nothing has been delivered â€” never "0 of 3 contacts used".

Quiet hours may be tested against a specific instant the user picks. That is a "what if we sent at this time" question and nothing more; it schedules nothing, and the UI must not present it as picking a send time.

The sample is bounded and carries a relationship reference, a safe display label and a reason code. There is no email address, phone number or device token to show, and no column should be reserved for one.

### Prepare and snapshots

Behind `vendor.crm.campaigns.prepare`. Preparation is asynchronous: the action returns a `BUILDING` snapshot and the UI polls until it reports `CURRENT` or `FAILED`. Pressing the button twice returns the same run â€” the UI should reflect that rather than showing two.

The snapshot summary presents the same reconciled funnel as the preview plus the frozen source segment version and generation, and must display the three send-time recheck obligations rather than hiding them as implementation detail. A snapshot is evidence of a past decision; its counts never change afterwards.

`FAILED` with `dead_lettered: true` means the run gave up and will not retry. `CRM_CAMPAIGN_AUDIENCE_NOT_CURRENT` means the source segment moved on and the fix is to prepare again, not to wait.

### Target list

Behind `vendor.crm.campaigns.targets.read`, which is a separate and larger disclosure than reading the campaign â€” the UI should treat entering this view as a deliberate step rather than a tab that opens by default.

Rows carry an opaque relationship reference, an optional safe display label, a cohort, the current actionability flag and the per-channel snapshot consent evidence. There is no contact detail anywhere in the response and none may be displayed, inferred or joined in from elsewhere on this screen.

`currently_actionable: false` needs its reason shown (`PRIVACY_REDACTED`, `PRIVACY_RESTRICTED`, `PRIVACY_MARKETING_SUPPRESSED`, `IDENTITY_MERGED`) or, when `service_recovery_active` is true, an explanation that the customer has an open complaint. Crucially, `currently_actionable: true` still does not mean "will be sent" â€” every channel row carries `send_time_recheck_required` and the UI must say so on the page, not only in a tooltip.

### Flags and failure modes

- `VENDOR_CRM_CAMPAIGNS_ENABLED` off: routes are not registered and requests return 404. That means "campaigns are not turned on for this deployment", not "campaign not found".
- `VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED` off: definitions, previews and already-prepared snapshots all keep working; only new preparations are refused with 503. An existing audience must never render as empty because preparation is frozen.
- `VENDOR_CRM_WRITES_ENABLED` off, or the global commerce kill switch on: reads and previews work, commands return 503.
- A branch user reaching a campaign outside their scope receives 404, not 403. The UI should render that as "not found" and must not disclose that the campaign exists elsewhere.

There is no rewards catalogue in Section 8E. Section 8F now defines backend contracts for campaign execution; this is documentation only and no frontend was implemented.

## Section 8F campaign execution UI contract

- A run composer offers Send now or Schedule, an RFC3339 schedule time, an approved template key or bounded plain-text title/body, and a final audience/version confirmation. Every create command keeps one Idempotency-Key and sends the displayed campaign `expected_version`.
- Run pages display `SCHEDULED`, `QUEUED`, `RUNNING`, `PAUSED`, `COMPLETED`, `COMPLETED_WITH_FAILURES`, `CANCELLED`, or `FAILED`, plus total, handed-off, delivered, suppressed, and failed counts. `HANDED_OFF` must be labelled â€œaccepted by Notificationâ€, never â€œdeliveredâ€.
- Pause, resume, and cancel use the run's current `version`. Cancellation cannot recall a delivery already handed off; the confirmation copy must say so.
- Delivery views paginate and show channel, cohort/variant, state, retry count, safe suppression reason, and transition history. They never show an email address, phone, token, or destination.
- Surface quiet-hours deferrals, frequency-cap suppression, current-consent/privacy blocks, control-group holdouts, channel-unavailable suppression, and retry/final failures as distinct safe reasons.
- SMS must not be offered while Notification Service has no production SMS channel.

## Section 8G corporate customer UI contract

- A corporate account page shows the organization, its `CORP-` reference, lifecycle status, scope (business-wide or a named branch set), account manager, and the actionable summary counts. Counts exclude archived children and privacy-suppressed contacts, so they describe what an operator can act on rather than everything the tables hold â€” label them accordingly.
- Lifecycle actions are `Activate`, `Put on hold`, `Deactivate` and `Archive`. Each sends the displayed `version` as `expected_version` with one Idempotency-Key. Archiving must warn that it cancels the account's open tasks and closes the account to new activity; nothing is deleted.
- **Contacts are people the vendor already knows.** The add-contact flow picks an existing customer relationship; it must never offer fields for an email address, phone number or postal address, because Section 8G stores none and returns none. Show the display label, and when it is absent show the contact as non-actionable rather than inventing a placeholder name.
- A contact whose relationship was redacted stays visible for continuity with `privacy_suppressed: true` and `actionable: false`. It must be rendered as unavailable rather than removed, and every action on it must be disabled â€” `actionable` is the only signal to branch on.
- Promoting a primary contact silently demotes the previous one server-side. Say so in the confirmation, so the change does not look like a bug.
- **Cost centres are labels.** Do not present a budget, limit, currency, balance or spend column, and do not imply that a cost centre authorises anything. The same applies to membership roles: `APPROVER` describes how a person participates, and approves nothing.
- Locations are the customer organization's own sites and are unrelated to the vendor's branches. Keep the two vocabularies visually distinct; a user who confuses them will scope an account wrongly.
- Archive, not delete, is the only removal on contacts, memberships, locations and cost centres. Copy must say the record is kept so past activity stays attributable.
- The account timeline and task list are the Section 8C surfaces with a corporate subject. Reuse the same components, and expect the same vocabularies, bounds and immutable-correction behaviour: a correction supersedes the original rather than editing it.
- Corporate activity needs the Section 8C permission **and** corporate read. A user who can log interactions but lacks corporate read must not see the corporate section at all, rather than seeing it and failing on click.

There is no corporate ordering, invoicing, credit, budget, contract or approval UI in Section 8G, because there is no backend for one. Do not design a placeholder for it.

## Section 8H CRM intelligence and export UI contract

- **Lead every intelligence view with availability.** Show `profile_count`, `available_count` and `unavailable_count` before any histogram. A business with no order data must read as "1,200 relationships, 0 scored, 1,200 unavailable â€” no commercial data observed", never as "0 customers". Surface the `unavailable_reasons` breakdown; it is what makes an empty dashboard actionable instead of mysterious.
- **Never render a score as a percentage or a probability.** Churn risk is an integer rank in 0-100 with a band. Copy must say "risk score" or "risk band", never "% likely to churn", "chance of leaving" or "prediction". RFM dimensions are quintiles of *this scope's scored cohort* and must be labelled as relative â€” a 5 means "top fifth of the customers we could score here", not "excellent".
- **A score is not permission, and the UI must not imply it is.** A high-risk list is a follow-up queue, not an audience. Any path from an intelligence view to a campaign must go through the ordinary segment and campaign flows, where consent, privacy, frequency and quiet hours are enforced. Never offer "message these customers" directly from a score.
- **Distinguish stale from absent.** A `STALE` score is real and readable, shown with its `source_as_of`; an unavailable one has no numbers at all. Do not grey them out identically, and never show a stale score without its as-of instant.
- **Show the model identity.** Every score view carries `model_key`, `model_version` and `source_as_of`. When a rebuild is running, keep showing the last good figures marked stale rather than blanking the page.
- **Rebuild is an operational action.** Confirm it, show that a run already in flight is returned rather than duplicated, and make clear it is scope-wide work rather than a refresh of the row in front of the user.
- **There is no way to edit a score, and there must be no UI that suggests otherwise** â€” no "mark as VIP", no "override risk", no manual RFM entry. Intelligence is derived.
- **Branch users see branch intelligence only.** Never show a business-wide figure to a branch-scoped operator, and when a user holds several branches make them choose one rather than silently picking the first.
- **Export request flow.** Dataset from a fixed list, format CSV or JSONL, an optional typed filter, and a row limit with the ceiling stated up front. Show the exact field list the dataset will emit before the user commits. Never offer a column picker, a sort, a search box or anything resembling a query.
- **Export progress and expiry.** Poll the job; show `QUEUED`/`RUNNING`/`READY`/`FAILED`/`CANCELLED`/`EXPIRED`, the row count, the checksum and â€” prominently â€” the expiry. An expired export must be visibly gone, not a button that fails on click.
- **Download requires its own step-up.** Explain why before prompting, and make clear the resulting link is short-lived and single-use. Never store, bookmark, email or re-display a download token; if the user needs the file again they ask again, and that is by design.
- **Failed and cancelled exports must show the reason code**, particularly `CRM_EXPORT_TOO_LARGE` (narrow the filter) and `CRM_EXPORT_STORAGE_UNAVAILABLE` (retry later, not a data problem).

There is no AI, recommendation, next-best-action or automated-campaign UI in Section 8H, because there is no backend for one. Do not design a placeholder for it.

## Section 8I â€” final UI/UX contract note

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

## Section 8 completion â€” loyalty tiers

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
  off. Confirm it explicitly â€” every customer's status disappears.

### Customer 360 â€” tier standing

Rendered inside the loyalty block, not as its own card. Three states, and they
must look different:

- `CURRENT` â€” show the tier name, and the progress to the next rung when
  `next_tier_key` is present. On the top rung there is no next tier; do not
  render an empty or 100% progress bar.
- `NO_TIER` â€” a ladder exists and this customer has not reached its first rung.
  Show what the first rung is and what it takes. Do **not** render this as a
  greyed-out bottom tier; they are not in the bottom tier, they are not on the
  ladder.
- `NOT_AVAILABLE` â€” this business has configured no ladder, or the relationship's
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

## Section 9A â€” food catalogue management (backend-derived requirements)

Backend only. Nothing below was implemented; it records what the Section 9A API
obliges a vendor console to do, and â€” more usefully â€” what it forbids it from
pretending.

### Menu management

A menu belongs to **one branch and one food capability** and neither can be
changed after creation. The create form must therefore ask for both up front,
and the branch picker must offer only branches the signed-in user's scope
reaches and that hold the chosen capability â€” a branch without it is refused
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
- **Archive is terminal.** The confirmation must say so plainly â€” an archived
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
  arithmetic. The response reports `currency` â€” display that, do not assume one.
  Zero is a valid price. Negative and out-of-range values are refused.
- **Status** is DRAFT / ACTIVE / PAUSED in the editor. Publishing an item
  (setting ACTIVE) needs the publish permission, so a Kitchen user's editor must
  show the field as read-only rather than letting them submit and be refused.
  ARCHIVED is not settable here â€” it has its own action.
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
`min_select > 0` â€” show it as a derived label, never as an independent checkbox
the user can set to disagree with the numbers. `max_select` must be at least
`min_select` and at least 1; both are capped at 50.

**Modifier groups are shared across the whole business.** The editor must say so
before the user saves: renaming "Add extras" or repricing "Extra cheese" changes
every item that uses it, across every branch. A branch-scoped user gets 403 on
create/edit/archive here and should see a read-only view with an explanation,
not a form that fails on submit. Attaching and detaching a group on their own
branch's item remains available to them.

Detaching a group from an item removes only the link; the group survives for
every other item. Archiving a group detaches it everywhere â€” the confirmation
should say how many items are affected.

### Concurrency conflict handling

Every mutation of an existing row carries `expected_version`, and a stale value
returns 409 with `FOOD_STALE_VERSION`. The console must not silently retry with
a refreshed version â€” that is how one editor's change disappears. Show the
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
now â€” schedules, sold-out state, effective price at a branch â€” is Section 9B and
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
This is a management console onlyâ€”no Shop storefront or customer discovery is
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

## Section 13 â€” Catalogue, Taxonomy and Search (DOCUMENTATION ONLY)

**No UI was built.** This records what a future platform operator console, a
future Vendor app screen and a future customer discovery surface will need. It
is a requirements handoff, not a description of anything that exists.

Three distinct audiences appear below, and conflating them would be a security
mistake rather than a design one:

- **Platform operator** â€” administers the canonical taxonomy and search for the
  whole marketplace. Not a vendor. Reaches the backend through an internal,
  service-authenticated surface, never with a vendor login.
- **Vendor staff** â€” classifies their own catalogue. Sees their own scope only.
- **Customer** â€” searches. Sees results.

---

### A. Platform taxonomy console (operator, internal)

> **Authority warning for whoever builds this.** Every screen in section A is
> backed by a root-mounted HMAC-v2 internal API. There is no vendor bearer token
> that reaches any of it, and no vendor role â€” not even Owner â€” grants the
> capability. If a design ever proposes putting these screens inside the Vendor
> app behind a permission toggle, that is the wrong shape: it would put the
> authority to reclassify the whole marketplace behind a tenant's login.

**Namespace selector.** FOOD and SHOP, always explicit, always visible. Never a
remembered default that could leave an operator editing the wrong vocabulary
without noticing. Every subsequent screen is scoped by this choice, and the
chosen namespace should remain visible in the header, not just in a dropdown
that has scrolled away.

**Version list.** Version number, status (`DRAFT` / `PUBLISHED` / `RETIRED`),
who created it, when it was published or retired. Exactly one version is
`PUBLISHED`; make that unmistakable rather than something the operator infers by
scanning a column.

**Draft inspection and comparison.** Side-by-side diff of two versions of the
same namespace: facets and nodes added, removed, renamed, re-parented,
archived, restored. Diffing across namespaces is refused by the backend, so the
target selector must only offer versions of the same namespace.

**Facet and node management.** Tree for hierarchical facets, flat list for flat
ones. Create, update, archive, restore. Archiving a facet or node with active
dependants is refused â€” surface that as "3 active nodes depend on this facet"
rather than as a generic error, because the operator's next question is always
"which ones".

**Vocabulary management.** Aliases, synonyms and related terms per node. This is
where Nigerian search actually gets its quality, so it deserves a real editing
surface, not a JSON textarea: `jolof`, `shwarma`, `moin moin`, `gari`, `paint`
are the difference between a customer finding food and giving up.

**Publication.** A two-step flow. Validate first, and show the structural
result. Then publish, with an explicit confirmation that names the consequence:
publishing retires the current published version and changes classification for
every vendor on the platform. Publication history is append-only and should be
readable from the same screen.

**Governance cases.** Queue by status and priority, with the subject, reason,
assignee and resolution. Transitions are revision-guarded, so a stale tab must
show a conflict and reload rather than silently overwriting another reviewer's
decision.

**Seed status.** Whether the namespace is seeded, the latest run's fingerprint
and counts, and the published version. **Read-only.** There is no "re-run seed"
button in the backend and there must not be one in the UI: re-seeding rewrites
the vocabulary every vendor is classified against.

---

### B. Vendor taxonomy screens (vendor staff, Vendor app)

**Classification of a catalogue item.** For one Food item or Shop product/
variant: its current canonical mappings, semantic tags and dietary claims, all
in one view. The vendor picks the resource from their own catalogue â€” never by
typing an identifier, because an identifier field invites guessing and the
backend answers "not found" for anything outside their scope anyway.

**Mapping management.** Choose a facet, then choose nodes within it. Saving
replaces every mapping in that facet at once, so the UI must present it as
"these are the categories for this item" rather than as incremental add/remove
that hides the replacement semantics.

Requires `vendor.taxonomy.mapping.manage`. A staff member with only
`vendor.taxonomy.read` sees the same screen read-only â€” disabled controls with a
reason, not a hidden section, so the vendor can see what they would need.

**Dietary claim state.** Per claim: the declared status, the verification policy
it requires (`DECLARATION`, `EVIDENCE_REQUIRED`, `PLATFORM_VERIFIED`), the risk
level, evidence reference if any, and verification/expiry timestamps.

Show the policy **before** the vendor declares, so "this claim will require
evidence" is knowledge they have in advance rather than a rejection afterwards.
Never present a declared claim as verified. An expiring claim needs a warning
before it lapses, because a lapsed dietary claim on food is a real safety
matter, not a data-quality one.

**Branch scope.** A branch-scoped staff member classifies only their own
branch's items. Business-wide resources (Shop products and variants) need a
business-wide grant; show that clearly instead of returning a bare error.

---

### C. Search operations screens (operator, internal)

**Ranking profile inspection.** Per namespace: every weight and every bound.
The commercial and promotional weights must be shown together with the non-text
ceiling and the exact-name weight, because the relationship between them is the
guarantee â€” a UI that shows a promotion slider without showing the ceiling it
cannot cross is hiding the only number that matters.

Editing must show the bound being approached and refuse with a clear reason,
not a validation code, when it would be crossed.

**Projection generation status.** Per namespace: the active generation, any
building/ready/failed generations, document and processed counts, source
revisions, failure code and reason, worker id and lease expiry.

**Rebuild progress.** Processed against expected, with the reassurance stated
plainly: the active generation keeps serving throughout. An operator watching a
rebuild needs to know discovery is not down.

**Failure and retry.** A failed generation shows its failure code and reason and
offers retry. Dead-lettered jobs are listed and individually replayable. The
active generation is never at risk from either.

**Health.** Food and Shop reported separately, with cache availability shown as
its own signal and labelled as affecting latency only. Do not render a single
composite "search health" light: during an incident that is precisely the
display that sends someone to fail over a healthy component.

---

### D. Customer discovery UX (customer app)

**Search input.** Suggestions appear as the customer types, capped at ten, drawn
from canonical vocabulary and real listing names. No personalization and no
history-based ranking â€” nothing to disclose, and nothing to explain.

**Food filters.** Provider type, cuisine, dish type, meal period, dietary claim,
availability, price range.

**Shop filters.** Category, brand, measurement, sell unit, pack size, fresh
attributes, temperature zone, availability, retail/wholesale, price range.

**Facets.** Counts alongside filter values, bounded and never an unbounded
vocabulary dump.

**Food and Shop are separate result surfaces.** A customer searching food should
not receive a sack of raw rice. If a single search box serves both, results must
be presented as clearly labelled separate sections, never interleaved into one
ranked list.

**Availability and serviceability.** Show what the projection observed, and show
`UNKNOWN` as unknown. Never render "we do not know" as "available" â€” the
customer is making a decision on it.

**A search result is not a reservation.** Availability may change between search
and checkout. Checkout revalidates against authoritative inventory, so the
checkout flow must handle "this is no longer available" gracefully as an
expected outcome rather than an error state.

**Sponsored results** are labelled. Always.

**Empty states.** Distinguish "no results for this query" from "no results with
these filters" (offer to relax them) and from "search is temporarily
unavailable" (offer retry). These are three different customer situations with
three different next actions, and collapsing them into one message wastes the
only chance to help.


## Section 14 â€” Generic Inventory UX handoff (DOCUMENTATION ONLY)

No frontend was built in Section 14. This section records what the backend can
support so a future Vendor app screen is designed against real capability
rather than against an assumption. Layout, spacing and visual design are not
specified here.

Every screen below is branch-scoped. A staff member with exact-branch authority
sees only their branch; the backend refuses a sibling branch rather than
returning an empty list, so the UI must treat that refusal as "not yours", not
as "nothing here".

### A. Inventory dashboard (branch overview)

- **Backend data**: open alert counts by type, branch inventory revision, and
  balance summaries per item.
- **APIs**: `GET /inventory/branches/{branchId}/alerts`,
  `GET /inventory/branches/{branchId}/balances`,
  `GET /inventory/branches/{branchId}/revision`.
- **Statuses**: alert `OPEN` / `ACKNOWLEDGED` / `RESOLVED`; alert types
  `LOW_STOCK`, `OUT_OF_STOCK`, `EXPIRING_SOON`, `EXPIRED`, `BACKORDER`,
  `BALANCE_MISMATCH`, `TRANSFER_EXCEPTION`.
- **Supported actions**: acknowledge an alert, resolve an alert.
- **Not supported**: reordering from an alert. Section 15 owns purchasing.

### B. Branch stock / inventory items / balances

- **Backend data**: per item, on-hand, reserved, quarantined, in-transit,
  incoming, backorder, available and expired quantities, plus a revision.
- **APIs**: `GET /inventory/branches/{branchId}/balances`,
  `GET /inventory/items/{inventoryItemId}/availability`,
  `GET /inventory/items/{inventoryItemId}/sellability`.
- **Design note**: available quantity is a derived figure that already excludes
  quarantined and expired stock and can never be negative. Do not recompute it
  in the client from on-hand minus reserved; that arithmetic will disagree with
  the server whenever lots are involved.
- **Not supported**: creating an inventory item directly. Item identity arrives
  from the owning catalogue â€” today, a Shop stock basis.

### C. Lots and expiry

- **Backend data**: lot quantities, received and expiry dates, lot status, and
  the expired quantity rolled into each balance.
- **APIs**: exposed through balances and availability; allocation ordering is
  FIFO or FEFO per item policy.
- **Design note**: expired stock still physically exists. Show it as present
  but unsellable rather than hiding it, because the operator has to dispose of
  it.
- **Not supported**: editing a lot's expiry after receipt.

### D. Goods receiving

- **Backend data**: draft and posted receipts with their lines and lots.
- **APIs**: the Section 12B Shop receipt routes
  (`POST /shop/inventory/receipts`, `.../post`, `.../cancel`). Section 14's
  generic receipt operations are service-layer only and have no public generic
  route.
- **Statuses**: `DRAFT`, `POSTED`, `CANCELLED`.
- **Design note**: a draft receipt has no stock effect. Only posting moves
  stock, and posting twice does not receive twice.
- **Not supported**: receiving against a purchase order. Section 15.

### E. Stock movements (ledger)

- **Backend data**: immutable movement rows, newest first, filterable by
  movement type and source reference.
- **API**: `GET /inventory/branches/{branchId}/movements`.
- **Design note**: this is an audit trail. There is no edit or delete affordance
  to design, because the database refuses both. A correction is a new movement.

### F. Adjustments, quarantine and disposal

- **Backend data**: quarantine cases with quantity, reason and status.
- **APIs**: `POST /inventory/quarantines`,
  `POST /inventory/quarantines/{quarantineId}/release`,
  `POST /inventory/quarantines/{quarantineId}/dispose`.
- **Statuses**: `OPEN`, `RELEASED`, `DISPOSED`.
- **Design note**: quarantine removes stock from sellability without destroying
  it; disposal destroys it and writes controlled-loss evidence. These are
  different operator intents and should not share one button.

### G. Transfers

- **Backend data**: transfer header, lines, and in-transit accounting.
- **APIs**: `POST /inventory/transfers` and the `approve`, `dispatch`,
  `receive`, `cancel` transitions.
- **Statuses**: `DRAFT`, `APPROVED`, `DISPATCHED`, `RECEIVED`, `CANCELLED`.
- **Design note**: the destination item must share the source's stock identity.
  A destination picker should offer only compatible items rather than letting
  the operator choose one and then surfacing a conflict.
- **Supported on receive**: received, damaged and missing quantities per line.

### H. Stocktakes

- **Backend data**: expected-quantity snapshot per line, counted quantity, and
  the resulting variance.
- **APIs**: `POST /inventory/stocktakes`, `PUT .../items/{id}/count`,
  `POST .../post`, `POST .../cancel`.
- **Statuses**: `OPEN`, `POSTED`, `CANCELLED`.
- **Design note**: posting a stocktake whose snapshot has gone stale is refused.
  The UI needs a real path for that outcome â€” re-open and recount â€” not a
  generic error toast, because it will happen in any busy branch.

### I. Reorder rules

- **Backend data**: minimum and target quantities, expiring-soon window,
  opaque supplier reference, rule status.
- **API**: `PUT /inventory/items/{inventoryItemId}/reorder-rule`.
- **Not supported**: supplier selection, price, lead time or purchase-order
  creation. The supplier reference is an opaque string until Section 15.

### J. Reservation visibility

- **Backend data**: reservation status, source, expiry and per-line
  allocations.
- **APIs**: `POST /inventory/reservations`,
  `GET /inventory/reservations/{reservationId}`, and the `confirm`, `commit`,
  `release`, `cancel` transitions.
- **Statuses**: `HELD`, `CONFIRMED`, `COMMITTED`, `RELEASED`, `EXPIRED`,
  `CANCELLED`.
- **Design note**: reservations mostly originate from other services rather
  than from a vendor staff member. Treat this as an operational read surface
  first; manual release is an exception path, not the primary action.

### Permissions

Inventory read and manage permissions are branch-scoped and follow the Section
5 IAM model already documented above. High-risk operations retain the existing
step-up requirement; ordinary reads do not require step-up.

### What this surface must not claim

It must not present purchasing, supplier management, recipe or production
information, or any order, payment or delivery state. Those belong to Sections
15, 16 and 17 and do not exist behind these APIs.


## Section 15 - Purchasing UX handoff (DOCUMENTATION ONLY)

No frontend was built. This records what the backend supports so a future
screen is designed against real capability.

Every screen is branch-scoped. Asking for a branch outside the operator's scope
is refused, not silently narrowed, so the UI must treat that as "not yours".

### A. Purchasing dashboard
- **Data**: purchase-order counts by status; overdue, materially overdue,
  expected today and expected soon; receiving desk counts; discrepancy counts
  with the age of the oldest open one; supplier rollups.
- **API**: `GET /purchasing/dashboard`.
- **Permission**: `vendor.purchasing.dashboard.read`.

### B. Suppliers and supplier products
- **Data**: supplier record, contacts, terms, categories; supplier products
  with purchase unit, exact conversion factor and mapped Inventory item.
- **Design note**: conversion is always exact. There is no approximate option
  to offer, and offering one would be a lie about received quantities.

### C. Purchase orders
- **Data**: header, lines, status, expected delivery, approval history.
- **Statuses**: DRAFT, SUBMITTED, APPROVED, ORDERED, PARTIALLY_RECEIVED,
  RECEIVED, CLOSED, REJECTED, CANCELLED.
- **Design note**: the approval policy is snapshotted at submission. Show the
  approvals the order actually requires, not today's policy.

### D. Approval queue
- **Data**: submitted orders awaiting a decision, with the required permission
  and approval count from the snapshot.
- **Design note**: some rules require step-up. The UI needs a real step-up
  path, not a generic failure.

### E. Goods receiving
- **Data**: receipt header, per-line delivered/accepted/damaged/rejected/short,
  lot and expiry, wrong-item and quality flags, supplier documents.
- **Statuses**: DRAFT, INSPECTED, POSTED, REVERSED, CANCELLED.
- **Design note**: the inspection screen is where `delivered = accepted +
  damaged + rejected` must be enforced visibly. Only accepted stock will reach
  Inventory, and the operator should see that before confirming.

### F. Discrepancies
- **Data**: type, status, quantity, cost variance amounts, age, linked
  supplier/order/receipt.
- **Design note**: cost-variance approval approves the variance. It does not
  change the approved order, and the UI must not imply that it does.

### G. Outstanding deliveries and alerts
- **APIs**: `GET /purchasing/outstanding-deliveries?window=OVERDUE|UPCOMING`,
  `GET /purchasing/alerts`, acknowledge and resolve.
- **Design note**: alerts resolve themselves when the order is received. A
  manual resolve is for alerts the operator has handled another way.

### H. Supplier performance
- **API**: `GET /purchasing/suppliers/{supplierId}/performance`.
- **Design note, and the important one**: every rate carries `metric_status`.
  When it is `INSUFFICIENT_HISTORY` the value is null and the UI must say so -
  "no deliveries yet", not "100%". Rendering a null as a full bar would invent
  a supplier's reputation.

### Not supported
No supplier payment, remittance or settlement screen. Purchasing records cost;
paying suppliers is a separate authority that does not exist yet.

<!-- SECTION 16B RECIPES -->
## Section 16B future UI requirements

Future Vendor interfaces require:

- Recipe list/detail
- typed Food/Pastry/Cloud target selection
- Recipe editor
- Ingredient selector
- Packaging requirement editor
- expected yield configuration
- expected waste configuration
- Recipe version history
- Recipe version compare
- validation readiness
- activate version
- retire version

Historical ACTIVE/RETIRED composition must be read-only.

No frontend implementation is included in Section 16B.

<!-- SECTION16C_IMPLEMENTATION -->
## Section 16C future UI contract

The future Vendor UI may expose:

- production-batch list and detail;
- planned RecipeVersion and exact scaled material requirements;
- Inventory reservation state;
- start/complete/cancel actions;
- actual Ingredient use;
- actual Packaging use;
- actual waste with reason;
- explicit stock top-up when actual use exceeds the hold;
- ON_DEMAND/BATCH_STOCK configuration;
- prepared-stock output and Inventory reference.

The UI must not imply that changing a Recipe changes an already-created batch.
It must not expose Section 16D cost/COGS data through the Section 16C screens.

## Section 16D - cost visibility

Cost is separately permissioned from recipes and production. A kitchen or
pastry user can hold full recipe and production access and no cost access at
all, so cost fields must be omitted rather than shown blank or zeroed.

An incomplete calculation must be presented as incomplete. When a component
could not be costed the backend returns an explicit reason
(`NO_PURCHASE_EVIDENCE`, `CURRENCY_MISMATCH`, `NO_INVENTORY_MAPPING`,
`NO_POLICY`, `NO_MANUAL_COST`), and the total is marked INCOMPLETE or UNKNOWN.
Rendering a missing cost as 0 would understate a recipe and is wrong.

Historical figures never move. A production cost from last month is immutable
evidence, not a live recalculation, and should be displayed as of its
calculation date.

## Section 17B - checkout UX handoff (DOCUMENTATION ONLY)

No screen is built in this section. What follows is the behaviour the backend
now enforces, written down so a future checkout screen is designed around what
is true rather than around what would be convenient.

**Checkout takes stock off the shelf.** The moment a customer starts checkout,
real units leave the vendor's available pool for a fixed window. That is what
makes "someone else bought it while you were paying" impossible, and it is
also why an abandoned checkout has a cost. Two consequences for design: the
remaining time must be visible, and cancelling must be one obvious tap rather
than something buried.

**A refusal is per-line, and the line is where the fix is.** The backend
returns a reason for every line it would not sell - sold out, menu closed,
below the minimum order quantity, out of stock, no longer listed. A screen
that collapses all of that into "checkout failed" throws away the only
information the customer can act on. Render the reasons against the items.

**The cart is read-only during checkout.** Every cart edit returns a conflict
while a session is live. The correct flow is "cancel checkout" then "edit
cart"; a design that offers an inline quantity stepper on the checkout screen
will produce errors the customer cannot interpret.

**A price change is a question, not a footnote.** When the authoritative price
differs from what the customer last saw, both figures are returned and the
line is marked CHANGED. It must be shown and acknowledged. The final quote
comes later (Section 17D); until then nothing has been promised at either
price.

**Variable weight needs honest language.** For a line sold by weight the
customer is agreeing to an expected measure and a tolerance, not to a fixed
amount. "About 1kg - final price depends on actual weight, up to 5% more" is
the truthful framing, and the backend returns exactly the numbers that
sentence needs.

**Nothing on this screen is a total to pay.** The two money figures are items
only: no delivery, no fees, no tax. Showing either as the amount payable would
be wrong now and wrong again once logistics pricing exists.

**Multi-vendor is normal.** A Shop checkout can span several shops and several
branches, each holding its own stock. Group by vendor; the delivery that
follows will be split the same way.

**Food and Shop behave differently, and that is correct.** A Food checkout
holds no stock - Food availability is a kitchen decision, not a warehouse
count - so the "reserved for you" framing belongs to Shop only.

**Expiry is a state the customer will hit.** Design it: what the screen says
when the timer runs out, and how the customer gets back to a cart that is
theirs again, unchanged, and immediately usable.


<!-- SECTION17A_CUSTOMER_CART -->
## Section 17C - fulfilment planning UX handoff (DOCUMENTATION ONLY)

No UI is built in this section. The notes below exist so the eventual checkout
screen is designed against what the backend actually answers.

**A multi-vendor order can have several delivery fees, and that is correct.**
Each vendor group is planned independently and may be served by a different
authority. Designing a single "delivery fee" line will misrepresent an order
where one vendor delivers with its own rider and two others go by bike and
truck. Show the per-mission breakdown, with the total as a sum.

**A partially-serviceable order is the interesting case.** The plan returns
every group, serviceable or not. The screen must let the customer act on that:
remove the refused vendor, change the address, or reduce the order. An
all-or-nothing error dialog discards the information the backend went to some
trouble to produce.

**Free delivery is not the absence of a fee.** The plan records the real cost
and who absorbed it. A vendor-funded free delivery should read as a benefit the
vendor gave, not as a delivery that cost nothing.

**Delivery estimates are per mission, not per order.** A consolidated mission
covering two co-located vendors has one ETA; three separate missions have
three, and they will differ.

**Plans expire.** `expires_at` reflects the soonest authority quote inside the
plan, which is typically minutes rather than hours. The screen needs a
re-plan path, and must not let a customer proceed on a lapsed fee.

**Nothing here is an authorization.** The plan is a candidate. Do not present
it as a confirmed order or a charged amount; 17D locks the customer-protected
total.

## Section 17A - Customer Cart UI/UX

Food and Shop carts must remain separate.

Food permits one Vendor business/branch per cart. Shop may contain and display
multiple Vendor business/branch groups.

Clients must retain the latest cart version/ETag and send If-Match on
existing-cart mutations. Retried commands must reuse the same Idempotency-Key.

Saved delivery locations are immutable cart snapshots. Receiver information is
minimal delivery-recipient information only.

Displayed cart prices and availability remain snapshots until checkout
revalidation.

## Section 17F - Commercial change and fulfilment benefit

An address or delivery-option change after order approval is a new quote, not
an inline edit. Show the original commitment beside the replacement quote,
including the new fingerprint, expiry and delta. The customer must explicitly
approve the replacement quote before any positive Wallet authorization. A
rejected or expired replacement leaves the original order unchanged.

If the final eligible customer-facing delivery charge is lower, show the
policy-backed benefit and its Wallet refund status. Never describe a lower
internal rider/provider operating cost as a customer saving.

## Section 18 - Vendor Order Lifecycle

Vendor queues must show food preparation and Shop picking states, per-line
availability, blocking substitution decisions and immutable purchased-item
history. Customers see only their own substitution prompts and approve or
reject with the displayed version. Refunds render `REFUND_PENDING`,
`REFUNDED`, or `PARTIALLY_REFUNDED` from Wallet authority. Delivery displays
distinguish Vendor Own Delivery from Dispatch/Hauling platform progression.

Shop picker UI must preserve the original agreement beside the final result:
ordered quantity, variant/sell unit, estimate and maximum, replacement
snapshot, actual quantity, unit price, refund/reapproval and final total.
Multiple quality preferences are displayed independently; a quality failure is
never presented as an automatic substitution decision. Customer substitution
prompts are case-based and remain the source of truth until a durable accepted,
rejected, expired or auto-refund state is returned.
## Section 21 commerce

Show explicit scheduled status, capacity reservation status and tentative
vehicle class as planning evidence (not a driver/provider assignment). Group
participants can only edit their own open selections; the organizer is the
single payer. RFQ status transitions are server-controlled and must not be
rendered as an order until conversion succeeds. RFQ screens must show the
immutable payment/deposit policy snapshot and Wallet-backed conversion status.
Corporate feeding screens show timezone, weekday, headcount, allowance,
destination and menu rotation snapshots; generated occurrences are immutable.

## Promotions and delivery funding UI handoff

No frontend was implemented. A future Vendor screen uses the seven
`/api/v1/vendor/promotions` operations and must show promotion type, lifecycle,
scope (branch/capability/category/item/variant/CRM segment), inclusive start and
exclusive end, order threshold, usage limits, coupon state, bundle lines,
quantity tiers, combinability/priority and immutable version. All money is
integer minor units; percentage input is basis points.

Show gross delivery cost separately from customer contribution and Vendor/
Platform funding. “Free delivery” means customer contribution zero, never cost
zero. Vendor staff cannot select Platform or Combined campaign funding and
cannot configure Platform-funded HYBRID gaps; those values may only be shown
when a platform-approved projection is returned. Never display coupon digests,
Wallet controls, customer segment membership data, or editable historical order
snapshots. Conflicts from `If-Match` require refresh rather than overwrite.

## Commerce settlement and refund UX handoff

Keep operational and financial status visually separate: `CANCELLED` or
`REJECTED` can coexist with `REFUND_PENDING`, and a Support case is not a
refund. Show one refund row per affected VendorOrder/item/delivery difference,
including bounded reason, amount returned to the customer, status and safe
operation/case reference. Do not expose internal recovery-source, ledger
account, funding-approval or provider-account identifiers.

For multi-vendor purchases, group outcomes beneath their VendorOrder so one
vendor's rejection never makes siblings appear refunded. Vendor-owned delivery
earnings and platform-provider delivery earnings must be labelled according to
the frozen delivery responsibility. Prepared Food and Shop return/refund copy
must remain distinct.

## Fulfilment group, parcel and mission model

Backend only. These are the experience requirements the implemented API
supports; no screen was built.

### Vendor operations

**Order fulfilment view.** One order's delivery plan: how many deliveries it
needs, who is carrying each, and where each one is. Split fulfilment must be
visible as split — an order needing two vehicles must never be shown as one
delivery.

**Packing.** Staff create packages against a pickup obligation, record what went
in each, and mark the obligation ready. A unit may legitimately need several
packages; the interface must support that rather than assuming one. After
collection, the package contents become read-only: they are the record of what
was handed over.

**Plan history.** When a delivery is replanned, the operator needs to see what
changed and why — "Plan v3 (capacity overflow), previously v2 (provider
unavailable), originally v1". Reasons come from a controlled list; route
internals are not exposed.

**Own delivery.** For vendor-executed deliveries: the mission, its assigned
rider, its packages, its pickup and receiver, and the step-by-step execution
actions. Steps are ordered and enforced server-side — there is no way to mark a
delivery complete without having gone through it, and the interface should not
offer one.

**Platform delivery.** A read-only progress view: which KarryGo service is
carrying it, the mission reference, and the last reported status with its
timestamp. No control anywhere may change it.

**Returns.** A returned package is the same package. The interface must keep
showing the same reference and the same history, with the return as a stage of
that journey rather than a new item.

### Customer-facing handoff

- An order with more than one delivery must say so plainly ("Your order arrives
  in 2 deliveries") and track each separately.
- Package references are stable and safe to show throughout, including after a
  replan, a provider change or a return.
- A delivery whose plan changed should communicate the outcome, not the
  mechanism: the arrival estimate and who is bringing it, not the routing
  decision behind it.

### Deferred

Taxi is not a commerce delivery option. It exists in the domain vocabulary so a
future integration needs no rework, but `driver-taxi-service` has no commerce
mission ingress and the server refuses it. Do not present it as a choice.

---

## Delivery Requirement and Vehicle Selection

Backend only in this section — no frontend was implemented. What follows is
what the surfaces will need, and what they must not show.

### Vendor Web

On an order's fulfilment view, show the delivery requirement beside the plan:
weight, volume, package count, the largest single package, and the handling
requirements the load carries. State plainly whether the figures are the
catalogue **estimate** or the **confirmed** packing measurement — a vendor
looking at "18 kg" needs to know whether that is what was promised or what was
weighed.

Show the selected vehicle with its reason in plain language, and when the plan
escalated, say why. "A larger vehicle was required" is the message; the
thresholds behind it are not. A vendor does not need to know the motorcycle
limit is 25 kg, and publishing it invites gaming.

Three states need their own treatment because they are actions, not
information:

- **Own delivery cannot carry this load.** The vendor's fleet does not cover
  the class the order needs. Show which class it needs and that the platform
  will carry it, since under HYBRID that is what happens next.
- **Plan changed after packing.** The confirmed weight moved the order to a
  different vehicle. Show both figures. This is the single most likely support
  question the feature will generate.
- **No suitable delivery method.** Say what cannot be satisfied — a
  temperature conflict, an unserviceable route — rather than a bare failure.

A temperature conflict deserves specific copy. "These items cannot travel
together" with the conflicting requirements named is actionable; a generic
error leaves the vendor repacking at random.

### Customer app

Say what is happening, never why in engine terms:

- "Delivery will use a motorcycle" / "Your order requires a van"
- "Your order requires a larger vehicle" when a plan escalates
- **"Delivery plan changed; your approved price has not increased"** — the
  reassurance matters more than the change. An operational escalation never
  alters a customer-approved price.

Do not surface reason codes, thresholds, candidate ladders or policy versions.
They are operator and support vocabulary.

### Support / operations console

The requirement snapshot is designed to answer four questions directly, and
the console should ask them in these words:

- Why was a van chosen? → primary reason plus the selected class.
- Why could a motorcycle not be used? → the candidate ladder, which records
  every class that was refused and the single reason each was refused for.
- Why did delivery change after packing? → estimated versus confirmed
  measurement, both retained.
- Why is this order not serviceable? → the decision-level reason code.

One operations view is worth building early: how often the minimum physical
class differs from the class actually executed. On this platform today that
gap is usually the disabled taxi integration rather than heavy orders, and it
is invisible without the comparison.


## Multi-vendor consolidation, delivery preference and basket optimization

### The delivery preference

Three options, and the copy should say what each BUYS rather than naming the
mode:

- **Fastest** - arrives soonest. May cost more, and may arrive as two separate
  deliveries.
- **Balanced** - the default. Chosen when the customer expresses no preference.
- **Saver** - lowest delivery cost, inside a wider arrival window.

Show each option's own delivered price and arrival estimate where the design
allows. They come from real logistics quotes, so they are comparable.

**Never show a mode as unavailable without a reason.** When a preference cannot
be honoured it is because a safety or service constraint refused it, and the
constraint is the useful half of the message.

### Explaining a split delivery

The plan carries every alternative it considered. The customer-facing sentence
is short; the evidence behind it is not.

- "Arriving in 2 deliveries" with a one-line reason.
- "These items cannot travel together" for a temperature or fragility split.
- "One of your shops is not ready until later" for a ready-window split.
- "These two shops are too far apart to combine" for the multi-stop case.

**Do not phrase the multi-stop case as a customer problem.** It is a platform
limitation today, and copy blaming the shops for it is both wrong and
unhelpful.

### Support and operations

Support needs a view of the alternatives that answers, without recalculating:

- Why were these vendors combined? -> the selected candidate and its reason.
- Why was the order split? -> the rejected consolidated candidate and ITS
  reason.
- Why did Fastest cost more? -> both candidates' delivered totals side by side.
- Why could Saver not combine these pickups? -> the constraint code plus the
  captured runtime capability.

One operations view is worth building early: how often
`MULTI_STOP_UNSUPPORTED` is the reason an order was split. That number turns "a
runtime limitation" into a figure somebody can prioritise against.

### Optimize My Cart

The panel must read as a SUGGESTION throughout. A customer who opened it to
look has not asked for their basket to be rearranged.

- Show the delta, not just the new total: what changes, from which shop to
  which, and how much it saves DELIVERED.
- Name every substitution with what it replaces and why the two are considered
  the same thing.
- Call out a vendor change separately from a product change. People often care
  more about which shop than which tin.
- Two buttons with equal weight: **Accept changes** and **Keep my cart**.
- After acceptance, tell the customer the total will be reconfirmed at
  checkout. It is not a caveat; it is why the number they are about to see may
  differ.

### Budget shopping

Inputs: budget, what they need, and their constraints - required brand,
substitutions allowed, quality floor.

Results are variants, each with merchandise, delivery, DELIVERED total, shop
count and an arrival range. Lead with the delivered total; it is the only
number they pay.

When nothing fits, do not show an empty state. Show:

- the cheapest basket that DOES satisfy every constraint,
- how far over budget it is,
- and which of the customer's OWN constraints is holding it there.

The last one is the actionable part. "Over budget because prices are what they
are" is true and useless; "over budget because you required one brand" is
something they can change.

When a requirement cannot be met at any price, say so as its own state. It is
not a budget problem and must not read as one.

### Recurring baskets

Present a template as a SHOPPING LIST, never as a saved order. That distinction
is the whole product: it re-shops every week, which is why it keeps working
when a shop runs out.

Each occurrence shows the comparison - same shops versus optimized - with the
merchandise, delivery, total, shop-count and arrival deltas.

Approval settings need care. The default is "ask me every time", and the
auto-approve options must state their bound in the customer's own words:
"automatically buy when nothing has changed, up to a limit I set". A spending
limit is mandatory for those options, and the form must say why rather than
just refusing.

**Never imply a generated basket has been bought.** A proposal is waiting for
them; nothing has been ordered and nothing has been charged.

## Dispatch Commerce delivery states

- Render pickup stops in the server-provided sequence. Keep each Vendor's name,
  ready window, manifest and handover state visible even when stops share the
  same coordinates.
- A queued mission is still active. Use “Finding a rider” for
  `NO_PROVIDER_QUEUED`; reserve failure treatment for the bounded terminal
  reason supplied by the authority.
- Show parcel identity consistently through provider replacement, transfer and
  return. Never generate a replacement parcel number in the client.
- Receiver unavailable must offer only server-permitted next actions:
  reschedule or return. The UI cannot mark a mission delivered.
- Pickup and receiver codes are short-lived secrets: mask them outside the
  active handover screen, never log them, and distinguish expired, invalid and
  attempt-limit responses.
- Delivery failures do not themselves mean “Refunded”. Show financial status
  only from the normal Vendor/Wallet projection.

## Operational evidence and recovery frontend handoff

- Show the immutable manifest reference/version, exact Parcel list and package
  count at every pickup and provider-transfer confirmation. Never substitute a
  mission-level total for the stop confirmation.
- Photo capture from the live camera is a provider-app UX control. The backend
  validates evidence type, capture time, coordinates and opaque media refs but
  does not claim to attest camera UX. Upload bytes privately through Media File
  Service; do not embed base64 or expose public evidence URLs.
- Render only the recovery actions returned by policy. Alternative receiver
  and controlled holding are not currently available; do not synthesize them.
- Keep `RETURNING`, `RETURNED`, `REPLACEMENT_REQUIRED`, `TRANSFER_PENDING` and
  `PROVIDER_TRANSFERRED` distinct. None implies a refund or a charge.
- A provider-transfer screen is two-party and must show the same mission,
  Parcels, package count, old/new provider and transfer location. Mask the
  short-lived challenge and never copy it into receiver verification.
- Prepared Food return wording must not promise Shop restock or resale.
- No provider frontend is implemented by Sections 33–35.

## Confidence ETA and notification UX handoff

- Display an ETA range, never a second-level predicted timestamp. Use the
  server's mission ordering and keep split-delivery ranges separate.
- Display LOW/MEDIUM/HIGH confidence and freshness without inventing a numeric
  percentage. An empty mission ETA means authoritative route evidence is not
  yet available.
- Keep promised delivery windows distinct from predictions. `AT_RISK`,
  `DELAYED`, `RECOVERED` and `NOT_APPLICABLE` are presentation states, not
  permission to rewrite the promise.
- Realtime ETA/order/parcel/mission notifications must trigger server read-back
  after reconnect or version gaps. Deduplicate by source event/version.
- Vendor views may show provider assigned/arrived/pickup and the current
  customer-delivery range. Customer views must not expose provider-private
  fields.
- Dispatch receiver codes may be shown only after the authoritative credential
  exists. VendorMission receiver handover remains dependency-gated; do not
  fabricate a code or reuse a Dispatch code.
- Notification channel preference and copy belong to Notification Service.
  Frontends cannot supply templates, transport destinations or recipients.
## Section 39 Analytics UX Requirements

Analytics surfaces should be quiet operational dashboards: compact metric
cards, tables and trend charts with explicit time windows. Each report must
show `window_start`, `window_end` and `data_as_of`.

Demand and lost-opportunity values must be labeled `ESTIMATE`. Price
intelligence must suppress low-sample cohorts and must never display competitor
identity, individual competitor price or confidential branch data.

Reliability presentation must show the score status and calculation version,
and must not visually imply verification, trust badge, star rating or
popularity ranking.

Order Guarantee is future-disabled and must not appear as a customer-facing
claim or compensation feature.

## Section 40 security repair UX

- Treat HTTP 429 on marketplace search, quote creation, Commerce support/report
  creation and Analytics reads as a bounded cooldown state; do not vary copy in
  a way that reveals a private account or resource.
- A restricted-goods response is a server policy decision. Do not offer a
  Vendor/customer override. Pharmacy and medicinal Commerce are excluded.
- Evidence MediaRefs remain opaque. If attachment is rejected because the ref
  is not authorized for the exact mission/Parcel, request a new private upload;
  never turn the ref into a public URL or attach it to another mission.
- No frontend capability is implemented by this security repair.
