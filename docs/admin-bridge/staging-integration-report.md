# Secure Admin staging integration — full implementation report

## Deployment-safety revision — 21 October 2026

This draft continues from `fa66ab5eb9895c7764981ee07068fb81f82d138c`.
Before edits, the review branch still matched that SHA; remote staging had moved
to `75aee3c64f6621207286a9699aeceb8f8151379a` (three newer commits). No staging
merge/rebase or deployment was performed. The original 13 staging-only commits
remain ancestors. Reassess current staging drift and mergeability before any
later explicit merge approval.

Both platform and Vendor deployment workflows are now manual-only, with exact
current staging SHA, explicit service selection, separately verified GitHub
environment approval, authorized initiator, environment-only credentials, exact
SHA acceptance and approved restore provenance. Neither push nor CI completion
automatically deploys owners. Broad selectors, test skipping, workflow reuse and
Dispatch/Admin/Analytics/Gateway/Partner selectors are rejected. Release images
are pinned by digest, authorization is rechecked before host access, and evidence
is preserved before mutation. No Nginx/cutover files or live settings are replaced
by the new release bundle mechanism. Admin default-disabled flags are retained
and the release script refuses enabled privileged flags.

The restore helper now propagates every failed command/SQL/validation and requires
artifact integrity, matching PostgreSQL version and operator-owned semantic
assertions. A genuine disposable PostgreSQL 18 backup/restore cycle passed locally,
including preserved audit trigger, migration ledger, original decision identity
and uncertainty fence. Seven failure scenarios were rejected. Linux CI additionally exercises the exact
network-isolated Docker verifier; its readiness check waits for TCP on the final
server, excluding the image entrypoint's temporary Unix-socket initialization server. The actual Dispatch
deployment and rollback shell scripts refuse unsafe transitions before host tools.
There is no automated Dispatch transition; live compatibility/consumer retirement
and recovery must be certified separately.

**Actual GitHub setting verification:** the staging environment API still showed
no required reviewers, admin bypass enabled and no branch restriction; separate
secret metadata responses show an empty environment and repository-scoped legacy
`VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY` credentials; the new
gate independently denied it. Initiator/reviewer identities have been requested,
not invented. New scoped release credentials, audit token and real staging restore
evidence have not been provisioned. Consequently the protected release process is
**BLOCKED_INFRASTRUCTURE**, not operationally certified. No production policy,
signing identity, access grant, suppression threshold or Media lifetime was added.

GitHub now reports conflicts with the three newer staging commits and does not
schedule PR-triggered CI. No native conflict resolution is attempted in this
deployment-only task. All six existing acceptance workflows remain mandatory and
now run on every PR/staging push and on pushes to this exact review branch; the new Deployment safety acceptance also certifies the exact
SHA. Fresh CI run IDs/results for the final revision are recorded in draft PR #15
and the final review evidence rather than copying old-head results here. Until
those results and actual infrastructure protection are confirmed, the merge and
runtime decisions remain **NO-GO**. No merge, server deployment, feature enabling,
Partner/Gateway cutover or Sections 11–15 work is authorized by this report.

See [the deployment gate runbook](../deployment/staging-release-approval.md) for
exact API configuration instructions, workflow inputs, evidence schema, rollout
boundaries and Dispatch stop/migrate/verify/start/recovery order.

---

Status: **PREPARED INTEGRATION; NOT MERGED, DEPLOYED OR BACKEND_READY**.
Date: 2026-10-20. This report describes the integration candidate; current-head
CI results and its final commit SHA are recorded in its staging-targeted PR.

## A. Branches, dependencies and actual staging status

The earlier development push went to `codex/secure-admin-sections-6-10` at
`12bf3bea03aad90e93117e18764772d4e1fc32d6`, not to staging. PRs #12, #13 and #14
were all open drafts and unmerged when this integration began.

The user subsequently authorized: **“Prepare staging integration with unapproved
capabilities disabled.”** Accordingly, integration starts from the actual remote
staging head `9aed946bcd8d6a2347390e33726b7708e6aef452` on the separate branch
`codex/staging-secure-admin-integration-20261020`. It incorporates the entire
PR #12–14 dependency stack. Staging had 13 commits absent from the feature branch;
the feature branch had 81 commits absent from staging. Both histories are retained.
There is no force-push, direct staging update, deployment or live flag change.

