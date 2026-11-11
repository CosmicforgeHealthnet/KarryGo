# Fleet onboarding final closure

Date: 2026-11-11. Scope: `staging` only, repository `CosmicforgeHealthnet/KarryGo`.

## 1. Executive summary and certification status

The six-step implementation and local end-to-end journey are complete. Release verification is in progress. **Full live staging certification is not yet claimed.** Deployment IDs/image evidence and remote CI results will be appended after observing their real outcomes. Live email-OTP testing needs an authorized test inbox and its code; the connected Gmail reader withholds OTP values.

## 2. Audit and confirmed Step 3 root cause

Baseline: `23c48c6d`, including the recent record-version, representative-status and registration-preservation fixes. Those fixes were inspected and retained.

The current-code failure was reproduced through the actual Fleet HTTP handler, authorization, use case and PostgreSQL database. Request ID `onboarding-step3-baseline-authority`, POST representative with `authorityType=LEGAL_REPRESENTATIVE`, returned HTTP 500, `internal_error`, “Something went wrong. Please try again.” The backend logged SQLSTATE `23514` and constraint `fleet_partner_organization_authorized_repr_authority_type_check`. The form offered authority values that the database never supported.

The corrected contract offers the persisted values `DIRECTOR`, `OFFICER`, `POWER_OF_ATTORNEY`, `OWNER`, `OTHER`; unsupported values return field-specific HTTP 422 before SQL. Valid create/update paths pass with real persisted versions. Stale versions return HTTP 409 instead of another generic 500.

This proves the current branch's failure mechanism. It does **not** attribute the original user's historical live request without its unavailable correlation evidence. The updated audit records this distinction.

## 3. Corrected defects

- Company type was accepted but discarded; it now persists in the existing organization record.
- Company responses omitted registration/type; both are returned, with the actual persisted version.
- Existing profile writes without a version could report success while discarding edits; they now conflict. Omitted unrelated fields are retained.
- Representative authority codes, validation, record selection, persisted response/version, status preservation, self/other identity boundaries and duplicate-save handling are repaired.
- Version errors now map to actionable 409 responses with support references.
- Client stage completion could forge business/internal progress; customer completion is assessed from actual records, and internal stages cannot be completed by the customer.
- Unsupported capabilities were silently dropped and failed loads could erase selections; codes now validate and failed reads prevent saving.
- Document removal was local-only; an authorized audited soft-removal route now updates the server. Replacement is atomic and media events cannot resurrect removed attachments.
- Media readiness wrongly accepted pending scans and rejected confirmed SAFE files. Readiness now accepts confirmed safe states and rejects deleted media.
- Ordinary JSON request limits could reject otherwise permitted uploads. Only the explicit upload route receives a bounded multipart envelope; unknown-length bodies are bounded too.
- Documents no longer fabricate dates, sizes, filenames or verification statuses.
- Review now displays actual records and exact backend missing requirements with edit-return navigation.
- Wallet provisioning no longer simulates status/retries.
- Browser hydration lost authentication state, mobile content overflowed, and submitted-company sign-in routing was wrong; regression tests drove the fixes.
- Multi-organization selection confused membership and company lifecycle statuses. The API adds `organizationStatus` without changing membership `status` semantics. The chooser uses actual lifecycle values.
- Tenant middleware ignored the organization selected in the durable session. It now resolves trusted session scope and rechecks membership for that organization; ambiguous unscoped sessions still fail closed.

## 4. Before and after journey

| Step | Final behavior |
|---|---|
| Company | Backend account-contact summary; legal fields and company type persist; owner membership and idempotent creation; safe versioned edits |
| Address | Saved contacts first, owner-contact suggestions where appropriate; distinct registered/operating addresses; same-address option; unrelated profile fields retained |
| Representative | Explicit self/another person, reliable prefill, exact record selection, supported authorities, safe identity link, versioned create/edit/reload |
| Services | Backend catalogue and saved selections; retryable fail-closed loading; supported replacements only |
| Documents | Existing signed Media authorization/upload/checksum/readiness/ownership flow; real attachments, replacement and soft removal |
| Review | Full saved summary, edit-return links, exact missing prerequisites, locked submission snapshot, real review status |

