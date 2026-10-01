# ART-007 — Feature flags, rollout and rollback

Phase baseline `9ebad225ea9ec25397778a4dc8d3ff842f2938a3`.
No secret value appears in this document.

**Phase update.** Two changes from the previous version: the `PAYROLL_ENABLED`
caveat is withdrawn (§5), and §15 records that FLAG-001 … FLAG-008 could not be
exercised against a deployed environment, with the reason.

Every flag below is read from the environment at boot. Defaults are the
compiled-in values verified against each service's `internal/config/config.go`
in this tree, not from documentation.

## 1. ROL-001 — Inventory of the corporate path

Only flags on the corporate/managed path are listed. Each service carries many
more domain flags (Fleet alone has ~170); those are not part of this rollout
and are summarised in §2.

**The platform default is OFF everywhere on this path.** A fresh deployment of
this tree does no corporate work, emits no completed-work record, resolves no
compensation and posts no managed settlement until an operator opts in. That
is the correct default and it is also the rollback target.

### 1.1 Taxi

| Flag | Default | Scope | Depends on | Safe-ON prerequisite |
|---|---|---|---|---|
| `TAXI_FLEET_PROVIDER_WORK_ENABLED` | `false` | service | Fleet reachable, Fleet service secret provisioned both ways | Fleet corporate-context read verified live; ROL-003 requires deployed certification first |
| `TAXI_FLEET_MANAGED_EXECUTION_ENABLED` | `false` | service | provider-work ON | provider-work stable, authorization refusals observed to be typed not generic |
| `TAXI_FLEET_COMPLETED_WORK_ENABLED` | `false` | service | managed execution ON; `taxi_outbox` migrated | outbox worker running, backlog near zero |
| `TAXI_MANAGED_SETTLEMENT_ENABLED` | `false` | service | completed work ON; Fleet resolver ON; Wallet settlement route reachable | Fleet resolver observed in SHADOW for a full period first |
| `TAXI_GOODS_DELIVERY_ENABLED` | `false` | service | Vendor commerce integration | capability declaration verified (`maxPickups=1`) |
| `TAXI_TYPED_FINANCIAL_VISIBILITY_ENABLED` | **`true`** | service | none | already on; see §6 |

### 1.2 Dispatch

| Flag | Default | Scope | Depends on | Safe-ON prerequisite |
|---|---|---|---|---|
| `DISPATCH_FLEET_RELAY_ENABLED` | `false` | service | Fleet reachable | corporate-context read verified |
| `DISPATCH_FLEET_COMPLETED_WORK_ENABLED` | `false` | service | relay ON; outbox migrated | outbox worker running |
| `DISPATCH_MANAGED_SETTLEMENT_ENABLED` | `false` | service | completed work ON; Fleet resolver ON | as Taxi |
| `DISPATCH_ORGANIZATION_ASSIGNMENT_ENABLED` | `false` | service | relay ON | — |
| `DISPATCH_ORGANIZATION_EXECUTION_ENABLED` | `false` | service | assignment ON | — |
| `DISPATCH_ORGANIZATION_RESERVATION_ENABLED` | `false` | service | assignment ON | — |
| `DISPATCH_PROVIDER_COMPANY_ASSIGNMENTS_ENABLED` | `false` | service | relay ON | — |
| `DISPATCH_PROVIDER_ORGANIZATION_OFFERS_ENABLED` | `false` | service | relay ON | operating-mode enforcement ON (delegated path) |
| `DISPATCH_PRIVATE_EXECUTION_ENABLED` | `false` | service | relay ON | — |
| `DISPATCH_MARKETPLACE_INTERNAL_API_ENABLED` | `false` | service | Fleet secret | — |
| `DISPATCH_MARKETPLACE_OUTBOX_ENABLED` | `false` | service | outbox migrated | — |
| `DISPATCH_COMMERCE_EVENT_DELIVERY_ENABLED` | `false` | service | Vendor reachable | — |
| **`DISPATCH_ASSIGNMENT_KILL_SWITCH`** | `false` | service | — | see ROL-006 |
| **`DISPATCH_PRIVATE_EXECUTION_KILL_SWITCH`** | `false` | service | — | see ROL-006 |

### 1.3 Hauling

