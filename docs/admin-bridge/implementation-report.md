# Sections 2–5 implementation and handover

Decision: **PARTIAL_BLOCKED** pending real HQ/VPS acceptance, reference PDF review,
approved per-action high-risk policy, and scoped domain adapters. The KarryGo-side
security/contract foundation is functional and ready for subsequent owner adapter
development. It is not staging/production certified and cannot execute domain
writes. The separate Centralized Admin repository was not accessed or modified.

## A. Audit

Current fetched staging and original audited SHA:
`1f4ff09380f6e9bf74a9f7cc6dbfaab9e85efe95`. No intervening commits; clean starting
tree. See `audit.md` for source-backed EXISTS/PARTIAL/MISSING/BLOCKED findings.
Existing HMAC v2, owner decisions, public customer/provider routes, platform
non-root image, logging and rollback were reused. The Admin scaffold was extended;
no new Admin UI or domain authority was created. Only the detailed pasted brief
was available; the named PDF was not attached.

## B. Completion by section

| Section | Status | Evidence / remaining dependency |
| --- | --- | --- |
| 2: public isolation | IMPLEMENTED | Gateway path decoding/denial, removal from default routes/configured upstreams, stripped forged headers, Nginx denial; `TestPublicAdminIsolation`, `TestForgedHQHeadersAreStrippedFromPublicRoutes`, full existing Gateway suite |
| 2: private deployment | IMPLEMENTED locally / BLOCKED live | Private-only compose, dedicated DB, reduced capabilities/resources, explicit certified-image deployment, network policy renderer/preflight. Real tunnel, ACL, NAT and public port checks need authorized operators |
| 3: HMAC v2 | EXISTS | Shared signer/verifier unchanged; compatibility suite passes |
| 3: workload/staff boundary | IMPLEMENTED | TLS 1.3, pinned URI/fingerprint/enrollment, fixed HQ principal, per-purpose HMAC keys, expiring pinned Ed25519 staff keys, independent request binding and revocation; real TLS and PostgreSQL integration tests |
| 3: policy/replay/data protection | IMPLEMENTED | Exact product/market/org/permission, approved ACRs, transactional nonce/JTI claims, durable identity/digest checks, quotas, request limits, sanitized output/logging and append-only audit. Scope/replay/concurrency/role tests pass |
| 3: high-risk execution | PARTIAL / BLOCKED | Typed MFA/four-eye/binding/expiry/incident checks and immutable approval storage exist. Approved per-action policies, independent evidence enrollment and domain adapters are not supplied. Every high-risk capability stays disabled; no approval bypass is possible through an executable command |
| 4: application/persistence | IMPLEMENTED | Separate private TLS bootstrap without browser CORS, wiring, dependency readiness, shutdown, truthful discovery, dedicated transactional migrations, least-privilege role guard and audit fallback |
| 4: typed owners | PARTIAL / BLOCKED | Fixed real Analytics dashboard read client with HMAC identity, bounded timeout/body, no redirects/retries and sanitized error classes. Not wired to remote reports because owner has no market/tenant filter. Existing shared advisory client is preserved; other adapters await scoped owner contracts |
| 5: canonical contract | IMPLEMENTED locally | OpenAPI 3.1, nine JSON Schemas, full endpoint/capability inventory, fixtures, signing profile, artifact hashes, independent schema/wire validation and conservative backward compatibility gate |
| 5: HQ consumption | BLOCKED | Real HQ signing/assertion producer and pinned cross-repository compatibility/inter-VPS test require the other system; no completed integration claimed |

## C. Changes and ownership

`internal/security` validates assertions/scopes and approval evidence primitives.
`internal/bridge` mounts the private API; `features/operations` owns orchestration
models, `internal/persistence` owns security metadata, `internal/clients` contains
one fixed owner read contract. Existing scaffold feature folders remain intact.

Migration `001_bridge.sql` owns `admin_remote_principals`,
`admin_delegated_operations`, `admin_approvals`, `admin_audit_entries`,
`admin_security_claims`, `admin_revocations`, `admin_rate_limits`; the runner owns
`admin_schema_versions`. It has uniqueness constraints/indexes, an atomic install
under advisory lock, immutable evidence triggers and explicit non-destructive
irreversibility. It changes no domain tables. Operation expected versions and
nullable observed source versions are distinct. Retention requires approved policy;
only expired replay/rate metadata is eligible for operator maintenance.

The runtime role cannot be the evidence owner/superuser/admin role. Separate DDL
credentials are mounted only to the migration job. Runtime append privileges and
forbidden audit read/update/delete/truncate, DDL and revocation administration are
tested against real PostgreSQL.

Dependencies inherited reachable vulnerabilities. The Admin module now pins
patched x/net 0.60.0, x/text 0.42.0 and their required crypto/sync/sys versions.
Admin CI/build/runtime requires Go 1.26.9+. Other service module constraints and
HMAC bytes were not modified. There are no vulnerability suppressions.

## D. Actual API

All paths are under `/internal/karryam-admin/v1`. Every request requires private
transport, verified mTLS workload, HMAC v2 and a request-bound staff assertion,
plus exact signed market/organization/purpose and the listed permission.

