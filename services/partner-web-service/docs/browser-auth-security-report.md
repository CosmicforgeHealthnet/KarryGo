# Fleet / Partner browser authentication security report

Repository: C:\KarryGoPush. Branch: staging.
Baseline and final HEAD: `2e46bcd5b557f4ae559668d519993f84401dd5f0`.
No commit, stage, push, pull, branch change, stash, reset, or cleanup was performed.

## 1. Security verdict before

FAIL for the requested browser contract. Fleet returned raw refresh credentials
in JSON and accepted them from JSON. Partner-web persisted its Gateway access
session using Zustand/localStorage and had no cookie refresh/restoration flow.

After this change: browser credential boundaries IMPLEMENTED; available non-DB
security tests PASS. Production/real-database/browser deployment certification
is still PENDING, not a production security PASS.

## 2. Root cause and discovered topology

The names concealed two deliberately separate existing account domains:

- partner-web -> Gateway /api/v1/partner/v1/auth/* -> Gateway partner_accounts and
  partner_sessions (merchant shipping, API keys, billing).
- Fleet corporate web contract -> Fleet corporate identities, organizations,
  memberships and corporate session families.

Gateway is not a Fleet auth proxy. Gateway's README, partneraccount model,
repositories, OTP/password login and composition root prove it owns the existing
merchant dashboard session. The authority exception explicitly allowed by the
user was used. Fleet remains sole authority for Fleet corporate sessions.
No Gateway -> Fleet call, account migration, duplicate session persistence or
new authentication system was invented. Accordingly, no report here claims an
executed browser -> Gateway -> Fleet authentication test; that chain does not
exist in this repository's Partner dashboard.

## 3. Exact files changed

These 55 files belong to this authentication task (including this report).
Unrelated existing dirty files, including Fleet's metrics alias, were preserved.
No Go module/dependency lockfile or real .env secret file was changed.

| File | Purpose |
| --- | --- |
| [services/api-gateway/internal/config/config.go](<C:/KarryGoPush/services/api-gateway/internal/config/config.go>) | Central browser security policy configuration |
| [services/api-gateway/cmd/main.go](<C:/KarryGoPush/services/api-gateway/cmd/main.go>) | Wire existing authority and scoped CORS |
| [services/api-gateway/internal/features/partneraccount/model.go](<C:/KarryGoPush/services/api-gateway/internal/features/partneraccount/model.go>) | Existing account/session contract integration |
| [services/api-gateway/internal/features/partneraccount/repository.go](<C:/KarryGoPush/services/api-gateway/internal/features/partneraccount/repository.go>) | Existing account/session contract integration |
| [services/api-gateway/internal/features/partneraccount/service.go](<C:/KarryGoPush/services/api-gateway/internal/features/partneraccount/service.go>) | Existing account/session contract integration |
| [services/api-gateway/internal/features/partneraccount/signup.go](<C:/KarryGoPush/services/api-gateway/internal/features/partneraccount/signup.go>) | Existing account/session contract integration |
| [services/api-gateway/internal/features/partneraccount/refresh.go](<C:/KarryGoPush/services/api-gateway/internal/features/partneraccount/refresh.go>) | Opaque refresh issuance/rotation/revocation |
| [services/api-gateway/internal/features/partneraccount/handlers.go](<C:/KarryGoPush/services/api-gateway/internal/features/partneraccount/handlers.go>) | Emit/read/expire refresh cookie; never serialize secret |
| [services/api-gateway/internal/features/partneraccount/routes.go](<C:/KarryGoPush/services/api-gateway/internal/features/partneraccount/routes.go>) | Cookie refresh/logout and origin boundary routes |
| [services/api-gateway/internal/features/partneraccount/fakes_test.go](<C:/KarryGoPush/services/api-gateway/internal/features/partneraccount/fakes_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/api-gateway/internal/features/partneraccount/routes_test.go](<C:/KarryGoPush/services/api-gateway/internal/features/partneraccount/routes_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/api-gateway/internal/features/partneraccount/browser_session_test.go](<C:/KarryGoPush/services/api-gateway/internal/features/partneraccount/browser_session_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/api-gateway/internal/features/partner/infra/postgres_sessions.go](<C:/KarryGoPush/services/api-gateway/internal/features/partner/infra/postgres_sessions.go>) | Atomic hashed refresh rotation in existing Gateway session rows |
| [services/api-gateway/internal/features/partner/infra/session_rotation_test.go](<C:/KarryGoPush/services/api-gateway/internal/features/partner/infra/session_rotation_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/api-gateway/migrations/browser_refresh_test.go](<C:/KarryGoPush/services/api-gateway/migrations/browser_refresh_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/fleet-partner-service/internal/config/config.go](<C:/KarryGoPush/services/fleet-partner-service/internal/config/config.go>) | Central browser security policy configuration |
| [services/fleet-partner-service/internal/app/wiring.go](<C:/KarryGoPush/services/fleet-partner-service/internal/app/wiring.go>) | Wire existing authority and scoped CORS |
| [services/fleet-partner-service/internal/features/auth/usecases/verify.go](<C:/KarryGoPush/services/fleet-partner-service/internal/features/auth/usecases/verify.go>) | Refresh field serialization exclusion |
| [services/fleet-partner-service/internal/features/auth/usecases/session_flows.go](<C:/KarryGoPush/services/fleet-partner-service/internal/features/auth/usecases/session_flows.go>) | Cookie-credential logout and locked family revocation |
| [services/fleet-partner-service/internal/features/auth/http/handlers.go](<C:/KarryGoPush/services/fleet-partner-service/internal/features/auth/http/handlers.go>) | Emit/read/expire refresh cookie; never serialize secret |
| [services/fleet-partner-service/internal/features/auth/http/browser_cookie_test.go](<C:/KarryGoPush/services/fleet-partner-service/internal/features/auth/http/browser_cookie_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/fleet-partner-service/internal/features/providers/http/internal.go](<C:/KarryGoPush/services/fleet-partner-service/internal/features/providers/http/internal.go>) | Resolve conflicting provider wildcard path |
| [services/fleet-partner-service/internal/integration/auth_http_test.go](<C:/KarryGoPush/services/fleet-partner-service/internal/integration/auth_http_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/fleet-partner-service/internal/integration/browser_refresh_test.go](<C:/KarryGoPush/services/fleet-partner-service/internal/integration/browser_refresh_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/fleet-partner-service/internal/section30contract/compatibility_contract_test.go](<C:/KarryGoPush/services/fleet-partner-service/internal/section30contract/compatibility_contract_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/fleet-partner-service/cmd/browser_routes_test.go](<C:/KarryGoPush/services/fleet-partner-service/cmd/browser_routes_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [shared/go/browserauth/cookie.go](<C:/KarryGoPush/shared/go/browserauth/cookie.go>) | Shared stateless cookie/origin boundary (no session storage) |
| [shared/go/browserauth/cookie_test.go](<C:/KarryGoPush/shared/go/browserauth/cookie_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [shared/go/providercompat/client.go](<C:/KarryGoPush/shared/go/providercompat/client.go>) | Correct existing membership-summary client path |
| [shared/go/providercompat/client_test.go](<C:/KarryGoPush/shared/go/providercompat/client_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/api-gateway/internal/config/browser_auth_test.go](<C:/KarryGoPush/services/api-gateway/internal/config/browser_auth_test.go>) | Focused auth/config/rotation/cookie/router/security evidence |
| [services/fleet-partner-service/internal/features/auth/repositories/sessions.go](<C:/KarryGoPush/services/fleet-partner-service/internal/features/auth/repositories/sessions.go>) | Existing family lock and logout revocation |
| [services/api-gateway/.env.example](<C:/KarryGoPush/services/api-gateway/.env.example>) | Cookie/origin/access/refresh TTL configuration |
| [services/api-gateway/README.md](<C:/KarryGoPush/services/api-gateway/README.md>) | Link browser-only session contract |
| [services/api-gateway/docs/browser-authentication.md](<C:/KarryGoPush/services/api-gateway/docs/browser-authentication.md>) | Authority, deployment, security, compatibility and certification notes |
| [services/api-gateway/docs/partner-browser-authentication.openapi.json](<C:/KarryGoPush/services/api-gateway/docs/partner-browser-authentication.openapi.json>) | Browser auth contract supplement |
| [services/api-gateway/migrations/010_partner_refresh_cookie.sql](<C:/KarryGoPush/services/api-gateway/migrations/010_partner_refresh_cookie.sql>) | Digest column, unique index and hash-shape check on existing sessions |
| [services/api-gateway/migrations/010_partner_refresh_cookie_down.sql](<C:/KarryGoPush/services/api-gateway/migrations/010_partner_refresh_cookie_down.sql>) | Non-destructive schema rollback |
| [services/fleet-partner-service/.env.example](<C:/KarryGoPush/services/fleet-partner-service/.env.example>) | Explicit corporate browser cookie/origin settings |
| [services/fleet-partner-service/docs/configuration.md](<C:/KarryGoPush/services/fleet-partner-service/docs/configuration.md>) | Cookie security and deployment rules |
| [services/fleet-partner-service/docs/passwordless-authentication.md](<C:/KarryGoPush/services/fleet-partner-service/docs/passwordless-authentication.md>) | Cookie-only flow and family concurrency semantics |
| [services/fleet-partner-service/docs/openapi/browser-authentication.openapi.json](<C:/KarryGoPush/services/fleet-partner-service/docs/openapi/browser-authentication.openapi.json>) | Cookie-only corporate auth contract supplement |
| [services/fleet-partner-service/docs/openapi/section30-backward-compatibility.openapi.json](<C:/KarryGoPush/services/fleet-partner-service/docs/openapi/section30-backward-compatibility.openapi.json>) | Correct membership-summary route |
| [services/fleet-partner-service/docs/section30-database-migrations-backward-compatibility.md](<C:/KarryGoPush/services/fleet-partner-service/docs/section30-database-migrations-backward-compatibility.md>) | Correct membership-summary route references |
| [web/partner-web/package.json](<C:/KarryGoPush/web/partner-web/package.json>) | Existing npm workflow: test and typecheck scripts |
| [web/partner-web/src/store/auth-store.ts](<C:/KarryGoPush/web/partner-web/src/store/auth-store.ts>) | Remove credential persistence; retain memory-only state |
| [web/partner-web/src/lib/browser-session.ts](<C:/KarryGoPush/web/partner-web/src/lib/browser-session.ts>) | Single-flight refresh, restore, logout serialization |
| [web/partner-web/src/lib/api-client.ts](<C:/KarryGoPush/web/partner-web/src/lib/api-client.ts>) | Credentials include, memory bearer, one controlled retry |
| [web/partner-web/src/services/auth-service.ts](<C:/KarryGoPush/web/partner-web/src/services/auth-service.ts>) | Cookie-backed logout |
| [web/partner-web/src/components/session-bootstrap.tsx](<C:/KarryGoPush/web/partner-web/src/components/session-bootstrap.tsx>) | Client-only restore before rendering authenticated routes |
| [web/partner-web/src/app/layout.tsx](<C:/KarryGoPush/web/partner-web/src/app/layout.tsx>) | Mount client session bootstrap |
| [web/partner-web/src/components/layout/sidebar.tsx](<C:/KarryGoPush/web/partner-web/src/components/layout/sidebar.tsx>) | Cookie logout without access-token prerequisite |
| [web/partner-web/src/app/(dashboard)/settings/page.tsx](<C:/KarryGoPush/web/partner-web/src/app/(dashboard)/settings/page.tsx>) | Cookie logout and unconfirmed logout warning |
| [web/partner-web/tests/browser-session.test.mjs](<C:/KarryGoPush/web/partner-web/tests/browser-session.test.mjs>) | Execute actual TS clients/store with mocked browser transport |
| [services/api-gateway/docs/browser-auth-security-report.md](<C:/KarryGoPush/services/api-gateway/docs/browser-auth-security-report.md>) | Final evidence report and exact task file inventory |

## 4. Auth flow before versus after

| Concern | Before | After |
| --- | --- | --- |
| Fleet refresh transport | JSON request/response | HttpOnly cookie only |
| Partner dashboard login | Long-lived access JWT, persisted in browser | Short-lived JWT in runtime memory plus opaque HttpOnly refresh cookie |
| Refresh | Gateway route absent | Cookie -> canonical Gateway session validation -> atomic replacement hashes -> new cookie/access JWT |
| Reload | Persistent browser token hydrate | Client effect calls cookie refresh before authenticated content |
| Expired access | No controlled refresh | One single-flight refresh, at most one original-request retry |
| Logout | Access bearer required | Cookie-based revocation and matching cookie deletion, even without access token |

All Gateway session-creation paths are covered: registration, password login,
signup OTP verification and login OTP verification. Existing JSON field name
`token` is retained for the access JWT; it is not a refresh credential.
Fleet verification and organization selection also issue cookie-only refresh
credentials. Identity-only onboarding sessions retain their restricted authority;
step-up references are not stored as refresh cookies.

## 5. Cookie settings

| Setting | Gateway Partner | Fleet corporate |
| --- | --- | --- |
| Default name | karrygo_partner_refresh | karrygo_fleet_refresh |
| Path | /api/v1/partner/v1/auth | /fleet-partner/v1/auth |
| HttpOnly | Always true | Always true |
| Domain | Omitted (host-only) | Omitted (host-only) |
| Secure | Required outside development/test | Required outside development/test |
| SameSite | Lax default; explicit Strict/None available | Same |
| Default refresh TTL | 720 hours | Existing 720 hours |
| Deletion | Negative Go MaxAge => Max-Age=0; past Expires; same flags/scope | Same |

None without Secure is rejected. Incompatible cookie prefixes are rejected.
Cookie expiry tracks actual server session expiry. Access JWTs default to 15
minutes; Gateway caps them at 15 minutes. HttpOnly/Domain/Path cannot be weakened
through environment configuration. Auth responses are no-store/no-cache.

## 6. Refresh rotation and replay protection

Gateway adds only migration 010 to the existing partner_sessions table: nullable
SHA-256 digest, uniqueness and digest-shape constraint. A 256-bit opaque random
refresh secret is never stored plaintext. Rotation is one conditional PostgreSQL
UPDATE matching old digest, account, unexpired state and non-revocation, replacing
both access and refresh hashes. One winner; the old credential is rejected.
This is not a second session store or Fleet family implementation.

Fleet reuses HMAC-hashed refresh credentials, retired-row retention, transactional
rotation and family replay containment. Family-scoped transaction locks now
serialize refresh, cookie logout and replay containment so a successor cannot be
missed by revocation. Normal cookie logout revokes active rows without falsely
marking them compromised. The existing repository owns this behavior; no Fleet
migration was needed.

Cookie logout is idempotent. Frontend waits for its in-flight refresh before
requesting logout. Failed server logout is reported as unconfirmed, not as proven
revocation. PostgreSQL races/migration execution are NOT certified in this run.

## 7. CORS and CSRF

Exact trusted-origin allowlists; credentialed explicit ACAO, never wildcard.
Missing/untrusted/null Origin on auth mutations returns 403. No Referer guessing.
OPTIONS permits Authorization/Content-Type and the auth operation headers.
Other Gateway application auth/CORS flows remain unchanged.
Production startup requires explicit HTTPS origins and secure cookie policy;
production-enabled Partner auth also requires a sufficiently sized existing JWT secret.
Disabled Partner deployments do not acquire unrelated startup requirements.

Actual hosted origin must be supplied. Same-site HTTPS hosting supports Lax;
cross-site hosting may require explicitly configured None+Secure and remains
subject to browser third-party-cookie policy. TLS/proxy/APM configuration was not
modified or certified. Cookie/Set-Cookie must also be redacted at those layers.

## 8. Frontend storage changes

Zustand auth persistence removed. Access token/account exist only in runtime.
Bootstrap deletes the obsolete karrygo-partner-auth key without reading it.
No access/refresh token writes to localStorage, sessionStorage, IndexedDB,
persisted stores or JavaScript cookies remain. Existing onboarding profile-only
persistence remains, without credentials.

Login/OTP, refresh and logout send credentials include. Runtime single-flight
deduplicates refreshes; delayed old-token 401s reuse the current token. Retry is
bounded to one; normal business failures are not misclassified as auth failures.
Logout waits for pending rotation and clears runtime state.

Next.js and React skill guidance informed the client-only bootstrap and effect
deduplication: no async client component, token-bearing server props or server
credential singleton was added.

## 9. Backward compatibility

Affected callers: Partner web auth-service/API client/store/bootstrap/sidebar/
settings; Gateway legacy registration/password and OTP HTTP consumers; Fleet
corporate verify/refresh/select-organization/logout consumers and integration tests.
No verified native Fleet refresh consumer was found requiring a JSON fallback.

Existing Gateway access field token and bearer-protected business APIs stay.
Existing API-key registration semantics stay. Legacy stored browser sessions
must sign in again to establish a cookie; pre-existing access JWTs expire under
their original lifetime unless explicitly revoked. Migration 010 must precede
the Gateway deployment.

Native Customer, Taxi, Dispatch and Truck auth contracts were not modified.
The directly shared provider membership client follows the user's authorized
Fleet route correction; this is not a native provider auth redesign.

## 10. Tests added/updated

- Gateway real HTTP handlers/usecases with memory repositories: password, OTP
  signup/login, issuance/deletion flags, trusted preflight, hostile/missing
  Origin, missing/invalid/expired/revoked cookie, refresh rotation, replay,
  authenticated read-back, repeated logout, 12-way refresh one-winner test.
- Gateway PostgreSQL repository test: real migration up/down/reapply, concurrent
  CAS winner, old hash invalidation and durable revocation. Environment-gated.
- Fleet JSON serialization guard, cookie-boundary Origin/preflight tests,
  cookie-aware existing HTTP integration tests, refresh/replay test and
  logout-versus-refresh family race test. DB tests remain gated.
- Shared production cookie config/deletion/prefix tests and corrected signed
  membership client route assertion.
- Gateway/Fleet OpenAPI auth operation parity; Fleet Gin route construction.
- Six frontend tests execute actual TypeScript client/store code with mocked
  browser transport: restore, credential transport, concurrent refresh, bounded
  retry/401 clearing, logout race, no persistence.

## 11. Commands run and results

All Go commands ran against the current local tree. Long output was redirected
to .codex-cache logs; skips are not counted as executed database coverage.

| Command / location | Result |
| --- | --- |
| go test ./... (Fleet module; JSON output captured) | PASS: final run 584 test/subtest pass events, 574 skips, 71 packages with passing tests |
| go test ./... (Gateway module; JSON output captured) | PASS: final run 253 test/subtest pass events, 1 skip, 15 packages with passing tests |
| go vet ./... and go build ./... (each module) | PASS |
| go test -count=1 ./browserauth ./providercompat (shared/go) | PASS |
| go vet / go build ./browserauth ./providercompat (shared/go) | PASS; later cookie-prefix hardening also passed focused tests/vet |
| go test -count=1 (Gateway partneraccount/config; Fleet auth/http; shared browserauth/providercompat) | PASS |
| go test -count=1 (Fleet auth/... integration cmd), -run 'Browser\|Refresh\|Logout\|Session' -v | Available tests PASS; DB tests SKIP |
| Router / membership / migration-shape focused tests | PASS |
| Gateway/Fleet BrowserOpenAPI tests | PASS; corrected a test fixture relative path, then reran successfully |
| npm test (partner-web) | PASS: 6/6 |
| npm run typecheck | PASS |
| Focused eslint on auth store/client/services/bootstrap/root and test | PASS |
| npm run lint | FAIL: scans existing generated .next output and pre-existing source issues |
| npm run lint -- --ignore-pattern '.next/**' --ignore-pattern 'node_modules/**' | FAIL: 10 existing source errors, 8 warnings |
| npm run build | FAIL: Turbopack cannot resolve parent-installed Next.js under partner-web root |
| npm run build -- --webpack | PASS |
| gofmt verification on all changed Go files | PASS |
| git diff --check | PASS |

The installed Next.js resolves to web/node_modules (16.3.0), while partner-web
declares 16.2.10. No package installation, upgrade, dependency-tree replacement or
lockfile edit was performed. The webpack success does not certify a clean
lockfile-pinned production dependency installation.

Logs:
- [Fleet full results](<C:/KarryGoPush/.codex-cache/fleet-cookie-full-final.jsonl>)
- [Gateway full results](<C:/KarryGoPush/.codex-cache/gateway-cookie-full-final.jsonl>)
- [Focused security results](<C:/KarryGoPush/.codex-cache/partner-cookie-security.log>)
- [Final session results/skips](<C:/KarryGoPush/.codex-cache/fleet-cookie-final-session-tests.log>)
- [Frontend tests](<C:/KarryGoPush/.codex-cache/partner-web-final-tests.log>)
- [Source lint findings](<C:/KarryGoPush/.codex-cache/partner-web-source-lint.log>)
- [Default build failure](<C:/KarryGoPush/.codex-cache/partner-web-build.log>)
- [Webpack build](<C:/KarryGoPush/.codex-cache/partner-web-webpack.log>)

## 12. Router startup status

PASS for local Gin router construction including both provider routes:
GET /internal/fleet-partner/v1/providers/:providerId
and GET /internal/fleet-partner/v1/providers/:providerId/services/:providerService/membership-summary.
The old sibling wildcard collision was present and is fixed. Shared client,
OpenAPI and contract references were updated. Live production startup was not
claimed; it needs configured dependencies/secrets.

## 13. Remaining risks / blockers

- FLEET_PARTNER_TEST_DATABASE_URL and API_GATEWAY_TEST_DATABASE_URL are unset.
  PostgreSQL rotation, revocation, family races and migration replay:
  BLOCKED_BY_TEST_INFRASTRUCTURE.
- Real deployed HTTPS browser/OTP/DB end-to-end flow has not run. The tested
  Partner path is browser client -> Gateway contract, not a nonexistent Fleet
  proxy. Production cookie delivery requires actual origin/topology configuration.
- Existing source lint debt and default Turbopack/dependency-layout issue remain.
- Separate tabs can race Gateway rotation and receive 401/re-authentication;
  single-flight is runtime-local, not a cross-tab credential broadcast.
- Gateway has CAS replay rejection, not Fleet's separate family compromise
  semantics. Do not claim those two authorities have identical replay policies.
- Apply migration/config/backend/frontend coherently. Do not release only the
  memory-store frontend against the old Gateway missing refresh.
- HttpOnly reduces credential exfiltration; it does not make XSS harmless.
  No unrelated CSP, IAM or auth-system redesign was performed.

## 14. git diff --stat

[Exact repository-wide output](<C:/KarryGoPush/.codex-cache/partner-cookie-final-diffstat.log>).
At final capture: 110 tracked files changed, 3436 insertions, 737 deletions.
This includes pre-existing unrelated work and excludes untracked new files;
it is NOT the diff size of this authentication task alone.

## 15. git status --short

[Exact full status output](<C:/KarryGoPush/.codex-cache/partner-cookie-final-status.log>).
At capture: 226 status entries, including pre-existing artifacts/unrelated work.
Nothing staged. HEAD unchanged. No files or existing Git changes discarded.
The pre-existing .codex-cache directory contains this run's diagnostic logs.
The pre-existing cmd.exe artifact was not created, staged or removed by this task.

Historical verdict above described the initial cookie implementation, not full
revocation-race certification. A subsequent audit found two mandatory defects:
Gateway digest-only logout could miss a rotated row; Fleet organization selection
could mint a new family from a revoked source access JWT.

## Focused revocation correction

Gateway now uses authenticated stable session correlation plus a rotating secret.
Only the SHA-256 secret digest is stored. Atomic rotation and stable-ID revocation
update the same existing row, so either ordering leaves the session revoked.
Unknown/invalid credentials return 401; already-revoked known sessions remain
idempotent. Earlier unsigned 43-character refresh cookies require sign-in again.
No new migration was needed; migration 010 remains unchanged.

Fleet organization selection now validates and locks the persistent source under
the existing family lock, checks identity/expiry/compromise, and rotates into the
same family. Originating-family logout/replay revokes the successor. Both source
and successor have transition history. No frontend/cookie-boundary change was
needed; Gateway and Fleet remain their existing independent identity domains.

New deterministic Gateway tests force refresh-first and logout-first repository
interleavings. Fleet adds source-validation unit cases and PostgreSQL tests for
valid-JWT/revoked-source rejection, expired sessions, same-family transitions,
logout/replay interleavings and compromised ancestors. The PostgreSQL tests are
gated on the canonical service test database variables. No live PostgreSQL,
deployed HTTPS or browser E2E certification is implied by this correction.
