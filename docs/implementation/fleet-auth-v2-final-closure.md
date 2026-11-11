# Fleet authentication v2 closure evidence

Status: implementation/local verification complete; staging deployment and live mail acceptance pending. This file is updated after deployment with actual evidence. No live staging pass is claimed at this checkpoint.

## Audit and architecture

See [audit](fleet-auth-v2-audit.md) and the updated [API/architecture documentation](../../services/fleet-partner-service/docs/passwordless-authentication.md). The audit confirmed shared registration/login behavior, delivery before eligibility, shared sign-in creating identities, missing one-identifier lookup and resend idempotency reuse. V2 separates SIGN_UP and SIGN_IN, uses the existing corporate identity tables and Notification Service, and retains staff invitations, multi-organization membership, session rotation and step-up. All OTPs are EMAIL. Phone association never claims SMS ownership verification.

Signup validates/normalizes both identifiers, checks eligibility before code generation, returns explicit email/phone conflicts, and creates the identity only after email OTP verification with transactional uniqueness recheck. Sign-in accepts email or phone, reads delivery email from the existing verified active identity, and never creates an identity. A registration challenge cannot authenticate an intervening account. Unknown/restricted sign-in returns the same safe error and no OTP. Staff invitation eligibility is validated before delivery and again during atomic acceptance.

## API and database

The updated browser OpenAPI lists dedicated /auth/sign-up/start and /auth/sign-in/start, signup-only deprecated /auth/start, verify/resend/refresh/logout/organization selection/step-up and authenticated /auth/phone/change. Requests and response/error contracts are documented in passwordless-authentication.md. Phone changes require a single-use email step-up grant bound to the exact new E.164 number, identity, session and organization; uniqueness and current authorization are checked in the update transaction.

Migration 000085 expands purpose CHECKs, adds a lower(email_normalized) partial unique index, preserves the authoritative phone index, and supersedes pre-upgrade pending/expired OTP challenges because purpose-bound HMACs changed. Existing identity IDs, memberships, consumed grants and invitation tokens remain. No replacement identity table was introduced. Legacy case collisions fail migration rather than merging accounts. Rollback/reapply tests preserve existing data and never resurrect old codes.

## Notification and security

Existing corporate.auth.otp templates, fallback content, signed service client, email provider, retries/status and deduplication remain. Each replacement resend has a fresh challenge ID/idempotency key. Acceptance is reported separately from actual inbox delivery. No SMS channel/provider was added. Purpose-bound HMACs, per-contact transaction locks, row locks, expiry, attempts, cooldowns, rate limits, security events, HttpOnly cookies and session replay containment remain or were strengthened. Rate-limit service errors fail unavailable. Rejected duplicate registration performs zero OTP generation/delivery.

## Local verification

- Full Fleet Go suite with disposable PostgreSQL 18: passed, including integration tests and migration replay. The full run preceded the final invitation preflight extraction; its new real-PostgreSQL invitation/multi-organization test and affected-package regressions also passed afterward.
- Focused authentication regression: 24 top-level passing tests; a Redis-specific test skipped because local Redis is unavailable. New staff invitation/multiple-organization regression passed. Required cross-service tests without their optional databases report skips; these are not live acceptance evidence.
- Go vet and backend builds passed.
- Full Notification Service Go suite passed after normalizing a Windows-only CRLF checkout mismatch locally; no Notification SQL content changed.
- Frontend: 19 tests passed; Next.js optimized build and TypeScript checks passed. Changed auth sources pass ESLint. Repository-wide lint remains at 223 errors/800 warnings in unrelated pre-existing screens; no blanket cleanup was performed.
- Actual Chromium → Next.js proxy → Fleet HTTP API → PostgreSQL test passed: signup/OTP/session/company-setup route, cookie refresh/logout, duplicate prompt and carried email, email/phone sign-in, mobile/desktop form layout. Its sender is a fixture and its onboarding boundary is the create-company route, not live company submission. It is explicitly separate from real Notification Service acceptance.
- Sanitized evidence: [local results](evidence/fleet-auth-v2/local-results.json), [browser checks](evidence/fleet-auth-v2/results.json), [mobile](evidence/fleet-auth-v2/mobile-sign-in.png), [desktop](evidence/fleet-auth-v2/desktop-sign-up.png).

## Deployment and live evidence

Implementation commit, Actions results, immutable VPS runtime image/revision, Vercel deployment/alias revision, actual mail registration/sign-in and any final blockers are pending this checkpoint. The automatic staging job now verifies actual Fleet API and worker image labels against the deployed SHA and API Docker health. A green job that skipped deployment is explicitly insufficient. Vercel Fleet project was verified as prj_kFUDZnnpo98MmSwuDFb73XgHOHBJ, root web/fleet, staging Git source; its Vercel slot is called production internally but the authorized business environment/site is staging only.

## Files

Changes span auth usecases/models/repositories/HTTP handlers, staff invitation validation, contact normalization/error/risk registries, migration 85 and replay/route contracts, Go PostgreSQL/HTTP/browser tests, browser OpenAPI/security/auth docs, signup and single-identifier sign-in UI/hooks/API/types/errors/store/OTP page, Playwright test/dependency, origin probes, staging deployment runtime evidence and the dedicated Fleet auth CI workflow. No main merge or production environment deployment is authorized.
