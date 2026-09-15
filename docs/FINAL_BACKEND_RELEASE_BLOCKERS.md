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

**Evidence.** ART-006 §6 and ART-004 §1. This run has no deployed staging
environment. Consequences, each recorded rather than papered over:

- Section 22 (Observability and Operational Readiness) stays **OPEN**:
  OPS-001 … OPS-014 are all `NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED`.
- No CERT scenario carries `PASS — DEPLOYED STAGING`.
- No persona walked the full offer → accept → execute → complete →
  compensate → settle chain over HTTP in one continuous run (ART-006 §2, last
  row). Each stage is certified; the seams between them are certified by
  contract tests, not by a physical run.
- CERT-023/024/025 ask explicitly for a *fresh staging* capability read. The
  strongest available evidence is a fresh **local** read from a compiled
  service.
- Six of seven load tests (`LOAD-001`, `-002`, `-004`, `-006`, `-007`, plus
  `LOAD-003` as load) were not run, because HTTP-level load against a laptop
  process measures the laptop.

**Why this is P0 and not P1.** Every dependency on this path is a signed HTTP
call between separately-deployed services. Signing, allowlisting, routing,
timeouts and TLS are exactly the class of thing that is correct in a test
harness and wrong in a deployment, and they are also the class of thing whose
failure mode on the money path is an ambiguous outcome rather than a clean
error.

**Closure.** Deploy the baseline plus this phase to staging, then run
ART-006's CERT list and the seven load tests against it, and re-run the
Section 22 OPS gates. Nothing in the code needs to change first.

**This is the sole reason the verdict is not `BACKEND RELEASE READY`.**

---

## P1 — release not blocked; flag stays OFF or work is scheduled

### P1-1 — `<api-base>/meta` reports no build commit

**Evidence.** ART-004 OBS-016; ART-007 ROL-013 step 2.

`serviceapp.Run` exposes `/health`, `/ready` and `<api-base>/meta`, and `meta`
reports the service name and capabilities but **no commit SHA**. A black-box
certification run therefore cannot record which commit it tested, and a
rollout checklist cannot verify the deployed version without trusting the
deployment pipeline's own claim.

**Why it is P1, not P2.** It is a prerequisite for P0-1's closure being
*meaningful*: certifying staging without being able to say what staging was
running produces a result nobody can rely on later.

**Closure.** Add a build-stamped commit field to the `meta` payload
(ldflags-injected). Small, one place, `shared/go/serviceapp`. It was not done
in this phase because it is a build-pipeline change as much as a code change,
and inventing a stamping mechanism the pipeline does not use would be worse
than naming the gap.

### P1-2 — CMP-027: payroll does not cross-check posted work-level settlements

**Evidence.** ART-006 CERT-031; ART-007 §5 caveat.

Fleet's compensation model separates **work-level** components (per-job,
commission — settled with the job's money) from **payroll-level** components
(salary, per-shift, allowance, overtime, bonus, deduction — accrued, never
charged to the job). That distinction is real and tested
(`TestImmediateCommissionIsPaidWithTheJobAndExcludedFromPayroll`,
`TestMixedPlanSplitsComponentsBetweenBothPlaces`,
`TestPayrollAccrualIsReportedButNotChargedToTheJob`,
`TestABonusWithoutProvenanceDoesNotEnterPayroll`). For a correctly-classified
component, double payment is structurally impossible.

**What is missing** is a guard for a component that is *mis*classified:
payroll calculation does not query already-posted work-level settlements
before including a component. The classification is the only defence.

**Gate.** `PAYROLL_ENABLED` must stay `false` in production until this is
closed. It is `false` by default, so no action is required to hold the gate —
but it must not be turned on as part of the corporate rollout.

**Closure.** In payroll calculation, exclude any component already carried by
a posted `managed_settlements` row for the same provider and period, and add a
test that a deliberately misclassified component is excluded rather than paid
twice.

### P1-3 — GAP-A: Hauling has no document-verification or expiry model

**Status: audited this phase, unchanged, and deliberately not invented.**

**Audit result.** Hauling's provider and truck models carry `license_type` and
some free-text licence fields, but there is no typed verification-step model,
no approval state, no expiry, and no readiness gate equivalent to Dispatch's
six-step model. A search of the repository's business and domain
documentation found **no authoritative list of required Nigerian truck or
haulage documents** — no roadworthiness, vehicle-licence, proof-of-ownership
or hackney-permit requirement is stated anywhere as a platform rule.

The section's own instruction is "do not invent required documents" and "do
not fabricate compliance requirements", so path 3 applies: **retained as a
documented product/compliance gap.**

**Launch impact.** A haulage provider can reach an online, dispatchable state
without any document having been verified or any expiry having been checked.
For an independent provider this is a pre-existing platform-wide posture. For
a **corporate** provider it matters more, because CERT-039 requires that
corporate membership never substitutes for a safety/readiness check — and on
Hauling there is no readiness check for it to fail to substitute for. The
invariant is not violated (membership is evaluated separately from readiness
everywhere it exists); there is simply nothing on the Hauling side.

