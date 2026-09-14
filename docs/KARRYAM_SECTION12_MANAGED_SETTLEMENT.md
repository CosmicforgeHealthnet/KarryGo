# KARRYAM Section 12 — Payment-Wallet managed settlement

This is the money path for a corporate provider's completed job, end to end:

```
Taxi / Dispatch / Hauling completes
  -> execution service reports canonical CompletedWork
  -> Fleet resolves the effective compensation plan
  -> Fleet creates an immutable CompensationResolution
  -> Fleet creates an immutable ManagedWorkSettlementInstruction
  -> Fleet submits that instruction to Payment-Wallet
  -> Wallet posts one balanced journal
  -> platform / organization / provider entitlements are recorded
```

## The division of authority, and why it is enforced rather than documented

Four services touch this path and each one is deliberately unable to do the
others' job.

**Fleet decides what each party is entitled to.** It resolves the provider's
effective plan and produces an instruction carrying resolved amounts — not
rates, not bases, not rules. `TestSubmissionCarriesFleetsResolvedFiguresVerbatim`
asserts the wire body contains no `rate_bps`, no `commission_basis`, no
`rules`: Wallet is handed answers precisely so it has nothing to recompute
from.

**Wallet decides how that entitlement is posted, and verifies it.** It checks
the instruction against the money it actually holds and refuses anything that
does not reconcile. It never adjusts an instruction to fit.

**The platform's own share stays Wallet's.** It was fixed when the customer
paid. Fleet may state it — and is checked against it — but cannot change it
(`TestInstructionPlatformShareCannotOverrideTheAuthorizedFee`). Two authorities
computing one number is how they drift apart.

**Fleet never posts a journal; Wallet never resolves a plan.**

## What Wallet verifies before anything is posted

Given a held payment intent of `amount` with `platformFee` already fixed:

```
distributable = amount - platformFee
require  instruction.grossBasis        == distributable
require  providerShare + organizationShare == grossBasis
require  platformFee + organizationShare + providerShare == amount
```

The invariant is restated against the intent rather than against the
instruction alone, so an instruction that internally sums correctly but was
resolved against the wrong figure is still refused. No residual is dropped and
none is invented (`TestUnderAndOverAllocationAreRefused`).

## Journal shapes

All amounts in minor units. Escrow is credit-normal, so releasing it debits.

**Commission provider** — fare 10,000, platform fee 1,000, Fleet resolved 1,500
provider / 7,500 organization:

| account | side | amount |
| --- | --- | --- |
| job escrow | debit | 10,000 |
| platform revenue | credit | 1,000 |
| organization payable | credit | 7,500 |
| provider payable | credit | 1,500 |

**Salaried provider** — same fare, Fleet resolved 0 provider / 9,000
organization:

| account | side | amount |
| --- | --- | --- |
| job escrow | debit | 10,000 |
| platform revenue | credit | 1,000 |
| organization payable | credit | 9,000 |

No provider line is written at all. A zero-amount entry would be rejected by
the ledger, and inventing a token credit would make a salaried provider look
like they earned per job. The provider is paid through payroll
(`TestSalariedProviderReceivesNoPerJobCredit`).

**Independent provider** — unchanged from before this section: the legacy
`CompleteJob` path still credits `amount - platformFee` directly
(`TestLegacyCompleteJobStillWorksForIndependentProviders`).

## The legacy boundary

`CompleteJob` pays the provider the whole amount minus the platform fee. For
organization-managed work that is the wrong answer, so the path now refuses it
outright rather than relying on every caller to remember not to call it.

Two signals, each covering the other's blind spot: the intent's own
`settlement_mode` marker, set when a managed executor is assigned, and an
existing `managed_settlements` row. Either is enough to refuse.

## Idempotency and the lost response

One `(instructionId, instructionVersion)` posts one journal. A retry returns
the original outcome rather than creating a second
(`TestResendingTheSameInstructionPostsOnce`), including when the synchronous
completion path and the reconciliation worker fire concurrently
(`TestConcurrentSendsOfOneInstructionPostOnce` — six goroutines, one journal).

The same identity sent with different amounts is a **conflict**, not a repeat.
Collapsing those two is how a changed instruction gets silently ignored.

A timeout is never read as failure. Fleet's posting machinery classifies an
ambiguous transport outcome as `OUTCOME_UNKNOWN` and reads the settlement back
by its idempotency key before deciding anything.

## Reversal

Corrections are additions, never edits. The original journal stays; a
compensating journal with the sides swapped carries the original instruction's
identity and a required reason code. Reversing twice moves the money once
(`TestReversalCompensatesWithoutErasingHistory`).

## Routes

All inside the HMAC-protected internal group, narrowed further by caller:
posting and reversal are Fleet's alone; read-back is open to Fleet and the
three execution services, because a service reconciling its own completion
needs to know whether the money moved.

```
POST /internal/finance/v1/managed-settlements
GET  /internal/finance/v1/managed-settlements/{instructionId}/{instructionVersion}
GET  /internal/finance/v1/managed-settlements/by-key/{idempotencyKey}
GET  /internal/finance/v1/managed-settlements/by-job/{sourceService}/{sourceJobId}
POST /internal/finance/v1/managed-settlements/{instructionId}/{instructionVersion}/reverse
PUT  /internal/finance/v1/managed-jobs/{sourceService}/{sourceJobId}
```

## Migration

`000013_managed_work_settlement` adds the `managed_settlements` table (unique
on instruction identity and on idempotency key, indexed by source job and by
open status) and `payment_intents.settlement_mode`. Apply → rollback → reapply
verified.
