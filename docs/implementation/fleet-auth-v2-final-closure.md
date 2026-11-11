# Fleet authentication v2 final closure

**Status: implemented, tested and deployed to staging; live authentication certification is BLOCKED pending operator-provided email OTP.** Do not interpret this report as a completed live signup/sign-in/onboarding certificate. No production service was deployed and no merge into main was made.

## Audit and confirmed old-system problems

The [audit](fleet-auth-v2-audit.md) covers staging baseline `6bf43b282db17c8fd3c625d4eaeedbebb9ce4b92`, Fleet web/API, identity and challenge migrations, Notification Service and deployment callers. Confirmed problems were shared signup/login intent; requiring both identifiers for login; sending OTPs before duplicate/restricted-account rejection; sign-in being able to create identities; no single-identifier lookup; concurrent start/resend gaps; and resending with the original notification idempotency key. Existing session/organization/invitation infrastructure was reused. A later compatibility check also found that staff invitation eligibility was not checked until OTP verification; it is now checked before delivery as well.

## Final architecture and flows

One Fleet identity retains its original ID, one unique primary email and one unique associated E.164 phone, and may belong to several organizations. Phone association is not phone ownership verification. OTP delivery is EMAIL only through the existing Karryam Notification Service. No SMS provider, SMS OTP, telecom integration or phone-verification prerequisite was added.

Signup requires email and phone, normalizes both, checks both database identifiers before code generation, rejects email/phone conflicts with structured errors and no OTP, and displays Sign In / Try Another Email. Only an eligible registration creates a SIGN_UP challenge. Successful email-code verification rechecks uniqueness transactionally, creates/activates the identity and issues an onboarding session. It cannot authenticate an intervening existing identity. Database uniqueness remains authoritative under concurrent registrations.

Sign-in takes one email-or-phone input with country selection for national formatting. It resolves an existing active eligible identity with verified registered email, binds SIGN_IN to that identity, and takes the delivery email exclusively from the database. Unknown, restricted and unverified identities receive the same safe sign_in_unavailable response and no usable challenge/delivery. Sign-in cannot create an identity. No-membership / one-membership / multiple-membership results retain onboarding / ready / organization-selection navigation.

Staff invitations retain exact contact-pair/token binding, preflight eligibility, transactional acceptance, roles and memberships. Step-up remains identity/session/organization/action bound. Refresh rotation, reuse containment, protected APIs, logout, revocation and identity restrictions remain. Phone changes use an authenticated email step-up grant bound to the exact desired E.164 phone; the same active session/organization must spend it once. The update preserves identity/memberships, checks uniqueness/current authorization/version transactionally, records an event and explicitly reports phoneAssociated=true, phoneVerified=false.

## API contracts

All routes are under `/fleet-partner/v1`; browser requests retain the trusted-Origin boundary and canonical success/error envelopes. See the [complete contract](../../services/fleet-partner-service/docs/openapi/browser-authentication.openapi.json) and [flow documentation](../../services/fleet-partner-service/docs/passwordless-authentication.md).

| Method and route | Contract / behavior |
| --- | --- |
| POST /auth/sign-up/start | email, phone, optional valid invitationToken; eligibility before OTP; 409 email_already_registered / phone_already_associated, 422 field validation |
| POST /auth/sign-in/start | identifier only; verified destination from database; 401 sign_in_unavailable for unknown/restricted/unverified identities |
| POST /auth/start | Documented deprecated signup-only alias; no arbitrary client purpose or existing-account login bypass; valid staff invitation remains separate |
| POST /auth/verify | challengeId, code, optional deviceLabel; purpose/identity-bound verification and session/nextStep |
| POST /auth/resend | challengeId; new challenge ID and notification key; stored destination/bindings, state/eligibility/cooldown rechecked |
| POST /auth/refresh | HttpOnly refresh cookie; rotation and family reuse containment |
| POST /auth/logout | Revoke session and clear refresh/routing cookies |
| POST /auth/select-organization | membershipId; persisted active session and scoped membership |
| POST /auth/step-up/start and /verify | Existing authenticated sensitive-action email proof |
| POST /auth/phone/change | phone, stepUpReference; exact-number/session/email proof and single-use grant |
| GET /sessions; DELETE /sessions/:sessionId | Existing session management and ownership controls |

Start/resend return challengeId, EMAIL channel, maskedDestination, absolute expiresAt/resendAvailableAt and relative timing metadata. ACCEPTED means Notification Service acceptance, not a delivery receipt. Errors distinguish otp_invalid, expired, superseded, consumed, missing, attempt limits, cooldown/rate limits and otp_delivery_unavailable. Frontend handles replacement IDs, backend deadlines, rapid clicks, retry/network states, eligibility races and account/delivery failures without mislabeling them as wrong codes.

