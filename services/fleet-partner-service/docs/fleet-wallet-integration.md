# Fleet-to-Wallet integration (WP8)

Fleet calls Payment Wallet only through the shared signed HTTP client. Every request carries the Fleet service identity, a fresh HMAC timestamp, a cryptographically random one-time nonce, correlation/causation IDs and the stable idempotency key. Organization and provider-account provisioning use Wallet's real private API and Fleet stores only opaque account IDs, safe status and source version.

Migration `000016_wallet_finance_integration` adds durable outbound finance operations, safe Wallet projections, independent aggregate source versions and operation history. Migration `000017_finance_projection_reconciliation` adds projection leases, bounded attempts, safe errors, due/lease indexes and immutable reconciliation history. Migration `000018_finance_operator_access` adds the requesting actor and branch on each operation, a branch on each projection, and the finance operator permission catalogue. No Wallet balance, ledger account or journal is stored in Fleet, and Fleet writes no ledger entry of its own — the integration test asserts the Fleet schema contains no journal or ledger table at all.

The default-off `FLEET_FINANCE_INTEGRATION_ENABLED` worker runs in both API-worker and dedicated-worker deployments. It claims with `FOR UPDATE SKIP LOCKED`, recovers expired leases, bounds retries, dead-letters exhausted work, and supports replay. A timeout is persisted as `UNKNOWN`; the next pass calls Wallet's idempotency-key read-back endpoint and does not resubmit the write.

Wallet events are accepted at `POST /internal/fleet-partner/v1/finance/events` only when `FLEET_FINANCE_EVENT_INGESTION_ENABLED=true`. The existing internal HMAC middleware authenticates the caller and the finance handler further allowlists `FLEET_FINANCE_ALLOWED_CALLERS` (default `payment-wallet-service`). Inbox event-ID dedupe and raw payload hash reject changed redeliveries. Aggregate versions are tracked independently; older deliveries are ignored and gaps create a `GAP_DETECTED` projection that cannot authorize financial behavior.

The worker sweeps aged projections to `STALE`, queues gapped/unknown records, uses `FOR UPDATE SKIP LOCKED`, recovers expired leases, performs tenant-scoped Wallet read-back, rejects mismatched identities and older source versions, and resolves to `CURRENT` only after a valid authoritative response. Bounded failures dead-letter; authenticated operator replay resets the attempt budget. History, audit and outbox are committed atomically.

`FLEET_FINANCE_PROJECTION_STALE_SECONDS` defaults to 900. Internal HMAC replay routes are `POST /internal/fleet-partner/v1/finance/operations/:id/replay` and `POST /internal/fleet-partner/v1/finance/projections/:id/replay`, both requiring `organizationId`.

## Authoritative job-payment lookup

Wallet owns the authoritative record that a unit of dispatched work was paid for (`finance_job_payments`, migration `000006_reversal_contract_and_job_payments`). Fleet reads it at `GET /api/v1/payment-wallet/internal/finance/job-payments/lookup?organizationId=&referenceType=&reference=`.

Supported `referenceType` values are `DISPATCH_REQUEST`, `TRIP`, `BOOKING` and `PAYMENT_REFERENCE` — all references another service already owns. No new identifier was minted for the lookup. Each one is uniquely indexed per organization, so a reference resolves to exactly one record or to nothing.

The response carries only the payment verification ID, organization ID, request/trip reference, integer minor amount, ISO currency, payment status, source version, paid timestamp, refund/reversal state, authoritative operation reference and a derived `settled` flag. It carries no balance, no journal, no ledger account, no bank or card detail and no provider secret.

Enforcement: HMAC signature, caller allowlist, timestamp freshness, persisted nonce replay rejection, tenant ownership in SQL, and `finance.job_payment.view`. A record belonging to another tenant, an unknown reference and an unsupported reference type all answer `404` identically, so the endpoint cannot be used to probe another organization's payments. Recording a job payment requires `finance.payment.confirm`, which the Fleet service identity does not hold: Fleet can verify a payment and can never mint one.

Wallet populates the record from its own payment flow. `PayFromWallet`, `CompleteJob` and the Paystack `charge.success` path each mirror the payment intent into the verification record when the intent carries an `organization_id` (or `organizationId`) in its metadata and is not a wallet top-up. A projection write failure never rolls back the payment; the verification is simply absent, and an absent verification blocks downstream money movement rather than authorizing it.