## 5. Data-prefilling architecture and persistence

A single onboarding provider loads the current account projection, resolved organization, profile, representatives, capabilities, documents and authoritative stages. Independent reads run concurrently. Failed required reads prevent destructive forms. Responses from an obsolete identity/organization scope cannot populate the new one.

Saved exact fields take precedence. Account contacts are suggestions only where appropriate; unsaved edits initialize once and remain in a provider-scoped in-memory draft while navigating. They are not replaced by delayed reads. Reloading a conflicting record is an explicit discard action. Previously saved state always reloads from the backend, including a separate Android Chrome emulation session. Logout/account changes and organization selection clear scoped caches.

## 6. Identity, owner, business and representative boundaries

`GET /identity/profile` projects only the authenticated session's own identity contacts and available current membership name/owner flag. It checks live session/account eligibility, ignores requested identity IDs, uses no second OTP and returns `Cache-Control: no-store`. A contact-free security event records the read.

Company contacts are editable profile fields and never update login identifiers. A representative may be the owner or another person. Self linkage derives from authenticated claims and an actual owner membership; another person's record has no owner identity reference. No SMS or phone-ownership verification is introduced.

## 7. Frontend changes and shared components

Shared provider/draft handling, fields/selects, address group, safe error notice, loading state, form/section, review values, navigation footer and six-step catalogue use existing Karryam design tokens. Native labels, required fields, focusable validation summaries, keyboard controls and screen-reader progress are retained. Mobile grid/flex constraints, safe-area footer spacing and narrow viewports are verified. All ISO country options are available for company registration. No Figma or unrelated dashboard redesign.

## 8. Backend changes and API contracts

Machine-readable contract: `services/fleet-partner-service/docs/openapi/onboarding-v2.openapi.json`.

| Endpoint | Contract |
|---|---|
| GET `/fleet-partner/v1/identity/profile` | New current-session contact projection |
| POST `/organizations` | Existing route; atomic owner creation, identity-scoped idempotency and registration-race protection |
| GET `/organizations/current`, PUT `/organizations/{id}` | Persist/return registration and company type; versioned updates preserve omitted legacy legal/settings values |
| GET/PUT `/organizations/{id}/profile` | Missing profile is null; existing writes require version and preserve omitted fields |
| GET/POST/PUT representatives | Full persisted model/version, canonical authorities, exact record, correct self/other association |
| GET/PUT capabilities | Supported codes only; persisted status map |
| GET onboarding-status | Actual prerequisites/missing fields plus supported capability/authority/company/document catalogues |
| PUT stages | Compatibility route rechecks real business requirements; internal stages denied |
| Upload authorization/content and POST documents | Existing Media Service; bounded files, checksums, readiness/ownership; optional `replaceDocumentId` |
| DELETE `/organizations/{id}/documents/{documentId}` | New authorized audited idempotent soft removal |
| POST submit | Tenant lock, prerequisite recheck, all required ready document types, immutable full snapshot, existing verification dispatch |
| GET verification/wallet, POST wallet retry | Existing actual status, retries and approval gates; no mock completion |

All organization routes remain scoped to authenticated membership/permissions and reject a foreign organization path. Authentication verification adds a company lifecycle projection while preserving the existing membership status field. Session selection still rotates/revokes through the existing session family infrastructure.

## 9. Database changes

Migration `000086_onboarding_persistence` adds `organization_type` to the existing tenant root. Legacy unknown legal types remain empty, never fabricated. Down migration refuses to erase collected types. Empty rollback/reapply and retention refusal are tested. Existing identity IDs, uniqueness, company/user/representative/document records, memberships and foreign keys are retained. Document removal is a status change, not deletion. No replacement identity tables.

## 10. Security, permissions and observability