## Database migration and live schema evidence

Migration `000085_auth_v2` extends the existing challenge-purpose CHECK with SIGN_UP and SIGN_IN, adds the partial lower(email_normalized) unique index, and supersedes pre-upgrade pending/expired challenges because OTP digest binding changed. It creates no replacement identity tables. Existing email/phone indexes, IDs, foreign keys, memberships, invitation tokens and consumed grants remain. Rollback removes only the new expression index and never resurrects old codes. Migration replay/data-retention tests cover both the previous migration and this new tip.

[Read-only staging inspection](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/42977917689) confirmed migration 85, all three expected email/phone unique indexes, expanded purpose CHECK, zero active legacy email-case collision groups and zero malformed legacy phones. The test inbox has zero identities and zero sessions because its OTP was never entered. Its challenges are one SUPERSEDED and one EXPIRED. [Database and delivery output](evidence/fleet-auth-v2/staging-database-and-delivery.txt) contains no codes, digests, tokens or message bodies.

## Notification integration and security

The production adapter still uses the signed shared notification client, corporate.auth.otp template/fallback, existing email transport, retries, status and deduplication. No additional notification provider was introduced. Each resend has a fresh challenge ID/idempotency key. Keyed OTP digests bind challenge ID, purpose, phone association and code; raw codes stay out of Fleet persistence/logs. Contact transaction locks serialize starts/verification across replicas. Challenge row locks, expiry, durable attempts, cooldowns and rate limits prevent replay/abuse. Rate-limit infrastructure errors fail unavailable. Security logs contain hashes/identity references, not raw OTPs or contacts. Refresh secrets remain HttpOnly cookies, not JSON/browser storage.