| Flag | Default | Scope | Depends on | Safe-ON prerequisite |
|---|---|---|---|---|
| `HAULING_FLEET_INTEGRATION_ENABLED` | `false` | service | Fleet reachable; **and** `HAULING_FLEET_*` credentials complete — `TestFleetEnablementRequiresCompleteProductionConfig` refuses a half-configured enablement at boot | — |
| `HAULING_FLEET_PROVIDER_WORK_ENABLED` | `false` | service | integration ON | corporate-context read verified live |
| `HAULING_PRIVATE_EXECUTION_ENABLED` | `false` | service | integration ON | — |
| `HAULING_FLEET_COMPLETED_WORK_ENABLED` | `false` | service | `HAULING_OUTBOX_WORKER_ENABLED` ON; migration 018 applied | outbox worker running |
| `HAULING_OUTBOX_WORKER_ENABLED` | `false` | service | migration 018 | must precede completed-work ON, or events persist and never leave |
| `HAULING_MANAGED_SETTLEMENT_ENABLED` | `false` | service | completed work ON; Fleet resolver ON | as Taxi |
| `HAULING_COMMERCE_INTEGRATION_ENABLED` | `false` | service | Vendor reachable | capability declaration verified (`maxPickups=10`, `maxSecurePickups=1`) |
| **`HAULING_FLEET_INTEGRATION_KILL_SWITCH`** | `false` | service | — | see ROL-006 |
| **`HAULING_COMMERCE_INTEGRATION_KILL_SWITCH`** | `false` | service | — | see ROL-006 |
| `HAULING_TYPED_FINANCIAL_VISIBILITY_ENABLED` | **`true`** | service | none | already on; see §6 |

### 1.4 Fleet

| Flag | Default | Scope | Depends on | Safe-ON prerequisite |
|---|---|---|---|---|
| `FLEET_COMPENSATION_RESOLVER_ENABLED` | `false` | service | legacy boolean, superseded by the mode below | prefer the mode |
| **`FLEET_COMPENSATION_RESOLVER_MODE`** | **unset → `OFF`** | service | `COMPENSATION_ENABLED`, plan data loaded | see ROL-005 |
| `FLEET_SETTLEMENT_INSTRUCTION_ENABLED` | `false` | service | resolver `ON`; Wallet route reachable; Wallet trusts Fleet | **this is the flag that moves money** |
| `FLEET_COMPLETED_WORK_PROJECTION_ENABLED` | `false` | service | ingestion running | — |
| `FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED` | `false` | service | — | — |
| `FLEET_MARKETPLACE_RECONCILIATION_ENABLED` | `false` | service | ingestion ON | — |
| `FLEET_TAXI_INTEGRATION_ENABLED` | `false` | service | Taxi secret | — |
| `FLEET_HAULING_INTEGRATION_ENABLED` | `false` | service | Hauling secret | — |
| `FLEET_DISPATCH_EXECUTION_START_ENABLED` | `false` | service | Dispatch secret | — |
| `FLEET_FINANCE_INTEGRATION_ENABLED` | `false` | service | Wallet secret | — |
| `PAYROLL_ENABLED` | `false` | service | `COMPENSATION_ENABLED`; compensation plans assigned | CMP-027 double-pay prevention certified DB-backed; run §13 as for any flag |
| **`FLEET_OPERATING_MODE_ENFORCEMENT_ENABLED`** | **`true`** | service, but gates a **per-organization** value | corporate context carries `operating_mode` | already on; see ROL-002 |
| **`FLEET_FINANCE_EXECUTION_KILL_SWITCH`** | `false` | service | — | see ROL-006 |
| **`FLEET_PRIVATE_OPERATIONS_KILL_SWITCH`** | `false` | service | — | see ROL-006 |
| **`FLEET_INTEGRATIONS_KILL_SWITCH`** | `false` | service | — | see ROL-006 |
| **`FLEET_COMPANY_WALLET_KILL_SWITCH`** | `false` | service | — | see ROL-006 |

### 1.5 Wallet

