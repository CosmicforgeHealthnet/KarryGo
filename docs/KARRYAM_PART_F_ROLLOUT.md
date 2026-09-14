# KARRYAM Part F — nine-stage rollout for Sections 12, 13 and 14

Every stage names the flags it changes, what the system does afterwards, what to
watch, what aborts it, and how to get back. No stage takes a flag from off to
globally on, and no stage moves money that a previous stage has not already
proven the shape of.

The order is not arbitrary. Each stage either produces evidence the next one
depends on, or removes a way for the next one to fail unnoticed.

## The flags

| Flag | Owner | Default |
| --- | --- | --- |
| `FLEET_COMPLETED_WORK_PROJECTION_ENABLED` | Fleet | off |
| `FLEET_COMPENSATION_RESOLVER_MODE` (`OFF`/`SHADOW`/`ON`) | Fleet | `OFF` |
| `FLEET_SETTLEMENT_INSTRUCTION_ENABLED` | Fleet | off |
| `TAXI_MANAGED_SETTLEMENT_ENABLED` | Taxi | off |
| `DISPATCH_MANAGED_SETTLEMENT_ENABLED` | Dispatch | off |
| `HAULING_MANAGED_SETTLEMENT_ENABLED` | Hauling | off |
| `PAYMENT_WALLET_MANAGED_RECONCILIATION_ENABLED` | Wallet | on |
| `FLEET_TYPED_FINANCIAL_VISIBILITY_ENABLED` | Fleet | off |

`FLEET_COMPENSATION_RESOLVER_ENABLED` is the boolean the mode replaced. It still
decides when the mode is unset, so an existing deployment keeps behaving exactly
as it did; the mode overrides it once set.

---

## Stage 1 — Wallet accepts instructions; nobody sends any

**Flags:** none change. Deploy the Wallet code carrying migrations 000013–000016.

**Behaviour:** the managed settlement routes exist and answer. Nothing calls
them, because no Fleet stage that produces an instruction is enabled.

**Metrics:** migration applied cleanly; `/health` and `/ready` green; zero rows
in `managed_settlements`; the reconciliation sweep runs and finds nothing.

**Abort if:** any migration fails, or the sweep logs an error on every tick.

**Rollback:** redeploy the previous Wallet build. Migration 000016's down is
data-aware and runs even on a database that used the feature.

**Invariant:** no journal exists that did not exist before.

**Operator check:** `SELECT count(*) FROM managed_settlements` returns 0.

---

## Stage 2 — Completed-work projection, one execution service

**Flags:** `FLEET_COMPLETED_WORK_PROJECTION_ENABLED=true`. Enable canonical
completed-work reporting on **Taxi only**.

**Behaviour:** Taxi reports completed corporate work to Fleet. Fleet records it
and stops: the resolver is `OFF`, so no compensation is computed and no
instruction is produced.

**Metrics:** completed-work rows appear at roughly the rate of corporate taxi
completions; Taxi's outbox drains; zero compensation resolutions.

**Abort if:** the outbox stops draining, or completed work is recorded for
independent providers.

**Rollback:** set the flag false. Recorded work stays and is harmless —
recording is what makes every later stage possible.

**Invariant:** no money has moved and no entitlement has been decided.

**Operator check:** completed-work count rises; resolution count stays 0.

---

## Stage 3 — Projection on the remaining two services

**Flags:** the same flag for Dispatch and Hauling.

**Behaviour:** as Stage 2, across all three services.

**Metrics:** per-service completed-work rates match each service's own
corporate completion rate. A service reporting far more or far less than its own
completions is the signal that matters.

**Abort if:** either new service's rate is implausible against its own
completions.

**Rollback:** per service, independently.

**Invariant:** unchanged — still nothing decided, still nothing paid.

**Operator check:** completed work grouped by source service.

---

## Stage 4 — Resolver in shadow

**Flags:** `FLEET_COMPENSATION_RESOLVER_MODE=SHADOW`.

**Behaviour:** Fleet resolves compensation for every recorded completion and
stores the resolution. Nothing acts on it: no instruction, no journal, no
payslip. This is the stage where the figures become checkable before any money
depends on them.

**Metrics:** every completed work has a resolution; provider plus organization
equals the eligible basis on every one; the distribution of provider shares
matches what each organization's plan says it should be.

**Abort if:** any resolution fails the conservation check, or the shares differ
from what an organization expects. Both are why this stage exists.

**Rollback:** set the mode back to `OFF`. Existing resolutions stay; nothing
ever acted on them.

**Invariant:** no money has moved. A resolution is a recorded opinion.

**Operator check:** compare a sample of resolutions against the organization's
own expectation, per plan, per service. Spend as long here as it takes.

---

## Stage 5 — Instructions, still unsent

**Flags:** `FLEET_COMPENSATION_RESOLVER_MODE=ON` **and**
`FLEET_SETTLEMENT_INSTRUCTION_ENABLED=false`.