CI detected GO-2026-6859 in the previously pinned Go 1.26.10 net/http runtime. Fleet Docker builds and relevant CI gates were upgraded to Go 1.26.11, the [official security-fix release](https://go.dev/doc/devel/release#go1.26.11). The resulting full mandatory workflow including vulnerability scanning passes. Only Fleet images were deployed; other service runtime images were not changed by this upgrade.

## Automated testing and results

- Full Fleet Go suite passed with disposable PostgreSQL 18 locally. Optional cross-service/Redis dependencies absent locally were not represented as live acceptance passes.
- Focused real PostgreSQL HTTP/auth/organization/staff regressions passed: duplicate combinations with zero sender calls; normalized identifiers; no identity before verification; same identity via email/phone; sign-in never creates; concurrent starts/registration/verification; invalid/expired/consumed/superseded OTPs; resend replacement/idempotency/cooldown; restrictions applied after challenge issuance; refresh/logout; phone-change proof binding/replay; staff invitation consumption and multi-organization membership.
- Go vet/backend builds passed. A real Chromium → Next.js proxy → Fleet HTTP API → PostgreSQL workflow passed locally, again with Go 1.26.11, and in CI. It covers signup/OTP/session/create-company navigation, HttpOnly refresh/logout, duplicate prompt/carried email, email/phone sign-in and desktop/mobile layouts. Its sender is a fixture; it does not claim real mail delivery or completed company submission.
- Frontend: **20 tests pass**, TypeScript and optimized Next.js build pass; changed authentication sources pass ESLint. Repository-wide lint still reports **223 errors / 800 warnings** in unrelated pre-existing screens. Those were not silently suppressed or expanded into this auth change.
- Full Notification Service Go suite passed locally after normalizing a Windows-only CRLF checkout mismatch for its existing campaign-outbox text assertion. No Notification SQL content change was committed. Its Linux CI regression also passes.
- Browser tests are explicitly built with `-tags fleet_auth_browser` in mandatory auth CI. This removes an unexpected skip from the Go-only suite without skipping the browser verification itself.
- [Latest auth CI](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/42977268195): **success** on frontend/code commit 0b75bd04; includes build, tests, real PostgreSQL and Chromium workflow.
- [Full Section 31 mandatory acceptance](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/42977268297): **success** on 0b75bd04. Fleet test gate: **mandatory skips=0; test events=2956**, followed by targeted gate **mandatory skips=0; test events=72**. Static/security, other module and provider-app jobs also completed successfully.
- [Deployment safety acceptance](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/42977268202): **success**.
- Initial CI health-command quoting and unexpected browser-skip failures were fixed and superseded by these successful runs; no failed/skipped run is represented as successful certification.

Durable evidence: [local results](evidence/fleet-auth-v2/local-results.json), [CI browser results](evidence/fleet-auth-v2/ci-browser-results.json), [deployment/live metadata](evidence/fleet-auth-v2/deployment-and-live-evidence.json), [local mobile](evidence/fleet-auth-v2/mobile-sign-in.png), [desktop](evidence/fleet-auth-v2/desktop-sign-up.png), [actual staging mobile browser](evidence/fleet-auth-v2/staging-mobile-sign-in.png). Staging mobile check used Pixel 7 Chromium touch/mobile emulation with national-number country selection and one input, with no horizontal overflow.

## Git and actual VPS deployment

- Main implementation: `70641f7b496dae7d3d5a8e1e4a9aab0efb195bdd`.
- Patched Fleet backend/security build: `6e7cfb82c398a6c1f0fbb1e2930faf312c9df75a`.
- Final frontend/code revision: `0b75bd0430fd882a518f995b6f12d21e2eb20c22`. Its backend tree is identical to 6e7cfb82 (`1980e6bf6e8eda685e595861acefffbb289dcb26`).
- [Actual backend release](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/42976834692) performed image build/push, a real predeploy database backup, server deployment and health checks. No server step was skipped.
- Running API **and worker** image ID: `sha256:2b5c177f893810f6c440bf2489cbb73537a68fcffa14062f89b36d8342898430`.
- Both running image OCI revision labels: `6e7cfb82c398a6c1f0fbb1e2930faf312c9df75a`; API Docker health: **healthy**. The job compares running image revisions with the intended SHA and fails on mismatch.
- Subsequent frontend/documentation-only pushes legitimately do not deploy a backend; their green/skipped server jobs are not the runtime evidence. Read-only runtime/database/delivery inspection confirmed actual state independently.

## Vercel deployment

Project `prj_kFUDZnnpo98MmSwuDFb73XgHOHBJ`, team `team_uHJL0LU40I2EAoL3QBpby5jf`, root `web/fleet`, Git branch `staging`.

- Deployment `dpl_G55Ws3uLtU6TQUYLYKP2G5A8ZhqX`: **READY**, source `0b75bd0430fd882a518f995b6f12d21e2eb20c22`.
- [Immutable frontend deployment](https://fleet-karryam-4k35b9s97-cosmicforgedevteamgmailcoms-projects.vercel.app).
- Requested [staging alias](https://fleet-karryam.vercel.app) was read back through the alias API and points to that exact deployment. Its rendered sign-in form has one email-or-phone input.
- Frontend source tree: `f314eb6cee72a9543907ed0cc7b05051d9e334e8`. A later documentation-only commit may advance the Git source revision while retaining this exact application tree.
- The existing Fleet Vercel project calls its public slot production internally, but its configured productionBranch is staging and this is the user-authorized staging website. No main/production Karryam service was deployed.
- Queued builds were inspected through final READY and alias assignment. No current provider rate-limit failure was observed; normal supported queues were used, with no limit bypass or false deployment claim.

## Real staging acceptance and remaining blocker

Observed against the actual frontend, Fleet API, database and Notification Service:

1. New email + reserved UK test phone submitted through the actual signup UI and dedicated endpoint returned 202, a SIGN_UP challenge, masked email and absolute deadlines.
2. Notification Service recorded **two email-only corporate.auth.otp messages sent**, each in one attempt: initial signup and a later normalized repeated start after cooldown, which superseded the original challenge. The test inbox received Karryam's verification email from `no-reply@karryam.com`.
3. Unknown email and unknown phone sign-in returned the same 401 sign_in_unavailable response. [Independent unknown-email inspection](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/42978404507) shows no identity, challenge or notification records for that identifier: [output](evidence/fleet-auth-v2/staging-unknown-identifier.txt).
4. Original superseded challenge, missing challenge and naturally expired replacement returned the correct distinct 401 errors. The actual page showed expiration/request-new-code state from backend deadlines.
5. Database inspection confirms zero test identities/sessions before successful code verification, rather than premature account creation.
6. Actual staging mobile Chromium emulation renders one input and country selection without overflow.

**Blocker:** the connected Gmail reader replaces OTP values with `[one-time authentication code withheld]`. The operator was asked for the code; none was supplied before expiry. Codes were not extracted through another channel, guessed, bypassed or fabricated. There is no authenticated live test session.

**Precise remaining operator action:** when available, reply `ready` so a fresh test OTP can be requested for the connected inbox. Supply that code when prompted, followed by the email-sign-in and phone-sign-in email codes. Then complete/observe signup verification → identity/session → company onboarding → logout → email sign-in → logout → phone sign-in, live refresh/dashboard/organization checks and applicable staff invitation acceptance. No SMS code or provider is needed. The existing expired test challenge can be replaced; do not enter a stale code.

The following are intentionally **not certified on live staging yet**: successful OTP-to-session verification, registered email and phone sign-in, consumed-code reuse rejection after a real successful session, live company onboarding/dashboard access, and live authenticated organization/staff/refresh/logout workflow. These have automated PostgreSQL/browser coverage; that is not a substitute for the requested real staging acceptance. All non-negotiable criteria therefore cannot be checked complete at this time.

## Files created or modified

- `.github/workflows/activate-fleet-origin.yml`
- `.github/workflows/deploy-staging-auto.yml`
- `.github/workflows/fleet-auth-staging-evidence.yml`
- `.github/workflows/fleet-auth-v2.yml`
- `.github/workflows/section31-mandatory.yml`
- `docs/implementation/evidence/fleet-auth-v2/ci-browser-results.json`
- `docs/implementation/evidence/fleet-auth-v2/deployment-and-live-evidence.json`
- `docs/implementation/evidence/fleet-auth-v2/desktop-sign-up.png`
- `docs/implementation/evidence/fleet-auth-v2/local-results.json`
- `docs/implementation/evidence/fleet-auth-v2/mobile-sign-in.png`
- `docs/implementation/evidence/fleet-auth-v2/results.json`
- `docs/implementation/evidence/fleet-auth-v2/staging-database-and-delivery.txt`
- `docs/implementation/evidence/fleet-auth-v2/staging-mobile-sign-in.png`
- `docs/implementation/evidence/fleet-auth-v2/staging-unknown-identifier.txt`
- `docs/implementation/fleet-auth-v2-audit.md`
- `docs/implementation/fleet-auth-v2-final-closure.md`
- `scripts/fleet/check-auth-origin.sh`
- `scripts/fleet/staging-auth-evidence.sh`
- `services/fleet-partner-service/Dockerfile`
- `services/fleet-partner-service/docs/openapi/browser-authentication.openapi.json`
- `services/fleet-partner-service/docs/passwordless-authentication.md`
- `services/fleet-partner-service/docs/security.md`
- `services/fleet-partner-service/internal/features/auth/http/handlers.go`
- `services/fleet-partner-service/internal/features/auth/models/models.go`
- `services/fleet-partner-service/internal/features/auth/repositories/challenges.go`
- `services/fleet-partner-service/internal/features/auth/repositories/identities.go`
- `services/fleet-partner-service/internal/features/auth/usecases/auth_service.go`
- `services/fleet-partner-service/internal/features/auth/usecases/phone_change.go`
- `services/fleet-partner-service/internal/features/auth/usecases/session_flows.go`
- `services/fleet-partner-service/internal/features/auth/usecases/verify.go`
- `services/fleet-partner-service/internal/features/staff/usecases/staff_service.go`
- `services/fleet-partner-service/internal/integration/auth_browser_test.go`
- `services/fleet-partner-service/internal/integration/auth_http_test.go`
- `services/fleet-partner-service/internal/integration/auth_invitation_v2_test.go`
- `services/fleet-partner-service/internal/integration/auth_test.go`
- `services/fleet-partner-service/internal/integration/auth_v2_test.go`
- `services/fleet-partner-service/internal/platform/contact/contact.go`
- `services/fleet-partner-service/internal/platform/errorcodes/errorcodes.go`
- `services/fleet-partner-service/internal/platform/riskregistry/registry.go`
- `services/fleet-partner-service/internal/section28contract/route_coverage_test.go`
- `services/fleet-partner-service/internal/section30contract/migration_replay_test.go`
- `services/fleet-partner-service/migrations/000085_auth_v2.down.sql`
- `services/fleet-partner-service/migrations/000085_auth_v2.up.sql`
- `web/fleet/package-lock.json`
- `web/fleet/package.json`
- `web/fleet/scripts/auth-e2e.mjs`
- `web/fleet/src/app/(auth)/sign-in/page.tsx`
- `web/fleet/src/app/(auth)/verify/page.tsx`
- `web/fleet/src/components/features/auth/ContactAuthForm.tsx`
- `web/fleet/src/components/features/auth/SignInForm.tsx`
- `web/fleet/src/lib/api/client.ts`
- `web/fleet/src/lib/auth/authErrors.test.mjs`
- `web/fleet/src/lib/auth/authErrors.ts`
- `web/fleet/src/lib/auth/challengeErrors.test.mjs`
- `web/fleet/src/lib/auth/challengeErrors.ts`
- `web/fleet/src/lib/hooks/useContactAuthForm.ts`
- `web/fleet/src/lib/services/authService.ts`
- `web/fleet/src/lib/stores/authStore.ts`
- `web/fleet/src/lib/types/auth.ts`