| Flag | Default | Scope | Depends on | Safe-ON prerequisite |
|---|---|---|---|---|
| `PAYMENT_WALLET_MANAGED_RECONCILIATION_ENABLED` | **`true`** | service | migration 000013 | already on; it only *surfaces* stuck jobs, it moves no money (`TestTheSweepMovesNoMoney`) |
| `PAYMENT_WALLET_REQUIRE_MANUAL_PAYOUTS` | **`true`** | service | — | safe default; turning it OFF automates payouts and is not part of this rollout |
| `PAYMENT_WALLET_FINANCE_OUTBOX_ENABLED` | `false` | service | `finance_outbox` migrated | worker running |
| `PAYMENT_WALLET_FINANCE_WORKERS_ENABLED` | `false` | service | outbox ON | — |
| `PAYMENT_WALLET_FINANCE_PRIVATE_API_ENABLED` | `false` | service | Fleet secret | — |
| **`PAYMENT_WALLET_FINANCE_EXECUTION_KILL_SWITCH`** | `false` | service | — | see ROL-006 |

The managed settlement **route** itself is not behind a boolean: it is gated by
caller allowlist (Fleet alone may post or reverse). That is deliberate — an
authorization gate cannot be accidentally left on by a config drift.

### 1.6 Vendor execution integrations

| Flag | Default | Scope | Depends on | Safe-ON prerequisite |
|---|---|---|---|---|
| `DISPATCH_COMMERCE_INTEGRATION_ENABLED` | `false` | service | Dispatch capability route reachable | capability read returns a version |
| `TAXI_GOODS_DELIVERY_ENABLED` | `false` | service | Taxi capability route reachable | capability read returns a version |
| `HAULING_COMMERCE_INTEGRATION_ENABLED` | `false` | service | Hauling capability route reachable | capability read returns a version |
| `PAYMENT_WALLET_COMMERCE_ENABLED` | `false` | service | Wallet secret | — |
| `VENDOR_OWN_DELIVERY_ENABLED` | `false` | service | — | no platform mission is created on this path |
| **`COMMERCE_GLOBAL_KILL_SWITCH`** | `false` | service | — | see ROL-006 |

Section 11 deliberately added **no new flags**: these three remain the single
source of truth for whether an authority participates, and a disabled authority
gets no capability client at all rather than a client that fails.

## 2. Flags outside this rollout

Fleet carries ~170 domain flags and Vendor ~55, nearly all defaulting `false`.
They gate that service's own product surface (orders, manifests, CRM, imports,
exports, warehousing) and are not on the corporate execution or money path.
They are out of scope here and unchanged by this phase. The four Fleet flags
defaulting `true` are `FLEET_PARTNER_WORKER_ENABLED`,
`FLEET_PARTNER_METRICS_ENABLED`, `FLEET_DATA_RETENTION_DRY_RUN` (dry-run is the
safe default) and `FLEET_OPERATING_MODE_ENFORCEMENT_ENABLED`.

## 3. ROL-002 — Organization-scoped rollout

**There is no organization-scoped feature-flag framework, and this phase did
not build one.** The section explicitly says not to add complicated
organization-scoped infrastructure if the existing framework cannot support it
safely, so the staged alternative is documented instead.

What *does* exist and is genuinely per-organization:

- **Operating mode** (`CENTRALIZED` / `DELEGATED` / `HYBRID`) is persisted per
  organization by Fleet and read on every request. `CENTRALIZED` refuses direct
  offer acceptance, and an unknown or missing mode refuses too — an
  unestablished policy is not a licence to proceed.
- **Membership status** is per organization-provider pair; only `ACTIVE` takes
  new managed work.
- **Compensation plan assignment** is per provider with an effective date, so
  an organization can be moved onto a plan independently of any other.

So the practical cohort mechanism is: enable the service-level flags, then
onboard organizations one at a time by giving them an operating mode, active
memberships and an effective compensation plan. An organization with no plan
assignment resolves nothing, which is a safe non-participating state rather
than an error.

**Staged rollout sequence (the alternative ROL-002 asks for):**

1. Non-production environment, all flags ON, synthetic organizations.
2. Staging, flags ON, one internal pilot organization.
3. Production, flags ON, Fleet resolver in `SHADOW` — real work is recorded and
   resolved, no money moves.
4. Production, resolver `ON` + settlement instruction ON, one small
   organization with active memberships. Every other organization is inert
   because it has no plan assignment.
5. Widen by adding plan assignments, cohort by cohort.
6. Global.

