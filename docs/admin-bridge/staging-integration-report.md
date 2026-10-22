# Secure Admin staging integration — current backend status

## 22 October 2026 — direct staging continuation after merged PR #15

**Code integration is verified; Sections 6–10 remain PARTIAL.** This is not a
BACKEND_FUNCTIONALLY_COMPLETE declaration. This dated report supersedes the
historical unmerged-PR and deployment-readiness statements retained below.
Work began from verified staging `f16fa7eda2558178cdab5f9cfc55659123f132aa`.
All new work is committed directly to staging using ordinary fast-forward-safe
Git operations. The baseline remains an ancestor. No historical Dispatch branch,
new branch, worktree, PR, reset or force-push is involved. The two pre-existing
untracked cleanup scripts are left untouched.

### Implemented and tested in this continuation

| Scope | Status | Completed engineering | Remaining engineering prerequisite |
|---|---|---|---|
| Section 6 Analytics | PARTIAL | Ten fixed scoped native routes, typed SDK and private Bridge paths; existing mission counts/rate preserved; readiness reused; source-backed outcome, recovery-event, mismatch, decline, individual-demand and quality component reducers; signed-source audit, membership recheck, frozen disclosure and suppression tests | Independent complete owner membership/coverage exports and remaining producer bindings; individual supply roster; complete scoped history for the existing forecast model; resolution-duration and quality-score semantics. Bounded components are partial and fully suppressed, not completed business reports |
| Section 7 Customer | PARTIAL | Versioned source-owned REQUESTED → ACKNOWLEDGED → CLOSED review work items, immutable transition ledger, idempotency, concurrent request fences, typed Bridge original-action GET reconciliation and native case evidence authorization | Canonical restriction/unrestriction and privacy/support disposition state machines, scoped delegated actions, independent authoritative result contracts. Closing a review explicitly changes no account standing or consent |
| Section 8 Taxi | PARTIAL | Existing operational read models preserved; native recovery case/subject evidence authority, actual vehicle membership, active/terminal-state checks, signed scopes and audit | Independently versioned incident/remediation contract and HQ action approval/readback bindings; canonical document-expiry interpretation and verification decisions. Existing operational recovery methods do not by themselves supply a safe delegated command contract |
| Section 9 Dispatch | PARTIAL | Incident recovery and return execution observations; UNKNOWN retained; native return child mission/booking/execution/group relationships checked; native incident/recovery/return evidence authority | Case-bound expected-version intervention contract, explicit HQ actor authority/approval and independent original-action readback for recovery/retry/reassignment/replanning |
| Section 10 Hauling/Fleet | PARTIAL | Native product-admission window observations, active organization membership and stale-callback checks; current booking cargo inputs with native units/null unstated weight; native recovery evidence authority; existing Fleet tenant and receipt boundaries preserved | Inspection/conclusion authority and modern object mapping; full versioned remediation/intervention contracts; standalone Fleet investigation-case ownership. Recorded admission/history does not authorize a new assignment |
| Shared Media | PARTIAL integration coverage; supported software path implemented | Four source owners / six finite case kinds; modern object ownership/subject/case binding; fixed private Bridge grant/content/own-grant revoke facade; current safety/quarantine/revocation/version/expiry and immutable disclosure audit | Authoritative legacy-asset/inspection-reference mapping and standalone Fleet case authority are absent. No implicit aliases are accepted |

Source constraints are concrete, rather than a request to weaken authorization:

- Analytics' legacy supply observers retain aggregate buckets, not independently
  authorized individual membership. Counts and incidental provider identifiers
  cannot become cohort membership. Native demand forecast already exists in
  `networkanalytics/usecases/service.go`; missing complete scoped historical input
  cannot be repaired by reusing global aggregates or inventing rounding.
- Customer review intake and queue transitions own review work, not customer
  standing, consent, privacy fulfillment or another domain's records.
- Taxi provider license expiry is stored as free text; this does not establish
  canonical expiry timezone, document decisions or a vehicle verification ledger.
  Taxi goods recovery selects through native capability/custody checks, but its
  existing operational actor/operation parameters do not establish delegated HQ
  expected-case-version authorization and immutable original-action readback.
- Dispatch `deliveryrecovery.RecordEmergencyRecovery` exists with Trip version,
  but does not provide the requested case-bound delegated HQ command and readback
  contract. Current execution observations cannot substitute for that contract.
  No customer identity is impersonated to call customer-owned recovery choices.