**Behaviour:** resolutions are now live decisions rather than shadow ones, but
no instruction is produced, so Wallet still sees nothing.

**Metrics:** resolutions continue at the Stage 4 rate and shape; zero
instructions; zero settlements.

**Abort if:** the resolution shape changes on the mode flip. It must not —
shadow and on compute identically, and a difference means something read the
mode where it should have read the figures.

**Rollback:** back to `SHADOW`.

**Invariant:** still no money moved.

**Operator check:** resolution figures before and after the flip are identical
for comparable jobs.

---

## Stage 6 — Instructions to Wallet, one organization

**Flags:** `FLEET_SETTLEMENT_INSTRUCTION_ENABLED=true`, restricted to a single
pilot organization.

**Behaviour:** Fleet instructs Wallet for that organization's completed work.
Wallet posts one balanced journal per instruction. Money moves for the first
time.

The execution services are still on the legacy payout path for everything else,
so the pilot organization must be one whose providers are managed — otherwise
nothing changes and the stage proves nothing.

**Metrics:** one journal per instruction; every journal balances; the platform
share on each equals the fee fixed when the customer paid; zero
`FAILED_TERMINAL` settlements; the reconciliation sweep opens no rows.

**Abort if:** any settlement is refused as unbalanced, any journal does not
balance, or the sweep opens a row (money held with no settlement).

**Rollback:** set the flag false. Journals already posted stand; a correction is
a reversal, never an edit.

**Invariant:** for every settled job, platform + organization + provider equals
what the customer paid.

**Operator check:** reconcile the pilot organization's payable against the sum
of its settlements for the period.

---

## Stage 7 — Managed settlement isolation, per execution service

**Flags:** `TAXI_MANAGED_SETTLEMENT_ENABLED=true`, then Dispatch, then Hauling —
one at a time, with a gap.

**Behaviour:** corporate work stops taking the legacy direct-to-provider payout
on that service. It is settled by Fleet instead. Independent work is untouched.

This is the stage that can pay a provider twice if Stage 6 is not solid, which
is why Stage 6 comes first and why the services are enabled separately.

**Metrics:** per service, legacy `CompleteJob` calls for corporate work drop to
zero; no corporate job has both a legacy payout and a settlement; independent
payout volume is unchanged.

**Abort if:** any job has both, or independent payouts change at all.

**Rollback:** per service. A corporate completion during the gap is left for
Fleet, which is correct: the completed-work record is durable.

**Invariant:** one completed job, one payment.

**Operator check:** for the enabled service, corporate jobs with a legacy
payout journal in the period must be zero.

---

## Stage 8 — Commerce managed delivery

**Flags:** no new flag. Vendor begins setting the delivery executor mode on
Commerce allocations.

**Behaviour:** a Commerce delivery executed by a corporate rider is settled
through Fleet with the customer's frozen amount split between provider and
organization. An independent delivery is unchanged. An allocation whose executor
is not yet set is refused the direct payout rather than defaulting to
independent.

**Metrics:** allocations with `delivery_executor_mode = 'UNKNOWN'` trend to zero;
refusals of the direct path are matched by managed releases; the customer's
delivery amount is unchanged by who executed it.

**Abort if:** UNKNOWN allocations accumulate — that means deliveries are being
blocked rather than routed — or any managed release does not sum to the frozen
amount.

**Rollback:** stop setting the executor mode. Allocations already settled stand;
refunds against them use the recorded split.

**Invariant:** the customer's delivery amount does not change when a provider is
assigned.

**Operator check:** per allocation, provider released plus organization released
equals delivery released.

---

## Stage 9 — Typed financial visibility

**Flags:** `FLEET_TYPED_FINANCIAL_VISIBILITY_ENABLED=true`.

**Behaviour:** organizations' real visibility policies take effect. Until now
every managed provider saw the conservative default — their own payroll and
nothing else — which is safe but is not what most organizations want.

Last deliberately: a visibility mistake shows a provider something they should
not see, and once seen it cannot be unseen. Everything before this stage is
recoverable by a reversal.

**Metrics:** no provider response carries an organization or platform class;
notification payloads are accepted (a refused one means a caller is trying to
send a concealed field); logs carry no concealed class and no credential.

**Abort if:** any provider-facing response or notification carries a concealed
class.

**Rollback:** set the flag false. Every managed provider returns to the
conservative default immediately, on the next request rather than the next
refresh.

**Invariant:** organization and platform economics are unreachable to a
provider under any policy.

**Operator check:** sample provider job and payroll responses across policies
and confirm no concealed class appears.

---

## What is never done

- No flag goes from off to globally on in one step.
- No stage is skipped because the previous one "looked fine" — Stage 4 and
  Stage 6 exist precisely to produce evidence, and evidence not looked at is
  the same as no evidence.
- A correction after money has moved is a reversal, never an edit and never a
  rollback of a flag.
