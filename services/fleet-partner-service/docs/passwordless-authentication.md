# Fleet authentication v2

Fleet uses email OTP only. Registration requires a unique email and E.164 phone. Sign-in accepts one identifier: email or phone. A phone is an account lookup association; no phone ownership or SMS verification is performed. Existing identity IDs, organization memberships and authorization boundaries remain intact.

## Public browser contracts

All paths start with `/fleet-partner/v1`. Browser requests require an exact trusted Origin. Success bodies use `{ data, meta }`; errors use the existing structured error envelope.

| Route | Request | Result |
| --- | --- | --- |
| POST /auth/sign-up/start | email, phone, optional invitationToken | 202 accepted email challenge; 409 email_already_registered / phone_already_associated before OTP generation; 422 field validation |
| POST /auth/sign-in/start | identifier | 202 existing identity challenge; 401 sign_in_unavailable for unknown, restricted or unverified identity; never creates an identity |
| POST /auth/start | registration DTO | Deprecated signup-only alias, including separate staff invitation purpose when a token is present; cannot log into an existing account |
| POST /auth/verify | challengeId, code, optional deviceLabel | Session and nextStep, or specific invalid/expired/superseded/consumed/attempt-limit error |
| POST /auth/resend | challengeId | Replacement challenge ID and deadlines; previous code/challenge unusable |
| POST /auth/refresh | HttpOnly refresh cookie | Rotating session; refresh reuse contains the family |
| POST /auth/logout | HttpOnly refresh cookie | Revokes session and clears cookies |

Challenge results include `challengeId`, `deliveryChannel: EMAIL`, maskedDestination, expiresAt, resendAvailableAt, expiresInSeconds and resendAfterSeconds. Delivery acceptance means Notification Service accepted the request; it is not an inbox-delivery receipt. Notification failure returns 503 otp_delivery_unavailable. A rapid repeat may return the already accepted pending challenge without another email, or 429 while delivery is pending. Verification uses the absolute backend deadline.

Registration checks both identifiers before generating a cryptographically secure six-digit OTP. No identity is created until successful verification. Verification rechecks uniqueness in a transaction and database unique indexes remain authoritative under concurrency. Existing email displays Sign In and Try Another Email; phone conflicts never identify its owner. Archived identifiers remain reserved by API policy.

Sign-in normalizes one identifier, resolves an active eligible identity with verified registered email, and binds the challenge to that identity and SIGN_IN purpose. Phone sign-in reads its email destination exclusively from the database. Unknown/restricted/unverified identifiers share one safe failure and signup/support path, without usable challenges or notifications. Rate limits apply to lookup, normalized contacts and source IP.

## Authenticated contracts

`POST /auth/select-organization`, `/auth/step-up/start`, `/auth/step-up/verify`, `GET /sessions`, `DELETE /sessions/:sessionId` retain their roles and session checks. Staff invitations retain token/contact-pair matching and transactional acceptance; single/multiple/no organization memberships retain READY / ORGANIZATION_SELECTION_REQUIRED / ONBOARDING_REQUIRED navigation.

To change the associated phone securely:

1. In the existing authenticated session, normalize the desired phone to E.164.
2. POST /auth/step-up/start with `{ "actionClass": "phone_change:<E.164>" }`. The code is emailed to the existing registered email, using existing step-up infrastructure.
3. Verify that challenge through /auth/step-up/verify and retain stepUpReference.
4. POST /auth/phone/change with `{ "phone": "<E.164>", "stepUpReference": "..." }` in the same session/organization. Exact phone binding, current identity/session eligibility, grant expiry, uniqueness and optimistic version are checked transactionally. Successful update spends the grant once and records IDENTITY_PHONE_CHANGED. It returns phoneAssociated=true and phoneVerified=false. It preserves the identity and memberships and cannot move a phone from another identity.

## Persistence and security

Migration 000085 adds SIGN_UP/SIGN_IN to the existing challenge-purpose CHECK, adds a lower(email_normalized) partial unique index, and supersedes pre-upgrade pending/expired challenges because purpose-bound OTP digests have changed. Existing verified step-up grants and staff invitation tokens are retained. It creates no replacement identity tables. Case-colliding legacy records cause migration to fail for operator resolution, never silent merging. Existing phone uniqueness stays unchanged.

OTP HMAC binds challenge ID, purpose, phone association and code; no raw OTP is stored by Fleet. PostgreSQL advisory locks serialize contact transitions across replicas. Challenge row locks, expiration and attempt limits prevent replay and brute force. Resend rechecks state/cooldown/eligibility transactionally, replaces the challenge and notification idempotency key, and preserves original destinations/bindings. Rate-limit infrastructure failure is unavailable, not unlimited access. Refresh secrets remain HttpOnly cookies; no JSON contract exposes them.

The notification adapter sends only EMAIL through existing corporate.auth.otp templates and fallback content. There is no SMS integration. Existing Notification Service retry/status/idempotency controls remain responsible for delivery.

## Verification

See `docs/implementation/fleet-auth-v2-audit.md` and `fleet-auth-v2-final-closure.md` at repository root for audit/deployment evidence. `.github/workflows/fleet-auth-v2.yml` runs auth security/contract tests, PostgreSQL regressions, frontend checks/build and an actual Chromium → Next.js proxy → Fleet API → PostgreSQL workflow. The local browser test uses a test-only sender fixture and is distinct from live staging mail acceptance.

The browser harness is compiled with `-tags fleet_auth_browser` and requires `FLEET_AUTH_BROWSER_TEST=1`. The dedicated auth CI workflow runs it mandatorily; the default Go-only suite does not report it as an unexpected skip. Fleet runtime and these CI gates use Go 1.26.11 to include the November 2026 net/http security fix (GO-2026-6859).
