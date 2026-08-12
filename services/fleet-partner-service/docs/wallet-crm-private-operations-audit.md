# Wallet, CRM and Private Operations Readiness Audit

Audit date: 2026-07-26

Final decision: **NO-GO**

This audit evaluates the repository as checked out. Interfaces, fail-closed
adapters, schema-only placeholders, skipped tests, and unconfigured production
transports are not counted as implemented capabilities.

## 1. Repository baseline

| Field | Result |
| --- | --- |
| Branch | `staging` |
| HEAD | `7320aaaeb3bb7427b1ffb15c003bc6389d549c1f` |
| Tracked modified files | 5 |
| Untracked files | 246 |
| Fleet files | Entire service is untracked (215 files) |
| Dispatch untracked files | 31 |
| Unrelated service changes | None; changes are `go.work`, fleet, and dispatch only |

Tracked modifications:

- `go.work`
- `services/driver-dispatch-delivery-service/.env.example`
- `services/driver-dispatch-delivery-service/cmd/main.go`
- `services/driver-dispatch-delivery-service/internal/config/config.go`
- `services/driver-dispatch-delivery-service/internal/router/router.go`

Untracked paths reported by `git status --short`:

- `services/driver-dispatch-delivery-service/internal/features/marketplace/`
- `services/driver-dispatch-delivery-service/internal/router/acceptance_to_canonical_trip_harness_test.go`
- `services/driver-dispatch-delivery-service/internal/router/marketplace_wiring_test.go`
- `services/driver-dispatch-delivery-service/internal/router/provider_reads_composition_test.go`
- Dispatch migrations `000031` through `000033`, both directions
- `services/fleet-partner-service/` (215 files, including all migrations,
  production code, tests, and this audit)

`git diff --stat` shows only five tracked files and therefore materially
understates the worktree: the whole fleet service and most of Sections 11-12
are untracked. Sections 11-12 are not committed at HEAD. The repository is not
in a safe state for another large package because there is no committed,
reviewable prerequisite baseline and 246 untracked files can be lost, mixed,
or partially staged.

## 2. Sections 11-12 prerequisite matrix

| # | Requirement | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Real canonical request-reservation contract | VERIFIED | Dispatch mounts authenticated internal reservation routes in `internal/features/marketplace/http.go:53-76`; real PostgreSQL marketplace tests passed. |
| 2 | Idempotent reservation creation | VERIFIED | Command fingerprint/key persistence and read-back exist; dispatch marketplace database suite passed. |
| 3 | Reservation ownership and concurrency | VERIFIED | Row locks, source versions, and unique live-reservation constraints are exercised by the marketplace database suite. |
| 4 | Real execution/canonical-trip contract | VERIFIED | `POST /api/v1/internal/marketplace/executions` is wired in the composition root and creates the canonical dispatch trip. |
| 5 | Execution read-back after unknown outcome | VERIFIED | Dispatch exposes idempotency read-back; the real E2E lost-response scenario performs one POST, one read-back, and keeps one trip. |
| 6 | Fleet uses authenticated HTTP, not dispatch DB | VERIFIED | Production uses `platformclients.DispatchExecution`; direct dispatch DB access appears only in the cross-service test harness. |
| 7 | Provider Accept and Decline enforced | VERIFIED | Nine focused Accept/Decline tests ran against PostgreSQL and passed. |
| 8 | Staff cannot accept for a provider | VERIFIED | Staff assignment and provider response are separate; provider acceptance is verified through the provider-bound route/session. |
| 9 | Provider identity is authoritative | VERIFIED | Dispatch provider action derives provider ID from provider auth; fleet validates the response binding. |
| 10 | No execution before provider acceptance | VERIFIED | Execution worker re-reads an `ACCEPTED` provider response, and dispatch independently verifies it over signed HTTP. |
| 11 | Delegated acceptance needs no second acceptance | VERIFIED | Delegated offer acceptance atomically creates the assignment response; focused test passed. |
| 12 | Expiry never becomes acceptance | VERIFIED | State/expiry guards exist; no expiry transition creates a response. |
| 13 | Cancellation blocked in `EXECUTION_START_PENDING` | VERIFIED | Focused real-PostgreSQL test passed. |
| 14 | Acceptance creates exactly one canonical trip | VERIFIED | Cross-service E2E passed and asserted one trip after concurrency and reruns. |
| 15 | Unknown outcome read-back creates no duplicate | VERIFIED | Cross-service E2E passed with a lost response, one read-back, and exactly one trip. |
| 16 | Existing customer dispatch unchanged | PARTIAL | Existing unit tests pass, but the database-backed full dispatch suite fails and new trip columns/migrations are uncommitted. |
| 17 | Existing independent-provider behavior unchanged | PARTIAL | Existing unit tests pass; the same failing database suite prevents a complete compatibility claim. |
| 18 | Provider DTO data minimization | VERIFIED | Two focused PostgreSQL tests passed; DTOs omit fare, wallet, commission, margin, and settlement values. |

The final E2E test exists as `TestAcceptanceToOneCanonicalTrip`. Exactly one
test matched and it ran (not skipped) against the isolated fleet and dispatch
PostgreSQL databases. It covered provider acceptance -> durable dispatch relay
-> fleet acceptance -> dispatch reservation -> execution-start operation ->
canonical trip -> `READY_FOR_EXECUTION`, including concurrency, ambiguous POST
outcome, read-back, and duplicate prevention.

Prerequisite result: **PARTIAL**. The core acceptance guarantee is real, but
Sections 11-12 are not a stable repository baseline and their event/cancellation
surface is incomplete for private operations.

## 3. Event and projection readiness

