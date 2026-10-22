# Media case access enforcement core

Status: NATIVE_CORE_AND_PRIVATE_TRANSPORT_TESTED. Production BLOCKED_DOMAIN / BLOCKED_POLICY.
Native Media now mounts a separate default-disabled private transport. No HQ
facade or concrete production source-case adapter is enrolled.
Default `CaseAccessConfig{}` denies before any ledger/object access. Do not treat
approval of a policy file as completion of missing source adapters or HQ wiring.

The native Go core is `Service.NewCaseAccess`. `Issue`, `Read` and `Revoke` operate
on a distinct ledger under normal Media migration 000007. An authenticated staff
actor must be supplied by the existing trusted boundary, never decoded from a
browser body. Every grant binds staff, all five scope dimensions, owner/case
kind/reference, subject type/reference, purpose, source permission, modern Media
object, object revision, source binding fingerprint, policy revision/checksum and
expiry. Raw random grant tokens are returned once and only hashes enter the
ledger. These tokens cannot redeem the existing ambient owner content path.

Both `media.case.content.read` (source contract `media.admin-case-content.v1`) and
the explicitly allowed owner case permission must be delegated and approved for
the same scope/purpose. A live `CaseAuthority` must independently resolve the
actual case, subject and object membership and native ownership/binding. Query
identifiers supply no facts. Media then locks its own authoritative object and
checks organization, exact owner service/type/reference, purpose, primary or
secondary binding, PRIVATE visibility, positively confirmed SAFE, deletion,
revocation and native access-retention rules. No owner service is impersonated.
Every redemption repeats current delegation, governance and live case checks;
a changed native/source/policy binding, expiry or missing proof returns no bytes.

The signed policy document has an optional `media` section with explicit
`max_grant_seconds` and `source_capabilities`. It has no defaults; absent or
invalid values block access. The implementation accepts lifetimes from 1 to
86400 seconds as a technical ceiling; no production lifetime is chosen or
approved here. Source-context and staff-assertion expiry further shorten each
grant. The only sample value in automated fixtures is explicitly TEST_ONLY.
Monotonic Media policy heads prevent a restart from reviving older revisions.

Grant/revocation evidence is immutable against UPDATE, DELETE and TRUNCATE.
Successful grant, content release and withdrawal audits commit atomically with
their transactions. Bytes are withheld if audit fails. Redemption holds the
original grant/object locks, so withdrawal serializes with an in-flight read and
all later reads deny. An authenticated staff member can withdraw their own grant
after case closure or a read kill; this can only reduce access. Migration rollback
refuses populated grant, revocation or policy-head evidence. Preserve evidence
when disabling code; do not truncate it to get a rollback through.

The real PostgreSQL/file-storage tests use actual signed modern Media uploads and
an explicitly TEST_ONLY case authority. They prove enforcement, not a real domain
case-to-object association. Legacy Dispatch capture uploads use `media_assets`
and their legacy owner/purpose vocabulary. There is no canonical migration or
membership map to modern `media_objects`; supplying a legacy asset ID cannot
create one. Concrete owner-supported mapping/authorization-context contracts,
live case adapters, production workload enrollment, HQ delegation/facade and
target-environment certification remain engineering work.
Approved lifetime, source capabilities/contracts, roles, purposes, scope mappings,
privacy/retention and initial owner approval remain BLOCKED_POLICY.

Native Media also now refuses evidence marked REVOKED in its shared downloadability
check. This protects both new ambient grants and older tokens after revocation,
under the existing owner contract, independently of this dormant case framework.

Validation: 166 complete native Media/private-client cases against real PostgreSQL and real
filesystem bytes, zero skips; 292 Bridge/Taxi/privacy/policy/SDK cases, zero skips.
Linux race Media certification is mandatory in the owner closure workflow through
`media_testgate.py`. The test-only cleanup transaction disables only the two ledger
statement guards in the verified, exclusively locked disposable database and
restores them before each test; operation tests run with every guard enabled.


## Private transport configuration

The fixed native POSTs are `/internal/media/v1/admin/case-grants`,
`/{id}/content` and `/{id}/revocations`. Content requires a newly authenticated
staff context and original request on every call; no token is placed in a URL.
The typed `shared/go/adminmedia` client uses only these paths, sends no browser
credentials, refuses redirects/proxies/retries and bounds response sizes. It
validates grant/object/expiry/contract identity and content contract/object headers.

`MEDIA_ADMIN_CASE_READ_ENABLED` and `MEDIA_ADMIN_CASE_REVOKE_ENABLED` default false.
If both are off, no case key or policy file is opened. The existing private Media
service and global Media kill switch remain prerequisites. Enabled transport
requires `MEDIA_ADMIN_CASE_SECRET_FILE` (raw base64url key of at least 32 bytes),
distinct from native owner HMAC keys, legacy upload bearer credentials and the
ambient download signer. Read/issuance policy uses `MEDIA_ADMIN_CASE_POLICY_FILE`
and `MEDIA_ADMIN_CASE_POLICY_KEYS_FILE` under the compiled production environment;
TEST_ONLY policies cannot be installed in a production binary. Revocation-only
operation needs no read policy and can only withdraw the authenticated staff's
own grant. No key, lifetime or production grant is supplied here.