| Method/path | Permission | Actual behavior / owner |
| --- | --- | --- |
| GET `/capabilities` | `bridge.capabilities.read` | Permission-filtered implementation/configuration registry and OpenAPI checksum; bridge metadata |
| GET `/health/aggregate` | `bridge.health.read` | Real bridge dependency readiness; owners explicitly UNAVAILABLE, writes DISABLED |
| GET `/operations/{id}` | `bridge.operations.read` | Real durable record, original staff and exact scope only; 404 hides other scopes |
| POST `/operations` | `bridge.operations.submit` | Strict schema/identity/digest/idempotency validation, durable REJECTED record, HTTP 503 `capability_blocked`, retryable false. No owner effect |

The remaining proposed paths are authenticated, scoped blocked handlers with
stable unavailable responses: overview, search, entities, cancel, support cases,
finance summary, verification cases, analytics network reports, events cursor,
exports and campaign dispatch/status. They advertise DISABLED/BLOCKED_DEPENDENCY,
never success or fabricated business data. See the matrix for each owner,
permission, risk, evidence requirement, test and operational fallback.

## E. Execution evidence

On isolated local PostgreSQL 18 and Go 1.26.3 initially:

* Admin suite: 80 passing test/subtest cases, zero failed/skipped tests. Includes
  real mTLS -> HMAC -> staff -> PostgreSQL -> audit/operation integration, revoked
  certificate on a live keep-alive connection, durable concurrency, role grants,
  failed migration rollback and intentionally irreversible down migration.
* Complete Gateway plus HMAC regression: 296 passing cases, zero failed/skipped
  tests, with `API_GATEWAY_TEST_DATABASE_URL` pointing to a separate disposable DB.
* `go vet ./services/admin-backoffice-service/... ./services/api-gateway/internal/features/routing/...`
  and `go build ./services/admin-backoffice-service/...` passed.
* `python scripts/admin-bridge/contracts.py`, independent `validate.py --wire-dir
  ... --base origin/staging`, network policy tests, `bash -n` and `git diff --check`
  passed. OpenAPI 3.1, nine schemas, fixtures and actual serialized wire samples
  validated; staging predates this first contract, so no old v1 compatibility is
  falsely claimed.
* The complete 80-case Admin and 296-case Gateway/HMAC suites were rerun under
  Go 1.26.9 after the scoped dependency patches and passed, with no test skips.
* Initial `govulncheck` failed on inherited Go/dependency vulnerabilities. After
  the scoped updates, module scan with Go 1.26.9 reports zero reachable/imported
  vulnerabilities. It reports one uncalled x/crypto module advisory with no fix;
  it is not suppressed.

Commands use `ADMIN_BRIDGE_TEST_DATABASE_URL`, `API_GATEWAY_TEST_DATABASE_URL`,
`ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR`. Test gate rejects any skipped test and
requires the actual security/database cases. Packages with no test files are
reported separately, not counted as executed tests.

Local Windows has no Docker or C compiler, so local image/Nginx container builds
and race instrumentation are not claimed. Initial remote CI successfully built
the image, verified non-root execution and resolved Compose. Its scanner installer
failed before scanning; the scanner version was corrected rather than bypassed.
Formatting and the signing-profile checksum manifest were corrected after the
first CI attempt. Dedicated GitHub CI runs PostgreSQL 16,
race suites, vet/build, schema/wire/compatibility, credential and dependency scans,
image build/scan, non-root checks, Compose resolution and Nginx syntax. CI is not
claimed passed until the PR checks actually complete. Existing staging already
has failed Section31/Vendor/Customer certifications; none were disabled or weakened.

Remote evidence at `b13ad107b761df21136ea42627a7da036fbd8d89`:
[Admin CI run 39117715405](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/39117715405)
passed both jobs. PostgreSQL 16 race suites executed all 80 Admin and 296
Gateway/HMAC cases with no skipped tests; schema/wire/compatibility, vet/build,
dependency/credential scans, image build/scan, non-root/Compose and Nginx checks
all passed. A subsequent test-only improvement explicitly traces connection reuse
for certificate revocation, and passes locally; runtime behavior is unchanged.
The delivered HEAD's current check state is reported in the PR/final response.

The broader repository checks remain separate blockers: global static build loop
fails on modules absent from go.work; Media fails its existing migration-tip
rollback assertion; Customer fails its existing credential gate. These areas
also failed on the audited baseline and were not weakened or bypassed here.

## F–G. Readiness and residual risks

See `runbook.md` for exact variables, port isolation, initial roles/secrets,
rotation/revocation, unknown outcomes, audit access/retention, rollback, kill
switch and HQ outage recovery. There is no write-enable flag. Remote ingress
defaults off and cannot auto-deploy through broad platform selectors.

Outstanding: real approved identities/SSO/MFA/assertion producer, policy approval,
private addresses and transport ACL/NAT/firewall acceptance, independent audit/
alerts and backup/restore evidence, source adapter scope contracts, missing PDF
review and all required CI passing/review. DB superusers can override triggers;
separation and independent audit storage remain operational requirements. A DB
outage uses sanitized host audit fallback and fails data access closed.

## H–I. Git delivery and decision

Dedicated branch: `codex/secure-admin-sections-2-5`, based on current staging.
Review commits separate edge isolation, bridge/contracts, and deployment/quality
gates. The final response and PR carry the delivered HEAD, PR link and exact
observed CI state. Do not merge until required checks pass and review completes.

**PARTIAL_BLOCKED**. Ready for scoped owner-service integration development;
not ready to enable domain commands or claim staging/inter-VPS certification.

## Affected files

See the complete generated inventory in `changed-files.txt` alongside this report.