| Capability | Status | Finding |
| --- | --- | --- |
| Durable dispatch request events | PARTIAL | Reservation and release write marketplace outbox rows; the full request lifecycle is not emitted durably. |
| Durable dispatch trip events | PARTIAL | `trip.created.v1` is persisted by the marketplace path; lifecycle/proof/cancellation use the existing non-transactional Redis publisher instead of this outbox. |
| Transactional dispatch outbox | VERIFIED | `dispatch_marketplace_outbox` is written in the request/trip transaction. |
| Dispatch outbox delivery worker | MISSING | Repository claim/retry methods exist, but no production worker delivers `dispatch_marketplace_outbox`. The wired worker is the provider-decision relay, a different table and flow. |
| Retry, lease, dead-letter, replay | PARTIAL | SQL/repository primitives exist; without an outbox delivery worker there is no reachable delivery or replay capability. |
| Fleet event-ingestion HTTP contract | MISSING | No dispatch event-ingestion handler or route is mounted. |
| Event service authentication/allowlist | MISSING | No event route exists to authenticate or authorize. |
| Fleet inbox deduplication | PARTIAL | Generic inbox production code handles duplicates and payload mismatch, but no dispatch consumer calls it. |
| Duplicate event handling | PARTIAL | Foundation exists; unreachable for dispatch events. |
| Same event ID/changed payload rejection | PARTIAL | `events.InboxRepository.Begin` rejects the mismatch; no dispatch ingestion path uses it. |
| Source-version ordering | PARTIAL | Projection repositories compare/store versions; no event consumer applies them. |
| Gap detection | PARTIAL | Tables and repository methods exist; no ingestion/reconciliation orchestration is wired. |
| Request projections | PARTIAL | Schema/repositories and direct reconciliation reads exist; event-fed operation is absent. |
| Trip projections | PARTIAL | Schema/repositories exist; event-fed lifecycle updates are absent. |
| Projection freshness | PARTIAL | `CURRENT`, `STALE`, `GAP_DETECTED`, `RECONCILING`, and `UNKNOWN` exist and gate commands; no complete refresh pipeline exists. |
| Reconciliation after missing/out-of-order events | PARTIAL | Read contracts and gap methods exist; no scheduled end-to-end reconciler was found. |
| Expiry/reassignment integration | MISSING | No durable source events delivered into fleet. |
| Cancellation integration | MISSING | No durable event integration and no mounted private internal trip-cancellation route. |

Fleet's own outbox delivery also does not count as complete: both API and
worker composition roots explicitly use `events.DisabledPublisher` (for
example `cmd/main.go:104-129`). Events remain pending by design.

Event/projection result: **NOT READY**. Private-order status, completion,
cancellation, proof, and settlement cannot safely depend on the current event
path.

## 4. Fleet foundation findings

| Foundation | Status | Finding |
| --- | --- | --- |
| Migration sequence | IMPLEMENTED | `000001`-`000011`, unique numbers, every up has a down. Entire sequence is untracked. |
| Organization model/lifecycle | IMPLEMENTED AND TESTED | Models, repositories, HTTP/use cases, activation/closure gates. |
| Managed-provider lifecycle | IMPLEMENTED AND TESTED | Memberships, invitations, consent, saga, and sub-wallet projection. |
| Organization permissions | IMPLEMENTED AND TESTED | Deny-by-default scoped authorizer and IAM tests. |
| Role templates | IMPLEMENTED AND TESTED | Seeded immutable system roles and permission catalogue. |
| Approval framework | PARTIAL | Rich schema only; `internal/features/approvals/doc.go` is an 8-line placeholder. |
| Self-approval controls | PARTIAL | Schema has `requester_may_approve`; no production approval engine enforces it. |
| Approval fingerprints | PARTIAL | Schema stores `request_fingerprint`; no production execution re-check exists. |
| Money value object | IMPLEMENTED AND TESTED | Integer minor units and currency-safe arithmetic. |
| Idempotency | IMPLEMENTED AND TESTED | Organization-scoped store and HTTP conventions. |
| Optimistic concurrency | IMPLEMENTED AND TESTED | Version/`If-Match` foundations and repository guards. |
| Audit | IMPLEMENTED AND TESTED | Append-only records and redaction. |
| Transactional outbox | PARTIAL | Persistence/worker logic exists; production transport is disabled. |
| Consumer inbox | PARTIAL | Dedupe/mismatch foundation exists; no dispatch consumer. |
| Durable jobs/leasing/recovery | IMPLEMENTED AND TESTED | Claim, heartbeat/recovery, retry, dead-letter foundations. |
| Dead-letter replay | PARTIAL | Job/execution primitives exist; no dispatch-event replay operation. |
| Source reference | IMPLEMENTED AND TESTED | Opaque source IDs and validation. |
| Projection freshness | PARTIAL | Model/repositories exist without complete event ingestion. |
| Media references | IMPLEMENTED | Opaque references only; no fleet binary storage. |
| Notification intents | MISSING | Communications is an 8-line placeholder; external contracts are unconfigured. |
| Marketplace assignments | IMPLEMENTED AND TESTED | Real production routes/use cases/repositories; focused DB tests pass. |
| OpenAPI | MISSING | No OpenAPI/Swagger artifact found. |
| Error envelopes | IMPLEMENTED | Central `platform/respond` convention. |
| Feature flags | IMPLEMENTED | Namespaced organization flags plus false-by-default dispatch execution flag. |

Existing wallet links are projections and workflow intentions, not a working
wallet integration. `platformclients.WalletContract` is only an interface and
the composition defaults to `UnconfiguredWallet`; no real payment-wallet HTTP
adapter is wired.

