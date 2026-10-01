# ART-008 — Backend release blockers

Phase baseline `3007d6d42b60bf256578cf2688fe9e330a4a2798`.
No secret value appears in this document.

Classification:

- **P0** — release is blocked until closed.
- **P1** — release is not blocked, but a named flag must stay OFF, or the item
  must be scheduled before the corporate path carries production volume.
- **P2** — technical debt with a written retirement trigger.

Nothing is listed here that the certification run did not actually establish.
Conversely, nothing established is quietly absorbed into a PASS elsewhere:
every `NOT RUN` and every `NEEDS HARDENING` in ART-004 and ART-006 that has
release consequence appears below.

---

## P0 — blocks release

### P0-1 — No deployed-environment certification

**Evidence:** `FINAL_DEPLOYED_STAGING_EVIDENCE.md`, which names the specific
blockers rather than restating the absence.

The previous report said this was environmental. This phase established
*precisely* what is missing, which is a materially different and more useful
statement:

1. **Only three of twelve services have any deployment path at all**
   (api-gateway, media-file-service, vendor-service). Fleet, Wallet, Taxi,
   Dispatch, Hauling, Notification, Verification, Support and Customer have no
   deploy workflow. **Not one deployable service is on the corporate money
   path.** Section 23's cross-service journeys therefore cannot be certified by
   any mechanism this repository currently defines — this is not a scheduling
   problem, there is no workflow to run.
2. **Vendor's deploy fails on a missing operator-provisioned file.** Every
   recent run ends `ERROR: /opt/karrygo/vendor-service/vendor.env is missing`.
   The image builds, the push succeeds, the SSH succeeds; the environment file
   has never been placed on the host. It holds staging secrets and must be
   created by an operator, mode 600. It cannot be created from here and must
   not be invented.
3. **`infra/docker-compose.yml` defines nothing** — 382 lines, 0 active. It
   cannot be used to stand a stack up locally either.
4. **A green api-gateway or media-file-service deploy is not evidence.** Both
   end at `docker-compose up -d` with no health gate and no version check; a
   green run means the SSH command returned 0. Only Vendor's workflow waits for
   health, and it is the one that cannot run.
5. **No hostname, TLS configuration, backup provider, monitoring backend or
   alerting destination is identified anywhere in the repository**, and the
   VPS credentials are GitHub Actions secrets unavailable to a certification
   session by design.

**Nothing in the code needs to change to close this.** The remaining work is
operator and infrastructure work, enumerated step by step in
`FINAL_DEPLOYED_STAGING_EVIDENCE.md` §7.

**This is the sole reason the verdict is not `BACKEND RELEASE READY`.**

### P1-1 — build commit metadata — **CLOSED this phase**

`serviceapp` now resolves `service`, `version`, `commit_sha`, `build_time` and
`environment`, stamped by the linker from the deployment pipeline, and surfaces
them on `/health`, on `<api-base>/meta` and in the structured startup log. Both
Dockerfiles take `BUILD_COMMIT_SHA` / `BUILD_TIME` / `BUILD_VERSION` as build
args; all three deploy workflows pass `${{ github.sha }}`.

No second metadata system was created — the existing `/meta` surface was
extended, as the section requires.

Verified on a running binary built with those flags; transcript in
`FINAL_DEPLOYED_STAGING_EVIDENCE.md` §5. An unstamped build reports `unknown`
rather than an empty string, so a deployment check can tell "running the wrong
commit" apart from "cannot tell me which commit it is".

**OPS-002 is closed in code. It still requires a deployment to exercise**,
which is P0-1.

### P1-2 — CMP-027 — **WITHDRAWN. This entry was wrong.**

**The previous version of this document claimed payroll does not cross-check
already-posted work-level settlements. That claim was incorrect**, and it
contradicted the Section 13 closure, which was right. Audited again from the
code rather than from documentation:

- `financeops.CalculatePayroll` reads `fleet_partner_work_settled_components`
  inside the same transaction as the calculation, keyed on
  `source_assignment_id + rule_id`, and skips exactly those components:
  `if workSettled[source+":"+ruleID] { continue }`, at the accumulation point
  for `PER_JOB`, `COMMISSION` and `BONUS`.
- The write side is wired: `fleetpartner/service.go` records
  `WorkSettledComponentsFor(...)` when the settlement instruction is produced.
- The match is on the assignment, not a time window — deliberately, because a
  job completed inside a period can settle moments after the period ends, and
  a windowed read would miss it and pay it again.
- `SettlesWithTheJob` and `AccruesToPayroll` are exact complements, so no
  configuration makes a component eligible for both.
- DB-backed certification exists and passes:
  `TestCMP027_PayrollDoesNotRepayWorkSettledComponent` and
  `TestCMP027_RecalculationDoesNotAccumulateDuplicates`, run against real
  PostgreSQL in this phase.