Git merged the histories without textual conflicts. The three automatically
merged files were reviewed: Taxi main, Hauling main and the Taxi environment
example. Newer staging initialization and existing customer/provider integrations
remain alongside the private default-disabled Admin registrations.

A contract metadata inconsistency found during this report was corrected: the
Hauling custody list/detail entries incorrectly named `driver-hauling-delivery-service`.
The canonical owner is `driver-hauling-service`, which runtime code already used.
Independent contract validation now checks all resource owner names. Native Admin contract JSON and authorization migration/schema SQL also have explicit LF checkout rules, correcting a Windows CRLF drift exposed during integration testing. Regenerated
OpenAPI/checksums retain the existing request, response and authentication schemas.

## B. Implemented scope and precise operating status

### Foundation and Partner/Gateway dependency work

The Admin Bridge implements private HQ authentication, delegated staff assertions,
bounded typed requests, durable replay protection, idempotent operation identity,
immutable audits, source-owned authorization and separate migration/runtime roles.
Ingress requires mTLS, HQ HMAC v2 and an Ed25519 staff assertion. The Partner Web
and public Gateway separation is included because PR #14 depends on it. The
Admin, Gateway and Partner cutover-sensitive services remain excluded from broad
automatic deployment selection. Their migrations/network/callback cutover require
the separate documented rollout and target-environment checks.

### Scoped resource reads: 26 kinds / 52 fixed list-detail paths

| Native owner | Resource kinds |
|---|---|
| Customer | customers; sharing preferences exposed under the existing consents route; booking-references; review-states |
| Taxi | providers; vehicles; independent-rides; recoveries |
| Dispatch | riders; motorcycles; missions; custody-missions; incidents; delivery-recoveries; returns; handover-states |
| Hauling | providers; trucks; missions; custody-missions; recoveries; restrictions; fit-decisions |
| Fleet | shipments; private-assignments; execution-links |

These are compiled, tested capabilities, not production access grants. The owner
and Bridge must both be enabled, and current source authorization, approved
policy and delegated staff scope must all authorize the request. Unmapped rows
remain inaccessible. Unknown source facts stay unknown. References do not confer
access to the referenced object's entire profile.

### Section 6 — Analytics

Implemented: ten fixed-path legacy clients; native event lineage validation;
independently signed producer context preparation/publication/revocation for Taxi,
Dispatch, Hauling and Vendor; the first scoped mission-success counts source,
native HTTP endpoint, typed SDK and private HQ facade; a separately authorized
rate projection over already disclosed completed/failed counts; and an internal
Vendor readiness component reducer.

Privacy enforcement supports configured minimum cohorts, small-cell/complementary
suppression, approved rounding, restricted filter combinations, query auditing,
query budgets, immutable cached releases and reconstruction safeguards. Cached
releases reauthorize their original private membership witness; they cannot
resample away a revoked member. Bucket totals never establish distinct membership.

**Current mission-success coverage is always partial and all values are withheld.**
The rate remains null for partial, suppressed, stale or undefined inputs. Numeric
complete-source examples are TEST_ONLY. Readiness reduces native vendor_order
observations into ready/on-time/late counts and timing components, deduplicates
explicit branch/order unit candidates, rejects invalid timing facts, and likewise
always withholds partial results. It has no runtime registration or report route.
No production privacy unit, threshold or grant has been selected.

Remaining: complete independently verifiable owner membership exports and producer
integration for all reports; eight other reducers; nine non-mission scoped report
transports; reviewed derived metrics; target-environment certification. The ten
reports are mission-success, recovery, capability-mismatches, declines, demand,
provider-quality, readiness, supply, mission-outcomes and demand-forecast.

### Section 7 — Customer

Implemented: source-owned directory, sharing-preference, authorized booking-reference
and review-state reads. The source validates actual Customer parent records.
Native review intake records a durable PENDING_REVIEW work item with immutable
original receipts and separate signed POST/GET contracts. The Bridge commits a
claim and audit before one owner POST; uncertainty, restart and dropped responses
allow only original GET reconciliation. Readback survives write kill switches.

