# Karryam Fleet: pending-verification dashboard access, final closure

Date: 2026-11-13. Branch: `staging` only; `main` and production untouched. No SMS provider involved.

## Summary

A company that has submitted its onboarding now goes **Submit → real Fleet dashboard with a compact "Your company is under review." banner → explore while operations stay restricted → approval unlocks them in the same session**. It is never returned to "Set up <company>" on refresh, sign-in (email or phone), re-opening the browser, a manual `/setup` or `/setup/submit` visit, organization switching, or token refresh. The backend now refuses operational writes from non-`ACTIVE` companies itself, so calling the API directly does not bypass the restriction.

Deployed to staging and verified live, except one thing: **an authenticated live walk-through still needs a staging account OTP** (section 11). Every authenticated scenario passes in CI-equivalent browser tests against PostgreSQL.

## 1. Root causes

1. **Stale status cookie plus middleware redirect (the loop).** Verification of a new account wrote `karrygo_org_status=DRAFT` (`verify/page.tsx`). Nothing updated it during onboarding or submission, and `middleware.ts` redirected every non-setup page to `/setup` while it said `DRAFT`. A submitted company opening `/` was therefore always sent back to setup, and the dashboard layout, the only place that would have refreshed the cookie, never ran.
2. **Wrong destination.** `organizationDestination` mapped `BUSINESS_SUBMITTED` and `UNDER_REVIEW` to `/setup/submit`, so every sign-in (`READY`) and organization selection landed in setup. Reproduced before the fix: an under-review company signing in landed on `/setup/submit` (`screens/before-under-review-sign-in-lands-in-setup.png`).
3. **Setup never released submitted companies.** The setup layout rendered "Set up <company>", the checklist and a "being reviewed" card for any status.
4. **Sign-in fallback guessed "draft".** If `/organizations/current` failed during sign-in, the app set the `DRAFT` cookie and went to `/setup`.
5. **Dashboard trusted stale state.** On a failed status read it rendered with the persisted organization store. The pending banner was commented out, and only `DRAFT` was redirected.
6. **No server-side lifecycle restriction.** Tenant resolution ignores lifecycle; `IsOperational` existed but was unused. The eligibility gate covers only provider onboarding, marketplace assignments, finance and activation, so a pending company's owner could create orders, dispatch plans, assets and so on through the API.
7. **Found during live verification:** a session-marker cookie that outlives the client token (for example, site data partly cleared) produced an endless spinner. The dashboard sent the user to `/sign-in` and the middleware sent them straight back to `/`. This predates this change and is fixed here.

## 2. Organization state mapping (one policy: `web/fleet/src/lib/auth/organizationDestination.ts`)

Derived from the backend lifecycle (`organizations/usecases/lifecycle.go`). There is no organization-level `APPROVED` or `REJECTED`: approval activates the company (`ACTIVE`), and a review that needs more returns it to `CHANGES_REQUIRED`. `PENDING_REVIEW` still accepts onboarding edits server-side (`AcceptsOnboardingEdits`); it is the legacy pre-submission draft, not a submitted company.

| Status | Access | Destination | May operate |
|---|---|---|---|
| no company | – | `/setup/create-company` | no |
| DRAFT, PENDING_REVIEW, CONTACT_VERIFIED | onboarding | `/setup` | no |
| CHANGES_REQUIRED | corrections | `/setup` (editable) | no |
| BUSINESS_SUBMITTED, UNDER_REVIEW | pending | `/` dashboard + banner | no |
| ACTIVE | operational | `/` dashboard | yes |
| SUSPENDED, CLOSED, ARCHIVED | restricted | `/` dashboard + restricted banner* | no |
| anything else | unknown | `/` dashboard, restricted, re-read | no |

\*The backend revokes sessions of suspended, closed and archived companies on refresh and on organization selection (`auth/usecases/session_flows.go`), so in practice these users are signed out. That security control was kept.

Used by verify (sign-in), choose-organization, the setup layout, the submit step and the dashboard layout. The decision is always made from a fresh `/organizations/current` read, never from a cookie or the persisted store.

## 3. Changes