- Hauling `recovery.SelectReplacement` exists, but its booking/actor interface is
  not a versioned HQ intervention contract. `inspection_ref` and certification
  references are opaque references, not modern Media IDs or inspection findings.
  Native `requirementFor` uses current booking inputs; no full frozen requirement
  snapshot is fabricated. Global restriction effects are not tenant commands.

These remaining contracts and producer coverage are **engineering gaps**, not
merely production policy approvals. No unsupported command returns fabricated
success. The above implemented subsets do not close every requested work package.

### API and migration inventory

Relative to baseline `f16fa7ed`, the private Bridge contract adds **24 fixed
paths/operations**: nine scoped Analytics paths, Customer work-item command and
readback, ten list/detail paths for five additional resource kinds, and three
Media grant/content/revoke paths. The current OpenAPI has **98 paths / 98
operations / 98 capability entries**, including blocked catalog entries;
**31 resource kinds / 62 resource paths**, **33 standalone schemas** and
**39 checksum-pinned generated artifacts**. These are contract counts, not a
claim that all catalog entries are enabled or functionally complete.

Owner contracts add nine scoped Analytics report routes, two Customer transition
route templates, and three fixed case-authority route templates per enrolled
source owner (Customer, Taxi, Dispatch, Hauling). The generic owner resource
transport gains explicit dictionaries rather than arbitrary proxy routes.
Source OpenAPI metadata describes new Dispatch/Hauling read contracts. Five owner
OpenAPI contracts, scoped Analytics and private Media contracts are validated.
No public Swagger or Gateway exposure is added.

New migrations are Customer `017_admin_review_work_items.sql` and Bridge
`005_customer_work_item_operations.sql`, with corresponding least-privilege runtime
grants. They run only in disposable tests in this work. Dispatch migrations
000070–000072 and their consumers, uncertainty, authorization and replay fences
are unchanged. No live migration, stop/start sequence or rollback was executed.

### Verification evidence

Code checkpoint `b8d50d6c801ec5bf26a8e56cabaf4784a4517caa` includes the cargo
addition and native recovery state/subject correction. Local fresh suites ran on
isolated task-owned PostgreSQL 18 / Redis on loopback, using actual owner migrations:

- **470** Bridge/privacy/security cases, zero failed or skipped tests.
- **146** source authority scenarios; all five owner migration suites executed,
  zero failed or skipped tests.
- **201** Media cases, zero failed or skipped tests, including an actual Customer
  executable → native work-item transitions → modern Media upload/scan/storage
  and private SDK evidence access. This is separate from Bridge mTLS/SDK tests;
  no live HQ enrollment or deployed certification is implied.
- Rebuilt the actual Taxi test executable. Native signed HTTP, real Bridge mTLS,
  cross-tenant denial, durable replay, audit persistence failure, concurrent and
  stale versions, original-action unknown-outcome recovery, case closure/revocation,
  foreign subject and actual legacy-asset rejection are exercised.
- Source wire validators, OpenAPI/schema generation, checksum manifests,
  backward compatibility, source contract checks, `go vet` on affected packages
  and `git diff --check` passed. Local Windows suites omit `-race` because no
  native C compiler is installed; canonical GitHub Linux workflows retain it.
- Actual Customer V4 route capture after dormant-route registration found zero
  undocumented/phantom routes or exposure/security mismatches. Initial failures
  (canonical LF checksum, disabled-route registration) were corrected without
  relaxing acceptance assertions.

Ignored local evidence is under `.codex-cache/sections6-10-checkpoint6-*.jsonl`,
`sections6-10-wire`, `sections6-10-owner-wire` and `sections6-10-routeaudit`.
CI retains its normal workflow artifacts; local caches are not repository artifacts.

All seven mandatory workflows passed on code checkpoint
`05723a6790082a636e04ec00e23833de6e94835b`:

| Workflow | Successful GitHub run |
|---|---|
| admin-bridge.yml | [39806447083](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/39806447083) |
| admin-owner-closure.yml | [39806447025](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/39806447025) |
| section31-mandatory.yml | [39806447071](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/39806447071) |
| customer-v4-certification.yml | [39806447056](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/39806447056) |
| vendor-section46-48.yml | [39806447018](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/39806447018) |
| partner-gateway-split.yml | [39806446916](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/39806446916) |
| deployment-safety.yml | [39806446997](https://github.com/CosmicforgeHealthnet/KarryGo/actions/runs/39806446997) |

Those results certify that exact earlier checkpoint. Final-head acceptance and
its exact remote SHA must be verified separately; the completion report records
those results, not an inference from this table or a passing PR.

### Preserved security and separate release gates

Every new privileged switch defaults false. Missing owner mappings, approved
policy, source enrollment or disclosure settings deny. Dedicated workload keys,
HQ mTLS/HMAC v2, delegated staff permissions, five-scope intersection, signed
policy revisions/revocation, durable nonce/idempotency, original-action identity,
audit-before-disclosure and Gateway isolation remain enforced. No production
access grants, signing keys, suppression threshold or Media lifetime is installed.

Both backend deployment workflows remain manual-only `workflow_dispatch`, with
no push/PR/reuse/CI-completion trigger. Ordinary staging pushes therefore do not
invoke them. No extra merge approvers or hosting configuration are introduced.
Staging is development/integration; production and main are not modified by this
work. No VPS access, service restart, deployment, Admin activation or Partner/
Gateway cutover is performed. Initial governance policies, real HQ enrollment,
networking and hosting rollout remain separate external release gates.

---

## Historical PR #15 integration report (superseded for current implementation status)

# Secure Admin staging integration — full implementation report

## Code integration revision — 21 October 2026

Staging is the development/integration branch. Hosting and runtime deployments
are managed separately by the hosting developer. Optional release-workflow
reviewers, initiator configuration, environment protection and restore attestations
are **not prerequisites for ordinary staging code merges**. No approvers are
selected or repository settings changed by this integration.

Starting reference: `fa66ab5eb9895c7764981ee07068fb81f82d138c`.
The review branch incorporates staging `75aee3c64f6621207286a9699aeceb8f8151379a`,
including its three newer development commits. All original 13 staging-only
commits are preserved, together with the feature history. The Taxi environment
example conflict preserves staging test-login/internal approval configuration
and the disabled private Admin configuration. Automatically merged Customer,
Taxi and Hauling composition retains both source-owned staging behavior and
private Admin registrations. Public profile photos remain distinct from private
documents and disabled Admin case access.

**Trigger verification:** GitHub reports both old deployment workflows active;
the existing staging versions contain push-to-staging deployment hooks. This is
an actual repository-level discrepancy with the hosting confirmation, although
it does not establish that the VPS accepts those jobs. This candidate removes
both push triggers: platform and Vendor are manual-only, with no workflow reuse,
CI completion or PR deployment trigger. A merge push therefore cannot invoke
these backend deployment jobs. No host inspection or server mutation occurred.

Useful optional deployment safeguards remain: explicit commit/service selection,
backup/restore failure propagation, immutable image evidence and guarded Dispatch
transitions. Their incomplete GitHub environment setup affects only use of those
manual workflows. It does not prevent integrating code into staging. See the
[optional deployment runbook](../deployment/staging-release-approval.md).

All unapproved Secure Admin capabilities retain disabled defaults. No access
grants, signing keys, suppression thresholds or Media lifetime policy are added.
The existing staging driver-approval endpoint remains source-owned and HMAC
restricted; preserving that staging change does not enable Bridge remote access.
Dispatch migration 070 and consumer transition are not executed; deployment and
unsafe rollback remain blocked pending operator compatibility verification.

CI and service image builders pin Go 1.26.10 after acceptance found GO-2026-6809;
mandatory vulnerability scanning remains enabled. All six acceptance workflows
and Deployment safety acceptance run against the integration head. Exact SHA,
run IDs and final results are recorded in PR #15 and the final evidence report;
older-head results do not certify this revision. The PR frontend scope guard
accepts only newly added, exact review-branch Vercel preview holds; application
code, pre-existing configuration replacements and other branch settings still
fail. Negative tests cover that narrow exception.

Code merge is ready only once that head has six successful mandatory workflows,
no unresolved integration conflicts and preserved staging ancestry. Runtime
rollout is a separate hosting decision. PR #15 stays draft; this work does not
merge into staging, change main, deploy to the VPS, enable Admin functionality,
execute Partner/Gateway cutover or start Sections 11–15.

---

Historical integration snapshot below (20 October; superseded by the revision above).

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
