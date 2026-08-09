# Section 28 — Security, Privacy and Audit

Status: **PARTIAL**. This document describes what exists today, what this
pass added, and exactly what remains — see [Verdict](#verdict) for the
itemized list. It supersedes nothing in Sections 13–27.

## Authentication ownership

`services/fleet-partner-service` is authoritative for its own corporate-user
identities, OTP challenges, sessions and refresh tokens
(`internal/features/auth`). It is **not** a JWKS/external-Auth-Service
consumer — access tokens are a local, custom HS256 format signed with a
single service secret (`shared/go/auth`), verified in-process by
`CorporateBearer` (`internal/middleware/auth.go`). There is no `alg`
negotiation (HS256 is hardcoded, so `alg=none` is structurally impossible),
no `kid`/JWKS, and no issuer/audience claim. Refresh sessions are owned
locally too: `internal/features/auth/repositories/sessions.go` implements
rotation, one-time use, family reuse detection and family revocation.

## Token validation

Signature and expiry are verified by `auth.TokenSigner.Verify`
(`shared/go/auth/token.go`). Claims carry subject, role, service, session
ID, type, issued-at and expiry. Organization/branch context is **not** a
token claim — it is resolved per request from trusted membership state
(`internal/platform/tenant`, `internal/middleware.ResolveTenant`), which is
the correct posture for a service that would otherwise have to reissue
tokens on every membership change.

## Session revocation (this pass's primary addition)

Before this pass, `ResolveTenant` re-resolved organization membership on
every request (so an org/membership suspension was already enforced in real
time) but never checked the **specific session's** own status — a session
revoked via `DELETE /sessions/:sessionId`, a credential reset, or reuse-
detected family revocation stayed honored by its still-valid access token
for up to `AccessTokenTTL` (capped at 1h).

Closed via `middleware.SessionStatusChecker` + `SessionRepository.Status`
(`internal/features/auth/repositories/sessions.go`): `ResolveTenant` now
checks session status on every tenant-scoped request, before membership
resolution. A revoked/expired/compromised session, or a session ID that no
longer exists, is rejected with 401 regardless of what the access token's
own expiry claim says. A lookup error fails closed (503), never open.
Wired via `app.Options.SessionStatus` (nil-safe, matching every other
optional dependency in this composition root) → `cmd/main.go` passes
`components.Sessions` (always constructed). Tests:
`internal/middleware/auth_test.go` (5 cases, no DB required).

## MFA / step-up

Already fully implemented at the auth-feature level: `POST
/auth/step-up/start` / `/auth/step-up/verify`, TTL-checked grants
(`Service.VerifyStepUpGrant`), rate-limited.

**Now actually enforced**, not merely recorded, on legal-hold
create/release and security-settings update — closing the gap this
document originally disclosed. `internal/platform/stepup.Verifier` is a
narrow port (`Verify(ctx, reference, identityID, sessionID, organizationID,
actionClass) error`) adapted over `auth/usecases.Service` in the
composition root (`internal/app/stepup_adapter.go`, same pattern
`documents.MediaPort` uses), wired into both
`legalholdusecases.Service` and `securitysettingsusecases.Service`.

The requirement is **conditional on the organization's own security
settings** (`MFARequiredForSensitiveActions`, defaulting to `true`) rather
than hardcoded — which also closes the "security-settings values not
consumed by the behavior they name" gap: `security.settings.manage` itself
requires step-up whenever the CURRENT (pre-change) value of
`MFARequiredForSensitiveActions` is true, so a caller cannot use one request
to both disable the requirement and skip proving they were allowed to.
Legal-hold reads this same setting through `SecurityPolicyReader`, a second
narrow port implemented directly by `securitysettingsusecases.Service` —
one source of truth, not two independently-drifting copies. Each action
class (`privacy.legal_hold.manage`, `privacy.legal_hold.release`,
`security.settings.manage`) is distinct, so a grant obtained for one action
cannot be replayed against another, and `identityID`/`sessionID` are read
from the verified access-token claims (`auth.ClaimsFromContext`), not
client input, so a grant cannot be transferred to a different session
either. A missing/wrong reference fails closed (`step_up_required`/
`step_up_expired`, propagated straight through from
`VerifyStepUpGrant` — no exceptions, no fallback-to-allow path).

An unwired verifier (`stepUp == nil`) still degrades to record-only, same as
every other optional dependency in this composition root — but in this
service's actual `cmd/main.go` wiring it is never nil. Verified against
real Postgres: `TestSection28LegalHold_StepUpEnforcedWhenOrgRequiresMFA`
(missing/wrong/valid reference, correct identity/session/org/action-class
asserted on every call, separate action classes for create vs release) and
`TestSection28LegalHold_StepUpSkippedWhenOrgDisablesMFA` (disabling MFA is
itself gated by step-up; once disabled, a subsequent hold create genuinely
never calls the verifier).

**Retention-policy updates remain step-up-free** — Section 9 lists
"Data-retention *execution*" as requiring step-up, meaning actually running
a deletion/anonymization pass, not configuring the policy number; since no
such execution worker exists yet (see
[Privacy and retention policy](#privacy-and-retention-policy-this-passs-fourth-addition)),
there is nothing to gate yet. Wiring it into retention-policy *configuration*
too would be a defensible extra precaution but wasn't in the spec's literal
list, so it was left out rather than silently over-scoped.

## Service-to-service authentication

HMAC-v2 (`shared/go/serviceauth`) is unchanged and already solid: per-caller
secret, timestamp freshness (5 min), full method/path/body signature. This
pass did not touch it — it was already closed by an earlier pass
(`internal/platform/noncestore` + `middleware.RejectReplayedNonce`, wired in
`internal/app/app.go`, persistent nonce replay protection against
`fleet_partner_service_nonces`, migration 000060). Caller scoping is
allowlist-by-name, not allowlist-by-route/method — a caller either has the
shared secret for this service or it doesn't; there is no per-caller
route/method restriction table. Not addressed this pass.

## Rate limiting

`internal/platform/ratelimit` (Redis-backed with in-process fallback)
already covered auth-flow scopes (challenge, verify, resend, refresh,
step-up, org-switch, invitation-accept) before this pass — unchanged.

**General, layered, risk-classed rate limiting is now built and applied to
this pass's own new routes**, reusing that same distributed limiter (no
second rate-limit store):

- `internal/platform/ratelimit/riskclass.go` — the five classes Section 28
  names (`LOW`/`STANDARD`/`SENSITIVE`/`HIGH_RISK`/`INTERNAL_SERVICE`) with
  illustrative default `Rule{Limit, Window}` values per class (disclosed as
  a starting point, not a load-tested capacity plan).
- `internal/middleware.RateLimit(limiter, metrics, riskClass, rule)` — a
  gin middleware that checks **every identity dimension the request
  actually carries, independently, under its own key**: organization,
  organization user, service caller, and always client IP as the floor.
  Any one dimension over budget refuses the whole request (429 +
  `Retry-After`) — this is what makes it "layered" rather than a single
  global counter: an organization spreading load across many users is still
  bounded by the org key even though each user individually stays under
  limit, and denials increment a
  `fleet_partner_rate_limit_denied_total{risk_class=...}` counter for
  observability.
- `internal/app.Components.Registrars` wraps the four features this pass
  built in per-risk-class sub-groups (`group.Group("")` with the middleware
  attached, zero changes to the four feature packages themselves):
  audit-log reads → `SENSITIVE`; legal-hold, security-settings and
  retention-policy → `HIGH_RISK`.
- **Explicit proxy trust**, added alongside this because an IP-based rate
  limit is meaningless if a caller can set its own `X-Forwarded-For`:
  `app.BuildRouter` now always calls `router.SetTrustedProxies(...)`
  (previously never called at all, which is gin's own documented "trust
  every proxy, only print a warning" default). Empty
  `config.Config.TrustedProxies` (`FLEET_PARTNER_TRUSTED_PROXIES`, unset by
  default) means trust none — confirmed against gin v1.12.0's actual source
  (`SetTrustedProxies(nil)` → `trustedCIDRs` stays `nil` →
  `isTrustedProxy` always `false` → `ClientIP()` falls back to the raw
  connection's `RemoteAddr`), not assumed from documentation alone.

**A real bug was caught by this pass's own middleware test** before it
shipped: the first draft of the test router omitted
`respond.ErrorHandler()` (present in production via `BuildRouter`, the
middleware that actually converts `httpx.Abort`'s queued error into a
written 429 status + body). Without it, `httptest.ResponseRecorder`
silently stayed at its zero-value 200 even though the middleware had
correctly aborted the chain — the test failure was in the test harness, not
the middleware, but it's exactly the kind of gap that makes a "passing"
test meaningless; fixed before any test was allowed to stay green.

**Rollout is now complete across the entire mounted API surface** (a later
pass superseded the "only four features" state above): a centralized
route-risk registry, `internal/platform/riskregistry`, classifies every
public/tenant/internal route (~250, growing as new routes are added) by
method+path, mechanically extracted from each feature's own manifest plus
individually-reasoned overrides for the handful of packages that predate
the manifest convention. `internal/middleware.GlobalRateLimit(limiter,
metrics)` looks a route up in the registry and applies its risk class's
rule; it is mounted **once per route class** (public, tenant-scoped,
internal) in `app.BuildRouter`, not per-feature, so a newly added route is
covered automatically as long as it is in the registry — proven, not
assumed, by `internal/section28contract/route_coverage_test.go`'s
`TestRouteRiskRegistry_CoversEveryMountedRoute`, which enumerates every
route each feature's real `TenantRoutes()`/`InternalRoutes()` methods
return (immune to the regex-extraction gaps a static-analysis pass can
miss) and fails the build the moment a mounted route has no classification.
A route with no registry entry — which the coverage test guarantees cannot
happen for anything actually mounted — still fails closed to the
`STANDARD` rule rather than going unbounded, read live from
`ratelimit.DefaultRiskRules` on every request (not a cached snapshot, so a
runtime rule change takes effect immediately).

Verified: `internal/middleware/ratelimit_test.go` (allow/deny/Retry-After,
nil-limiter no-op, org-vs-user independent-dimension proof, key
composition, registry-driven classification, unclassified-route fallback,
trusted/untrusted proxy `ClientIP()` behavior) plus a real-Redis integration
test, `internal/integration/ratelimit_redis_test.go`
(`TestRateLimit_RedisBackedLimiterSharesStateAcrossTwoInstances` — two
separate `RedisLimiter` values sharing one Redis, standing in for two
process instances behind a load balancer, prove the counter is genuinely
shared and not per-instance in-memory state).

## Authorization order and object-level authorization

Unchanged from the existing, already-solid pattern: `accesscontrol.
Authorizer.Require` (deny-by-default, role-version-cached) is called after
tenant resolution in every handler this pass added, with tenant condition
baked into every repository query (`organization_id = $1` is never optional
in any new SQL). All four new features (audit reads, legal holds, security
settings, retention policies) follow the identical
`requireTenant → authorize → repo(tenant-scoped SQL)` shape every other
feature in this service uses.

## Field-level authorization

- Audit-log **list** responses never carry `previousSummary`/`newSummary`/
  `networkMeta`, even for a caller holding `audit.read_sensitive` — those
  fields are detail-route-only (`internal/features/audit/http.SummaryView`
  vs `DetailView`).
- Audit-log **detail** responses populate those three fields only when the
  caller independently holds `audit.read_sensitive` in addition to
  `audit.read` (`Handler.authorize` returns both an admission decision and a
  sensitivity flag from one authorization pass).

## Secrets and log redaction (this pass's second addition)

Before this pass, two independent, drifting redaction lists existed:
`internal/observability/logging.go` (exact-match map — `"access_token"`
would **not** match `"token"`) and `internal/platform/audit/audit.go`
(substring match, narrower key list). Centralized into
`internal/platform/redact` (`IsSensitiveKey`, substring + case-insensitive),
now the single source both packages delegate to. Key list covers every term
Section 18 lists (`password`, `passcode`, `token`, `authorization`,
`cookie`, `secret`, `api_key`, `otp`, `mfa`, `nin`, `bvn`, `bank*`,
`account_number`, `card*`, `cvv`, `signed_url`) plus the pre-existing
`passport`/`ssn`/`document`. Tests:
`internal/platform/redact/redact_test.go`,
`internal/observability/metrics_test.go` (`TestLogRedactsSensitiveFields`,
rewritten to capture real log output and assert no secret value leaks).

Request/error log allowlisting, response-body truncation and
dependency-error redaction were already in place before this pass
(`internal/middleware/middleware.go`) and are unchanged.

## Sensitive column inventory

A machine-readable inventory now exists:
`docs/section28-sensitive-column-inventory.json`, generated by
`internal/section28contract.BuildSensitiveColumnInventory` from every
migration's actual `CREATE TABLE` / `ALTER TABLE ADD COLUMN` statements —
not hand-typed, and not limited to Sections 13–28: sensitivity does not
depend on which section's migration introduced a column, so the parser
covers all 70 migration files. Same two-layer design
`internal/platform/riskregistry` already established for route
classification: a mechanical keyword pass (priority-ordered categories —
`CREDENTIAL` > `GOVERNMENT_ID` > `FINANCIAL` > `BIOMETRIC` > `LOCATION` >
`CONTACT_PII`) plus a small, individually-reasoned `overrides` list (empty
today — no real column has needed one yet). 85 columns across 44 tables are
currently inventoried: 39 `FINANCIAL` (mostly `*_amount_minor`/`*_kobo`
monetary fields and payment/wallet/settlement references), 28
`CONTACT_PII` (name/email/phone/address), 12 `LOCATION` (GPS
latitude/longitude), 4 `CREDENTIAL` (token/session hashes — all already
stored as HMAC hashes, never raw), 2 `GOVERNMENT_ID` (business registration
numbers).

**This is a naming-convention inventory, not a data-flow analysis.** It
cannot know a column holds sensitive data unless its name says so, and it
is a starting point for a real encryption/tokenization/access-review pass,
not a substitute for one.

**A drift detector, not a snapshot**:
`TestSensitiveColumnInventory_MatchesLiveSchema` re-parses every migration
on every test run and fails if the result differs from the committed JSON
— regenerate deliberately with
`UPDATE_SENSITIVE_COLUMN_INVENTORY=1 go test ./internal/section28contract/... -run TestSensitiveColumnInventory_MatchesLiveSchema`
and review the diff before committing, so a future migration that adds a
sensitive column cannot silently go uncatalogued the way a hand-maintained
document would.

**Two real parser bugs were found and fixed while building this**, both by
manually reviewing the first generated output against the source
migrations rather than trusting the mechanical pass blindly:

1. The bare keyword `"nin"` (for Nigerian National Identification Number)
   matched as a substring inside `warNINgs`, `traINING` and
   `owNINg_service` — three real, unrelated columns were misclassified
   `GOVERNMENT_ID`. Fixed by requiring bare, underscore-free keywords
   (`nin`, `bvn`, `email`, `token`, etc.) to match a WHOLE underscore-split
   token, never a raw substring; keywords that already contain an
   underscore (`bank_account`, `_kobo`, `national_id`) keep substring
   matching, since the underscore already anchors them to token
   boundaries.
2. This schema commonly declares more than one column on the same physical
   line (e.g. `latitude numeric(9,6), longitude numeric(9,6), accuracy
   numeric(10,3),` in migrations 000035 and 000044). The parser's first
   draft split `CREATE TABLE` bodies line-by-line, silently keeping only
   the FIRST column on any such line — `longitude` was missing from both
   real tables' inventory entries. Fixed by rewriting the parser to split
   on top-level commas (depth-aware, so `NUMERIC(9,6)`/`CHECK(...)`/
   `REFERENCES t(id)` are never mistaken for column separators) across the
   whole body instead of by line; also fixed the same class of bug for
   `"biometric"`/`"face_scan"` false positives, where the bare keyword
   `"fingerprint"` matched ~20 real columns (`request_fingerprint`,
   `content_fingerprint`, `approved_fingerprint`, etc.) that are all
   change-detection hashes of request payloads (confirmed by reading the
   migrations that introduce them), never biometric data anywhere in this
   codebase.

Verified: `internal/section28contract/sensitive_columns_test.go` (7 tests:
the drift detector itself, category-priority pinning, table-level-clause
skipping, the two regression tests above, and multi-column
`ALTER TABLE ADD COLUMN`).

## Encryption and tokenization

Not audited or changed this pass. The existing convention (bank/identity
references stored as opaque strings pointing at owning services — wallet,
verification-compliance — rather than raw values in Fleet's own tables) was
observed to already hold in every table this pass touched or read, but a
full column-by-column audit of Sections 1–27's existing schema was not
performed against the new inventory above (built in the same session, after
this observation) — the inventory now exists to make that follow-up audit
tractable, but the audit itself is still real, disclosed remaining work.

## Media/file security

Unchanged and already solid (`internal/features/documents`): signed URLs are
never persisted or logged, scan/malware status is Media-authoritative,
quarantined/rejected/expired files cannot download, `documents.
download_restricted` gates the two most sensitive visibility tiers on top of
ordinary `documents.download`. Not touched this pass.

## Append-only audit log (this pass's primary structural addition)

The write path (`internal/platform/audit.Recorder`, table
`fleet_partner_audit_records`, `BEFORE UPDATE OR DELETE` trigger
`fleet_partner_forbid_mutation` since migration 000001) already existed and
was already correct. **What did not exist: any HTTP route to read it.**

New: `internal/features/audit` (`repositories` + `http`), migration 000065.

- `GET /fleet-partner/v1/audit-logs` — filters: `branchId`, `actorType`,
  `actorId`, `action`, `resourceType`, `resourceId`, `requestId`,
  `correlationId`, `highRiskOnly`, `from`/`to` (RFC3339, must be supplied
  together, bounded to 90 days), `cursor`/`limit` (keyset pagination,
  max 200). `highRiskOnly` matches a centralized action-name allowlist
  (`repositories.IsHighRisk` — `suspend`, `revoke`, `legal_hold`,
  `credential`, `role_change`, `permission_change`, `retention_execute`,
  `security_setting`, `dead_letter_replay`, `session.revoke`,
  `organization.close`, `membership.remove`).
- `GET /fleet-partner/v1/audit-logs/{auditId}`.
- Permissions: `audit.read` (owner, admin, operations, compliance) and
  `audit.read_sensitive` (owner, compliance only — narrower, matching the
  `documents.download_restricted` precedent).
- Organization isolation and branch isolation (via
  `PermittedBranchIDs`, correctly using `COALESCE(cardinality(...),0)=0`
  rather than the `cardinality(...)=0` pattern a WP5 audit in an earlier
  session found silently excludes every row for a nil Go slice) are baked
  into the repository SQL, not applied after the fact.
- No dedicated audit export route — audit-log export would go through the
  existing generic export framework (`internal/features/exports`) once an
  `AUDIT` dataset adapter is registered there; not built this pass.

## Audit tamper resistance

The `fleet_partner_forbid_mutation` trigger (migration 000001) remains the
primary defense: `fleet_partner_audit_records` physically cannot be
`UPDATE`d or `DELETE`d by ordinary runtime code. Additively, migration
000069 adds a **tamper-evident hash chain**
(`internal/platform/audit/chain.go`): each row's `entry_hash` is an
HMAC-SHA256 (a standard, well-reviewed keyed MAC — never a bare hash and
never reversible encryption, per Section 25's "no invented custom
cryptographic primitives" guidance) over a canonical, fixed-order
serialization of the row's own fields plus the previous row's hash, keyed
by `FLEET_PARTNER_AUDIT_CHAIN_SECRET`. Chains are partitioned per
organization per calendar month (`chain_partition =
"<organizationId>:<YYYY-MM>"`) so verification and the per-append
`pg_advisory_xact_lock` concurrency guard never serialize across unrelated
organizations. Pre-migration rows keep `hash_version = 0` and are counted
as legacy, never flagged as tampered — there is no way to retroactively
chain history that predates the mechanism.

**`Record` is safe to call with a bare connection pool, not only a
transaction.** The advisory lock the chain relies on for concurrent-append
safety is transaction-scoped; a caller with no natural domain-mutation
transaction to join (for example, an audit event for a read-authorization
action) would otherwise pass a bare `*pgxpool.Pool`, where each statement
in the lock/read-tip/insert sequence could be handed a different pooled
connection and the "lock" would not actually serialize anything. Rather
than rely on every one of `Record`'s ~30 call sites to remember to wrap a
standalone call in `database.WithinTx`, `Record` now detects a bare pool
and opens its own transaction internally. This was found and fixed as a
real bug during this session, not a hypothetical: `TestAuditChain_
InitialEntryConcurrentAppendAndValidVerification` (8 concurrent goroutines
appending to the same partition) failed with `previous_entry_hash does not
match` until this fix landed, and the exact same call shape already existed
in production at `internal/features/documents/usecases/service.go`'s
`document.download.authorized` audit event — that call site is fixed for
free by the same change, with no code change needed there.

**Verification is reachable, not just implemented**:
`POST /fleet-partner/v1/audit/verify-chain` (permission
`security.audit_chain.verify`, granted to `rol_system_owner` and
`rol_system_compliance`) recomputes and checks one organization's chain for
a given calendar month (defaults to the current UTC month), scoped strictly
to the caller's own `tenant.Context.OrganizationID` — never a caller-
supplied organization id, so the route can never be used to probe another
tenant's chain. The response reports entry counts and, for any detected
break, the record id and reason — never record content, so this route does
not additionally require `audit.read_sensitive`.

Verified: `internal/integration/audit_chain_test.go` (5 tests: concurrent
append + verification, tenant-partition isolation, a modified row detected,
a broken link detected, legacy pre-chaining rows skipped not flagged) and
`internal/integration/audit_verify_chain_http_test.go` (4 tests: the whole
wire through the real router — happy path, permission denial, malformed
month rejected, and an organization's verify-chain call proven to never see
another organization's entries).

## Audited actions

This pass's own four new mutations (`legal_hold.created`,
`legal_hold.released`, `security_settings.updated`,
`retention_policy.updated`) are all recorded via `audit.Recorder.Record`
inside the same `database.WithinTx` transaction as the mutation itself — an
audit-write failure rolls back the mutation, per Section 24's "audit failure
must fail high-risk mutations" rule. Every other audited action listed in
Section 26 (assignment lifecycle, document actions, invoices, imports,
exports, jobs, etc.) was audited by the sections that built those features
and is unchanged by this pass.

## Audit access

Covered above — see [Append-only audit log](#append-only-audit-log-this-passs-primary-structural-addition).

## Security settings (this pass's third addition)

New: `internal/features/securitysettings`, migration 000067. One versioned
row per organization (`fleet_partner_security_settings`), `GET`/`PUT
/fleet-partner/v1/security-settings`. Fields: `mfaRequiredForSensitiveActions`,
`stepUpFreshnessSeconds` (300–3600, DB `CHECK` **and** Go validation —
platform minimum/maximum enforced at both layers), `sessionRevocationPolicy`
(`IMMEDIATE`/`STANDARD`), `allowedProviderAccessPolicy`
(`ACTIVE_MEMBERSHIP_ONLY`/`ASSIGNED_ONLY`), `sensitiveExportThresholdKobo`,
`restrictedDocumentStepUpRequired`, `auditCapturePolicy` (`FULL`/`NONE`),
`retentionPreferenceDays`. `expectedVersion=0` creates the first row for an
organization; a positive value updates under optimistic concurrency.
Permissions `security.settings.read`/`security.settings.manage`. Updates are
audited in the same transaction, and are themselves gated by step-up
whenever the CURRENT (pre-change) `mfaRequiredForSensitiveActions` is true —
see [MFA / step-up](#mfa--step-up). `MFARequiredForSensitiveActions` is now
load-bearing: legal-hold create/release reads it (via the
`SecurityPolicyReader` port) to decide whether to require step-up at all.
The other six fields remain configuration-only —
`stepUpFreshnessSeconds` does not yet override the auth feature's own
step-up TTL, `sessionRevocationPolicy`/`allowedProviderAccessPolicy`/
`sensitiveExportThresholdKobo`/`restrictedDocumentStepUpRequired`/
`auditCapturePolicy`/`retentionPreferenceDays` are stored and validated but
nothing yet reads them back to change behavior.

## Privacy and retention policy (this pass's fourth addition)

New: `internal/features/retentionpolicy`, migration 000068. Fourteen
retention classes exactly matching Section 30
(`TEMPORARY`…`COMPANY_CONFIDENTIAL`), each with an illustrative platform
minimum in `models.PlatformMinimumDays` (**not a substitute for a
jurisdiction-specific legal review** — see the inline doc comment).
`GET /fleet-partner/v1/retention-policies` (all fourteen, override-or-
minimum), `GET`/`PUT /retention-policies/{class}`. An organization's write
can only raise a class's retention above the platform minimum, never below
— enforced in `usecases.Service.Update` before the repository is even
called. Permissions `privacy.retention.read`/`privacy.retention.manage`.

This is a **policy model only**. It does not itself delete or anonymize any
business-domain record — no worker reads `EffectiveDays()` and acts on it
for customers, orders, trips, etc. `internal/features/retention` (Section
27, pre-existing) is a different, narrower thing: an operational-table
cleanup worker scoped only to `fleet_partner_jobs`/
`fleet_partner_scheduled_task_runs`, which already disclaimed doing anything
for business-domain tables. Building the actual deletion/anonymization
workers for all fourteen classes across all their owning features is
unstarted — a multi-session undertaking on its own, consistent with how
Section 25's search/export/import generalization took multiple dedicated
sessions.

## Legal hold (this pass's fifth addition)

New: `internal/features/legalhold`, migration 000066. Ten of Section 31's
eleven scopes (`ORGANIZATION`, `CUSTOMER`, `PROVIDER`, `TRIP`, `ASSIGNMENT`,
`INCIDENT`, `SUPPORT_CASE`, `INVOICE`, `EXPORT`, `AUDIT_RANGE`) — **`DOCUMENT`
is deliberately excluded**: document-level legal hold already exists
(`POST /documents/{id}/retention`, propagates to media-file-service, the
only service that can actually refuse to destroy file bytes) and a second,
disconnected legal-hold flag on a document would be exactly the risk that
feature's own code comment warns against.

`POST/GET /legal-holds`, `GET/POST .../{id}` and `.../{id}/release`.
Three permissions: `privacy.legal_hold.read` (owner, compliance, admin,
operations), `.manage` (create — owner, compliance), `.release` (owner,
compliance — deliberately separable from `.manage`, per spec). Release is
version-guarded and refuses a second release (409, not a silent no-op).
Create and release both require step-up when the organization's security
settings say so — see [MFA / step-up](#mfa--step-up).
`IsUnderHold(orgID, scopeType, scopeReference)` is exposed as the one query
a future domain deletion path must call — **not yet called by any domain**;
none of Sections 1–27's deletion/anonymization paths (there are very few
today) currently consult it. Concurrent hold-creation-vs-deletion is
therefore not yet a real race in this codebase, because nothing yet deletes
records this table could protect.

## Account and organization closure

A full closure orchestration workflow exists in
`internal/features/organizations/usecases/closure.go`: `RequestClosure` →
`AssessClosure` (evaluates every blocker — see below — and moves the
request to `APPROVAL_REQUIRED` or `BLOCKED`) → `ApproveClosure` →
`ExecuteClosure` (revokes every corporate session for the organization,
marks the organization `CLOSED`, publishes `organization.closed.v1`; no
record is ever hard-deleted — immutable financial, trip, safety,
compliance and audit records are always retained) → `GetClosureStatus`.
Blockers are fail-closed by design: `AssessClosure` accepts a set of
`ClosureBlockerChecker`s, and `DefaultClosureCheckers()` returns an
`UNKNOWN` (blocking) result for every obligation whose authoritative
service contract (marketplace requests, trips, wallet settlement,
withdrawals, support claims, compliance obligations) does not exist yet —
an unreachable or not-yet-built checker can never silently read as
"cleared". The one real, DB-backed checker today is active managed-provider
memberships.

**A real, previously-undiscovered gap was found and fixed this session**:
`RequestClosure`, `AssessClosure`, `ExecuteClosure` and `GetClosureStatus`
were already fully implemented and reachable over HTTP
(`POST .../close-request`, `GET .../closure-status`), but **nothing in the
entire service ever moved a request from `APPROVAL_REQUIRED` to
`APPROVED`** — `ExecuteClosure`'s own precondition (`status ==
ClosureStatusApproved`) could therefore never be satisfied by any reachable
code path, and the workflow was permanently stuck one step before
completion. Fixed by adding `ApproveClosure` (refuses from any status other
than `APPROVAL_REQUIRED` — approval can never itself clear a blocker) and
`RejectClosure` (ends an in-flight request from any non-`EXECUTING` status),
plus the four HTTP routes gating them and `AssessClosure`/`ExecuteClosure`
behind the whole workflow's route surface:
`POST .../closure/assess|approve|reject|execute`, all behind a new
`organization.closure.manage` permission (migration 000070, granted to
`rol_system_owner` only — deliberately not `rol_system_compliance`, which
reads/audits the closure process but does not decide to shut the
organization down; `organization.close` continues to gate only the
reversible initial request).

Verified: `internal/integration/organization_closure_http_test.go` (3
tests: the full lifecycle driven end to end through the real HTTP router,
`REQUESTED → BLOCKED → APPROVAL_REQUIRED → APPROVED → COMPLETED`, with the
organization itself confirmed `CLOSED` afterward, plus approve-while-
blocked refused 409; reject stopping an in-flight request and allowing a
fresh one afterward; permission denial across all five workflow steps).

## Provider self-service history (this pass's sixth addition)

`internal/features/marketplace/http/internal_handlers.go`
`RegisterProviderReadRoutes` (`GET /providers/{id}/offers[/…]`,
`GET /providers/{id}/assignments[/…]`) already existed, already used the
correct trust model (the calling driver/execution service has authenticated
its own provider and vouches for the ID; Fleet independently re-scopes every
query through the membership join — the same pattern
`documents/http/external.go` uses), and already covers the bulk of Section
34's "assignment history, acceptance history, decline history" requirement
(decline is a status/action on an assignment/offer, not a separate table).

**It was never mounted.** A repo-wide grep for the call site before this
pass found none — `buildMarketplaceInternalRoutes`
(`cmd/marketplace_routes.go`) called `RegisterInternalMarketplaceRoutes` and
`RegisterProviderResponseRoutes` but not `RegisterProviderReadRoutes`. Fixed
with a one-line addition, same guard (`if handler != nil`) as its neighbors.

**`GET /providers/{id}/memberships` is now built too**, closing the last
named gap in Section 34's read surface: `providers/repositories.Repository.
ListMembershipsByProvider` (new — every membership, any status, across every
organization, newest first), a new `marketplace/usecases.
MembershipHistoryReader` port, `Service.ListProviderMemberships` (a safe
view: no financial fields, since a membership record carries none), and
`app.NewMembershipHistoryReader` (composition-root adapter, same shape as
`NewMembershipReader`/`NewCandidateMembershipLister` already in
`marketplace_ports.go`), wired into `BuildMarketplaceService` and mounted
alongside the other provider-read routes.

### A second, more consequential bug found while wiring the memberships route

Building the memberships route required tracing exactly what value
`fleet_partner_organization_provider_memberships.provider_service` actually
holds — and it does not hold what the provider-read HTTP layer was sending
it.

`platformclients.OwningService` (what `providers/usecases` actually writes
into that column at membership creation) returns the caller's raw HMAC
service name: `"driver-dispatch-delivery-service"`,
`"driver-taxi-service"`, `"driver-hauling-service"`. But both
`documents/http/external.go`'s `requireProviderCaller` and marketplace's own
`callerProviderService` resolve a caller through `callerOwnsProviderService`,
whose **values** are the marketplace-internal abbreviation
`"DISPATCH"`/`"TAXI"`/`"HAULING"` — correct for marketplace's own
assignments/offers tables (`sourceServiceFor` maps that abbreviation onto
`fleet_partner_marketplace_assignments.source_service`, a genuinely separate
column with a genuinely separate, self-consistent vocabulary), but wrong for
a membership lookup. `documents.ResolveProviderMembership` /
`FindLiveByProvider` would query
`WHERE provider_service = 'DISPATCH'` against a column that only ever
contains `'driver-dispatch-delivery-service'` — **matching nothing, for
every real provider, in production.**

This went undetected because the only existing test
(`TestProviderDocumentAccess_ScopedToOwnMembership`,
`internal/integration/documents_external_access_test.go`) calls
`usecases.Service.ListProviderDocuments` directly with a membership's own
`ProviderService` field, bypassing `requireProviderCaller`/the
`callerOwnsProviderService` translation entirely — no test exercised the
actual HTTP-layer caller resolution end to end.

**Fixed both call sites** to store/return the caller's raw name instead of
the abbreviation: `documents/http/external.go`'s `requireProviderCaller`
(the map is still used, but only as the allowlist — its values are no
longer read) and a new `marketplace/http.InternalHandler.
callerRawProviderService` (deliberately separate from
`callerProviderService`, which legitimately still needs the abbreviation
for `ListProviderOffers`/`ListProviderAssignments`). New tests pin the
distinction directly: `documents/http/external_test.go`
(`TestRequireProviderCaller_StoresRawCallerNameNotMarketplaceAbbreviation`)
and `marketplace/http/internal_handlers_test.go`
(`TestCallerRawProviderService_ReturnsRawNameNotAbbreviation` alongside
`TestCallerProviderService_ReturnsMarketplaceAbbreviation`, proving both
helpers now return their own, correct, and different values from the same
input).

## Feature flags

No new `_ENABLED` flags were introduced. Every route this pass added is
mounted unconditionally whenever the pool exists (`h.service == nil` /
`h.repo == nil` answers `503 Unavailable` rather than 404 — same
"disabled capability must be observable" rule Sections 13+ already
established), which is appropriate for read/policy-configuration surfaces
that carry no destructive default-on risk. This differs from, e.g.,
`EXPORTS_ENABLED` gating an actual data-egress capability — nothing this
pass added moves or deletes data by default.

## Permissions (all new this pass)

| Permission | Granted to |
|---|---|
| `audit.read` | owner, admin, operations, compliance |
| `audit.read_sensitive` | owner, compliance |
| `privacy.legal_hold.read` | owner, compliance, admin, operations |
| `privacy.legal_hold.manage` | owner, compliance |
| `privacy.legal_hold.release` | owner, compliance |
| `privacy.retention.read` | owner, compliance, admin, operations |
| `privacy.retention.manage` | owner, compliance |
| `security.settings.read` | owner, admin, compliance, operations |
| `security.settings.manage` | owner, admin |
| `security.sessions.revoke` | owner, admin |
| `security.credentials.manage` | owner, admin |
| `security.audit_chain.verify` | owner, compliance |
| `organization.closure.manage` | owner |

`security.sessions.revoke` and `security.credentials.manage` are seeded
(migration 000067) but **not yet enforced by any route** — the existing
`DELETE /sessions/:sessionId` is self-service (a user revoking their own
session, gated by ownership not this permission); there is no admin route to
revoke *another* user's session, and no service-credential-management HTTP
surface exists anywhere in this codebase. Seeding the permission ahead of
its route is a forward declaration, disclosed as such, not a claim of
enforcement.

## Migrations

000065 (audit permissions) → 000066 (legal holds) → 000067 (security
settings) → 000068 (retention policies) → 000069 (audit hash chaining) →
000070 (closure-workflow `organization.closure.manage` permission). All six
purely additive (no `DROP COLUMN`/`DROP TABLE` in any `.up.sql`, verified by
`migrations/migrations_test.go`), each with a paired `.down.sql`. Migrations
000001–000064 were not modified.

## OpenAPI and contract tests

`docs/openapi/section28-security-privacy-audit.openapi.json` +
`internal/section28contract/openapi_test.go` — the same route-manifest-vs-
JSON parity pattern every prior `sectionNNcontract` package uses, covering
all 12 routes across the four original new features plus
`verify-chain` (audit ×3, legal-hold ×4, security-settings ×2,
retention-policy ×3). Passes. The organization-closure routes are covered
by `internal/section28contract/route_coverage_test.go`'s manifestless-route
list instead (the `organizations` package predates the manifest
convention), not by this JSON-parity file.

## Operational monitoring / incident response / rollback

Not addressed this pass beyond what already existed (structured logging,
metrics counters via `internal/observability`). No new alerting or
incident-response runbook content was written.

## Verdict

**SECTION 28: PARTIAL.**

Real, tested against a live Postgres database, and wired-and-reachable this
pass:

- Mid-lifetime session revocation (closes a genuine gap in existing code).
- Centralized, corrected sensitive-key redaction (fixes an existing exact-
  match bug that would have missed `access_token`/`apiKey`/etc.).
- Audit-log read API, two-tier permissioned, field-redacted, tenant- and
  branch-isolated, bounded date range, high-risk filter.
- Generic legal hold across ten of eleven scopes, version-guarded release,
  audited.
- Versioned organization security settings, platform-minimum-enforced.
- Organization-configurable retention policy across all fourteen classes,
  platform-minimum-enforced, distinct from and non-duplicative of the
  existing Section 27 operational-retention worker.
- A real, previously-dead-code bug fix: provider self-service assignment/
  offer history routes now actually mounted.
- Step-up actually enforced (not merely recorded) on legal-hold create/
  release and security-settings update, conditional on each organization's
  own `MFARequiredForSensitiveActions` setting — closing both the "step-up
  recorded but not verified" gap and the "security-settings values not
  consumed" gap this document originally disclosed.
- Provider self-service memberships route (`GET /providers/{id}/memberships`)
  — the last named gap in Section 34's read surface — now built and mounted.
- **A second, more consequential bug found while building it**: the
  provider-facing document external-access feature and the marketplace
  provider-read routes were both resolving a caller to the wrong
  `provider_service` vocabulary (`"DISPATCH"` instead of
  `"driver-dispatch-delivery-service"`), so a membership lookup would have
  matched zero real rows in production for every provider. Fixed at both
  call sites, with tests that pin the distinction directly rather than
  relying on end-to-end coverage alone.
- **General, layered, risk-classed rate limiting across the entire mounted
  API surface**, not just a handful of routes: a centralized
  `internal/platform/riskregistry` classifies every route (~250, generated
  from each feature's real manifest plus reasoned overrides), applied via
  `internal/middleware.GlobalRateLimit` mounted once per route class
  (public/tenant/internal) rather than per-feature, so new routes are
  covered automatically — proven by a Go-native coverage test that calls
  every feature's real route-listing methods (immune to the regex-
  extraction gaps a static pass can miss) and fails the build on any
  mounted route with no classification. Explicit gin proxy trust
  (`SetTrustedProxies`, previously never called at all) so an IP-based key
  can't be spoofed via `X-Forwarded-For`. Proven against a real Redis (two
  separate `RedisLimiter` values sharing one backend, standing in for two
  process instances) that the counter is genuinely distributed, not
  per-instance memory.
- **Audit tamper-evidence via HMAC-SHA256 hash chaining**, partitioned per
  organization per calendar month, reachable through a real, tenant-scoped,
  permission-gated verification route
  (`POST /audit/verify-chain`) — not just a write-side mechanism with no
  way to ever check it. A genuine concurrency bug (the chain's advisory
  lock not actually serializing appends made through a bare connection
  pool) was found by this pass's own concurrent-append test and fixed at
  the `Record` layer itself, which also silently fixes the identical
  pre-existing bug at `documents/usecases/service.go`'s
  `document.download.authorized` audit call.
- **A second real, previously-undiscovered gap found and fixed**: the
  organization closure workflow (request → assess → approve → execute,
  fail-closed blocker checkers, session revocation, non-destructive —
  nothing hard-deleted) was already fully built, but no code path in the
  entire service could ever move a request from `APPROVAL_REQUIRED` to
  `APPROVED`, so `ExecuteClosure` could never run. Fixed by adding
  `ApproveClosure`/`RejectClosure` and mounting the whole workflow
  (`assess`/`approve`/`reject`/`execute`) behind a new
  `organization.closure.manage` permission; proven end to end through the
  real HTTP router reaching `COMPLETED`, not just unit-tested in isolation.
- A production fail-closed startup check for the audit-chain HMAC key
  (`FLEET_PARTNER_AUDIT_CHAIN_SECRET`): `config.Validate()` now refuses to
  boot with `AppEnv=="production"` and the key unset, closing the one real
  security weakness among this pass's own new config (an unkeyed chain is
  still self-consistent but not confidentially keyed).
- A machine-readable sensitive-column inventory
  (`docs/section28-sensitive-column-inventory.json`, 85 columns across 44
  tables, generated from every migration's real schema, all 70 files —
  not limited to Sections 13–28) with a drift-detecting contract test, not
  a hand-written document that goes stale. Two real parser bugs were found
  and fixed by reviewing the mechanically-generated output against the
  source migrations rather than trusting it blindly: a bare `"nin"`
  keyword substring-matching inside unrelated words (`warNINgs`,
  `traINING`), and a line-based column parser silently dropping every
  column after the first when more than one is declared on the same
  physical line (a real, common style in this schema) — both closed with
  regression tests, not just patched.

Explicitly not done this pass, each with the specific reason rather than a
vague "more work needed": service-to-service per-route/method/event-type
scoping (the caller allowlist is still all-or-nothing per caller name, not
narrowed further); service-credential rotation (no rotation mechanism
exists for the HMAC secrets themselves); retention policy having any actual
archive/anonymize execution worker behind it (so retention-policy
*configuration* stays step-up-free — Section 9 gates *execution*, and there
is no execution yet); encryption/tokenization remediation against the new
sensitive-column inventory (the inventory now exists to make this
tractable, but the column-by-column remediation pass itself was not done in
the same session that first built the inventory); a systematic object-level-authorization
IDOR sweep and a field-level-authorization/serialization-leak test matrix
across every feature group (each feature follows the same
`requireTenant → authorize → tenant-scoped SQL` shape, but this has not
been independently re-verified feature-by-feature with live IDOR attempts
in this pass).

**Security-readiness fail-closed checks — partially closed.**
`config.Validate()` now refuses to boot with `AppEnv=="production"` and
`FLEET_PARTNER_AUDIT_CHAIN_SECRET` unset (`TestProductionRequiresAuditChainSecret`,
3 cases: missing-in-production fails, set-in-production passes,
missing-outside-production is not flagged) — closing the one config gap
from this pass that is a genuine security weakness if left unset (an
unkeyed chain is still self-consistent but not confidentially keyed, so a
compromised superuser or an edited backup could recompute a fully "valid"
chain over their own tampering). The other two candidates considered are
each disclosed as deliberately NOT checks, not oversights:
`FLEET_PARTNER_TRUSTED_PROXIES` unset is the SAFE state (trust no proxies —
requiring it to be set would wrongly reject a valid production deployment
that has no reverse proxy at all); and requiring a distributed
(Redis-backed) rate-limit backend in production would mean deciding
`FLEET_PARTNER_REDIS_REQUIRED`'s production default for the whole service
(today it defaults `false` service-wide, not just for rate limiting — every
other Redis-dependent feature, not only Section 28's, silently degrades the
same way), which is a broader architectural decision than this Section 28
gap-closure pass should make unilaterally; the fact that an unconfigured
Redis makes the rate limiter per-instance rather than distributed is
disclosed here as a real, live consequence of that existing default.

### Live database verification

Docker Desktop's backend was unreachable for most of this session (`docker
ps` returning a 500 from the Windows named-pipe endpoint); it came back up
mid-session after a relaunch, at which point the existing
`karrygo-fleet-partner-test-db` container (port 5446) was restarted and used
to verify for real rather than leaving this as a disclosed-but-unchecked
gap:

- `TestMigrationsReplayAndRollback` — migrations 000001 through 000068
  (including all four new ones) apply cleanly, roll back cleanly, and
  replay cleanly.
- The full pre-existing `internal/integration` suite (~280 tests, 220s) —
  zero regressions across the entire Sections 1–27 surface after the new
  migrations and route wiring landed.
- A new `internal/integration/section28_test.go` (5 tests, real Postgres,
  no mocks) exercising the four new features end to end: legal-hold create
  → tenant-isolated `IsUnderHold` → stale-version-refused → release →
  double-release-refused; security-settings defaults → create → duplicate-
  create-refused → stale-version-refused → below-platform-minimum-refused →
  valid update; retention-policy platform-minimum enforcement and tenant
  isolation across two organizations; audit-log tenant isolation,
  resource-type filter and high-risk classification against rows written
  through the real `audit.Recorder`.
- **One real bug was caught by these tests and fixed**: the first draft of
  the audit high-risk test asserted only `organization.suspend` should
  match `highRiskOnly=true`, but `legal_hold.created` legitimately matches
  too (`"legal_hold"` is deliberately in the high-risk allowlist — Section 9
  lists legal-hold creation/release as requiring step-up). The test's
  expectation was wrong, not the classifier; fixed by using a genuinely
  non-high-risk comparison action (`document.viewed`) instead.
- Step-up enforcement: `TestSection28LegalHold_StepUpEnforcedWhenOrgRequiresMFA`
  and `TestSection28LegalHold_StepUpSkippedWhenOrgDisablesMFA` — missing/
  wrong/valid references, correct identity/session/org/action-class on
  every verifier call, separate action classes for create vs release, and
  that disabling MFA (itself step-up-gated) genuinely stops requiring it
  afterwards.
- Provider self-service memberships:
  `TestListProviderMemberships_CrossOrganizationHistoryAndProviderIsolation`
  and `TestListProviderMemberships_UnwiredReaderReportsUnavailable`, plus
  the `documents/http` and `marketplace/http` unit tests pinning the
  `provider_service` vocabulary fix directly.
- A `fleet-partner-redis-test` container (created by an earlier session,
  never started) was started too, giving real Redis for the rest of the
  session: `TestSection26RealRedisTransactionalDedupDeadLetterAndReplay`
  (pre-existing, previously always skipped) now actually runs and passes,
  and the new `TestRateLimit_RedisBackedLimiterSharesStateAcrossTwoInstances`
  proves the rate limiter's distributed behavior for real rather than by
  code inspection.

Full `go build ./...`, `go vet ./...`, `gofmt -l .` and `go test ./...`
(every package, both Postgres- and Redis-gated tests running for real) are
all clean with zero regressions.
