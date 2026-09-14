# KARRYAM Sections 12, 13 and 14 — closure status

This records what is finished, what is not, and why — for the phase that began
at `d8c24ab`. Pricing was out of scope throughout and no fare, quote, estimate
or pricing algorithm was modified in any service.

## Verdict

| Section | Status | Reason |
| --- | --- | --- |
| 12 — Managed settlement | **OPEN** | The money path is complete and certified end to end, including Commerce. What remains is Commerce refund/dispute reversal for a managed delivery (WAL-029/030), which has no implementation and no test. |
| 13 — Compensation | **OPEN** | The release blocker (CMP-027, double-pay prevention) is closed and enforced in production code. The salary-proration, allowance-basis, overtime, bonus and deduction rules (CMP-006/011/012/013/014) are not implemented, so the resolver flag stays off (CMP-001/019). |
| 14 — Visibility | **OPEN** | The policy itself is certified and cannot be bypassed. VIS-007/013/014/015/017/018 — the per-job provider view, provider-scoped payroll view, typed explanation, and the notification and logging redaction audits — were not done. |

Closing any of the three would mean calling work certified that has not been
written. The parts that are finished are finished; the list below says which.

## Section 12 — what is closed

**The instruction contract.** Fleet decides what each party is entitled to and
hands Wallet resolved amounts — not rates, not bases, not rules. Wallet checks
them against the money it actually holds and refuses anything that does not
reconcile, restating the invariant against the payment intent rather than the
instruction alone, so an instruction that internally sums correctly but was
resolved against the wrong figure is still refused.

**The execution services.** Taxi, Dispatch and Hauling each had the Section 12
seam and nothing behind it. Each now confirms with the money authority whether
a completed job's money moved, through one read-only shared contract
(`shared/go/managedsettlement`). They cannot instruct a settlement and nothing
in the package computes an amount.

Three outcomes are kept apart because collapsing any two pays somebody twice or
not at all: the money moved, it definitively did not, and Wallet could not be
asked. The third is not a failure — an unreachable Wallet answering "nothing
settled" is exactly how a job gets paid a second time.

Dispatch's two settlement paths (the synchronous completion and the
reconciliation worker) derive one identity in one place. If each built its own,
the worker would ask Wallet about a job under a name it has never held,
conclude nothing was paid, and pay the rider again. Dispatch keys on the trip
rather than the booking, because one booking can produce more than one trip.

**Durable events.** Posted, failed and reversed events are appended to the
outbox inside the transaction that posts or reverses the journal. An event
written afterwards can be lost while the money has already moved. A caller's
retries are not events; a terminal refusal is recorded in its own transaction
after the posting rolls back, because the refusal has to survive precisely
because the posting did not. Transient and ambiguous errors are deliberately
not recorded as failures.

**The stuck-job sweep.** A job marked `ORGANIZATION_MANAGED` is closed to the
legacy direct payout. If Fleet never instructs a settlement the money stays in
escrow with no error anywhere — correct, and invisible. The sweep makes it
visible, one row per job rather than one per tick, resolving before it scans so
a job that settled between sweeps is closed out rather than reported once more
on its way out. It moves no money and a test pins that.

**Commerce (Part B).** A platform Commerce delivery used to credit the whole
frozen delivery payable to the executing rider. Independent deliveries are
unchanged. A corporate rider's entitlement is now Fleet's to decide: the frozen
amount leaves escrow once, the provider gets what Fleet resolved (frequently
zero — that is the salaried case and writes no provider line at all), and the
organization gets the rest. The customer's amount does not change because a
corporate rider took the job. `UNKNOWN` is refused rather than treated as
independent: a refused release can be retried once the executor is known, a
payment to the wrong party cannot be taken back.

## Section 12 — what is open

**WAL-029/030, refund and dispute reversal for a managed Commerce delivery.**
The existing refund path reduces `provider_delivery_payable_minor` and recovers
from the provider. A managed delivery's money is now split between the provider
and the organization, so a refund has to recover proportionally from both, and
that arithmetic does not exist. Nothing has been written and nothing is
claimed. The managed release path is safe to enable; a refund against a
managed delivery is not.

## Section 13 — what is closed

**CMP-027, the release blocker.** A compensation component could be paid twice
— once with the completed job's own money and again by payroll. The two are
computed by different code, at different times, from different tables, and
nothing downstream can detect the duplicate because a duplicated component
looks exactly like a legitimate second one.

Every rule now carries an explicit, plan-versioned settlement strategy
(`IMMEDIATE_WORK_SETTLEMENT` or `PAYROLL_ACCRUAL`). The defaults reproduce
existing plan behaviour exactly, so adopting the field changes no plan's
outcome, and an immediate salary or per-shift rule is refused because it has no
per-job meaning. Work settlement records what it paid, in the same transaction
that creates the instruction authorising the payment; payroll reads that record
and skips exactly those components.

Recording happens only once an instruction exists. With settlement dark no
money moves at work level, so payroll must still pay those components and an
exclusion row would wrongly suppress them.