The sole case caller is `admin-backoffice-service` with its dedicated case key.
The Bridge must derive actor metadata and expiry from its existing verified HQ
assertion for every request. Exact-body HMAC, bounded strict JSON, duplicate/case-
alias rejection, fixed method/path, no query overrides and durable namespaced
nonces protect the native boundary. Existing replay storage/cleanup is reused;
no historical migration is changed. Content is attachment-only/no-store/nosniff.

### Owner-backed case integration — 22 October 2026

Runtime can now enroll the compiled owner-case adapters through the optional
`MEDIA_ADMIN_CASE_SOURCES_FILE`. Its strict JSON contains `sources`, each with
`owner`, `origin` and `secret_file`. Only Customer, Taxi, Dispatch and Hauling are
recognized. Origins and credentials are operator configuration, never request
selectors. Source keys must differ from each other, the Bridge case key, legacy
owner keys and the ambient download signer. No enrollment file is supplied.

Customer review work items, Taxi recovery cases, Dispatch incidents/delivery
recoveries/returns and Hauling recovery cases have fixed source proof paths:
`/internal/{domain}/v1/admin/case-authority/proofs/{kind}/{id}/{subject}/{reference}`.
They accept only the `media-file-service-case-authorizer` workload audience.
Each source uses its real records, native active/terminal states and exact subject
references. A separate signed allocation under `media-case:<kind>` and the new
`{domain}.case.evidence.read` permission is mandatory. Ordinary directory grants
cannot authorize evidence. Scope follows actual canonical parents; unknown
dimensions still require independent owner allocation. Closed or revoked cases,
replayed requests, stale policy and failed immutable source audit deny access.
Independent preparation/publication uses the same case-authority prefix and a
distinct publisher credential; preparation creates no access grant.

The modern object must already belong to that source workload, organization,
subject and an allowed purpose, and must have the exact typed native case binding.
The adapter queries `media_objects` only. No legacy UUID, URL, inspection reference
or arbitrary metadata becomes a modern object ID. Legacy mapping and a standalone
Fleet investigation-case contract remain unavailable. Current safety, quarantine,
retention, object version, revocation and immutable Media disclosure audit are
rechecked by the existing framework. Grant expiry is bounded by the actual signed
source allocation, signed disclosure policy and authenticated staff expiry.

All new flags default false: `{CUSTOMER,TAXI,DISPATCH,HAULING}_ADMIN_CASE_SOURCE_READ_ENABLED`
and `_ADMIN_CASE_CONTEXT_PUBLISH_ENABLED`. Read and publisher credentials use
`_ADMIN_CASE_SOURCE_SECRET_FILE` and `_ADMIN_CASE_CONTEXT_PUBLISH_SECRET_FILE`.
Existing source policy and owner public-key files are reused; no signing endpoint,
policy, grant, key or Media lifetime is installed. Missing enrollment or policy
keeps disclosure blocked.

Verification includes a real Customer executable with actual migrations, Redis,
native review intake/ACK/CLOSE, independent ephemeral TEST_ONLY signatures,
modern Media upload and real filesystem bytes through the private Media SDK.
It denies foreign subjects/tenants/cases, actual legacy assets, source audit failure,
revoked authorization and closed cases. Actual Taxi, Dispatch and Hauling migration
tests independently exercise their source proof handlers, scope, terminal-state,
policy, audience and replay boundaries. These are software tests, not production
policy approval or live environment certification.

### Private Bridge facade

Three fixed POSTs under `/internal/karryam-admin/v1/media/case-grants` issue a
grant, redeem `/{id}/content`, or withdraw `/{id}/revocations`. Scope and purpose
are the exact signed `market`, `zone`, `organization`, `tenant`, `purpose` query.
Bodies contain only native case/subject/modern object identifiers (plus the
original token for content), or a revocation reason. Actor, expiry, source
capability, destination and policy are never request-body selectors. Actor and
expiry come from the existing verified HQ assertion; the compiled case registry
selects the source permission. Media and source permissions, current signed
policy, approved disclosure settings and all five delegated scopes intersect.

`ADMIN_MEDIA_CASE_READ_ENABLED` and `ADMIN_MEDIA_CASE_REVOKE_ENABLED` default
false. `ADMIN_MEDIA_CASE_ORIGIN` and `_SECRET_FILE` are fixed operator enrollment;
the case credential cannot reuse HQ, active resource, Taxi, scoped Analytics or
Customer review keys. Read policy uses existing `ADMIN_ACCESS_POLICY_FILE` and
`_KEYS_FILE`. Revocation requires `media.case.grant.revoke` and can only reduce
the current staff member's own native grant; it remains possible after read/policy
kills. No permission grant or production enrollment accompanies these definitions.

Existing mTLS/workload identity, HMAC v2, delegated JWT, principal/session
revocation, durable nonce and rate limits apply to every request. Immutable Bridge
audit commits before native access and again before returning a token or bytes.
Content uses a bounded, attachment-only binary response with no-store/nosniff;
no raw URL, ambient download credential, private content or case token enters the
Bridge audit. Native Media independently rechecks membership/safety and commits
its own audit. Failed audit, malformed native response or outage withholds content.
No automatic POST retry is introduced. Public Gateway tests explicitly deny the
new HQ, native Media and source-case paths, including encoded traversal variants.

The Bridge mTLS/PostgreSQL/SDK contract tests and the actual Customer-process →
modern Media tests are separate complementary suites. They do not represent live
HQ enrollment, production policy approval or a deployed environment certification.