Tenant-isolated reads/writes, real role authorization, trusted session scope, owner-association checks, CAS versions, serialized customer saves/submission, idempotency, registration uniqueness and actual prerequisite gates are enforced. Files use the existing signed Media Service, ownership, authorization token, MIME/size and checksum controls. Audit records record document changes; contact values/OTP/token secrets are not added to operational logs. Safe errors include request IDs. Verification/activation/financial approval gates are preserved.

## 11. Test evidence

- `go test ./services/fleet-partner-service/... -count=1 -timeout 20m`: passed with local PostgreSQL. Local suites requiring other service/Redis infrastructure retain their normal environment gates; mandatory remote CI is recorded separately.
- New actual HTTP/PostgreSQL tests cover the authority failure, legal preservation, profile conflicts/merge, owner linkage, representative create/edit/status/multiple records, duplicate/concurrent creation, unsupported services, stage forgery, missing-document submission and cross-tenant denial.
- `npm test`: 20 passed, zero failed/skipped.
- TypeScript, changed-file ESLint and production Next build: passed. Next's existing middleware-convention deprecation remains informational.
- `TestAuthV2BrowserWorkflow`: passed; signup, email/phone sign-in, cookie refresh, duplicate actions and mobile/desktop auth regression.
- `TestOnboardingV2BrowserWorkflow`: passed; signup/email OTP, all six stages, owner/another-person representative edits, offline save retention, capability-load failure recovery, signed media upload/readiness, persistent removal/reupload, review editing, submission without approval, phone and independent email sign-in, Android Chrome emulation and organization switching without cross-tenant prefill.
- Safe browser outcome evidence: `docs/implementation/evidence/fleet-onboarding-v2/browser-results.json`. Full local logs/screenshots are in `.codex-cache/fleet-onboarding-*`; remote workflow uploads browser artifacts.

The local browser suite uses real Fleet HTTP, authorization, PostgreSQL and signed Media client calls against test authorities. Its email sender is a fixture; verification is deliberately not auto-approved. These results are not represented as live Notification/Media/Verification delivery certification. Android is Chromium device emulation, not a physical handset.

## 12. Git release

Implementation commit: pending the release commit. Branch remains `staging`; no merge to main or production operation.

## 13. GitHub Actions

Pending observation after push. The existing Fleet authentication workflow now also runs onboarding HTTP/browser tests and uploads both sets of artifacts. The mandatory repository workflow remains authoritative for its infrastructure-backed checks.

## 14. VPS deployment evidence

Pending actual staging deployment job, API/worker running-image revision checks and migration read-back. A green build or skipped server step is not accepted as deployment evidence.

## 15. Vercel deployment evidence

Confirmed target: project `fleet-karryam`, `prj_kFUDZnnpo98MmSwuDFb73XgHOHBJ`, root `web/fleet`, Git production branch `staging`, business staging alias `fleet-karryam.vercel.app`. Actual new deployment/alias revision pending observation. Provider rate limits will not be bypassed.

## 16. Live end-to-end results

Pending an authorized test account and email OTP, after both deployments are verified. No live registration/submission result is invented from fixture tests. The original live Step 3 correlation remains unavailable until a real authorized reproduction can be traced.

## 17. Remaining limitations and operator action

Provide an authorized staging test inbox and its current email OTP when requested, or complete sign-in in the connected Fleet browser. Then execute the live signed-upload/submission/logout/email/phone sign-in journey and capture persisted state plus Notification/Verification outcomes. Full certification remains open until those observations exist. Physical Android testing is also not claimed from emulation.

## 18. Files created or modified

