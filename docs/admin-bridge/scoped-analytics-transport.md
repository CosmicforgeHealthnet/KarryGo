# Scoped mission-success Analytics transport

Implementation status: tested framework; production BLOCKED_POLICY. This transport
publishes counts-only partial metadata, not the dictionary's completed/(completed
+ failed) success rate. Every cell value is null while coverage is incomplete.
No unit, threshold, production grant or owner approval is selected by configuration.

The fixed native GET is `/internal/analytics/v1/admin/scoped-network/mission-success`.
The private HQ GET is `/internal/karryam-admin/v1/analytics/scoped/network/mission-success`.
Both require the report-specific permission `analytics.scoped.mission-success.read`;
the broad legacy permission cannot enable them. No fallback calls a legacy report.

Native startup requires normal Analytics migrations through 008, including immutable
membership witnesses (007) and durable scoped HTTP nonces (008). Enable only through
`ANALYTICS_ADMIN_SCOPED_NETWORK_READ_ENABLED=true`, with a distinct raw base64url
secret file `ANALYTICS_ADMIN_SCOPED_SECRET_FILE`. Governance uses
`ANALYTICS_ADMIN_SCOPED_POLICY_FILE` and `ANALYTICS_ADMIN_SCOPED_POLICY_KEYS_FILE`.
An approved rule must name analytics-service and exactly match
`analytics.scoped-mission-success.v1:<explicit-native-unit-field>`.
`ANALYTICS_ADMIN_MISSION_SUCCESS_UNIT_FIELD` has no default. Producer public keys
use `ANALYTICS_ADMIN_SOURCE_CONTEXT_KEYS_FILE`, an object mapping canonical owner
names to key-ID/base64url Ed25519 public-key objects. Loading keys creates no source
allocation. Production builds reject TEST_ONLY policy and source contexts.

Bridge startup requires both `ADMIN_ANALYTICS_READ_ENABLED=true` and
`ADMIN_ANALYTICS_SCOPED_READ_ENABLED=true`, plus `ADMIN_ANALYTICS_SCOPED_ORIGIN` and
`ADMIN_ANALYTICS_SCOPED_SECRET_FILE`. Reuse of the HQ credential is rejected. The
existing Bridge signed access policy must approve the same report and source
contract. Existing mTLS, HQ HMAC, staff assertion, delegated permission, purpose,
five-dimensional scope, policy revision, rate and immutable audit checks apply.
All flags default off; disabled startup reads no scoped credential files.

Requests bind product, market, zone, organization, tenant, purpose and time window,
with explicit authority/product-code filters. HQ callers cannot supply staff,
policy, limit or cursor. The SDK supplies the fixed single-window limit. Exact
source facts and independent producer allocations must authorize the native units;
GPS, aggregate bucket totals and caller-supplied tenant labels establish no access.
Cached disclosures reauthorize every original member without resampling. Missing
witnesses, changed facts, revocation, expiry or audit failure deny release.

Validation covers real PostgreSQL event ingestion and source contexts through the
native handler and typed SDK, and private TLS 1.3/mTLS Bridge delegation through
the SDK to a strict owner endpoint. Independent schemas validate both actual wire
captures. This is not a combined deployed-process or staging certification.

Remaining engineering: complete independently verifiable membership exports and
producer publication integration, complete-source rate validation, the other nine report
adapters and their scoped routes, and end-to-end environment certification.
Privacy/Analytics and source owners must approve units, mappings, suppression,
composition, freshness and release semantics before any production activation.
Never synthesize missing membership or retry a denied release under a fresh key.

## Producer allocation ingestion

Normal migration 009 adds durable publication nonce storage. Native fixed POSTs
`/internal/analytics/v1/admin/source-contexts/{canonical-owner}` are available only
for driver-taxi-service, driver-dispatch-delivery-service, driver-hauling-service
and vendor-service.
They require `ANALYTICS_ADMIN_CONTEXT_PUBLISH_ENABLED=true`, pinned producer public
keys in the existing source-context keys file, and
`ANALYTICS_ADMIN_CONTEXT_PUBLISH_SECRETS_FILE`: a JSON object mapping exactly those
configured owners to distinct raw base64url workload secrets of at least 32 bytes.
Every secret must differ from the scoped read key, legacy keys and other publishers.
Caller identity is `owner-authorization-context-publisher`; HQ read identity fails.
Publication stays independent of the report read kill gate so revocations can land.

The typed `analyticsadmin.PublisherClient` sends only an already signed envelope to
its fixed canonical owner path, with a fresh HMAC and no automatic retries. Maximum
body is 32 KiB. The receiver checks the independent owner signature, environment,
expiry, native PROCESSED event/unit fingerprint and monotonic context revision.
History and current head commit atomically; revocation blocks subsequent source
resolution and cannot be undone by an older allocation. History failure rolls back
the head. This ingestion endpoint issues no signatures, source mappings or complete
coverage assertion. Owner-side approval issuance, automated
producer export/publication integration still require engineering and governance.


## Native binding preparation

