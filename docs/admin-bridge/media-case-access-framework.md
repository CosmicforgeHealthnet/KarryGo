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

Runtime intentionally supplies a nil case authority until real owner case and
modern-object membership/remediation contracts exist. Consequently even a read
flag plus valid workload key cannot disclose bytes under current production code.
The private HTTP/SDK tests use real PostgreSQL, actual signed native uploads,
real filesystem storage and an explicitly TEST_ONLY case authority. They establish
transport/enforcement behavior, not a real source case association or HQ/staging
certification. Source route audit shows no new public exposure or contract drift.
