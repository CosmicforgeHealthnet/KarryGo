# Karryam Fleet onboarding — end-to-end audit and implementation brief
Date: 2026-11-11
Branch: staging
Scope: web/fleet and services/fleet-partner-service; no main/production changes.

## Audit status
Repository inspection completed. The reported Step 3 HTTP error has not been reproduced from live backend logs, because the user's browser request ID/response was not available. The source demonstrates deterministic update-contract defects, and changes to those defects are committed separately on staging. End-to-end onboarding certification remains OPEN.

## Current flow
The frontend has six visible steps defined at `web/fleet/src/app/(setup)/setup/steps.ts`:
1. Company details (create-company/profile; backend OWNER_CONTACT + BUSINESS_PROFILE)
2. Business address (headquarters)
3. Authorized representative
4. Services you offer (capabilities)
5. Documents (business-docs)
6. Review & submit (submit)

The backend's `organizations/models/onboarding.go` includes additional internal stages, such as verification, settings, territories, wallet provisioning and activation. These are not six independent customer entry forms and must not be presented as such.

## Verified issues and evidence
| Area | Current implementation | Risk / required correction |
|---|---|---|
| Step 1 company creation | Owner email and phone are mandatory frontend inputs but `CreateOrganizationRequest` sends only `ownerName`, not email/phone. The frontend writes owner contact to persisted `onboardingStore.companyData`. | Data is duplicated after verified signup and is not durably tied to an authenticated identity in the organization API. Replace browser-only propagation with an authenticated server-backed identity/contact projection. |
| Step 1 company edit | `profile/page.tsx` previously sent empty `registrationNumber` and `organizationType` when saving, while `UpdateOrganization` overwrites stored registration_number. | Legal data could be erased. Staging correction preserves registration number on omitted input; verify persisted value and API contract before declaring closed. |
| Step 2 business address | `headquarters/page.tsx` begins with blank business email/phone; operatingAddress is always overwritten with the registered address; updating an existing profile lacked If-Match/version and hit an INSERT ON CONFLICT DO NOTHING path. | Duplicate entry, inability to specify a different operating location, silent non-persistence on edit. Staging fix adds the profile version. Add explicit 'same as registered address' toggle and server-backed editable contact defaults. |
| Step 3 representative | `authorized-representative/page.tsx` loads only representatives, not owner contact; on existing rep edits it omitted status and If-Match version. Backend UPDATE requires exact version. | Repeat entry and deterministic edit failure. Staging correction sends persisted version/status; live response remains unconfirmed. Implement owner/self versus different representative selection and controlled prefill. |
| Step 3 rep selection | Code selected repsArray[0] by creation order, not the onboarding-owner representative. | Potentially updates the wrong person. Staging correction prefers `isOnboardingOwner`. Confirm representative records and permissions. |
| Progress state | Pages wrote local stage keys like `profile`, `headquarters`, `authorized-representative`, `capabilities` and `documents`, while server returns `BUSINESS_PROFILE`, `HEADQUARTERS`, `AUTHORIZED_REPRESENTATIVE`, `OPERATING_CAPABILITIES`, `BUSINESS_DOCUMENTS`. | Progress indicators can diverge. Standardize all keys and use backend stages as the sole authority. Changes staged for known pages. |
| Step 4 services | Capabilities fetch errors fall back to an all-disabled local list. | A later save can accidentally overwrite previously chosen capabilities. Present an explicit read/retry error and disable save until state is known. |
| Step 5 documents | `handleDelete` filters document from local React state, but no DELETE document API exists under OrganizationRoutes. | The UI falsely reports removal while the server retains the document. Hide/remove fake deletion or implement authorized backend deletion and persisted refresh; keep Replace. |
| Step 5 document metadata | UI synthesizes upload metadata such as fallback current time and default Uploaded status. | Do not invent uploaded-at timestamps/status; show unavailable state and source-backed values only. |
| Step 6 review | Shows completed step statuses, not the submitted company/address/representative/service/document details. | User cannot review or amend data from a consolidated summary. Build a read-only review with Edit links, missing-field guidance and final submission gating. |
| Wallet provisioning | `/setup/wallet-provisioning` uses MOCK_WALLET_STATE and mock retry. | Not a certified real onboarding stage; use existing `getWalletProvisioning` / `retryWalletProvisioning` API or keep it out of the real customer journey. |
| Generic errors | Per-step catch blocks fall back to generic strings; field violations/correlation request ID are not consistently shown. | Render precise safe error types (validation, permission, expired session, conflict, network, server) plus support reference, with no PII or sensitive internals. |