Normal migration 010 adds immutable binding-preparation audit. Under the same
publisher persona, fixed GET `/internal/analytics/v1/admin/source-contexts/{owner}/bindings/{report}:{native-field}/{event-id}?purpose={purpose}`
returns `DRAFT_PENDING_OWNER_APPROVAL`, the native fingerprint/source contract and
known canonical product only. Geographic, organization and tenant dimensions remain
empty. Ten report names and five native unit field selectors are explicitly
bounded; event type/unit validity is checked against the actual PROCESSED event.
`PublisherClient.PrepareBinding` accepts only a single path segment for kind/event
ID, rejects path/query injection and validates the exact contract and draft state.
No proposed mapping is accepted. The audit must commit before any binding leaves
the owner; failure returns no fingerprint. Owner review and independent signing
remain mandatory before publishing an allocation. This eliminates direct database
queries for preparing fingerprints, without manufacturing approval or coverage.


## Mission-success rate projection

The private HQ GET `/internal/karryam-admin/v1/analytics/scoped/network/mission-success/rate`
now derives `completed/(completed+failed)` exclusively from the existing privacy-
released component counts. It requires both `analytics.scoped.mission-success.read`
and `analytics.scoped.mission-success-rate.read`, all existing workload/staff/
five-scope controls, and `ADMIN_ANALYTICS_SCOPED_RATE_READ_ENABLED=true` in addition
to the global/scoped counts flags. The dedicated approved rate rule must exactly
name `analytics.scoped-mission-success-rate.v1:<the-counts-rule-unit-field>`.
An old counts-only approval or a rate-only delegated permission is insufficient.
The native owner still independently authorizes/discloses the underlying counts.

The response names the formula and `DISCLOSED_COMPONENT_COUNTS` basis: approved
source count rounding is retained, never replaced by raw/private inputs. Partial
coverage, stale data or suppressed completed/failed cells yield a null rate and
an explicit reason. A zero denominator remains unknown; invalid/overflowed source
arithmetic fails closed. No cohort members, cohort size or private witness is
returned. This deterministic projection adds no information beyond counts the
same staff must already be authorized to receive. Native frozen membership,
privacy journal, budgets and reconstruction guards still apply to the underlying
count request; no unscoped legacy endpoint is called.

The actual native source remains obligatorily partial and therefore yields no
production rate. The numeric 3/(3+1)=0.75 boundary scenario is explicitly TEST_ONLY,
not a complete producer export or production source proof. This closes the rate
calculation/facade step while complete verifiable membership/export integration,
the remaining nine report reducers/routes and initial privacy policy stay pending.


## Vendor readiness lineage prerequisite

Vendor now has the same separate fixed publication and draft-binding preparation
paths under `/internal/analytics/v1/admin/source-contexts/vendor-service`.
`ANALYTICS_ADMIN_SOURCE_CONTEXT_KEYS_FILE` may pin its independent Ed25519 keys;
`ANALYTICS_ADMIN_CONTEXT_PUBLISH_SECRETS_FILE` requires a distinct Vendor workload
key. It cannot reuse another producer, scoped-read or legacy service key.
There are four compiled publisher owners; arbitrary owner names remain rejected.

The native readiness event is `vendor.branch.readiness_observed.v1`; the branch
unit must be a recorded canonical UUID. Preparation returns only the immutable
native lineage fingerprint and known product. Operational zone hints do not
supply market/zone/organization/tenant authorization. A current independently
signed Vendor allocation is required, including after publication; monotonic
revocation and immutable history apply. Driver keys/signatures cannot publish
Vendor authority, and Vendor publication grants no HQ report access.

Real native ingest → runtime → signed HTTP → typed SDK preparation/publication
and revocation is tested with an ephemeral TEST_ONLY allocation. Production
runtime rejects TEST_ONLY, and all defaults remain disabled. The readiness
reducer, scoped report route, complete membership exports and initial approved
privacy/access rules remain missing; no readiness value is released by this step.


Vendor source audit: `cmd/readiness_orchestration_wiring.go` emits readiness
from the native `vendor_order` transaction with a recorded branch ID. The lineage
validator now requires that actual entity contract and a canonical order UUID;
a fabricated branch snapshot cannot acquire a binding. Business/branch IDs are
not treated as organization/tenant authority. The production runtime rejects the
TEST_ONLY allocation before the native test installs its ephemeral authority.
Validation: 313 combined Bridge/Taxi/Analytics/policy cases passed with no skips;
final Analytics/SDK regressions also pass. Source route capture/classification,
OpenAPI drift and route-register checks have zero violations/stale entries.


A private readiness component Source adapter is now implemented and tested over
native Vendor order facts for explicitly selected branch/order units. It remains
unregistered, always partial and fully suppressed; it implements no score or HQ
readiness route. Native timing unknowns deny, distinct units are deduplicated and
frozen original-member revocation is enforced. See the
[precise remaining dependencies](readiness-cohort-reducer.md). Eight other report
reducers and nine non-mission scoped report transports remain engineering work,
along with complete verifiable owner membership exports for all reports.