CRM/private-operation search result: customers, contacts, locations,
opportunities, quotations, contracts, SLAs, credit profiles, service catalogue,
private orders/stops, shipments, packages/scans, cargo, manifests, recurring
orders, bulk imports, and private execution operations are absent. Their fleet
packages are 8-line placeholders. This is acceptable as greenfield scope, but
none counts as implemented.

Fleet foundation result: **REUSABLE BUT INCOMPLETE**.

## 5. Payment-wallet-service findings

| Capability | Classification |
| --- | --- |
| Organization master wallet accounts | ABSENT |
| One wallet per organization/currency | PARTIAL - generic uniqueness could support it, but no organization owner type or contract exists |
| Provider sub-wallet accounts | ABSENT |
| Organization-provider wallet relationship | ABSENT |
| Double-entry ledger enforcement | PARTIAL - application validation exists; database does not enforce balanced journals |
| Journal entries | PARTIAL - generic ledger transactions/entries only |
| Transaction records | IMPLEMENTED BUT UNTESTED - repository/use-case packages have no tests |
| Available balance | IMPLEMENTED BUT UNTESTED |
| Pending balance | IMPLEMENTED BUT UNTESTED |
| Reserved balance | PARTIAL - job escrow exists; no organization/provider reserve contract |
| Provider earnings | IMPLEMENTED BUT UNTESTED |
| Organization deposits/settlement receipts | ABSENT |
| Provider allocations | ABSENT |
| Approved deductions | ABSENT |
| Reversals | PARTIAL - refund/withdrawal statuses exist, no required organization contract |
| Provider withdrawals | IMPLEMENTED BUT UNTESTED |
| Company-controlled withdrawal approval | ABSENT |
| Bank-account references without raw exposure | PARTIAL - bank records exist, but API models expose `account_number`; no opaque fleet-facing reference contract |
| Idempotent financial writes | PARTIAL - keys/indexes exist; zero matching idempotency tests |
| Optimistic concurrency | ABSENT |
| Unknown-outcome read-back | ABSENT |
| Reconciliation | ABSENT |
| Transactional outbox | ABSENT |
| Wallet events | ABSENT |
| Authenticated internal HTTP | IMPLEMENTED AND TESTED for existing payment/refund routes |
| Caller allowlisting | PARTIAL - HMAC secret map exists; default map does not include `fleet-partner-service` |
| Migration replay | ABSENT - only a static SQL substring test |
| Real PostgreSQL test harness | ABSENT |

Required contract status:

| Contract | Status |
| --- | --- |
| `EnsureOrganizationWallet` | ABSENT in wallet service; fleet has an unwired interface only |
| `EnsureProviderSubWallet` | ABSENT in wallet service; fleet has an unwired interface only |
| `CreateProviderAllocation` | ABSENT |
| `RequestProviderWithdrawal` | PARTIAL - provider self-request exists, not organization/sub-wallet semantics |
| `ApproveProviderWithdrawal` | ABSENT |
| `RejectProviderWithdrawal` | ABSENT |
| `ReadOrganizationWallet` | ABSENT |
| `ReadProviderStatement` | ABSENT |
| `ReadOperationByIdempotencyKey` | ABSENT |

Section 13 must first add wallet-owned organization/sub-wallet schemas and
relationships, company approval policy, allocation/deduction/reversal journals,
opaque beneficiary references, all nine authenticated/read-back contracts,
transactional outbox/events, optimistic versions, reconciliation, and a real
PostgreSQL migration/ledger/idempotency test harness. No fleet-local ledger is
needed or permitted.

Wallet readiness result: **NOT READY**.

## 6. Dispatch findings

| Capability required by private delivery | Status |
| --- | --- |
| Private request creation | MISSING - only existing customer request creation and organization reservation/read APIs exist |
| Request read-back | VERIFIED |
| Provider validation | VERIFIED |
| Vehicle validation | VERIFIED |
| Managed-provider assignment binding | VERIFIED |
| Provider Accept/Decline | VERIFIED |
| Canonical trip creation | VERIFIED |
| Trip read-back | VERIFIED |
| Private internal cancellation | MISSING - fleet client declares it, but `RegisterInternalRoutes` mounts no cancel route |
| Trip lifecycle events | PARTIAL - existing Redis events are not the durable marketplace outbox path |
| Proof completion events | PARTIAL - event model/publisher exists, not durable fleet integration |
| Organization/settlement metadata | VERIFIED on canonical trip creation/read |
| Idempotency/unknown-outcome | VERIFIED for reservation/execution creation |

Dispatch result: **PARTIAL**. The acceptance/canonical-trip path is real; the
private request, cancellation, and durable lifecycle integration needed by
Sections 14-15 is not.

## 7. Taxi status

`driver-taxi-service` is a scaffold: configuration, service shell, and README
placeholders only. Taxi private execution must remain disabled. No route may be
fabricated; only typed fail-closed contracts and documentation are safe until
the owning service is implemented.

Taxi result: **DISABLED / NOT IMPLEMENTED**.

## 8. Hauling status

Hauling has real customer/provider booking, matching, provider lifecycle,
payment, cancellation, and proof code with tests. It does not expose the
authenticated organization-private reservation/execution/read-back contracts,
unknown-outcome handling, durable integration events, or canonical private
execution metadata required here.

Hauling private execution must remain disabled and fail closed.

Hauling result: **SERVICE IMPLEMENTED; PRIVATE CONTRACT ABSENT**.

## 9. Supporting-service findings