**There is no unresolved financial double-pay risk, and `PAYROLL_ENABLED` is
not gated by this.** The gate recorded in `FINAL_FEATURE_FLAG_ROLLOUT_ROLLBACK.md`
has been removed for the same reason.

The entry is left here rather than deleted because a reader of the previous
report needs to know it was withdrawn, and why. The failure was reading the
domain helpers and assuming nothing called them, instead of grepping the
caller — `WorkSettledExclusionsForAssignments` in the repository genuinely has
no caller, which is what misled the audit, but payroll does the same query
inline and in-transaction, which is stronger.

### P1-3 — GAP-A: Hauling document verification — **PRODUCT / COMPLIANCE REQUIREMENT NOT DEFINED**

Audited again this phase, specifically for an authoritative requirement list,
because the instruction is explicit: do not invent required documents and do
not fabricate compliance requirements.

**Search result: there is still no authoritative list of required Hauling or
truck documents anywhere in this repository.** No roadworthiness certificate,
vehicle licence, proof of ownership, hackney permit, insurance certificate or
tonnage permit is stated as a platform rule in any doc, config, migration or
code path. The only hits are prose examples: Fleet's `document-domain.md` uses
"insurance certificate" to illustrate a capability, and `document_type` in
`fleet_partner_documents` is free text with no enumerated set.

So path 3 of the instruction applies. It is retained as a product/compliance
gap, not implemented.

**The implementation path is nonetheless known, which is worth recording so
the next attempt does not re-derive it:**

- For **corporate** providers, Fleet's document domain already has the whole
  shape — owner, category, type, issue and expiry dates, verification decision,
  replacement, revocation, retention, legal hold — and already refuses to store
  bytes or URLs. A Hauling requirement set would be types within it, not a new
  domain.
- For **independent** providers, Dispatch's six-step model
  (`identity`, `licence` optional, `vehicle`, `face`, `guarantor`, `emergency`)
  is the proven pattern, with `calculateOverallStatus` gating readiness.

**Launch impact.** A haulage provider can reach an online, dispatchable state
with no document verified and no expiry checked. This is a pre-existing
platform-wide posture for independent providers, not something introduced here.
For a **corporate** provider it matters more, because CERT-039 requires that
membership never substitutes for a readiness check — and on Hauling there is no
readiness check for it to substitute for. The invariant is not violated
(membership is evaluated separately wherever readiness exists); there is simply
nothing on the Hauling side to evaluate.

**Severity: P1, and it is a product decision, not an engineering one.** It
blocks no flag in this rollout and introduces no incorrect behaviour. It needs
the authoritative document list from the compliance owner first.

### P1-4 — GAP-B: safety incident organization dimension — **CLOSED this phase**

Audited again and implemented. As the previous audit found, the schema half was
already present (`complaints.organization_reference`, migration 006), so **no
migration was added.** What was missing was resolution at provider-complaint
creation, and that is now in place:

- `supportclients.FleetOrganizationResolver` reads Fleet's corporate context
  over the same HMAC v2 contract the execution services use, reusing
  `providercompat.Client` rather than adding a second corporate-context client
  — two readers of one contract drift, and the one that drifts is always the
  one nobody is looking at.
- `CreateComplaint` and `CreateSOS` snapshot the organization onto the case.
- **It is a snapshot, written once.** Re-reading Fleet on every view would
  silently rewrite history the first time a provider changes organization, and
  would make an old case unreadable whenever Fleet is down.
- **Fail-soft, without exception.** An unreachable Fleet, an unknown provider
  and a genuinely independent provider all produce "no organization" — same
  correct outcome, and the case is created either way. `TestSOSSucceedsWhenFleetIsUnavailable`
  pins it: trading an emergency for a reporting dimension would be wrong every
  time.
- An independent provider is never given an organization. That would be a
  fabrication, not a default.
- Support holds no Fleet database access, and this adds none; it is a
  read-only call on an existing route.

Six tests, including the typed-nil resolver path that an unconfigured
deployment actually takes.

**Remaining deployment action** (in `FINAL_DEPLOYED_STAGING_EVIDENCE.md` §7):
Fleet's `PROVIDER_MEMBERSHIP_ALLOWED_CALLERS` code default now lists
`support-dispute-service`, but a deployment that overrides that variable must
include it, and Support needs a Fleet HMAC secret. Until then the resolver is
inert and cases are created without the label — which is the pre-existing
behaviour, not a regression.

## P2 — technical debt with a retirement trigger

### P2-1 — GAP-C: Dispatch legacy pickup-credential compatibility encoding

Audited again this phase, and **not removed blindly** as instructed.

**Consumer audit.** Three consumers, all inside `trip`, all behind an existing
customer/provider ownership check: `compatibilityPickupView`,
`ResendPickupCredential`, `DecodeDeliveryCode`. No other service and no client
depends on the encoding.

