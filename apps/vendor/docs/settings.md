# Business settings

The More tab opens Business settings. Mock remains the default; Live uses the selected business and the public Vendor contracts without demo fallback.

## Implemented flows

| Area | Screens and actions | Backend contract |
| --- | --- | --- |
| Business profile | Read, edit all six supported fields, pause and resume | Business GET/PATCH; POST pause/resume |
| Brands | Paged list, detail, create and edit display name, description and opaque media references | Business brands GET/POST/PATCH |
| Branches | Paged list, detail, create/edit full address, coordinates, timezone and brand; pause/resume | Business branches GET/POST/PATCH and status actions |
| Operating hours | Seven-day editor, multiple non-overlapping intervals, closed days, remove intervals | Branch hours GET/PUT; expected version belongs to the branch |
| Delivery area | Radius and centre coordinates | Branch service-area GET/PUT; its own version, zero when not configured |
| Order preferences | Accepting new orders and pickup enabled | Operations-settings GET/PUT; schema version 1 and its own resource version |
| Business types | Available-type catalogue, paged configured list, detail, add, pause/resume | capability-types and business capabilities routes |
| Branch types | Actual link status/effective reason, link available business types, pause/resume | Branch capabilities GET/PUT/status routes; path uses vendor capability ID |
| Team | Staff list/detail, provision using email or phone and role/scope, assign/remove roles, suspend/reactivate | Staff, role assignment and assignment DELETE routes |
| Roles | System/custom role list, detail, create custom role, edit custom permissions, catalogue search and category groups | Roles and permissions catalogue routes |

Editing a business sends the complete replacement fields: legal_name, trading_name, support_email, support_phone, currency and timezone. Editing a branch sends its complete supported location and brand fields. Existing values are retained in the form. Conflicts require an explicit refresh; reloading an existing edit asks before replacing unsaved work. Profile changes may create updated verification requirements; Business setup remains available for verification and settlement-bank onboarding.

Complex editing uses full screens with safe-area actions. Focused delivery settings, role/scope selection, hours intervals and confirmations use sheets. Status colours distinguish active, pending/paused, neutral and failed/suspended states. Each network action owns its busy state and stops before result feedback. Routes reuse the existing motion-aware transitions. Sheets respond to keyboard insets.

## Security and failure behaviour

High-risk permissions come from the public catalogue, not a client-maintained permission policy. Creating a role, replacing permissions or assigning a role containing them requires purpose `iam.high_risk_permission_change`. The authentication flow refreshes before issuing the challenge and verifies the code against the same session. The returned grant goes directly into the change; the app does not automatically refresh authentication or replay the write.

Business writes use the declared Idempotency-Key and retain it when an outcome is uncertain. IAM routes do not declare that header, so the app does not attach one. A lost response, conflict or service error requires checking refreshed records before another change. A 403 disables that action in the current controller. Server scope, delegation and permission checks remain authoritative. Staff suspension explicitly warns about session revocation; suspending the current staff member signs out locally after a known successful response.

## Backend boundaries

- New branches, business capabilities and branch capability links start as Draft. Public settings have no activation endpoint. Pause/resume is available only for ACTIVE/PAUSED records; the UI never pretends a Draft record is active.
- Activated PAUSED businesses can still enter the workspace and reach Resume. Home displays the paused state. New unactivated businesses still enter onboarding.
- Staff reads expose staff ID, business ID, status, version and creation time, plus role assignments. They do not expose names, email or phone. Read screens use IDs and actual role/branch labels. Adding staff provisions access; the UI does not claim an invitation email was delivered.
- System roles are read-only. No public role rename/delete, brand delete or branch delete flow is invented.
- Brand details now expose Manage brand images: target-bound upload, LOGO/COVER/GALLERY choice, preview/removal and version-checked save. Verification invalidation follows the existing backend. See media.md for Live deployment and preserved legacy-reference boundaries.
- Settings manage the selected business. Creating a different business and selecting a different vendor are separate bootstrap/authentication flows.

## Mock coverage

Changes persist across the settings, onboarding and dashboard stores for the runtime session. New records obey Draft states and version rules. The permission catalogue comes from the backend migration seed; role fixtures are representative and do not simulate full server delegation policy. Staff reads deliberately omit contact/name projections. Mock security grants are purpose-bound, expiring and single-use. Additional dashboard scenarios are Settings access limited and Settings unavailable. Live errors never select those fixtures automatically.

Mock creation forms contain realistic supported values. No fake extra fields are added to requests. The implementation has no new dependency or backend route.