| Service | Finding |
| --- | --- |
| Media-file | Upload/list/get/delete/download and owner metadata exist. Malware/scan status, import-file workflow, generated error reports, and a safe private download authorization contract are missing. `routes.go:22` mounts download outside the protected group. |
| Notification | Authenticated send intent, idempotency, push/email/realtime/in-app channels, delivery status, retries, attempts, and dead-letter queue exist. Fleet-specific intents/templates are not wired. |
| Verification-compliance | Scaffold only; no beneficiary/bank verification or cargo compliance decision. Required paths must fail closed. |
| Support-dispute | Real complaint/dispute/evidence/audit capability exists; no private-order reconciliation-case integration is wired. |
| Analytics | Scaffold only; no event/metric ingestion contract for these sections. |

## 10. Microservice-boundary findings

No prohibited production boundary violation was found:

- Fleet does not query wallet or dispatch databases.
- Fleet has no local ledger or authoritative balance.
- Canonical trips are created only by dispatch through authenticated HTTP.
- CRM placeholders are not used for provider authentication.
- Fleet stores opaque wallet/file/source references, not raw bank details.
- Unknown execution writes are read back rather than blindly retried.
- Taxi and hauling private adapters fail closed.
- No production direct import of another service's repository was found.
- No shared writable schema, cross-service foreign key, or cross-service
  database transaction was found.

Direct dispatch SQL in
`internal/integration/acceptance_to_canonical_trip_e2e_test.go` is test-only
verification and is not a production boundary violation.

Architecture-boundary result: **PASS**, with missing contracts rather than an
implemented cross-service violation.

## 11. Migration status

| Service | Latest | Pairing/duplicates | Executed audit result |
| --- | --- | --- | --- |
| Fleet | `000011` | No duplicate numbers; every up has a down | Focused rollback/reapply test exists, but the full real-DB suite exceeded the 5-minute audit limit; sequence is entirely untracked |
| Wallet | `001` | No duplicate; no down migration | Only static SQL-content test; no fresh apply, no-op reapply, rollback, or reapply harness |
| Dispatch | `000033` plus legacy `001`-`003` naming | No duplicate files within either naming sequence; up/down pairs present | Migration database tests ran, but the full real-DB service suite failed with a missing `trips.package_type` column |

Tracked migrations were not modified. Fleet and dispatch `000031`-`000033`
migrations are new/untracked, so applied-migration rewrite safety cannot be
established from Git history. Each service has a distinct database URL and all
foreign keys are service-local.

## 12. Test and build results

| Command | Result |
| --- | --- |
| `go vet ./services/fleet-partner-service/...` | PASS |
| `go vet ./services/payment-wallet-service/...` | PASS |
| `go vet ./services/driver-dispatch-delivery-service/...` | PASS |
| `go build ./services/fleet-partner-service/...` | PASS |
| `go build ./services/payment-wallet-service/...` | PASS |
| `go build ./services/driver-dispatch-delivery-service/...` | PASS |
| `go test -count=1 ./services/fleet-partner-service/...` without DB env | PASS, but DB integration tests skipped |
| Same fleet command with real DBs | INCOMPLETE - exceeded 5-minute audit timeout |
| `go test -count=1 ./services/payment-wallet-service/...` | PASS; repository/use-case packages have no tests |
| `go test -count=1 ./services/driver-dispatch-delivery-service/...` without DB env | PASS, but DB integration tests skipped |
| Same dispatch command with real DB | **FAIL** - `TestPostgresRepositoryTripFoundation`: `trips.package_type` missing |

Focused-test accounting:

| Focus | Matches | Executed result |
| --- | ---: | --- |
| Acceptance to one canonical trip | 1 | PASS against real fleet+dispatch PostgreSQL |
| Unknown execution read-back/key preservation | 1 plus E2E scenario | PASS |
| Cancellation in `EXECUTION_START_PENDING` | 1 | PASS |
| Provider Accept/Decline | 9 selected focused tests | PASS |
| Provider data minimization | 2 | PASS |
| Wallet ledger balance/schema | 2 | PASS, but application/static-SQL tests only |
| Wallet idempotency | 0 | NOT TESTED; zero-match PASS rejected |
| Wallet unknown-outcome reconciliation | 0 | NOT TESTED; zero-match PASS rejected |

## 13. Security risks

1. Media download is mounted outside the protected route group and performs no
   caller/owner authorization suitable for private documents.
2. Wallet bank-account DTOs include raw account numbers; no opaque,
   organization-scoped beneficiary reference contract exists.
3. Approval self-approval/fingerprint enforcement is schema-only.
4. Dispatch-to-fleet event authentication cannot be evaluated because no
   ingestion route exists.
5. Default wallet service caller secrets do not allow fleet, and the required
   fleet contracts do not exist.
6. Untracked production security/auth code has no immutable review baseline.

## 14. Compatibility risks

1. The database-backed dispatch suite fails on a schema/code mismatch for
   `trips.package_type`.
2. New trip/marketplace migrations and composition changes are untracked.
3. Existing customer/independent-provider compatibility is therefore only
   partially established.
4. Event consumers cannot keep private projections current after completion,
   proof, cancellation, expiry, or reassignment.
5. Wallet defaults currency to NGN; required organization contracts need
   explicit currency semantics.

## 15. Missing contracts

- All nine required wallet contracts listed in section 5.
- Dispatch private request creation.
- Dispatch internal private trip cancellation.
- Durable request/trip lifecycle event delivery.
- Authenticated fleet event ingestion and consumer wiring.
- Projection reconciliation orchestration.
- Media scan/import/error-report and private download authorization.
- Beneficiary and cargo compliance decisions.
- Taxi and hauling private execution contracts.
- CRM/private-order/shipments/packages/manifests/recurrence/import contracts.

## 16. Reusable components

- Fleet tenant/auth/permissions/role foundations.
- Organization and managed-provider lifecycle.
- Money, idempotency, optimistic concurrency, audit, job, lease, outbox/inbox
  repositories, source-reference and freshness types.