## 4. ROL-003 — Taxi Fleet source

`TAXI_FLEET_PROVIDER_WORK_ENABLED` stays `false`. This phase certified Taxi's
corporate path at component and DB-backed level only (ART-006 §2). **Unit and
component tests passing is explicitly not sufficient** to enable it globally,
and this document does not recommend doing so. It is gated behind deployed
certification, which ART-008 lists as an open blocker.

## 5. ROL-004 — Managed settlement and legacy retirement

The managed path is controllable at three independent points: the execution
service's `*_MANAGED_SETTLEMENT_ENABLED`, Fleet's
`FLEET_SETTLEMENT_INSTRUCTION_ENABLED`, and Wallet's caller allowlist.

**These are not two competing algorithms, and the boundary is enforced in
code, not by configuration.** `CompleteJob` — the legacy per-job payout —
*refuses* organization-managed work on either of two signals: the payment
intent's own managed marker, or the existence of a settlement row for that job
(`TestLegacyCompleteJobRefusesManagedWork`,
`TestManagedSettlementRefusesAJobAlreadySettledLegacy`). A misconfiguration
that enabled both paths therefore produces a refusal, not a double payment.
Independent settlement is byte-for-byte unchanged
(`TestLegacyCompleteJobStillWorksForIndependentProviders`).

**Retirement criteria for the legacy managed payout path** (ROL-004 and
ROL-014 both require these to be written down):

1. Every organization has an effective compensation plan assignment.
2. Zero `CompleteJob` refusals attributable to a managed marker for one full
   settlement period — i.e. nothing is still trying the legacy route.
3. `managed_settlements` reconciliation backlog at zero for the same period.
4. No dead-lettered settlement instruction outstanding.
5. A completed reversal exercise in staging.

When all five hold, `*_MANAGED_SETTLEMENT_ENABLED` and
`FLEET_SETTLEMENT_INSTRUCTION_ENABLED` become permanent-ON and are removed in a
follow-up change; the managed marker check in `CompleteJob` stays forever,
because it is a safety invariant rather than a migration aid.

**Correction to the previous version of this document:** it carried a caveat
holding `PAYROLL_ENABLED` OFF until CMP-027 was closed. **That caveat was based
on an incorrect reading and has been withdrawn.** CMP-027 is implemented and
wired: `CalculatePayroll` reads `fleet_partner_work_settled_components` inside
its own transaction, keyed on assignment and rule, and skips exactly those
components at the accumulation point for `PER_JOB`, `COMMISSION` and `BONUS`.
The write side records them when the settlement instruction is produced. It is
certified DB-backed by `TestCMP027_PayrollDoesNotRepayWorkSettledComponent` and
`TestCMP027_RecalculationDoesNotAccumulateDuplicates`, both re-run against real
PostgreSQL this phase.

`PAYROLL_ENABLED` is therefore gated only by its own rollout checklist, like
any other flag. There is no unresolved double-pay risk.

## 6. ROL-005 — Compensation resolver OFF → SHADOW → ON → rollback

Implemented as a real three-state setting, not a boolean:
`FLEET_COMPENSATION_RESOLVER_MODE`.

| Mode | Resolves | Records | Acts (instruction / journal / payslip) |
|---|---|---|---|
| `OFF` (unset default) | no | no | no |
| `SHADOW` | yes | yes | **no** |
| `ON` | yes | yes | yes — *and only if* `FLEET_SETTLEMENT_INSTRUCTION_ENABLED` is also ON |

Certified:

- **Shadow posts nothing.** `ActsOnResolution` is false for `SHADOW`, and
  `service.go` gates the settlement stage on `ActsOnResolution(mode) &&
  flags.SettlementInstruction` — two gates, because "resolve" and "pay" are
  different decisions.
- **Rollback is immediate.** `TestRollingBackStopsTheResolverImmediately`.
- **Backward compatible.** `TestTheLegacyBooleanStillTurnsTheResolverOn` — an
  existing deployment using the old boolean keeps behaving identically until it
  opts into the setting.
- **A typo does not start paying people.** An unrecognised mode is *refused* at
  normalization rather than defaulted; an unset mode is `OFF`.

