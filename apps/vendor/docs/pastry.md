# Pastry & bakery workspace

Open More > Pastry & bakery. Mock remains the default; Live calls the existing mounted pastry API. The workspace covers catalogue, request, quote, design-review and production workflows. Configuration coverage and deployed verification boundaries remain below.

## Implemented

- Products: list/detail/create/edit/archive. Product types and creation statuses come from the database constraints. Editing preserves the existing currency, type and status because the PATCH handler does not update those fields.
- Reusable option groups: list/detail/create/edit/archive. Actual group types, selection bounds and version checks.
- Options: list/create/edit/archive, additional pricing, workload, position and supported capacity classes. Option writes update attached product versions in Mock as the backend does.
- Discover attached groups through the scoped product option-group read endpoint; attach and remove groups with required selection, position and the latest product version. Removing an attachment retains its reusable group. Live discovery requires deployment of the updated vendor service.
- Product delivery handling: supported temperature, spill risk, weight, package size and boolean requirements. Existing vehicle suitability hints are preserved.
- Custom requests: list/detail/create/edit, submit and eligibility-aware cancellation with explicit review confirmation when required by the server.
- Specifications: immutable revision history and direct revisions using supported occasion, servings, theme, inscription, notes and required date fields. Unedited saved fields, including nested requirements and media references, are preserved. Terminal state and server change eligibility govern editing.
- Durable command recovery: save the exact body, references, versions and idempotency key before submission. Unconfirmed outcomes block new writes; retries reuse the saved command. Explicit success-only acknowledgements are supported for the relevant handlers.
- Full-screen forms/details with the existing animated routes, scoped action spinners, themed skeletons, semantic state colours and feedback dialogs. Mock examples fill form fields; Live forms use server values.

## Quotes, reviews and production

- Quote list/detail/create/revise, typed multi-pool capacity allocations, optional eligible production lines and reservation evidence. Creating a revision opens the new quote. Expiry and current effective specification govern approval and acceptance; pending/declined proposed designs are excluded from quote context.
- Approval uses the existing OTP screen for pastry.high_value_quote_approval and the actual X-Step-Up-Grant header. The generated client metadata was corrected from the mounted handler; no Go handler or security rule was changed.
- Grants stay out of the durable journal. Recovery preserves the original body/version/idempotency key and obtains fresh verification when approval is pending.
- Accept/decline are existing vendor controls. Acceptance shows the returned conversion reference and reserved capacity; it never claims a customer order/payment/dispatch was completed. There is no public read of the conversion intent in the quote detail contract.
- Standard production: genuine branch/capability pool selection, quantity/time/eligible line selection, preview and confirmed batch creation. Capacity is rechecked by the server when creating the batch; open the returned job in Production operations.
- Design proposals, before/after comparison, immutable decisions and decision history. Direct design revisions and proposed revisions remain distinct.
- Six ordered milestones with transition notes/history, block/resume/cancel and readiness. The shared production queue is available from milestones.

## Design and request configuration

- Full-screen design editor: occasion, required date, servings, theme, inscription, notes, colours, active registry-backed option IDs and existing Media reference IDs. Saved option IDs remain visible even when no longer active. The request contract has no product reference, so the editor does not assume product-specific selection rules or calculate option pricing.
- Request creation includes change/cancellation minute windows and milestone cutoffs, delivery handling and the five supported risk flags. Saved configuration is visible in request detail. The existing request PATCH does not support later policy/risk changes, so those controls are creation-only.
- Existing production/logistics maps can be edited through their saved keys and value types. Primitive values and nested maps are supported; arrays/null/unknown shapes remain intact and read-only. Their creation contract remains unspecified; no new nested fields are offered. Media references accept existing identifiers; this does not implement image upload or Media-access discovery.
- Mock prefilled configuration uses the same request keys as Live. Live starts from existing values or backend defaults.

## Remaining integration boundaries

- Creating new nested requirement fields needs a defined backend schema; Reference-file upload/access is implemented through the authorized Vendor/Media bridge; see media.md. Saved primitive/map requirements can be edited without adding keys or changing types; lists and unknown shapes are retained.
- Deployed reference discovery, scope/permission and recovery checks.

Mock uses the existing production pool/reservation/job stores, including capacity conflicts, quote holds, valid-hold reuse on revision, confirmation on acceptance, release on decline/cancellation and milestone-driven readiness. Command writes roll back local changes on failure and replay confirmed commands without duplicate jobs or grant consumption. Empty scenarios remain empty. Mock has no external order conversion, payments, dispatch or background expiry worker. Local checks do not certify deployed services or the phone.

Sources: services/vendor-service/internal/features/pastry/http.go and service.go; migrations/000036_vendor_pastry_bakery.up.sql and later production/closure migrations.

## Verified reference selection

Custom request creation selects optional actual customers/corporate accounts for the chosen branch through GET /api/v1/vendor/pastry/request-references. Customer choices return canonical customer_ref, never CRM profile_ref. The bridge explicitly maps corporate_profile_ref to active CRM corporate profile IDs for new pastry requests and revalidates them; the Cloud-kitchen field is still a separate unresolved contract. Guest/external, archived, privacy-restricted/redacted, pending/merged-source and branch-unrelated customers are excluded. Saved references remain unchanged on request editing.

Discovery requires pastry.manage plus the corresponding CRM read permission and both branch scopes. It uses existing CRM list/read authority, safe labels and bounded pagination. Creating with references checks current CRM access/state and locks eligible CRM rows in the request transaction. Optional unavailable CRM linking permits unlinked creation; manually entered identifiers are unavailable. Selecting both references does not claim contact membership or order/payment authority.

Mock selection shares existing CRM fixtures/lifecycle; empty fixtures remain empty. Canonical/stale reference and backend eligibility checks passed. Deploy the updated vendor service before Live reference discovery is available.

Reference media now supports real image/PDF upload and authorized access in Mock/Live. Use references returns the draft to the existing specification/change save; it does not independently commit a design. Deploy Vendor, Media and the configured scanner before Live use. See media.md.