**Producer audit — the new finding.** All four write paths
(`StartTrip`, the delivery generation, and both resend paths) now call
`NewCompatibilityCodeRecord`, which stores an opaque `handover_`-prefixed
reference. **There is no remaining producer of legacy plaintext.** The verbatim
branch in `CompatibilityCode` can therefore only ever serve rows written before
that change.

**What it is not.** It is not a weaker credential: it reuses the secure-handover
PRF and the established verification hash, and it is unreachable on a protected
handover, where `secureHandoverRequired` short-circuits the legacy routes with
`409 SECURE_HANDOVER_ROUTE_REQUIRED`.

**Retirement trigger, now pinned by a test.**
`TestEveryGeneratedCredentialCarriesTheOpaquePrefix` asserts every generated
record carries the prefix, so the plaintext branch cannot quietly reacquire a
producer. The retirement condition is therefore purely a data question: when no
row in `trips` has a `pickup_code` or `delivery_code` lacking the `handover_`
prefix, delete the verbatim branch. That question needs staging or production
data to answer and could not be answered from here.

**Not a release blocker**, because protected handover remains correct and is
not served by this path.

### P2-2 — Observability items marked NEEDS HARDENING

**Evidence.** ART-004 OBS-003, OBS-008, OBS-009, OBS-010, OBS-013.

Five series are computable from durable timestamps but not emitted as metrics:
corporate-context read latency specifically, completion→compensation latency,
settlement latency, idempotent-replay count, and payroll-specific series. Each
has a documented derivation, and every ROL-012 rollback trigger except two is
derivable from a series that already exists. The two that are not
(stage-to-stage latency, replay rate) are the reason this is listed rather
than omitted.

**Severity P2** because the *alert-worthy* conditions — backlog growth,
dead-letter growth, outbox age, reconciliation backlog, error rate — are all
covered by series added this phase.

### P2-3 — Database connection interruption not injected

**Evidence.** ART-006 §4, last row.

Deliberate. The section says "where safe"; interrupting connections under a
live balanced-journal write on the shared certification database risks leaving
the certification data itself in a state that would make every other row in
that document unreliable. Lease expiry and crash recovery (CERT-013) cover the
observable half of the same failure class.

**Closure** belongs with P0-1: inject it in staging, against a database whose
state nobody else's evidence depends on.

---

## Not blockers — recorded so they are not rediscovered as surprises

These are **documented platform policy**, not gaps. Each looks like a hole on
first reading, which is why it is written down.

1. **Suspension between accept and start does not re-gate the job.** A start
   is not an acquisition. `providercompat.AllowsActiveWorkCompletion` is the
   explicit carrier of that decision (CERT-006).
2. **Suspension mid-execution still records the completion.** Refusing it
   would erase money the provider had already earned. The next *acquisition*
   is blocked instead (CERT-007).
3. **A salaried provider's zero share writes no provider ledger line**, rather
   than a zero-amount one. Inventing a token credit would make a salaried
   provider look like they earned per job (CERT-015).
4. **Taxi has no named kill switch.** `TAXI_FLEET_PROVIDER_WORK_ENABLED=false`
   returns a clean 503 with the route still mounted, which is an equivalent
   control (ART-007 §7).
5. **No organization-scoped feature-flag framework exists**, and this phase
   did not build one — ROL-002 explicitly permits the documented staged
   alternative instead. Per-organization control is achieved through operating
   mode, membership status and plan assignment, all of which are real
   (ART-007 §3).
6. **Zero migrations were added this phase.** Every schema object the new
   observability work reads already existed, and the required indexes were
   verified present rather than duplicated (ART-001).

---

## Verdict input

| Class | Count | Blocks release |
|---|---|---|
| P0 | 1 | yes — environmental, not code |
| P1 | 1 open (GAP-A) | no — product/compliance decision, holds no flag |
| P2 | 3 | no |

Resolved this phase: **P1-1 FIXED**, **P1-2 NOT ACTUALLY A DEFECT** (withdrawn),
**P1-4 FIXED**. **P1-3 (GAP-A) REQUIRES A PRODUCT/COMPLIANCE DECISION** and is
the only P1 still open.

**There is no unresolved financial double-pay risk.** CMP-027 is implemented,
wired and certified DB-backed.

No P0 item requires a code change. The single P0 is the absence of a deployed
environment to certify against — now described precisely, not generically —
which is why the verdict is
**`BACKEND CODE COMPLETE — DEPLOYMENT CERTIFICATION REMAINS`** rather than
`BACKEND RELEASE READY`, and why it is not `BACKEND NOT RELEASE READY`.

One defect was introduced by the previous phase and found by CI in this one:
`shared/go/outboxmetrics/sources_test.go` queried other services' tables from a
module that owns no schema, so it passed locally against a populated database
and failed in CI against an empty one. It is fixed — the schema assertion now
lives in each owning service, where the migrated schema genuinely exists, which
is strictly stronger than what it replaced.