**Review intake changes no account standing, verification, restriction, legal
consent, privacy fulfillment, Trip or Wallet state.** Sharing preferences are not
proof of legal consent. Richer lifecycle, verification, privacy/support contracts
and legitimate owner administrative commands remain unfinished.

### Section 8 — Taxi

Implemented: corporate ride list/detail/history plus provider, vehicle,
independent-ride and native passenger/goods recovery read models. Actual Taxi
market/zone relationships are checked. Goods scope is never inferred from GPS,
quotes or Vendor strings. Owner allocation preparation/publication supports
otherwise unmapped records without granting access automatically.

Remaining: separate incident/remediation contracts, richer verification/expiry
semantics, executable restrictions/interventions and live protected Media access.
Unknown mutation versions cannot be used to authorize administrative writes.

### Section 9 — Dispatch

Implemented: the eight resource kinds above; actual Trip/booking/attempt parent
validation; bounded typed custody and multi-stop inventory; exact frozen manifest
selection; minimal stored secure-handover metadata without OTP/QR/challenge,
recipient contacts or evidence URLs. Native assignment/offer readbacks preserve
original action identity. Uncertain Fleet actions reconcile by GET only with
atomic audit and per-claim leases; they cannot trigger a replacement POST.

Remaining: executable HQ retry/replan/reassignment/recovery interventions, richer
incident/return detail and live protected Media integration. No forced completion,
recipient bypass or custody override is introduced.

### Section 10 — Hauling and Fleet

Implemented: the ten resource kinds across Hauling/Fleet above. Truck reads retain
recorded physical capacity/equipment true/false/unknown facts. Canonical planning
organization/tenant validation excludes settlement ownership as authorization.
Restriction reads require actual provider/asset subjects and current organization
links. Historical fit reads validate booking/provider/truck/captured mission
parents and expose stored outcome/shadow/captured revision metadata only.

**Historical ELIGIBLE is not current eligibility or assignment authority.** Raw
fit evidence, distance, contacts and inspection URLs are excluded. Captured
capability counters are not full mutation versions. Fleet original-action
receipts grant no current execution authority. Remaining work includes protected
inspections, richer suitability/cargo-fit flows, remediation/intervention commands
and live Media mapping. Native cross-organization restriction effects must not
be exposed as an organization-only HQ apply/clear command.

### Crosscutting protected Media

Implemented and tested: native safety/revocation repairs; case-grant enforcement;
immutable grant/revocation ledger; policy revision pins; separate signed private
HTTP boundary and typed SDK; durable nonces; current staff/scope/case/subject/object
checks; per-redemption reauthorization; atomic audit; and separate read/withdrawal
kill switches. Native current safety and evidence revocation block old ambient
tokens where applicable. The new case interface returns attachment bytes, not
storage URLs or borrowed owner credentials.

**Production runtime supplies no live case authority adapter.** Flags and policy
files alone cannot disclose case bytes. Tests explicitly use TEST_ONLY case proof.
Live source-case adapters, canonical modern-object membership/remediation and the
HQ facade remain unfinished. Legacy media_assets IDs cannot be assumed to be
modern Media object IDs. Existing Fleet lifecycle restrictions remain authoritative.

## C. Security boundaries and governance state

Effective access is the intersection of delegated staff permissions, current
independent source-owner authorization and approved policy across product, market,
zone, organization and tenant. Missing/ambiguous mappings deny. Current native
parents, source fingerprints, policy identity/version and revocation are rechecked.
No GPS inference, settlement fallback, wildcard tenant or caller-supplied tenant
can replace canonical authority. Policy rollback and same-version rewrites deny.
Audits must persist before disclosure or owner command submission.

All three policy documents remain **DRAFT — PENDING OWNER APPROVAL**:
- market-zone-tenant-mapping-v1.md
- analytics-suppression-v1.md
- admin-data-access-matrix-v1.md, with its machine-readable JSON matrix

No production roles, scope tuples, policy values, signing keys, suppression
thresholds or Media grant lifetimes are invented. TEST_ONLY approvals are rejected
by normal production builds. No generic status override, direct Wallet/Trip
mutation, forced completion or custody bypass exists.

## D. Validation and evidence

