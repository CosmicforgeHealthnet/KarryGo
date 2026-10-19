# Analytics disclosure and suppression policy v1

**DRAFT — PENDING OWNER APPROVAL**

Status: **BLOCKED_POLICY**. Production cohort sizes, rounding, query budgets,
freshness limits, allowed dimensions/values and release periods are **unset**.
The A-to-Z specification defines requirements; it does not approve these values.

## Proposed release mechanism

* Only owner-resolved, scoped cohort facts may enter the disclosure engine.
  Legacy unscoped network reports are not an approved source adapter. Aggregate
  counts are not automatically a count of distinct subjects; cohort provenance
  must be established by the owner query/metric definition.
* An approved policy explicitly sets minimum cohort size, rounding multiple,
  freshness, fixed release bucket, query budget, cell allowlist, exact permitted
  filter-key combinations and permitted filter values. No permissive defaults
  exist. Only clearly named `TEST_ONLY` signed fixtures contain test values.
* Small or unknown cells trigger whole-release complementary suppression. The
  engine withholds all complementary cells and never releases raw cohort sizes
  or totals. Unknown, stale, partial, suppressed and verified zero are distinct.
* Approved rounding is deterministic downward rounding. Raw counts do not enter
  the immutable released cache or query audit. Missing approved cells or unknown
  labels refuse disclosure rather than being assumed zero or dropped silently.
* Requests use aligned fixed buckets. A durable privacy-domain journal reserves
  all intersecting UTC days for one immutable release shape, across users,
  reports, filter choices, process restarts and changing source versions.
  Alternate filters/reports/windows over an already reserved partition are
  refused. An identical repeat returns the same rounded/suppressed snapshot;
  stale cached values are withheld, never refreshed into a differencing oracle.
* Policy changes cannot resample an old release partition. Approval rollback and
  same-version rewrites are refused. Every cached read rechecks owner scope.
* Query budgets apply across the staff member's queries, independent of purpose
  and query shape. Budget exhaustion, restricted filters, source-scope denial,
  attempted reconstruction and successful/frozen disclosures are audited in
  Analytics' own database. Audit failure releases no data and rolls back a new
  release/partition.

This is deliberately conservative pending review: reserving whole UTC days can
deny otherwise useful sub-day reports, and a broad privacy domain can permit
only one report/filter shape for that scope/day. Reviewers must assess the
complete approved release cube and cross-report composition before changing
that restriction. This is not differential privacy or a claim that arbitrary
combinations of approved reports are safe. No such broad disclosure is enabled.

## Approval-ready values

| Decision | Production value | Responsible reviewer | Required evidence |
|---|---|---|---|
| Minimum distinct cohort size | Unset | Privacy + Analytics | Re-identification assessment and owner cohort query |
| Complementary suppression | Proposed whole-release withholding | Privacy | Subtraction and missing-cell tests |
| Rounding | Unset | Privacy + metric owner | Utility/privacy evaluation; no raw-count side channel |
| Allowed dimensions/filter values | Unset | Domain + Privacy | Rare-combination and cross-report assessment |
| Privacy-domain grouping | Unset | Analytics + Privacy | All overlapping metrics and cohorts grouped together |
| Fixed release bucket and timezone | Unset | Analytics + domain | Canonical UTC windows, source reconciliation, approved display semantics |
| Freshness threshold | Unset | Metric owner | Source watermark/delay evidence; stale/partial handling |
| Per-staff query budget/window | Unset | Security + Privacy | Concurrency, repeat-query and cross-user threat assessment |
| Retention/archival | Unset | Privacy + legal/governance | Preserve disclosure partitions for all still-queryable history |

Do not delete disclosure guard records as a cache cleanup. Purging a guard while
the same historical window remains queryable reopens reconstruction. A separate
reviewed retention/epoch design is required before archival or policy-domain
changes. Field-level metric definitions are in Analytics'
`docs/admin-metrics-dictionary.json`; their null approved-policy values remain
intentional.

## Release gates

The framework has unit and real PostgreSQL tests. Actual HQ Analytics reports
remain disabled until all ten owner adapters have approved scope/cohort
provenance, signed production policy, reconciled source tests, scoped transport,
and relevant security/CI evidence. A test-only cohort fixture certifies the
mechanism, not the production report or its governance approval.

Engineering continuation (2026-10-19): [source-owned resource context contracts](../../admin-bridge/owner-resource-contexts.md) provide signed owner remediation and deny-by-default typed reads. This draft supplies no context signature or production allocation. The sixteen-kind initial approval matrix remains **BLOCKED_POLICY**; Analytics thresholds/cohorts, Media grants and command approvals remain separate requirements.

## Canonical lineage validation (engineering only)

`adminprivacy.LineageReader` validates a fixed report/unit-field selection
against Analytics' retained, PROCESSED canonical event, original producer,
event classification/version and recomputed minimized-payload digest. It binds
that fact to an independently signed source-owner context using the existing
context framework. An analytical zone, including a GPS geohash, supplies no
market/zone/organization/tenant authorization. Missing dimensions stay unknown
until legitimate source-owner allocation and governance approval.

Repeated provider facts use the same owner/provider deduplication key across
original/replacement role fields. Source-entity keys retain their exact native
type and identity; no booking/Trip relationship or cross-owner person identity
is inferred. These hashes are internal identifiers, not anonymization. The
privacy unit remains UNAPPROVED and no production threshold is supplied.

Supply snapshots and demand buckets have no retained individual roster. Their
counts and bucket identities are explicitly rejected as individual membership,
even if an incidental provider field exists. Existing scalar counts cannot
backfill a legitimate distinct-subject cohort. Complete native roster/export
contracts, membership coverage, reconciliation, all ten report reducers and the
scoped reporting facade remain engineering work. Installing Analytics migration
006 or preparing a signed unit binding enables no report and supplies no scope
grant. The report registry covers all ten dictionary IDs but is not ten completed
cohort adapters. Every affected HQ report remains disabled.

`MissionSuccessSource` now implements the first bounded native source adapter.
Its privacy unit is an explicit field in the source contract, with no default.
Every contributing event requires a current independently signed source-owner
context; authority, product and UTC window filters use actual event facts.
Source events are held stable while their context/unit is checked. Distinct
provider UUIDs normalize before deduplication. It always sets partial coverage
until a legitimate membership/export proof is implemented, and the disclosure
service withholds all values. This software behavior is tested; neither the
unit choice nor production scope/threshold values are approved by this draft.
The other report adapters and reporting facade remain unimplemented.