Rollback from `ON` is `ON → SHADOW` (keep resolving, stop paying) or
`ON → OFF`. Neither touches a journal already posted — see ROL-008.

## 7. ROL-006 / ROL-007 — Kill switches and active work

21 kill switches exist across the platform. The ones relevant to this path:

| Switch | Stops |
|---|---|
| `DISPATCH_ASSIGNMENT_KILL_SWITCH` | new Dispatch assignment acquisition |
| `DISPATCH_PRIVATE_EXECUTION_KILL_SWITCH` | new Dispatch private execution |
| `HAULING_FLEET_INTEGRATION_KILL_SWITCH` | Hauling corporate acquisition |
| `HAULING_COMMERCE_INTEGRATION_KILL_SWITCH` | Hauling Commerce missions |
| `FLEET_FINANCE_EXECUTION_KILL_SWITCH` | Fleet finance execution incl. settlement instruction |
| `FLEET_PRIVATE_OPERATIONS_KILL_SWITCH` | Fleet private operations |
| `FLEET_INTEGRATIONS_KILL_SWITCH` | Fleet outbound integrations |
| `FLEET_COMPANY_WALLET_KILL_SWITCH` | Fleet company wallet operations |
| `PAYMENT_WALLET_FINANCE_EXECUTION_KILL_SWITCH` | Wallet finance execution |
| `COMMERCE_GLOBAL_KILL_SWITCH` | all Vendor commerce |

**Taxi has no named kill switch**, which looks like a gap and is not one:
setting `TAXI_FLEET_PROVIDER_WORK_ENABLED=false` returns a clean
`503 Unavailable` ("Taxi Fleet provider work is not enabled.") while leaving the
route mounted, so callers get a typed retryable refusal rather than a 404 that
looks like a deployment error. That satisfies ROL-006's requirement for a
control that blocks new acquisition. ROL-007 is satisfied separately because
execution and completion are gated by `TAXI_FLEET_MANAGED_EXECUTION_ENABLED`
and `TAXI_FLEET_COMPLETED_WORK_ENABLED` — turning acquisition off does not turn
completion off.

**ROL-007 — active work during a kill.** Every switch on this path blocks
*acquisition*. None of them deletes custody history, and none is on a
completion, credential-verification or settlement-readback path. That is the
deliberate shape: a provider holding goods when the switch is thrown still
completes, still gets their completed-work record, and still gets paid.
`providercompat.AllowsActiveWorkCompletion` is the explicit carrier of the same
policy for membership changes
(`TestMembershipChangeNeverStrandsWorkAlreadyInCustody`,
`TestSuspensionDuringWorkStillRecordsTheCompletion`).

**A kill switch must never be used to make custody history go away.** Throwing
one leaves every recorded transition intact; the outbox drains what it already
holds. If an operator needs work *stopped* rather than *not-started*, that is a
support/recovery action against the specific mission, not a flag.

## 8. ROL-008 — Financial rollback

**Do not roll back money by deleting or updating rows.** The only sanctioned
mechanism is a compensating journal.

- `managed_settlements` reversal writes a compensating journal and keeps the
  original: `TestReversalCompensatesWithoutErasingHistory`.
- Reversal is Fleet-only by caller allowlist.
- Ledger entries are positive-amount with an explicit side; there is no
  negative-amount correction and no update-in-place.
- A settlement that was never posted needs no reversal — read it back first
  (`TestSettlementCanBeReadBackByEveryRoute`) rather than assuming.

Rolling the *code* back does not roll the *money* back, and must not be
expected to. Flag rollback stops new postings; existing postings are corrected
by reversal only.

## 9. ROL-009 — Secure handover fallback

There is no fallback that marks a protected handover delivered without
verification, and none may be added. An unavailable credential dependency
returns `OTP_DEPENDENCY_UNAVAILABLE` (503). Vendor's `handover.Unavailable`
authority cannot manufacture a credential or fall back to local verification.
A stuck handover is resolved through the explicit operator/support recovery
path, which is auditable, not through a flag.

The `DELIVERY_HANDOVER_SMS_ALLOWED=false` default is a *channel* control, not a
verification bypass — the credential is still required whichever channel
carried it.

## 10. ROL-010 — Database rollback compatibility

Full per-migration analysis is in ART-001. Summary for rollout purposes:

- **This phase added zero migrations.** Every schema object the new
  observability work reads already existed.
- Every migration in scope is paired `.up`/`.down` and replay-verified
  (`TestMigrationsReplayAndRollback`, and the equivalent per service).
- The corporate migrations are **additive**: new tables, and new nullable
  columns on existing tables. Old code running against the upgraded schema
  ignores columns it does not select, so a code rollback is safe against an
  upgraded schema. This is the forward-compatible strategy ROL-010 asks for,
  and it is why the schema may be migrated ahead of the code.
- The one ordering rule: `HAULING_OUTBOX_WORKER_ENABLED` must not be ON before
  migration 018 is applied, or the worker queries a column that does not exist.
- Protected historical migrations (Fleet `000077`, the Taxi corporate
  migration, Hauling `018`, Wallet `000013`+, Dispatch `000057`) were not
  rewritten.

## 11. ROL-011 — Version skew

| Skew | Compatible | Why |
|---|---|---|
| New Vendor + old execution service | yes, degraded | Vendor reads the authority's declared capability version; an older declaration simply advertises less. An authority that cannot be *asked* produces no refusal — a capability outage must not fail a paid order. |
| Old Vendor + new execution service | yes | New capability fields are additive; an old Vendor ignores `maxSecurePickups` and its missions stay within the older bound it already respected. |
| New Fleet + old execution service | yes | Completed-work ingestion validates the payload it receives; an older payload missing an optional field is refused with a typed error, not a partial write. |
| Old Wallet + new Fleet | **no — ordering constraint** | The managed settlement route does not exist on a Wallet without migration 000013 and the route. Fleet must not have `FLEET_SETTLEMENT_INSTRUCTION_ENABLED` ON before Wallet is deployed. An attempt fails closed (404 → classified, retried, then reconciled) rather than paying twice, but it should not be relied on. |

**Deployment order is therefore: Wallet → Fleet → execution services → Vendor.**
Enablement order is the reverse of the dependency, i.e. flags go ON bottom-up.

Skew tolerance is deliberately narrow: one version back, not unlimited history.

## 12. ROL-012 — Measurable rollback triggers

These are thresholds, not impressions. Series names are from ART-004.

| Trigger | Measure | Threshold | Action |
|---|---|---|---|
| Authorization failure spike | rate of `FailureKind` refusals on corporate-context reads | > 5% of corporate requests over 15 min, or any sustained `FLEET_DEPENDENCY_UNAVAILABLE` | kill switch on acquisition; investigate Fleet |
| Settlement failure | `managed_settlements` non-posted outcomes | > 1% over 1 h, **or any single unexplained double-posting attempt** | `FLEET_SETTLEMENT_INSTRUCTION_ENABLED=false`; resolver to `SHADOW` |
| Dead-letter growth | `outbox_dead_letter_events` | any increase on the settlement or completed-work target | stop the rollout; operator inspect before requeue |
| Outbox age | `outbox_oldest_pending_age_seconds` | > 900 s sustained 15 min | investigate worker; do not widen cohort |
| Reconciliation backlog | `reconciliation_backlog_operations` | > 0 sustained 1 h, or any growth trend | investigate; do not widen cohort |
| Error rate | 5xx on corporate + settlement routes | > 1% over 15 min | roll back the most recent flag |
| Latency regression | settlement p99 | > 500 ms sustained, or throughput falling while concurrency rises (see ART-006 §5) | reduce settlement concurrency to at or below pool size first; roll back if unresolved |
| Credential verification anomaly | `dispatch_handover_verification_total` failure ratio | failure ratio doubling week-on-week, or a spike in LOCKED outcomes | investigate before any handover flag change; **never** relax verification |

Every one of these is derivable from a series that exists today except the two
marked NEEDS HARDENING in ART-004 (stage-to-stage latency, replay counter).

## 13. ROL-013 — Per-flag rollout checklist

Run this before **each** flag, not once per rollout.

1. Migrations for that flag's tables applied and verified present.
2. Deployed version verified — **currently blocked**: `<api-base>/meta` reports
   no commit SHA, so a black-box run cannot record which commit it tested.
   ART-008, P1.
3. Dashboards for the relevant series healthy and populated (not merely
   existing).