**Frontend (`web/fleet`)**
- `src/lib/auth/organizationDestination.ts`: the lifecycle policy (`organizationAccess`, `organizationDestination`, `belongsInSetup`, `canOperate`, `isLifecycleRefusal`), plus `organizationDestination.test.mjs`.
- `src/middleware.ts`: no longer routes on the status cookie, and deletes it from any request that carries it.
- `src/app/(auth)/verify/page.tsx`, `choose-organization/page.tsx`: no status cookie; route through the policy. A failed status read goes to the dashboard, which re-reads, instead of guessing "draft".
- `src/app/(setup)/setup/layout.tsx`: any setup URL for a company that does not belong in setup redirects, rendering only the loading indicator (no flash of setup).
- `src/app/(setup)/setup/submit/page.tsx`: after a successful submission, re-reads the status and continues to the dashboard.
- `src/app/(setup)/setup/page.tsx`, `useSetupStatus.ts`: removed the unreachable "Your company is being reviewed" card. Draft companies see the same page, pixel-identical.
- `src/app/(setup)/setup/company-review.tsx`: the review sections extracted once and shared by the submit step and the dashboard company page. Read-only mode adds document Preview.
- `src/app/(dashboard)/layout.tsx`: access comes from a fresh status read. It shows a spinner until access is decided, then a retryable error rather than a stale fallback. It renders the existing `PendingVerificationBanner`, re-reads status on focus (at most once a minute) and every 5 minutes until active, and hosts the restriction notice.
- `src/components/features/PendingVerificationBanner.tsx`: the specified wording, a restricted variant and a "View submission" link; same styling.
- `src/components/features/RestrictedActionNotice.tsx` and `src/lib/api/client.ts`: a backend lifecycle refusal anywhere in the dashboard opens **"Company verification required"** / "This feature will be available after your company is approved." (existing card and button styles).
- `src/app/(dashboard)/company/page.tsx`: the submitted information and verification status, read only, with document preview.
- `src/lib/auth/sessionMarker.ts`: ends the orphaned session-marker loop.

**Backend (`services/fleet-partner-service`)**
- `internal/middleware/organization_lifecycle.go`, `internal/app/organization_status.go`, `internal/app/app.go`, `cmd/main.go`: `RequireOperationalOrganization` on all tenant routes (section 4).
- `internal/platform/riskregistry/registry.go`, `objectauth/registry.go`, `section28contract/route_coverage_test.go`: classified 11 live onboarding routes that were mounted but unregistered, as `STANDARD`, which is the limit they already fell back to.

## 4. Backend access-control verification

`RequireOperationalOrganization` runs after tenant resolution. Reads (`GET`/`HEAD`/`OPTIONS`) are unaffected, since they are already confined to the caller's organization. A state-changing request from a non-`ACTIVE` company is refused (403) unless it is one of 23 pre-activation routes:
- onboarding edits, uploads, submission and wallet-provisioning retry;
- the closure workflow (its own lifecycle rules apply);
- staff invitations, suspend and reactivate;
- security settings and session revocation;
- support cases.

Refusal codes: `organization_verification_pending` (submitted or under review), `organization_changes_required`, `organization_onboarding_incomplete`, or `organization_not_eligible` for anything else.

The status is read from the database per request, so approval takes effect immediately and a client cannot claim a status. A failed read refuses (503). Every exception is checked by a test to be a real registered route. Tenant isolation is unchanged: other organizations get 404 on read and write.

## 5. Tests and results

- Frontend unit: `npm test` **38/38** (9 new policy tests: every status, legacy `PENDING_REVIEW`, unknown statuses fail safe, refusal detection).
- Backend unit: `internal/middleware/organization_lifecycle_test.go`: all 9 lifecycle statuses, reads open, pre-activation routes allowed, `ACTIVE` allowed, fail-closed on status read error, exceptions are registered routes.
- PostgreSQL integration `TestPendingVerificationAccessAndLifecycleGate`: sign-in after submission, reads, refused direct writes, permitted account management, corrections allowed, approval unlocking the same session (verified by the providers validation response, which proves the request passed the gate), cross-organization 404. A mutation check (removing the gate from the router) makes it fail.
- Browser journey `TestOnboardingV2BrowserWorkflow` (CI), **15/15 checks** (`browser-results.json`). New checks:
  - submission enters the dashboard with the banner;
  - refresh, direct `/`, `/setup`, `/setup/submit`, `/setup/business-docs` and `/setup/profile` all stay on the dashboard;
  - a stale `DRAFT` cookie and a tampered stored status are ignored, and the cookie is removed;
  - a restricted action shows "Company verification required", and a direct API call gets 403 `organization_verification_pending`;
  - the read-only `/company` page with preview works, and there is no mobile overflow;
  - sign-out, then phone sign-in, lands on the dashboard; independent Android-emulated email sign-in lands on the dashboard;
  - organization switching (draft to setup, submitted to dashboard, no cross-tenant data);
  - a slow status API shows neither setup nor a stale dashboard;
  - an expired access token refreshes and stays on the dashboard;
  - changes requested returns to setup; back under review returns to the dashboard;
  - approval removes the banner and the API restriction in the same session;
  - an orphaned session marker reaches sign-in.