- `.github/workflows/fleet-auth-v2.yml`
- `docs/implementation/evidence/fleet-onboarding-v2/browser-results.json`
- `docs/implementation/fleet-onboarding-end-to-end-audit-2026-11-11.md`
- `docs/implementation/fleet-onboarding-final-closure.md`
- `scripts/fleet/staging-auth-evidence.sh`
- `services/fleet-partner-service/docs/openapi/onboarding-v2.openapi.json`
- `services/fleet-partner-service/docs/section31-test-coverage.json`
- `services/fleet-partner-service/internal/features/auth/http/handlers.go`
- `services/fleet-partner-service/internal/features/auth/repositories/sessions.go`
- `services/fleet-partner-service/internal/features/auth/usecases/profile.go`
- `services/fleet-partner-service/internal/features/auth/usecases/session_flows.go`
- `services/fleet-partner-service/internal/features/organizations/http/handlers.go`
- `services/fleet-partner-service/internal/features/organizations/models/models.go`
- `services/fleet-partner-service/internal/features/organizations/models/onboarding.go`
- `services/fleet-partner-service/internal/features/organizations/repositories/onboarding.go`
- `services/fleet-partner-service/internal/features/organizations/repositories/organizations.go`
- `services/fleet-partner-service/internal/features/organizations/usecases/onboarding_service.go`
- `services/fleet-partner-service/internal/features/organizations/usecases/onboarding_validation.go`
- `services/fleet-partner-service/internal/features/organizations/usecases/submission_activation.go`
- `services/fleet-partner-service/internal/integration/documents_harness_test.go`
- `services/fleet-partner-service/internal/integration/onboarding_browser_test.go`
- `services/fleet-partner-service/internal/integration/onboarding_v2_test.go`
- `services/fleet-partner-service/internal/integration/organization_closure_http_test.go`
- `services/fleet-partner-service/internal/integration/organization_onboarding_resume_test.go`
- `services/fleet-partner-service/internal/middleware/auth.go`
- `services/fleet-partner-service/internal/middleware/middleware.go`
- `services/fleet-partner-service/internal/platform/respond/respond.go`
- `services/fleet-partner-service/internal/platform/riskregistry/registry.go`
- `services/fleet-partner-service/internal/platformclients/contracts.go`
- `services/fleet-partner-service/internal/platformclients/media_file.go`
- `services/fleet-partner-service/internal/section30contract/migration_replay_test.go`
- `services/fleet-partner-service/internal/section30contract/onboarding_migration_test.go`
- `services/fleet-partner-service/migrations/000086_onboarding_persistence.down.sql`
- `services/fleet-partner-service/migrations/000086_onboarding_persistence.up.sql`
- `web/fleet/scripts/onboarding-e2e.mjs`
- `web/fleet/src/app/(auth)/choose-organization/page.tsx`
- `web/fleet/src/app/(auth)/verify/page.tsx`
- `web/fleet/src/app/(setup)/setup/address-group.tsx`
- `web/fleet/src/app/(setup)/setup/authorized-representative/page.tsx`
- `web/fleet/src/app/(setup)/setup/business-docs/page.tsx`
- `web/fleet/src/app/(setup)/setup/capabilities/page.tsx`
- `web/fleet/src/app/(setup)/setup/company-form.tsx`
- `web/fleet/src/app/(setup)/setup/create-company/page.tsx`
- `web/fleet/src/app/(setup)/setup/headquarters/page.tsx`
- `web/fleet/src/app/(setup)/setup/layout.tsx`
- `web/fleet/src/app/(setup)/setup/onboarding-context.tsx`
- `web/fleet/src/app/(setup)/setup/onboarding-ui.tsx`
- `web/fleet/src/app/(setup)/setup/page.tsx`
- `web/fleet/src/app/(setup)/setup/profile/page.tsx`
- `web/fleet/src/app/(setup)/setup/steps.ts`
- `web/fleet/src/app/(setup)/setup/submit/page.tsx`
- `web/fleet/src/app/(setup)/setup/useSetupStatus.ts`
- `web/fleet/src/app/(setup)/setup/wallet-provisioning/page.tsx`
- `web/fleet/src/lib/auth/organizationDestination.ts`
- `web/fleet/src/lib/countries.ts`
- `web/fleet/src/lib/services/organizationService.ts`
- `web/fleet/src/lib/stores/authStore.ts`
- `web/fleet/src/lib/stores/onboardingStore.ts`
- `web/fleet/src/lib/stores/organizationStore.ts`
- `web/fleet/src/lib/types/auth.ts`
- `web/fleet/src/middleware.ts`
