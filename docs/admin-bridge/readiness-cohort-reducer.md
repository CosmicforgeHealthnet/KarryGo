# Private Vendor readiness cohort reducer

Status: tested internal source adapter; **UNEXPOSED / BLOCKED_POLICY**.
There is no runtime flag, registration, owner HTTP endpoint, typed report client
or HQ readiness route for this adapter. Production cannot select it by configuring
an existing counts or rate flag.

`ReadinessSource` implements the existing transactional privacy Source port.
It requires the distinct `analytics.scoped.readiness-components.read` permission,
`analytics.scoped-readiness-components.v1:<explicit native unit>` source approval,
Vendor authority and exact product filter. The only supported native unit fields
are explicitly selected `branch_id` and `source_entity_id`; neither is a default
or an approved production privacy unit.

It reads at most 100 independently owner-authorized native PROCESSED readiness
events within the exact requested scope/window. Native `vendor_order` UUID,
source version, minimized payload hash, recorded branch UUID and current Vendor
allocation are checked. Operation/GPS hints and business ID supply no scope.
A branch UUID deduplicates repeated branch events; order units use distinct
canonical native order IDs. Native bucket totals are never cohort memberships.

Private components match the native readiness projector: ready/on-time/late
counts, lateness seconds total, preparation seconds total and preparation count.
Missing, malformed, fractional, negative, overflowing or contradictory timing
facts deny instead of becoming false/zero. Explicit zero preparation remains
observed zero and contributes no invented preparation-duration sample. These
components are not a readiness score or approved rounded rate.

Every snapshot is **PARTIAL**, even when every selected row has an authorization.
That is not proof of a complete owner export. The disclosure framework withholds
all values, applies approved query budgets/filter/composition rules, and atomically
stores release/audit/original-member witness. Frozen releases recheck every
original member; revocation denies the old release without resampling it.

Real PostgreSQL tests ingest native events and independently signed TEST_ONLY
allocations for both candidate units. They verify native totals, deduplication,
withholding, old-member revocation despite surviving general scope, wrong authority
and unsupported-unit denial. Generic disclosure audit-failure/reconstruction
regressions apply to the same source port. Mandatory CI requires these tests.

Remaining engineering: complete verifiable Vendor membership/coverage export and
producer integration, scoped source HTTP/SDK/HQ transport, policy-reviewed derived
metrics and target-environment certification. Vendor, Analytics and Security/Privacy
must approve cohort unit, roles, scope, purpose, composition, rounding, freshness
and retention before any production activation. No such values/grants are added.


Readiness reducer validation: 317 combined Bridge/Taxi/Analytics/policy test cases
passed with real PostgreSQL, zero skips, plus Go vet, deterministic contracts and
independent actual-wire validation. Mandatory Linux race/acceptance remains a
separate current-head CI gate. No production capability is enabled by this result.