- Auth browser workflow: pass. Full Fleet Go suite (`go test ./... -p 1`, PostgreSQL + Redis): **1452 passed, 0 failed**. The 10 skips need CI-provisioned databases and containers and are unrelated. CI ESLint command, build and typecheck: pass.

## 6. UI regression

`web/fleet/scripts/dashboard-screenshots.mjs` captured the original build (`c18b9c4`) and this change against the same API (`visual-regression.json`):

| Capture (desktop and mobile) | Pixels differing |
|---|---|
| Draft company setup home | 0 |
| Active company dashboard | 0 |
| Under-review dashboard with the banner removed vs the original dashboard | 0 |

The only visible additions are the banner, the restriction notice (shown only on a refused action) and the `/company` page reached from the banner. No layout, sidebar, header, typography, color or spacing changed.

## 7. Commits

- `d3406aa` feat(fleet): server-side lifecycle gate for companies awaiting verification
- `0d5c8f0` fix(fleet-web): submitted companies enter the dashboard while under review
- `4579f1a` fix(fleet-web): end the sign-in loop when a session marker outlives its token
- this report and its evidence (documentation only)

## 8. GitHub Actions

- `0d5c8f0`: [Deploy staging (automatic)](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43445959610) success; [Fleet authentication v2](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43445959363) success (frontend build/lint/tests, auth and onboarding browser workflows); [Deployment safety](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43445959502) success. Its Section 31 run was cancelled by the newer push.
- `4579f1a`: [Section 31 mandatory acceptance](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43447084787) and [Fleet authentication v2](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43447085003): results recorded in section 12.

## 9. Deployment evidence

- **Backend:** at 14:53:33 UTC both `karrygo-fleet-partner-api` and `karrygo-fleet-partner-worker` ran image `sha256:54faec10e4f69098ac6012b05655df83654c4ee0c5d90516221b4e6859fea8ee`, revision `0d5c8f0cc9df0445011afe6f1412ade129e06fe5` (includes the gate from `d3406aa`), API health `healthy`, after the workflow's pre-deploy database backup. `4579f1a` changes only the frontend.
- **Frontend (Vercel, `fleet-karryam.vercel.app`):** the live chunks for `/` and `/company` contain the new banner copy, the restriction notice and the lifecycle refusal code. Live behaviour changed exactly when each commit deployed: the stale-cookie redirect disappeared at 14:48:48 UTC (`0d5c8f0`), and the orphaned-marker loop ended at 14:55:54 UTC (`4579f1a`). The Vercel deployment ID was not read, because this session has no Vercel API credential.

## 10. Live staging results (no account required)

`live-stale-cookie-results.json`, `live-orphan-session-results.json`:
- Before deployment: `GET /` with `karrygo_has_session=1; karrygo_org_status=DRAFT` returned **307 → /setup**, the original trap. After: **200**, with `Set-Cookie: karrygo_org_status=; Expires=1970…` deleting the stale cookie.
- In a real browser (desktop and mobile) carrying the old stale cookie, `/`, `/setup` and `/setup/submit` never reach any setup page, never show "Set up …", and end on sign-in rather than looping.

## 11. Remaining blockers

- **Authenticated live walk-through on staging.** Signing in to staging needs an email OTP for an authorized staging account, which this session does not have (the blocker recorded in the previous closures). Still to observe on the live URL with a real submitted company: dashboard and banner after sign-in, refresh, email and phone sign-in, the restricted-action notice, and approval. All of these pass in the CI browser journey against PostgreSQL. To finish: provide a staging test inbox OTP, or sign in in a connected browser.
- No physical Android device was used; Android is Chromium device emulation.

## 12. CI results for the final commit

- [Fleet authentication v2 on `4579f1a`](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43447085003): **success** (frontend tests, build, lint, auth and full onboarding browser workflows including every scenario in section 5).
- [Section 31 mandatory acceptance on `4579f1a`](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/43447084787): **success**, every job including `fleet-mandatory` (full Fleet PostgreSQL/Redis suite with the skip gate), `static-and-contract`, security scans, all Go modules and provider apps.