**Severity: P1, product/compliance, not engineering.** It blocks no flag in
this rollout and introduces no incorrect behaviour. It requires a product
decision on the authoritative document list before any implementation, and
that decision is not this phase's to make.

**Closure.** Obtain the authoritative document list from the compliance owner,
then implement typed verification steps and expiry on Hauling mirroring
Dispatch's model (which is already proven and would be the pattern, not a new
design).

### P1-4 — GAP-B: a safety incident carries no corporate organization dimension

**Status: audited this phase. Partially closed already; the remaining half is
real.**

**Audit result.** The schema half is **already present** — migration
`006_organization_case_contract` added `complaints.organization_reference`
with a partial unique index and an organization index, and there is a working
organization-case contract that Fleet and Admin can drive. So **no migration
is needed to close this.**

What is missing is the *provider-raised* path. `SupportService.CreateSOS`
resolves the complainant's name and phone from the owning service over
HMAC (`resolveSnapshot`) but does **not** resolve or persist the provider's
organization. A corporate provider raising an SOS mid-custody therefore
produces an emergency case that does not say which organization's provider it
is.

**Is `Support → Fleet corporate-context` now justified?** Yes, and the
existing machinery makes it cheap and safe: `resolveSnapshot` is already an
HMAC-authenticated, timeout-bounded, fail-soft cross-service read that writes
a **historical snapshot** onto the complaint rather than joining live, and
support-dispute-service has no statement touching any Fleet table (certified
in ART-003 SEC-018). Extending that same call to also snapshot organization
reference and membership id satisfies every condition the gap register lists —
HMAC-authenticated, timeout, fail-safe, historical snapshot, no direct Fleet
DB access — with no new dependency shape.

**Operational impact and launch severity: P1.** Goods are not orphaned — that
half of CERT-040 is certified (custody state is durable, a failed attempt
leaves custody where it is, the return path ends with the sender). The impact
is on *response*: an operator handling a corporate provider's emergency cannot
see, from the case, which organization to contact, and organization-level
safety reporting is incomplete. It degrades incident response; it does not
lose custody or money.

**Closure.** Extend `resolveSnapshot` to snapshot organization reference for
provider complainant types, fail-soft exactly as it does today, and set
`organization_reference` on SOS and provider complaints.

---

## P2 — technical debt with a retirement trigger

### P2-1 — GAP-C: Dispatch legacy pickup-credential compatibility encoding

**Status: audited this phase. Retained deliberately; retirement trigger
written down.**

**Audit result.** The compatibility layer is
`securehandover.NewCompatibilityCodeRecord` / `CompatibilityCode`, consumed by
`trip.compatibilityPickupView`, `trip.ResendPickupCredential` and
`trip.DecodeDeliveryCode`. Its actual shape matters and is easy to misread:

- It is **not** a weaker credential. It reuses the secure-handover PRF and the
  established verification hash — the comment "this is not a second challenge"
  is accurate, and the stored value is an opaque `handover_`-prefixed
  reference, not a redisplayable OTP.
- It is **only** reachable on the unprotected legacy trip path.
  `secureHandoverRequired` short-circuits the legacy routes with
  `409 SECURE_HANDOVER_ROUTE_REQUIRED` for any trip that requires protection,
  so a protected handover cannot be served by this path at all.
- `CompatibilityCode` returns the stored value verbatim when it lacks the
  `handover_` prefix. **That branch is the actual debt**: it is the read path
  for historical rows whose pickup/delivery code is still stored as plaintext.

**Consumers.** Three, all inside `trip`, all behind an existing
customer/provider ownership check. No other service and no client depends on
the encoding.

**Migration path.** Already staged: new records are written as opaque
references; the plaintext branch exists only to read rows written before that
change.

**Retirement condition (the trigger this register requires):** when no row in
`trips` has a `pickup_code` or `delivery_code` lacking the `handover_` prefix —
i.e. when the guarded migration that removes expired historical plaintext has
drained every such row. At that point delete the verbatim-return branch, then
the whole compatibility pair once the legacy unprotected trip path itself is
retired, with a migration test asserting no plaintext remains.

**Not removed blindly**, per the instruction. Removing it now would break
in-flight legacy trips whose credential is still stored in the old form.

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
| P1 | 4 | no; two hold a flag OFF, two are scheduled work |
| P2 | 3 | no |

No P0 item requires a code change. The single P0 is the absence of a deployed
environment to certify against, which is why the verdict is
**`BACKEND CODE COMPLETE — DEPLOYMENT CERTIFICATION REMAINS`** rather than
`BACKEND RELEASE READY`, and why it is not `BACKEND NOT RELEASE READY`.