- Provider assignment, Accept/Decline, execution-start worker, and real
  dispatch adapter.
- Dispatch reservation/execution/read-back contracts and canonical-trip
  guarantee.
- Wallet generic account/ledger schema and balanced-entry validator, after
  hardening and contract expansion.
- Notification intent and retry infrastructure.
- Support/dispute core.

## 17. Required feature flags

All new write flags must default false:

- `fleet.wallet.organization.enabled`
- `fleet.wallet.provider_subwallet.enabled`
- `fleet.wallet.provider_allocation.enabled`
- `fleet.wallet.provider_withdrawal.enabled`
- `fleet.crm.enabled`
- `fleet.quotations.enabled`
- `fleet.contracts.enabled`
- `fleet.private_orders.enabled`
- `fleet.private_dispatch_execution.enabled`
- `fleet.private_taxi_execution.enabled`
- `fleet.private_hauling_execution.enabled`
- `fleet.package_scanning.enabled`
- `fleet.manifests.enabled`
- `fleet.recurring_orders.enabled`
- `fleet.bulk_imports.enabled`

Dispatch private execution cannot be enabled until cancellation, events,
projections, and the full database suite pass. Taxi and hauling flags remain
false.

## 18. Deferred capabilities

Taxi and hauling private execution, advanced route optimization, workforce,
maintenance, fuel, payroll, tax/general ledger, procurement, broad warehouse
management, automated marketplace pricing, subscriptions, and any capability
not required by Sections 13-15 remain deferred.

## 19. Recommended Sections 13-15 implementation order

1. **Prerequisite stabilization package (must precede Section 13):** place
   Sections 11-12 on a clean reviewable baseline; fix the dispatch DB harness
   schema mismatch; make full fleet/dispatch DB suites finish and pass.
2. Complete durable dispatch integration: lifecycle outbox writes, real outbox
   delivery worker, authenticated fleet ingestion, inbox processing,
   source-version/gap handling, reconciliation, and private cancellation.
3. Implement wallet-owned Section 13 primitives/contracts/events and its real
   PostgreSQL harness; then wire a real fleet HTTP adapter.
4. Implement and enforce fleet approval workflows for allocations,
   deductions, and withdrawals.
5. Build CRM/service-catalogue foundations behind false flags.
6. Build delivery-only private orders, then shipments/packages/proof,
   recurrence/imports; enable only after durable execution and supporting
   service contracts pass. Keep taxi/hauling disabled.

CRM-only schema work should not begin in this dirty tree. Once steps 1-2 have a
clean baseline, CRM can proceed in parallel only if it does not claim wallet or
private execution readiness.

## 20. Final readiness decision

**NO-GO**

The acceptance-to-one-canonical-trip guarantee is verified, and the service
boundaries are sound. That is insufficient for Sections 13-15 because required
database tests are not green, the repository baseline is unsafe, durable event
delivery/ingestion is missing, private cancellation is missing, and wallet
Section 13 contracts are absent and untested.

## 21. Exact blockers

1. Entire fleet service and most Sections 11-12 work are untracked; 246
   untracked files and 5 tracked modifications make the baseline unsafe.
2. Real-database dispatch full test fails: `trips.package_type` is missing in
   `TestPostgresRepositoryTripFoundation`.
3. Real-database fleet full suite did not complete within five minutes, so the
   required full result is not accepted.
4. Dispatch marketplace outbox has no production delivery worker.
5. Fleet has no authenticated dispatch event-ingestion route/consumer; generic
   inbox and projections are unreachable from source events.
6. Durable request/trip lifecycle, proof, cancellation, expiry, and
   reassignment event integration is incomplete.
7. Dispatch has no mounted internal private trip-cancellation route.
8. Wallet lacks organization wallets, provider sub-wallets/relationships,
   allocations, company withdrawal approval/rejection, statements/read-back,
   reconciliation, outbox/events, optimistic concurrency, and fleet caller
   contracts.
9. Wallet idempotency and unknown-outcome focused tests match zero tests; wallet
   has no real PostgreSQL harness or down/replay verification.
10. Approval policy, self-approval, and fingerprint execution are schema-only.
11. Media private download authorization/scan/import/error-report contracts and
    compliance decisions are missing.
12. Taxi is a scaffold and hauling lacks private execution contracts; both must
    remain disabled.

## 22. 2026-07-28 prerequisite remediation update

This historical audit predates the production-contract remediation. Wallet
organization operations, Media private authority, signed Fleet/Hauling
execution, event ingestion, reconciliation and Hauling cancellation/read-back
now have production implementations. Hauling payment-required paths fail
closed when Wallet is absent; ambiguous holds and settlements use authoritative
read-back and remain `UNKNOWN` when unresolved. Neither Fleet nor Hauling owns
a balance or ledger.

No CRM, allocation, withdrawal, quotation, contract, SLA or customer-credit
scope was added by this remediation. Section 13/14 readiness remains governed
by the current final verification matrix, not by the obsolete NO-GO list above.

The current final matrix is recorded in `prerequisite-stabilization-report.md`.
In particular, Wallet remains the sole ledger/balance authority, Fleet and
Hauling store only opaque references and projections, and unresolved financial
writes remain `UNKNOWN`. The historical blockers in sections 20-21 above are
retained as audit history and must not be read as the current wiring state.

---

# Sections 13-14 implementation audit (2026-07-29)

This section supersedes the 2026-07-26 NO-GO decision above for Sections 13-14.
Everything before this line is retained as audit history.

Decision: **Section 13 GO. Section 14 PARTIAL - schema and catalogue
delivered, service layer deferred. Section 15 deferred by design.**

> **Superseded for Section 14 by the 2026-07-29 service-layer audit at the end
> of this document.** The Section 14 resume point recorded in section 5 below
> has been completed; it is retained as audit history rather than deleted.