## Canonical identity and contact ownership

1. **Corporate auth identity:** unique email and phone supplied during registration; verified via EMAIL OTP only; no SMS provider. Protected authentication identifiers cannot be silently reassigned by editing a company or representative contact form.
2. **Organization owner:** owner membership and owner display name, linked to the authenticated identity. Show authenticated owner contact as a saved source, not an editable second login record. If the business needs different contacts, save organization business-contact fields separately.
3. **Business contacts:** optional organization email/phone; default from existing authorized contact when creating the business profile; editable independently of login credentials. User must explicitly confirm the submitted business contact.
4. **Authorized representative:** either the account owner (prefill name and email/phone after explicit 'I am the authorized representative' selection) or another person (separate editable fields). Do not silently equate those roles. Persist representative ID, version and status; do not overwrite a different representative.
5. **Legal and address data:** one authoritative backend value per data field. On revisit, fetch current version and prefill from the server; do not rely on another user's browser cache.

Design an authenticated read endpoint or reuse an already existing safe identity endpoint to fetch the user's verified email and phone. Investigate membership name lookup. No SMS verification or new provider is permitted.

## Standard six-step UX
- Single accessible wizard shell with concise title, progress indicators, Save & continue, Back, Save draft, and an All steps overview.
- Shared Field, Select, AddressGroup, PhoneInput, ErrorBanner, LoadingSkeleton, SaveFooter and Status components; use consistent styling with existing Karryam visual system.
- Step 1: legal company fields + owner summary from authenticated identity; ask only missing details. Edit company-only contacts without changing authentication.
- Step 2: registered address, 'operating address same as registered' toggle, editable business contacts populated where known.
- Step 3: select 'I am the representative' or 'Someone else', prefill according to selected source and permit edits; show required legal authority and title.
- Step 4: selectable logistics capabilities populated from current backend status; explicit retry on load failure.
- Step 5: document upload and replacement with real statuses, no local-only delete or fake upload metadata; show missing requirements.
- Step 6: actual review of saved records with per-section Edit links, actionable missing fields and final backend submission.
- Works on 320px+ phones, Android Chrome and desktop; avoids sticky-stepper overlap and preserves keyboard focus, safe-area spacing and scroll restoration.
- Never discard partially entered edits on navigation/network failure; preserve unsaved changes with confirmation and server-side drafts.

## Backend contract and consistency requirements
- Audit and preserve `GET/PUT /organizations/current`, `POST /organizations`, `PUT /organizations/{id}`, `GET/PUT /organizations/{id}/profile`, `POST/GET/PUT /organizations/{id}/representatives`, capabilities, documents, onboarding status and submit.
- Use If-Match (version) on existing updates; missing/stale version must yield safe, explicit conflict, never report success without updating.
- Do not wipe fields excluded from a partial editing form. Use explicit PATCH semantics or read/merge with version control.
- Existing representative updates must preserve status, document reference and legally relevant metadata unless intentionally changed.
- Fix the mutable stage API so stage completion cannot be asserted before actual domain-required data is persisted/validated.
- Return field-level errors and request IDs without disclosing private server details.
- Preserve review locks, company-status transitions, tenant scoping, permissions, staff/organization associations, and audit trail.

## Test and deploy gates
1. Backend tests for version conflicts, absent profiles, persistence, duplicate representative prevention, legal-field preservation and consistent stage statuses.
2. Frontend tests for authenticated contact prefill, switching representative person, changed business contact values, reload/resume/edit, Save & continue, field errors, expired sessions and mobile.
3. Document tests: uploaded documents remain visible after refresh; missing delete contract never masquerades as success.
4. End-to-end using real staging records: signup -> company create -> address -> representative -> capabilities -> document upload -> review -> submit. Reopen each step, edit values, refresh and confirm authoritative persisted state.
5. Audit actual API response code and request ID for a failing Step 3 save. Determine whether source is 401, 403, 409, 422 or 500; never claim a specific live status without evidence.
6. Run TS/Go tests, builds and mandatory CI; confirm the staging backend image and Vercel frontend commit both update (not skipped workflow). Keep main/production unchanged.

## Scope and completion
This audit is not a complete onboarding redesign. Immediate concurrency/data preservation fixes were committed separately to staging. Complete remaining items as a coherent frontend/backend implementation, not isolated UI tweaks. Do not mark production-ready until all six live journeys pass.
