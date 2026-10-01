# KARRYAM Sections 12, 13 and 14 — closure status

This records what is finished, what is not, and why — for the phase that began
at `d8c24ab`. Pricing was out of scope throughout and no fare, quote, estimate
or pricing algorithm was modified in any service.

## Verdict

| Section | Status | Reason |
| --- | --- | --- |
| 12 — Managed settlement | **CLOSED** | The money path is complete and certified end to end, including Commerce refund and dispute reversal (WAL-029/030). |
| 13 — Compensation | **CLOSED** | Every required CMP item is implemented and certified. The resolver can be enabled safely and stays OFF by default, with a shadow stage between off and on. |
| 14 — Visibility | **CLOSED** | The policy, the provider views, the Wallet surface audit, notification enforcement and log redaction are all implemented and certified. |

Closed here means: implemented, certified by named tests, and — where a live
round trip adds something a database-backed test cannot — certified against a
running service. The evidence is in `KARRYAM_PART_G_FIN_MATRIX.md` and
`KARRYAM_PART_E_LIVE_WIRING_EVIDENCE.txt`.

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

## Section 12 — WAL-029/030, now closed

Recovery is computed from the durable per-mission record of what was actually
paid, not from the customer-facing delivery payable and not from today's
compensation policy — a plan that changed since the delivery would otherwise
decide how much a provider gives back from a job they were paid under the old
one. A salaried rider who received nothing gives back nothing, and a refund that
tries to take the organization's share from the rider is refused.

The split is integer-only: floored shares with the leftover units handed out by
largest fractional remainder and a fixed tie-break, so the two parts always sum
to exactly the amount refunded and a replay recovers the same amounts. A dispute
reversal is its own operation drawing on the same pool, so in either order a
refund and a dispute can never between them take back more than was paid.

Three real defects surfaced while building it: a per-mission release was refused
unless it settled the whole outstanding amount, so a second mission was
impossible and WAL-026 could not actually happen; the managed release journal
was keyed on the order, so a second mission's journal collided with the first;
and the refund journal was keyed on the order alone, so a second partial refund
could never post.

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

## Section 13 — the remaining CMP items, now closed

Salary proration (CMP-006/006A/006B) is deterministic and integer-only against a
canonical basis of whole UTC calendar days, and carries enough provenance to be
checked by hand. Allowance bases are declared rather than inferred (CMP-011),
overtime comes only from an approval record that cannot be reused in another
period (CMP-012), and bonuses and deductions carry source, reason, authoriser
and period (CMP-013/014). An unauthorised line is refused rather than entered at
zero, because a silent zero and a silent full payment are the same class of bug
and only one is visible.

Organization share (CMP-021) is a residual, which is what makes economic
conservation checkable, and the check refuses a rule settled at both levels. A
multi-service provider (CMP-033) is paid one salary, not one per service.

The resolver flag (CMP-001/019) gained a shadow stage: it records exactly what
it would pay while nothing acts on the figures. It remains OFF by default, and a
misconfigured mode resolves to OFF rather than being guessed at.

## Section 14 — what is closed

One `providercompat.FinancialVisibilityPolicy` is enforced server-side by all
three execution services. The organization and platform classes are
unreachable under any policy rather than merely off by default, the
conservative default shows a managed provider nothing but their own payroll,
and an unclassified money field is withheld rather than guessed at.

## Section 14 — the remaining VIS items, now closed

The per-job and payroll views (VIS-007/014) are constructed shapes with no
passthrough: the organization's and the platform's figures are inputs to the
builder precisely so its refusal to emit them is testable. The payroll view
refuses a requester who is not the provider and does not even disclose whose
payroll it was. The compensation explanation (VIS-015) is typed rather than an
internal calculation blob.

The Wallet provider surface (VIS-013) is audited against the real router, so a
route added later is covered the day it is added. Notification payloads
(VIS-017) are enforced rather than audited: a provider-addressed notification
carrying an organization or platform field is refused. Log redaction (VIS-018)
removes credentials and conceals organization and platform economics while
leaving what an operator actually needs.

## Parts E, F and G

**Part E** drove 31 real HMAC-signed checks against a running
payment-wallet-service, including a full Fleet-instructs, Wallet-posts,
execution-service-reads-back journey with retry, conflict, legacy-path refusal
and reversal. Evidence: `KARRYAM_PART_E_LIVE_WIRING_EVIDENCE.txt`; the probe
itself is in `docs/liveprobe`.

It found a production bug no unit test could have. Wallet stored `source_service`
as the instruction carried it — "taxi" — while the shared read client normalises
to "TAXI" so the synchronous and reconciliation paths agree. The by-job lookup
matched case-sensitively, so an execution service could never find its own
settlement: the read returned nothing, which reads as "not settled", which is
exactly how a job gets paid twice.

**Part F** is `KARRYAM_PART_F_ROLLOUT.md`: nine stages, each with flags,
behaviour, metrics, abort condition, rollback, invariant and operator check.

**Part G** is `KARRYAM_PART_G_FIN_MATRIX.md`: FIN-01 through FIN-35, every row
naming the implementation, the test and the live check. 35 PASS, 0 FAIL.

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

Migrations `000078` (Fleet) and `000014`, `000015`, `000016` (Wallet) each
verified apply → rollback → reapply. Fleet `000077`, Taxi `012`, Hauling `018`
and Wallet `000013` were not modified.

Migration `000016`'s rollback is data-aware, and deliberately so. Narrowing the
operation-type constraint or recreating the old one-release-per-allocation index
would fail on exactly the databases that used the feature, so the constraint
keeps the new value and the index is restored only where the recorded data still
satisfies it. A rollback that cannot run on a database that used the feature is
not a rollback, and deleting records of money that moved is not an option.

The Vendor deadlock regression was re-run under the exact default pool
configuration that previously hung (`MaxConns` 4 on a 4-core machine): all ten
`TestSection17FCheaper*` tests complete in 1.39s, the concurrent one in 0.24s.

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