## 1. Repository baseline

| Field | Value |
| --- | --- |
| Branch | `staging` |
| Starting HEAD | `7e8da87afea7e2fa301b22cd6b222cf31d2538f4` |
| Modified files at start | 121 |
| Untracked files at start | 138 |
| Staged files at start | 0 |
| `git diff --check` | clean |

No branch was created; nothing was pulled, merged, pushed, reset, cleaned,
restored, staged or committed. All pre-existing modified and untracked files
were preserved. The backup at `C:\KarryGoPush-backup-20260726-213208` was not
touched.

## 2. Focused prerequisite check

Scoped to what Sections 13-14 depend on, not repeated across WP1-WP14.

| Foundation | Result |
| --- | --- |
| Fleet migrations 000001-000021 | Apply cleanly on a fresh database |
| Generic finance operation framework | Present and reusable (`internal/features/finance`) |
| Finance projections and reconciliation | Present, with lease recovery and read-back |
| WP9 approval framework | Present; reused through a narrow port |
| Durable eligibility gate | Present; reused through a narrow port |
| Organization wallet link | Present as `fleet_partner_organization_wallet_provisioning` |
| Provider sub-wallet link | Present as `fleet_partner_provider_subwallet_links` |
| Wallet HMAC client, allowlist, nonce replay | Present |
| Media and Notification clients | Present |
| Taxi | Disabled, fail-closed, unchanged |
| Hauling | Production-backed, unchanged |

One defect was found and fixed **in this work package's own new code**, not in
the existing foundations: the allocation duplicate-purpose check ran before the
idempotency replay lookup, so an identical retry was rejected as a duplicate
instead of replaying. Caught by
`TestAllocationIsIdempotentAndFingerprintChecked`.

One test-harness gap was fixed: the Wallet integration harness restores the
seeded permission catalogue from a hardcoded migration list that did not
include the new migration. Adding `000007` to that list is the minimum fix.

## 3. Reuse decisions

**The existing earning account IS the provider sub-wallet.**
`finance_earning_accounts` already carried organization ownership, opaque
provider identity, currency, status, ledger link and version. Migration
`000007_provider_subwallet_operations` additively adds
`organization_wallet_id`, `provider_id`, `organization_membership_id` and
`provider_service_category`, with a partial unique index enforcing one active
account per membership, service category and currency. A second provider
account model was **not** created: two models would be two answers to "how much
is owed to this provider", and the balance would follow whichever one a given
code path read.

**The generic finance-operation framework was not duplicated.** Section 13 has
its own instruction and withdrawal tables because they carry business semantics
the generic operation record does not (allocation type, business reference,
reason code, provider-visible explanation, policy mode, approval binding). They
reuse the same *patterns* - idempotency key, request fingerprint, expected
version, status machine, append-only history, outbox - rather than a second
framework. `fleet_partner_finance_operations` is untouched and still in use.

**No second approval or eligibility framework.** Both are reached through
single-method ports (`ApprovalPort`, `EligibilityPort`) implemented by adapters
in the composition root over the existing WP9 services.

## 4. Section 13 delivered

payment-wallet-service, migration `000007`:

- Sub-wallet corporate ownership metadata on `finance_earning_accounts`
- `finance_provider_allocations` with business-purpose and single-reversal
  unique indexes
- `finance_provider_withdrawals` with lease, retry, dead-letter and an
  open-withdrawal unique index
- Journal types `PROVIDER_ALLOCATION`, `PROVIDER_DEDUCTION`,
  `PROVIDER_ALLOCATION_REVERSAL`, `PROVIDER_WITHDRAWAL_*`
- Five permissions and a versioned `PROVIDER_WITHDRAWAL_APPROVAL` policy
- Nine internal HMAC routes under the existing allowlist, nonce and replay
  protection
- A payout execution worker with read-back on ambiguous outcomes

fleet-partner-service, migration `000022`:

- Extended wallet-link and sub-wallet-link state vocabularies (spec 9.2)
- `fleet_partner_organization_withdrawal_policies`
- `fleet_partner_provider_allocation_instructions` + append-only history
- `fleet_partner_provider_withdrawal_requests` + append-only history
- `fleet_partner_wallet_transaction_projections`
- Nine `wallet.*` permissions wired into the system role templates
- A full vertical slice: models, repositories, use cases, HTTP handlers,
  workers, production composition-root wiring, feature flags, kill switch

Section 13 completion gates 1-22 are met. The settlement-separation gates are
proven by assertions on the postings, not on naming.

## 5. Section 14: delivered and deferred

**Delivered and verified:**

- Migration `000023_company_crm` creating all 18 Section 14 tables with the
  required constraints: organization-scoped uniqueness, immutable issued
  quotation versions, immutable acceptances, one conversion intent per accepted
  version, immutable activated contract versions, append-only SLA observations
  and credit history, one primary contact per purpose, timezone and coordinate
  validation, basis-point probability, integer-minor money, decimal quantity
- A 24-permission `crm.*` catalogue including a separate `crm.sensitive.read`
  grant, so ordinary CRM read does not carry PII
- All 30 Section 14 stable error codes
- All 17 Section 14 feature flags, defaulting false
- Fresh apply, rollback and re-apply verified against real PostgreSQL

**Not delivered - the resume point:**

The Section 14 Go service layer does not exist. There is no
`internal/features/crm` package: no models, repositories, use cases, HTTP
handlers, workers or wiring. Therefore no CRM route is mounted, no CRM event is
produced, no CRM worker runs, the `crm.*` permissions are granted but not yet
enforced anywhere, and the `CRM_*` flags are read into config but have no
consumer. This is stated plainly rather than presented as gated-off
functionality.