4. `outbox_pending_events` and `reconciliation_backlog_operations` near zero for
   the affected target.
5. No dead-lettered events outstanding on that target.
6. Every dependency in §1's "Depends on" column reachable and signed —
   verified by an actual signed call, not by config inspection.
7. Smoke test of the specific path passes.
8. Rollback path confirmed: the flag can be set back and the service restarted
   without a migration.
9. For any flag that moves money, a named operator watching for the first
   settlement period.

## 14. ROL-014 — Flag retirement plan

| Group | Retire when | Then |
|---|---|---|
| `*_MANAGED_SETTLEMENT_ENABLED`, `FLEET_SETTLEMENT_INSTRUCTION_ENABLED` | the five ROL-004 criteria hold | remove the flag; keep the `CompleteJob` managed-marker refusal permanently |
| `FLEET_COMPENSATION_RESOLVER_MODE` | resolver `ON` globally for two settlement periods with zero rollbacks | remove `OFF`/`SHADOW`; also remove the legacy `FLEET_COMPENSATION_RESOLVER_ENABLED` boolean |
| `*_FLEET_PROVIDER_WORK_ENABLED`, `*_FLEET_COMPLETED_WORK_ENABLED` | corporate work is the normal path for all cohorts | remove |
| Vendor `*_COMMERCE_INTEGRATION_ENABLED` | all three authorities live | keep — they are also the disabled-authority control, which stays useful |
| Kill switches | never | they are operational controls, not migration aids |
| `*_TYPED_FINANCIAL_VISIBILITY_ENABLED` (default true) | — | remove the flag, keep the behaviour permanently on; a flag that can turn privacy off is itself a risk |

**Two managed corporate payout algorithms must not be maintained forever.** The
criteria above are the commitment. The follow-up change is a code deletion, not
a config change, and it should be scheduled rather than left to drift.

## 15. FLAG-001 … FLAG-008 — deployed flag certification

Every gate in this section requires a deployed environment in which a flag can
be flipped and its effect observed. There is none —
`FINAL_DEPLOYED_STAGING_EVIDENCE.md` names the specific missing pieces, chief
among them that **none of the services carrying these flags has a deploy
workflow at all**.

| ID | Status | What exists instead |
|---|---|---|
| FLAG-001 inventory of deployed values | **NOT RUN — NO ENVIRONMENT** | §1 is the inventory of compiled defaults, read from each `config.go`. Deployed values live in per-service env files on the VPS that have not been created. |
| FLAG-002 resolver OFF | PASS — UNIT | `NormalizeResolverMode` returns `OFF` when unset; `ResolvesCompensation` is false, so nothing is computed. |
| FLAG-003 resolver SHADOW | PASS — UNIT | `ActsOnResolution` is false for `SHADOW`, and the settlement stage is gated on it *and* `FLEET_SETTLEMENT_INSTRUCTION_ENABLED`. Shadow computes and records; it posts nothing. |
| FLAG-004 resolver ON | PASS — DB-BACKED (behaviour), **NOT RUN** (deployed) | The financial flow under `ON` is certified by the managed-settlement suite. Enabling it for one real organization in staging is not possible. |
| FLAG-005 rollback | PASS — UNIT | `TestRollingBackStopsTheResolverImmediately`. Already-posted journals are untouched by a flag change by construction: rollback is a compensating journal, never a delete (ROL-008). |
| FLAG-006 marketplace kill | PASS — COMPONENT | Kill switches gate acquisition only; none sits on a completion, credential-verification or settlement-readback path, so active work completes. |
| FLAG-007 managed settlement kill | PASS — DB-BACKED | The important half is certified in code and is stronger than a flag: `CompleteJob` **refuses** organization-managed work on the intent's managed marker or an existing settlement row, so disabling the managed path produces a refusal to reconcile, never an unsafe fallback into legacy provider payout. `TestLegacyCompleteJobRefusesManagedWork`. |
| FLAG-008 secure handover | PASS — DB-BACKED | An unavailable credential dependency returns `OTP_DEPENDENCY_UNAVAILABLE` (503). There is no branch that completes a protected handover without verification, and none may be added (ROL-009). |

The labels are the ones the certification matrix defines. None of these rows
claims deployed evidence, because there is none.