Certified with a control assignment, so a payroll pass that had simply stopped
producing per-job items — a regression, not a fix — would not pass. Removing
the guard fails both tests.

## Section 13 — what is open

CMP-006/006A/006B (salary proration for mid-period start and end, in integer
minor units), CMP-011 (allowance bases), CMP-012 (overtime from approved
records only), CMP-013 (bonus provenance), CMP-014 (deduction audit), CMP-021
(organization-share semantics) and CMP-033 (multi-service provider) are not
implemented. Because they are not, the compensation resolver flag stays off
(CMP-001, CMP-019): turning it on would resolve compensation with rules that do
not yet handle a provider who starts mid-month.

## Section 14 — what is closed

One `providercompat.FinancialVisibilityPolicy` is enforced server-side by all
three execution services. The organization and platform classes are
unreachable under any policy rather than merely off by default, the
conservative default shows a managed provider nothing but their own payroll,
and an unclassified money field is withheld rather than guessed at.

## Section 14 — what is open

VIS-007 (per-job provider view), VIS-013 (Wallet provider reads cannot bypass
policy), VIS-014 (provider-scoped payroll view), VIS-015 (typed compensation
explanation), VIS-017 (notification audit) and VIS-018 (logging and redaction
audit) were not done.

## Also not done

Parts E, F and G of the specification — live wiring certification against
running services, the nine-stage flag rollout order, and the FIN-01…FIN-35
certification matrix as a single document. The individual financial invariants
those parts enumerate are covered by tests where the corresponding feature
exists; the matrix itself was not produced.

## The Vendor deadlock (Part H) — closed

`TestSection17FCheaper_ConcurrentApplicationsRefundOnce` hung indefinitely.
This was a production defect, not a test artefact.

`lockAdjustment` pinned a pooled connection while taking a blocking
session-level `pg_advisory_lock`, and the guarded critical section then needed
several more connections from the same pool. With as many concurrent callers as
the pool has connections, every connection was held by a lock holder or waiter:
the holder could not get a connection to finish, so it never unlocked, and the
waiters never woke. Both waits are unbounded, so it hung rather than erroring.

Proof: `pool_max_conns=20` passed in 0.14s; the default pool (MaxConns 4 on a
4-core machine) was still running at a 150s timeout. Production uses
MaxConns=10, so ten concurrent callers on one adjustment reproduce it.

The lock now retries `pg_try_advisory_lock` on a bounded deadline, releasing
the connection between attempts, and returns a conflict rather than waiting
forever. The unlock still runs on the connection that took the lock, because a
session advisory lock belongs to its session and returning the connection
without unlocking would leak the lock into the pool.

All ten `TestSection17FCheaper*` tests now pass in 1.28s under the exact
default-pool configuration that previously deadlocked, and the full Vendor
suite passes 68/68 packages.

A sweep confirmed the only remaining blocking `pg_advisory_lock` is in
`tools/migrate/main.go`, which is correct: a single-process migration binary
with no pool contention.

## Defects found and fixed along the way

- **Wallet's read-back allowlist refused two of the three execution services.**
  It held directory-style names, but Taxi signs as `taxi-service` and Dispatch
  as `dispatch-delivery-service` — the identities
  `PAYMENT_WALLET_SERVICE_SECRETS` was provisioned with long before the route
  existed. A correctly configured caller passed HMAC and was then refused, in a
  way that looks like a credential problem and is not one.
- **Hauling discarded the handoff error**, so a completion whose settlement
  state is unknown — the one case an operator needs to see — was silently
  dropped.
- **The managed Commerce release keyed its ledger reference on the mission
  alone.** A mission reference is only unique within its own order, so two
  orders that each named their first mission the same way collided on the
  ledger's unique reference and the second release failed outright.

## Verification

All seven Go modules, against real PostgreSQL 16 and Redis, zero failures:
`shared/go`, `payment-wallet-service`, `fleet-partner-service`,
`driver-taxi-service`, `driver-dispatch-delivery-service`,
`driver-hauling-service` (build, vet, `go test -p 1 ./...` each), and
`vendor-service` through its own isolated runner (68/68 packages).

Migrations `000078` (Fleet), `000014` and `000015` (Wallet) each verified
apply → rollback → reapply. Fleet `000077`, Taxi `012`, Hauling `018` and
Wallet `000013` were not modified.

No test was deleted, skipped or weakened.

## Test-infrastructure notes

Carried forward and confirmed again, because each costs an hour to rediscover:

- Fleet's harness refuses a database whose name does not contain `test`.
  Wallet's does too — a run against `wallet_full2` fails 64 tests for that
  reason alone.
- Fleet and Dispatch need their migrations applied to a fresh database before
  their suites pass; Hauling must have an **empty** one, because it applies its
  own.
- Vendor must be run through `tools/vendor-test-runner`, which gives each
  package its own database. Running `go test ./...` against one shared database
  fails 71 tests purely from cross-package contamination — every one of those
  packages passes in isolation.
- Vendor's Section 17A/17B certifications parse the DSN and require a TCP
  hostname; a unix-socket DSN fails them.