The schema is additive and fully reversible and nothing in the running service
reads or writes those tables, so the checkpoint is green.

To resume, in order:

1. `internal/features/crm/models/models.go` - mirror the enums in
   `migrations/000023_company_crm.up.sql`
2. `internal/features/crm/repositories/` - customers, contacts, locations,
   opportunities, quotations, contracts, slas, credit, notifications; follow
   the organization-scoped SQL and cursor pattern in
   `internal/features/companywallet/repositories/repository.go`
3. `internal/features/crm/usecases/` - quotation submit/issue/accept is the
   hard part; acceptance must create exactly one conversion intent and no order
4. `internal/features/crm/http/handlers.go` - follow
   `internal/features/companywallet/http/handlers.go`
5. Wire in `internal/app/wiring.go` beside `companyWalletHandler`
6. Tests in `internal/integration/crm_test.go`, following
   `internal/integration/companywallet_test.go`
7. Docs listed in section 45 of the specification

## 6. Security boundaries

- Fleet stores no bank details, only an opaque destination reference, excluded
  from list responses and from every event
- Fleet stores no Wallet secret or provider credential
- Provider statements are built field by field, never pruned; asserted on
  serialized bytes
- Provider routes resolve the membership from the provider's own session, never
  from request input; corporate staff cannot impersonate a provider
- Every repository query carries `organization_id` in SQL; branch scope applies
  in SQL where relevant
- Cross-tenant identifiers return the same response as missing ones
- No organization, provider, customer, quotation or contract ID is a metric
  label

## 7. Migration decisions

- Fleet continues at `000022`, `000023`; Wallet at `000007`
- No applied migration was rewritten
- All three verified: fresh apply, rollback, re-apply
- Down migrations normalize rows into the older status vocabulary before
  restoring the narrower CHECK constraint, so a rollback cannot fail on data
  the newer schema allowed
- No duplicate migration numbers

## 8. Explicit deferrals

- **Section 15 is deferred.** No service catalogue, private order, order stop,
  shipment, package, package scan, cargo item, manifest, recurring order, bulk
  import, order planning, route planning, warehouse operation or fulfilment
  table, route, event, worker or permission was created.
  `TestNoSection15TablesExist` asserts this against the live schema.
- Quotation acceptance creates a **conversion intent**, not an order. It mints
  no order ID and creates no shipment, package, assignment or canonical trip,
  and moves no money.
- Pricing remains manually supplied: no rate card, no formula, no marketplace
  pricing, no commission calculation, no tax calculation.
- Fleet owns no financial ledger and no accounts receivable.
- CRM exports are deferred: no reusable asynchronous export framework exists in
  this service, and the specification directs documenting the deferral rather
  than creating an insecure synchronous endpoint.
- **Taxi remains disabled** and fail-closed; startup rejects the enabling flag.
- **Hauling remains production-backed** and was not modified.


---

# Section 14 service-layer implementation audit (2026-07-29)

This section supersedes the Section 14 PARTIAL verdict above. Everything before
this line is retained as audit history.

Decision: **Section 14 GO. Section 15 remains deferred by design.**

## 1. Repository baseline

| | |
| --- | --- |
| Repository | `C:\KarryGoPush` |
| Branch | `staging` |
| Starting HEAD | `7e8da87afea7e2fa301b22cd6b222cf31d2538f4` |
| Modified entries at start | 121 |
| Untracked entries at start | 151 |
| Staged entries at start | 0 |
| `git diff --check` | clean — CRLF advisories only, no whitespace errors |

Nothing was staged, committed, pushed, reset, cleaned or restored. No branch was
created. Migration `000023` was not rewritten. The external backup at
`C:\KarryGoPush-backup-20260726-213208` is untouched.

## 2. Confirmed starting state

The audit in section 5 above was accurate. `internal/features/crm` did not
exist: no models, repositories, use cases, handlers, workers or wiring. No CRM
route was mounted, no CRM event was produced, no CRM worker ran, the 24 `crm.*`
permissions were granted but enforced nowhere, and the `CRM_*` flags were read
into config with no consumer.

Migration `000023_company_crm` was applied and correct. Its 19 tables, their
organization-scoped uniqueness, immutability triggers, coordinate and timezone
checks, basis-point probability and integer-minor money were verified against
the live schema and reused unchanged.

## 3. Reused foundations — nothing was rebuilt

| Capability | Reused from |
| --- | --- |
| Approvals, quorum, separation of duties, fingerprint binding | WP9 `features/approvals` |
| Transactional outbox | `internal/events` |
| Audit records and redaction | `platform/audit` |
| Tenant context and branch scope | `platform/tenant`, `middleware.ResolveTenant` |
| Permission evaluation | `features/accesscontrol` |
| Cursor pagination | `platform/cursor` |
| Optimistic concurrency | `platform/concurrency` |
| Email and phone normalization | `platform/contact` |
| Rate limiting | `platform/ratelimit` |
| Media authority client | `platformclients.MediaFileClient` |
| Notification client | `shared/go/notifications` |
| HMAC v2 service auth, nonce replay, freshness | `middleware.InternalServiceAuth` |
| Error envelope and stable codes | `platform/errorcodes`, `shared/go/apperrors` |
| Transaction helper | `database.WithinTx` |

No second approval framework, outbox, audit trail, idempotency store or rate
limiter was created.

## 4. Additive migration `000024_crm_history_documents_tokens`

`000023` was not modified. A genuine gap required the next additive migration:
the specification mandates customer, contact, location, quotation and SLA
history endpoints, quotation documents and hashed customer access tokens, none
of which `000023` created.

Added:

- `fleet_partner_crm_customer_history`, `fleet_partner_crm_contact_history`,
  `fleet_partner_crm_location_history`, `fleet_partner_quotation_history`,
  `fleet_partner_sla_history` — all append-only, trigger enforced
- `fleet_partner_quotation_documents` — Media references for quotations,
  mirroring the contract document table
- `fleet_partner_quotation_access_tokens` — hashed, scoped, expiring customer
  links
- columns: opportunity `hold_reason`; quotation `internal_notes`, `issued_at`,
  `resolved_at`; contract `activated_at`; contact `consent_source`; observation
  `event_id`, `payload_hash`, `causation_id`; causation IDs on acceptances,
  conversion intents and notification operations; notification
  `dead_letter_reason`
- `idx_fleet_crm_sla_observation_source` — the unique key that makes a duplicate
  observation distinguishable from a contradicting one
- a renewal-window index so the contract sweeps stay index scans

Fresh apply, rollback and re-apply verified against real PostgreSQL. No
duplicate migration numbers. No cross-service foreign key. No local financial
ledger. No Section 15 table.

## 5. What was implemented

`internal/features/crm` — models, repositories, usecases, http — plus
`internal/app/crm_wiring.go`, route mounting in `internal/app/wiring.go` and
`internal/app/app.go`, and worker startup in both `cmd/main.go` and
`cmd/worker/main.go`.

About 60 corporate routes, 3 customer secure-link routes and 1 internal HMAC
route are mounted. Six durable workers are wired. 39 event types are produced
through the existing outbox.

Design decisions worth recording:

- **UNKNOWN is never optimism.** An unverified Media document blocks a quotation
  issue and a contract activation exactly as an unsafe one does. An SLA outcome
  that could not be determined is `UNKNOWN` or `INSUFFICIENT_DATA` and is never
  collapsed into `MET`. An ambiguous notification send stays retryable rather
  than being written off in either direction.
- **Media is re-read immediately before issue and activation**, outside the
  transaction. A stale local projection cannot let an unchecked file back a
  commercial commitment; `TestCRMQuotationIssueFailsClosedOnUnreadyDocument`
  asserts that hand-editing the projection to `READY` does not survive the
  refresh.
- **Data minimization is by construction.** Masked and unmasked views are
  distinct Go types, and every event payload is built field by field. A model
  that grows a tax reference cannot start publishing it.
- **The customer token reuses the session secret** rather than introducing a new
  mandatory secret whose absence would break startup for deployments that do not
  use CRM.
- **Acceptance replay survives token revocation.** Accepting revokes the link;
  the accept and reject paths still resolve a revoked token when the quotation is
  terminal, so a customer whose response was lost in transit gets their replay
  rather than a broken link. This only ever reaches an idempotent replay, because
  every write path refuses a terminal quotation.
- **Approval is asked, never assumed.** An empty approval ID means the policy
  declined to raise a request; it is never read as "approved".

## 6. Defects found and fixed during verification

Six were found by the integration suite. Four were production defects:

1. `NormalizePair` required both an email and a phone — right for an
   authentication identity, wrong for a CRM contact. Each half is now normalized
   independently.
2. `numeric(18,6)` rendered `2.5` as `2.500000`, so a quantity did not
   round-trip and would not fingerprint stably. Now `trim_scale`d on read.
3. A unique violation on observation insert aborted the caller's transaction, so
   the duplicate-versus-conflict read failed with `25P02`. Now
   `ON CONFLICT DO NOTHING`, which leaves the transaction usable.
4. Acceptance replay was blocked by the token revocation it had just caused.

Two were test defects: an assertion that expected a stale `READY` projection to
survive the pre-issue Media refresh (it must not), and a helper that acquired
the suite's per-test database lock twice and deadlocked.

## 7. Explicit Section 15 deferral

No service catalogue, private order, order stop, shipment, package, package
scan, cargo item, manifest, recurring order, bulk import, route planning,
warehouse or fulfilment table, route, event, worker or permission was created.

Quotation acceptance creates a **conversion intent** with
`conversion_type = FUTURE_PRIVATE_ORDER` and `status = PENDING_SECTION_15`. It
mints no order, shipment, package, assignment, reservation or canonical trip,
and moves no money. `assertNoSection15Artifacts` asserts this against the live
schema on every acceptance test.

Also deferred: **CRM exports**. `crm.exports.create` is seeded, but no reusable
asynchronous, Media-backed export framework exists in this service, and the
specification directs documenting the deferral rather than creating an insecure
synchronous endpoint.

## 8. Unchanged

Section 13 is unchanged. WP1–WP14 are unchanged. Taxi remains disabled and
fail-closed. Hauling remains production-backed and was not modified. No change
was made to Dispatch, Wallet, Media or Verification.

## 9. Section 14 machine-readable contract

The Section 14 CRM API now has a service-owned OpenAPI 3.1 contract at
[openapi/section14-crm.openapi.json](openapi/section14-crm.openapi.json). It is
limited to Section 14: Section 13 is documented separately and Section 15 is
still deferred. In particular, acceptance documents only a
`FUTURE_PRIVATE_ORDER` / `PENDING_SECTION_15` conversion intent and no order or
Wallet operation.

From the repository root, `go test -count=1 -v
./services/fleet-partner-service/internal/features/crm/http -run OpenAPI`
validates JSON structure and references and checks the mounted production
routes against the specification in both directions, including permissions,
security class, feature flags, idempotency, expected versions, minimized public
schemas, and Section 15 absence. The JSON can be opened in an OpenAPI
3.1-compatible editor or rendered temporarily with `npx @redocly/cli
preview-docs services/fleet-partner-service/docs/openapi/section14-crm.openapi.json`;
no public documentation server or viewer asset is added to Fleet.