The original feature head `12bf3bea` passed all six acceptance workflows: Admin
Bridge; native owners/Media/Dispatch safety; Customer V4; Vendor; Partner/Gateway;
and Section 31. Its Linux evidence includes 317 core cases, 114 owner-authority
scenarios across all five native migration suites, 166 Media cases, 437 Dispatch
safety scenarios, 45 Fleet receipt scenarios and 11 custody scenarios, with no
failed/skipped required tests. These results certify that head, not this merge.

The staging integration reruns affected core/owner suites against disposable
PostgreSQL/Redis, builds the merged actual Taxi process, verifies actual wire
schemas and old v1 compatibility, and reruns all six acceptance workflows on the
combined commit. Local combined-state validation passed: **317 core cases and 114 source-authority
scenarios, zero failed/skipped required tests**, using PostgreSQL/Redis and the
merged actual Taxi executable. Actual Bridge/native wire validation, all four
contract generators, v1 compatibility against the feature head and Go vet passed.
The initial Windows CRLF failure was corrected with canonical checkout rules and
the full core suite rerun successfully. Current-head Linux results are recorded
in the integration PR; pending checks must not be treated as passing. Contracts include OpenAPI 3.1, 16 JSON
schemas and 22 deterministic bundle artifacts. The source owner/Analytics/Media
contracts have separate validators.

Four Vercel preview checks on the earlier PR were rate limited: fleet-karryam,
karry-go, karry-go-fleet and karryam-landing. Passing the six acceptance workflows
does not mean those preview deployment checks passed or any site was deployed.
Actual preview/check status must be reviewed separately on the integration PR.

## E. What can operate, what remains disabled, and outstanding risks

| Category | Disposition |
|---|---|
| Existing native safety and original-action reconciliation fixes | Can retain existing owner authorization/enforcement semantics when integrated and deployed through their normal reviewed process; this preparation deploys nothing |
| New HQ resource reads, review intake and Analytics access | Default disabled; require approved governance, current owner authorization, enrolled workload credentials and target validation |
| Readiness component source | Internal/unregistered; cannot be activated by existing report flags |
| New Media case disclosure | Disabled and missing live case adapters; flags cannot replace those adapters |
| Generic privileged intervention families | Unsupported/blocked until legitimate owner contracts and safeguards exist |
| Initial mapping/privacy/retention/role decisions | BLOCKED_POLICY; software tests are not owner approval |
| Real HQ/native owner environment and cutover | Not certified by local/CI fixtures; deployment/callback/network steps remain separate |

Source engineering gaps and policy approval gaps are distinct. Approving policy
will not implement missing reducers, exports, commands or case adapters. The
A-to-Z PDF was referenced but not supplied; available pasted requirements and
actual native source contracts governed the implementation.

## F. Staging integration and eventual rollout

This candidate is submitted as a separate draft PR targeting staging. Preserve
all staging commits and require current-head checks. Review the entire dependency
stack, generated contracts, canonical owner correction and this report. Keep
ADMIN_REMOTE_ENABLED, domain read/write/recovery flags and owner Admin read/write
flags disabled. Keep Analytics source publication/report and Media case flags
at their default false; enroll no production grants, approvals or test keys.

Merging this candidate is not approval to enable privileged capabilities or to
perform the Partner/Gateway callback cutover. The dedicated runbooks specify
migration/runtime role separation, normal owner migrations, private routing,
credential audiences, rollout sequencing and independent kill switches. Do not
force-rewrite staging, bypass owner APIs, rollback populated immutable ledgers,
or replay unknown mutations under new operation identities.

## G. Delivery conclusion and references

**The staging integration candidate is reviewable; Sections 6–10 remain partially
implemented and are not BACKEND_READY.** No staging push/merge, live deployment,
callback cutover or privileged activation is implied by this report.

Detailed references in this directory: sections-6-10.md; sections-6-10-runbook.md;
customer-review-intake.md; owner-resource-contexts.md; scoped-analytics-transport.md;
readiness-cohort-reducer.md; dispatch-handover-read-model.md;
hauling-fit-decision-read-model.md; hauling-restriction-read-model.md;
media-case-access-framework.md; provider-decision-readback.md. Policy documents
are under ../admin/policies (repository docs/admin/policies). The capability
matrix and pinned schemas are under services/admin-backoffice-service/docs/openapi.
