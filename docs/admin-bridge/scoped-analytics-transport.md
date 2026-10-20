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
producer publication pipeline, success-rate reduction, the other nine report
adapters and their scoped routes, and end-to-end environment certification.
Privacy/Analytics and source owners must approve units, mappings, suppression,
composition, freshness and release semantics before any production activation.
Never synthesize missing membership or retry a denied release under a fresh key.