Fleet's job-finance guard calls this lookup as the authority. Fleet's own `PAYMENT_VERIFICATION` projection is consulted only to fail closed: if Fleet holds a copy that is `STALE`, `GAP_DETECTED`, `UNKNOWN`, `RECONCILING`, or ahead of the authoritative source version, the operation is blocked. Unverified, refunded, reversed and unreachable all block commissions, earnings, fees, hold release, settlement and refund/reversal finalization.

## Durable reversal contract

Wallet owns a durable reversal aggregate (`finance_reversals`). Fleet requests a reversal and reads its status; Wallet approves and executes it.

States: `REQUESTED`, `PENDING_APPROVAL`, `APPROVED`, `PROCESSING`, `UNKNOWN`, `COMPLETED`, `FAILED_RETRYABLE`, `FAILED_PERMANENT`, `REJECTED`, `RECONCILING`, plus `DEAD_LETTER` when the retry budget is exhausted.

Each row records the reversal ID, organization, original journal reference, reason code, integer minor amount, ISO currency, request fingerprint, idempotency key, expected version, status, external operation reference, attempt count, retry schedule, correlation ID, causation ID, creating actor, approval reference and policy version, and the resulting compensating journal.

Routes (versioned, private, HMAC):

| Method | Path | Permission |
| --- | --- | --- |
| `POST` | `/reversals` | `finance.reversal.request` |
| `POST` | `/reversals/:id/approve` | `finance.reversal.approve` |
| `POST` | `/reversals/:id/reject` | `finance.reversal.approve` |
| `GET` | `/organizations/:organizationId/reversals` | `finance.transaction.view` |
| `GET` | `/organizations/:organizationId/reversals/:id` | `finance.transaction.view` |
| `GET` | `/organizations/:organizationId/reversals/:id/history` | `finance.transaction.view` |
| `GET` | `/operations/REVERSAL/:id` | `finance.transaction.view` |
| `GET` | `/operations/REVERSAL/by-idempotency` | `finance.transaction.view` |
| `POST` | `/operations/REVERSAL/:id/replay` | `finance.reconciliation.run` |

Rules enforced and covered by tests:

- The original posted entry is immutable; completion creates a separate compensating journal recorded on the reversal.
- A full-value reversal posts the exact mirrored entry and claims `reverses_journal_id`, which the database permits only once per original. A partial reversal is linked by causation and its lines are scaled by largest remainder within each side, so debits and credits still agree exactly.
- A reversal may not exceed the reversible amount: the original's debit total less live refunds and live reversals. A rejected reversal releases its claim.
- A duplicate identical request returns the original result; a conflicting fingerprint under the same key is rejected with `409`.
- A cross-tenant original journal is rejected before any state is created.
- A gateway timeout becomes `UNKNOWN`; the next pass reads the authoritative outcome back rather than executing again. An abandoned lease also becomes `UNKNOWN`.
- A reversal cannot execute twice: a compensating journal already posted under the reversal's key resolves as success, `resulting_journal_id` is uniquely indexed, and the ledger permits one full reversal per original.
- Approval is governed by the versioned `REVERSAL_APPROVAL` policy with self-approval disabled, so the requester cannot approve their own reversal.
- Every transition writes history, an audit record and an outbox event in the same transaction.

Execution, read-back, retry, dead-lettering and replay run inside the existing Wallet financial worker (`RunFinancialWorkers` / `RunFinancialPass`) using the same lease, attempt and backoff machinery as settlements and refunds. No second operation framework was introduced. Reconciliation reports reversals that stayed ambiguous past the window (`REVERSAL_UNKNOWN_OUTCOME`) and reversals claiming completion without a journal (`REVERSAL_COMPLETED_WITHOUT_JOURNAL`); it reports rather than repairs.

## Fleet finance operator routes

Corporate, tenant-scoped, deny-by-default routes under `/fleet-partner/v1`:

| Method | Path | Permission |
| --- | --- | --- |
| `GET` | `/finance/operations` | `finance.operations.read` |
| `GET` | `/finance/operations/:operationId` | `finance.operations.read` |
| `GET` | `/finance/operations/:operationId/history` | `finance.operations.read` |
| `GET` | `/finance/operations/:operationId/audit` | `finance.operations.read` |
| `POST` | `/finance/operations/:operationId/replay` | `finance.operations.replay` |
| `GET` | `/finance/projections` | `finance.operations.read` |
| `GET` | `/finance/projections/:projectionId` | `finance.operations.read` |
| `GET` | `/finance/reconciliation-cases` | `finance.reconciliation.read` |
| `GET` | `/finance/reconciliation-cases/:projectionId` | `finance.reconciliation.read` |
| `POST` | `/finance/reconciliation-cases/:projectionId/replay` | `finance.reconciliation.manage` |
| `POST` | `/finance/reconciliation-cases/:projectionId/read-back` | `finance.reconciliation.manage` |
| `POST` | `/finance/settlements` | `finance.settlements.request` |
| `POST` | `/finance/refunds` | `finance.refunds.request` |
| `POST` | `/finance/reversals` | `finance.reversals.request` |

The permissions are seeded by migration `000018_finance_operator_access` and granted to the owner and finance-manager system roles in full; administration, operations, compliance and analyst roles receive read permissions only.

Enforcement:

- Deny by default. A member holding no role reaches nothing; a read permission never implies replay or a money-moving request.
- Organization scope comes from the resolved membership, never from the request body or query.
- Object-level ownership is part of every query. A record belonging to another tenant, or to a branch outside the caller's authority, answers `404` — revealing that an identifier is real would itself be a disclosure. Replay proves ownership before mutating.
- Branch scope: a branch-restricted membership sees its own branches plus organization-level rows; a write aimed at another branch is refused with `403`.
- Suspended staff are rejected by the existing tenant-resolution middleware before any route runs.
- Write routes require an `Idempotency-Key` header. A repeated key with different values is a `409` fingerprint conflict, never a silent rewrite.
- Settlement requires a `CURRENT`, non-reconciling projection and, when supplied, an expected version matching the authoritative source version.
- Separation of duties: an operator may not request the reversal of an operation they requested themselves.
- Every operator action writes an audit record and an outbox event in the same transaction as the mutation.
- Responses carry the operation's own lifecycle and the opaque Wallet reference only — never balances, ledger identifiers or credentials.

## Orchestration gates

`RequestJobEarning` requires, in order: an authoritative Wallet payment lookup returning a settled payment; no disagreeing local projection; a `CURRENT` Dispatch trip projection; a completed delivery; no authoritative failure; no authoritative cancellation; and recipient confirmation when policy requires it. Duplicate completion events resolve to the same operation through the organization-scoped idempotency key, so a repeated completion cannot create a second earning.

`RequestSettlement` additionally requires no unresolved reconciliation case for the subject account. `RequestRefund` verifies the referenced payment's amount and currency against the authoritative record when a payment reference is supplied. `RequestReversal` enforces separation of duties against the recorded requester.

## Cross-service test coverage

Fleet and Wallet are separate Go modules and neither may import the other, so the proof is split into two suites that share the production signing and verification code in `shared/go/serviceauth`:

- `services/payment-wallet-service/internal/integration/cross_service_finance_test.go` drives the real Wallet gin handlers over HTTP against real PostgreSQL, signing with the shared signer: valid HMAC, invalid signature, unsigned, non-allowlisted caller, stale timestamp, replayed nonce, job-payment lookup by every supported reference, not found, tenant mismatch, data minimization, refund state closing the gate, reversal request, duplicate, fingerprint conflict, cross-tenant rejection, over-reversal, currency mismatch, read-back by ID and by key, listing, self-approval refusal, approval, timeout to `UNKNOWN`, authoritative read-back, completion, compensating journal shape and balance, no second execution, history/audit/outbox, rejection, dead-letter replay, replay permission, crash-restart recovery, partial reversal balancing, and reconciliation findings.
- `services/fleet-partner-service/internal/integration/finance_cross_service_test.go` drives the real Fleet `WalletClient` over HTTP against a server running the same shared verifier, plus the real Fleet operator routes against real PostgreSQL: valid HMAC, invalid signature, wrong caller, stale timestamp, replayed nonce, tenant mismatch, payment not found, unauthorized finance routes, cross-tenant IDOR, branch scope, operation replay, reconciliation replay, read-back trigger, settlement gates, separation of duties, idempotency, failed delivery blocking earnings, cancelled delivery blocking settlement, the recipient-confirmation gate, refunded payment blocking earnings, stale projection blocking earnings, and duplicate completion not duplicating earnings.
