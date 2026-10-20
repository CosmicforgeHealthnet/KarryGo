# Media case access enforcement core

Status: FRAMEWORK_TESTED, NOT_EXPOSED. Production BLOCKED_DOMAIN / BLOCKED_POLICY.
This is native Media enforcement software. No HTTP route, Bridge route, runtime
consumer or concrete live source-case adapter is enrolled by this increment.
Default `CaseAccessConfig{}` denies before any ledger/object access. Do not treat
approval of a policy file as completion of the missing transport/source adapters.

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
live case adapters, dedicated private workload enrollment, typed HTTP clients,
HQ delegation/facade and target-environment certification remain engineering work.
Approved lifetime, source capabilities/contracts, roles, purposes, scope mappings,
privacy/retention and initial owner approval remain BLOCKED_POLICY.

Native Media also now refuses evidence marked REVOKED in its shared downloadability
check. This protects both new ambient grants and older tokens after revocation,
under the existing owner contract, independently of this dormant case framework.

Validation: 152 complete native Media cases against real PostgreSQL and real
filesystem bytes, zero skips; 292 Bridge/Taxi/privacy/policy/SDK cases, zero skips.
Linux race Media certification is mandatory in the owner closure workflow through
`media_testgate.py`. The test-only cleanup transaction disables only the two ledger
statement guards in the verified, exclusively locked disposable database and
restores them before each test; operation tests run with every guard enabled.
