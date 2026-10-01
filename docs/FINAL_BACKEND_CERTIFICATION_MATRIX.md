# ART-006 — Backend certification matrix

Phase baseline `9ebad225ea9ec25397778a4dc8d3ff842f2938a3`.

**Phase update.** Re-run at this baseline. The CERT rows are unchanged in
substance — no scenario gained deployed evidence, because there is still no
deployed environment (`FINAL_DEPLOYED_STAGING_EVIDENCE.md`). Two rows changed:
CERT-031 is now a full PASS (CMP-027 is implemented and certified DB-backed;
the previous report's claim to the contrary was wrong and is withdrawn in
`FINAL_BACKEND_RELEASE_BLOCKERS.md`), and CERT-040's corporate-context half is
now implemented. §8 records the E2E-001 … E2E-030 journeys, all of which
require deployment.
No secret value is printed here or in any certification output.
No production customer data and no real money movement were used. Every
financial assertion ran against throwaway PostgreSQL databases created and
dropped inside this session.

## 0. How to read the evidence labels

The section forbids calling component coverage a live end-to-end run, so the
labels are used strictly:

| Label | Means |
|---|---|
| `PASS — UNIT` | Pure logic, no database, no process. |
| `PASS — COMPONENT` | Real usecase/handler wiring with fakes at the process boundary. |
| `PASS — DB-BACKED` | Real PostgreSQL (and Redis where the path needs it). Transactions, constraints, triggers and concurrency are real. |
| `PASS — LIVE LOCAL` | A real compiled service process, booted and driven over HTTP on this machine. |
| `PASS — DEPLOYED STAGING` | Driven against a deployed staging environment. |
| `NOT RUN` | Not executed at the level the row names. |
| `FAIL` | Executed and failed. |

**There is no `PASS — DEPLOYED STAGING` row anywhere in this document.** This
certification run has no deployed environment. Where a scenario's own text
demands staging ("use fresh staging capability"), the row says so and reports
the strongest level actually reached.

A row can carry two labels. `PASS — DB-BACKED` on the money and
`NOT RUN` on the deployed variant is a common and honest combination: the
behaviour is proven, the deployment is not.

## 1. Provider personas

| Persona | Level | Evidence |
|---|---|---|
| A — Independent | `PASS — DB-BACKED` | `TestLegacyCompleteJobStillWorksForIndependentProviders` and `TestIndependentCommerceDeliveryStillPaysTheProviderInFull` post the legacy journal byte-for-byte unchanged. `TestCompletedWorkPayloadFor_IndependentProducesNothing` proves no corporate record is manufactured for an independent provider. |
| B — Corporate salaried | `PASS — DB-BACKED` | `TestSalariedProviderReceivesNoPerJobCredit`, `TestSalariedCommerceRiderReceivesNothingPerDelivery`, `TestASalariedPlanGivesTheWholeJobToTheOrganization`. A zero provider share writes **no provider ledger line at all**, rather than a zero-amount one. |
| C — Corporate commission | `PASS — DB-BACKED` | `TestManagedSettlementPostsOneBalancedJournalForACommissionProvider`, `TestCommissionCommerceRiderGetsOnlyTheAuthorisedShare`, `TestImmediateCommissionIsPaidWithTheJobAndExcludedFromPayroll`. |
| D — Corporate per-job | `PASS — DB-BACKED` | `TestPerJobProviderReceivesExactlyTheConfiguredAmount` plus the exactly-once set in §3 (CERT-010). |
| E — Suspended / expired | `PASS — UNIT` + `PASS — COMPONENT` | `TestSuspendedExpiredAndTerminatedAreRefusedAndDistinct`, `TestOnlyActiveTakesNewManagedWork`, `TestUnrecognisedMembershipStatusAuthorizesNothing`. Placement of the gate on acquisition paths only is pinned by `TestCorporateGateStaysOnAcquisitionPathsOnly`. |
| F — Mid-period compensation change | `PASS — DB-BACKED` | `TestWorkBeforeAndAfterAnEffectiveDateUsesDifferentRules` — work completed before the effective date resolves on the old plan version, later work on the new one, in one run. |

## 2. Persona × service coverage

The personas are certified at the layer that owns the decision, not once per
service, because the decision is not per-service: compensation resolution is
Fleet's and settlement posting is Wallet's, and both are shared by all three
execution services through one contract.

| | Taxi | Dispatch | Hauling |
|---|---|---|---|
| Corporate context read | `PASS — COMPONENT` | `PASS — COMPONENT` | `PASS — COMPONENT` + `PASS — LIVE LOCAL` (boot) |
| Completed-work emission | `PASS — DB-BACKED` | `PASS — DB-BACKED` | `PASS — DB-BACKED` |
| Managed settlement routing | `PASS — COMPONENT` | `PASS — COMPONENT` | `PASS — COMPONENT` |
| Independent path unchanged | `PASS — DB-BACKED` | `PASS — DB-BACKED` | `PASS — DB-BACKED` |
| Full offer→settle over HTTP | `NOT RUN` | `NOT RUN` | `NOT RUN` |

The last row is the honest cost of having no deployed environment, and it is
carried into ART-008 as the single largest outstanding item.

## 3. CERT-001 … CERT-040

| ID | Level | Evidence and caveats |
|---|---|---|
| CERT-001 centralized accept | `PASS — DB-BACKED` per stage; `NOT RUN` as one continuous run | Every stage is individually certified against real PostgreSQL — assignment CAS, accept, execution transitions, completion, completed-work enqueue (`TestEnqueueIsIdempotentPerCompletion`), resolution, instruction, journal (`TestManagedSettlementPostsOneBalancedJournalForACommissionProvider`). What is **not** certified is one physical provider walking the whole chain over HTTP in one process tree. Component boundaries are fakes at exactly the points a deployed run would exercise. |
| CERT-002 centralized decline | `PASS — DB-BACKED` | `TestOfferDeclineValidatesReasonAndFlagsSafety`, `TestDeclineReasonCatalogueAndSafetyClassification`; the decline does not set accepted state and leaves the assignment reassignable (`TestRecovery_ResetsOrphanedAwaitingAcceptance`, `TestReplacementCancelledOrExpiredNeverCreatesOfferPostgres`). |
| CERT-003 delegated offer | `PASS — DB-BACKED` | `TestPrivateExecutionPostgres_IdempotencyReadbackMetadataAndOutbox` covers offer → execution → readback → outbox on the delegated path; settlement joins the same contract as CERT-001. |
| CERT-004 hybrid conflict | `PASS — DB-BACKED` | `TestAssignmentAndManifestHardRaceConstraints`, `TestIncidentRecoveryClaimReplayAcceptanceAndNoSecondTripPostgres` — a provider already executing incompatible work does not acquire a second trip. Capacity policy stays with the authoritative service; no new policy was introduced here. |
| CERT-005 membership expires before accept | `PASS — UNIT` | `TestSuspendedExpiredAndTerminatedAreRefusedAndDistinct` — EXPIRED authorizes no new managed work and is distinct from SUSPENDED, so the refusal reason is not collapsed. Fails closed: `TestFleetOutageIsNotAMembershipRefusal` proves an unreachable Fleet is a 503, never an "independent" fallback. |
| CERT-006 suspension after accept, before start | `PASS — UNIT` + `PASS — COMPONENT` | `TestOnlyActiveTakesNewManagedWork` plus `TestCorporateGateStaysOnAcquisitionPathsOnly`. **Documented platform policy, restated because it looks like a gap:** a start is not an acquisition, so an accepted-but-unstarted job is not re-gated. `providercompat.AllowsActiveWorkCompletion` is the explicit carrier of that decision, and there is no silent managed→independent fallback on any path. |
| CERT-007 suspension mid-execution | `PASS — UNIT` + `PASS — DB-BACKED` | `TestSuspensionDuringWorkStillRecordsTheCompletion` and `TestMembershipChangeNeverStrandsWorkAlreadyInCustody`. Refusing the completed-work record for a mid-trip suspension would erase money the provider had already earned; the platform records the completion and blocks the next acquisition instead. |
| CERT-008 duplicate accept | `PASS — DB-BACKED` | `TestMatch_AcceptAtTimeoutBoundaryNotClobbered`, `TestBindingCommandsAreIdempotentAndRejectFingerprintReuse`, `TestPrivateExecutionPostgres_ConcurrentDuplicateCreatesOneExecution`. |
| CERT-009 duplicate decline | `PASS — DB-BACKED` | `TestGateDuplicateCallsDoNotDuplicateAuditOrEvent`; repeat declines are absorbed without a second audit row or a second event. |
| CERT-010 duplicate completion | `PASS — DB-BACKED` | The full five-link chain, each link pinned separately: one completed-work record (`TestEnqueueIsIdempotentPerCompletion`, `TestCompletedWorkIdempotencyKeyIsStable`), one resolution (Fleet posting fingerprint), one instruction (`UNIQUE (instruction_id, instruction_version)`), one journal (`TestResendingTheSameInstructionPostsOnce`, `TestConcurrentSendsOfOneInstructionPostOnce` — six concurrent sends, one journal), and a differing amount under the same identity is a conflict rather than a second posting (`TestSameIdentityWithDifferentAmountsIsAConflict`). |
| CERT-011 Fleet timeout after completion | `PASS — DB-BACKED` | `completedwork/worker_integration_test.go`; `TestSubmitCompletedWork_TransportFailureIsUnknown` proves an ambiguous transport outcome classifies UNKNOWN and is retried under the **same** idempotency key, never assumed failed. |
| CERT-012 Wallet timeout after posting | `PASS — DB-BACKED` | Fleet's `OUTCOME_UNKNOWN` path reads the settlement back rather than reposting; `TestSettlementCanBeReadBackByEveryRoute` certifies read-back by instruction, by idempotency key and by source job. `TestAManagedJobHeldWithNoSettlementIsSurfaced` and `TestAStuckJobThatLaterSettlesIsResolved` close the reconciliation loop. |
| CERT-013 restart with pending outbox | `PASS — DB-BACKED` | `TestScheduler_LeaseRecoveryAfterCrash`, `TestExpiredLeaseRecovery`, `TestReconciliationRecoversAbandonedLease`, `TestDeletionWorkerRecoversAbandonedLease`. Leases are reclaimed by expiry, so a process that dies holding one does not strand its rows. |
| CERT-014 dead letter | `PASS — DB-BACKED` + `PASS — LIVE LOCAL` | `TestBulkImports_DeadLetterReplayLeaseRecoveryAndFeatureControls`, `TestJobs_ReplayResetsErrorClassAndDeadLetteredAt`, `TestOutboxDeadLetterAndOperatorReplay`, `TestOperatorOutboxRefusesUnauthorizedCallers`. Live: Dispatch's operator surface answered 200 to an authorized inspect, 401 unsigned, 403 to a legitimate-but-unauthorized caller, 409 on a requeue that reached the handler, 400 on an invented status. Requeue preserves the event identity — it resets delivery state, it does not mint a new event. |
| CERT-015 salaried | `PASS — DB-BACKED` | Persona B above, plus `TestPayrollAccrualIsReportedButNotChargedToTheJob` — the accrual is visible on the resolution without contributing to the job's journal. |
| CERT-016 commission | `PASS — DB-BACKED` | Persona C. Basis and rounding are now resolved rather than assumed: an unrecognised basis fails resolution and an unsupported rounding policy is refused at validation. |
| CERT-017 per-job | `PASS — DB-BACKED` | Persona D + CERT-010 exactly-once + `TestImmediateCommissionIsPaidWithTheJobAndExcludedFromPayroll` for the no-payroll-duplicate half. |
| CERT-018 finance privacy | `PASS — UNIT` + `PASS — COMPONENT` | `TestOrganizationAndPlatformDataAreNeverVisible` — the organization and platform classes are unreachable under **every** policy, not merely off by default. `TestAJobViewNeverCarriesOrganizationOrPlatformEconomics`, `TestUnclassifiedMoneyFieldIsWithheld` (an unclassified money field is withheld, not guessed at), `TestManagedSettlementIsNotOnTheProviderSurface`. Closed on four alternate routes this phase: provider API, notifications, error payloads and **logs** (new — see CERT-029). All three execution services project through the same `internal/apiresponse`. |
| CERT-019 Commerce pickup | `PASS — UNIT` + `PASS — LIVE LOCAL` | `TestCommerceAndDirectPickupPointOppositeWays`, `TestVerifierIsExclusivePerPurpose`. Live: the compiled Hauling build reported `holder=PROVIDER verifier=VENDOR_SENDER` for the Commerce purpose, and no service other than Vendor could reach the Commerce pickup verification surface (403). |
| CERT-020 direct Taxi pickup | `PASS — UNIT` | `TestThreePickupDirectionsStayDistinct` — direct pickup is customer-held / provider-verified and cannot collapse into the Commerce direction. `TestNoPurposeLetsOnePartyApproveItself` closes the self-approval hole for every purpose. |
| CERT-021 delivery credential | `PASS — UNIT` + `PASS — DB-BACKED` | `TestDeliveryCredentialIsReceiverHeld`; custody `provider → receiver` pinned by `TestHappyPathIsSenderThenProviderThenReceiver` and `TestCustodyCannotReverseOutOfTheReceiver`. |
| CERT-022 Vendor cannot select a provider | `PASS — UNIT` (structural) | `TestNoVendorMissionRequestCarriesProviderSelectionAuthority` walks `logistics.MissionIntent`, `orders.MissionRequest`, `commercemission.Request` and both `dispatchcommerce` request types by reflection and fails on any field naming a provider, driver, rider, vehicle, fleet member or assignee — with a companion test proving the walker would actually catch one if added. A second test forbids Vendor importing `providercompat`/`fleetproviderwork` or addressing Fleet's completed-work route. This is exactly the "structural test should fail if provider-selection fields are added" the scenario asks for. |
| CERT-023 Taxi Goods multi-pickup refused | `PASS — UNIT`; staging variant `NOT RUN` | The declaration and the matcher refusal are certified in `shared/go/logisticscapability`. The scenario's "use fresh staging capability" cannot be honoured without staging; the previous phase's fresh **local** capability read is the strongest evidence available. |
| CERT-024 Dispatch multi-pickup | `PASS — LIVE LOCAL`; staging variant `NOT RUN` | Live read from a booted Dispatch: `maxPickups=12 maxSecurePickups=12`. Matching runtime truth, not the stale assumption. |
| CERT-025 Hauling capability truth | `PASS — LIVE LOCAL`; staging variant `NOT RUN` | Live read from a booted Hauling: `maxPickups=10`, `maxSecurePickups=1` — the old "Hauling supports one pickup" assumption is wrong and was not used. A protected three-pickup haul was refused live with `SECURE_PICKUP_LIMIT_EXCEEDED`. This gap was **found** by this work: Hauling declared 10 protected pickups while issuing one credential per booking stage, and `MaxSecurePickups` exists because of it. The pre-existing test that had pinned the over-promise as correct was corrected, with the reason recorded in the test. |
| CERT-026 credential replay | `PASS — DB-BACKED` | `custodychain` records `SingleUse` for every purpose and `TestEveryPurposeConsumesItsCredential` fails if one becomes reusable; terminal challenge states cannot be reactivated (database trigger); `TestVendorOwnedHandoverPostgresDuplicateRelayIdempotent`, `TestSecureDeliveryEvidenceGatesCompletionAndIsAppendOnly`. |
| CERT-027 wrong provider | `PASS — DB-BACKED` | The credential hash binds `ProviderID`, so provider B's submission of A's code does not hash to A's stored value. `TestProviderReassignmentBeforePickupRevokesTheOldCredential`, `TestHandoverReassignmentVersusVerificationRacePostgres`, `TestTransitionMustAgreeWithTheRecordedHolder`. |
| CERT-028 cross-service spoof | `PASS — DB-BACKED` + `PASS — LIVE LOCAL` | `TestCrossServiceCompletedWorkSpoofingIsRefused` — twelve wrong caller/source pairings refused with **zero rows written**, including Vendor and Wallet impersonating an execution service and Fleet claiming its own job. Live: Fleet refused 403 on Vendor's commerce capability, Vendor refused 403 on Dispatch's outbox, unsigned 401, provider token on an internal route 401. |
| CERT-029 Wallet read privacy | `PASS — DB-BACKED` + `PASS — UNIT` | `TestManagedSettlementIsNotOnTheProviderSurface` (the route does not exist on the provider surface at all), `TestUnrelatedCallersCannotReadManagedSettlements`, `TestEveryExecutionServiceMayReadItsOwnSettlementBack`. **New this phase:** logs were the remaining leak — `organizationShareMinor` is an ordinary name holding an ordinary integer, so neither the secret-word list nor the type check caught it. `providercompat` now registers the financial registry as a `logging` redactor; `TestOrganizationAndPlatformEconomicsAreConcealedInLogs`, `TestConcealmentReachesNestedFields`, `TestSpellingDoesNotDefeatConcealment`, and `TestOperationalFieldsAndProviderOwnFiguresStillLog` proves the rule is narrow enough not to blind operations. |
| CERT-030 compensation effective date | `PASS — DB-BACKED` | `TestWorkBeforeAndAfterAnEffectiveDateUsesDifferentRules`, `TestEffectiveDays_HonorsExtendedOverride`, `TestEffectiveDays_NeverBelowPlatformMinimum`. |
| CERT-031 payroll double-pay | `PASS — DB-BACKED` | **Corrected this phase.** The previous entry claimed the cross-check was not enforced. It is: `financeops.CalculatePayroll` reads `fleet_partner_work_settled_components` inside its own transaction and skips exactly those components at the accumulation point for `PER_JOB`, `COMMISSION` and `BONUS`; the write side records them when the settlement instruction is produced; the match is on the assignment rather than a time window, deliberately, because a job completed inside a period can settle moments after it ends. Certified by `TestCMP027_PayrollDoesNotRepayWorkSettledComponent` and `TestCMP027_RecalculationDoesNotAccumulateDuplicates`, re-run against real PostgreSQL this phase, plus the classification suite (`TestImmediateCommissionIsPaidWithTheJobAndExcludedFromPayroll`, `TestMixedPlanSplitsComponentsBetweenBothPlaces`, `TestABonusWithoutProvenanceDoesNotEnterPayroll`). |
| CERT-032 Commerce managed salary | `PASS — DB-BACKED` | `TestSalariedCommerceRiderReceivesNothingPerDelivery`, `TestManagedCommerceDeliveryCannotBePaidDirectlyToTheProvider`. `TestDeliveryWithAnUnknownExecutorIsRefusedRatherThanAssumedIndependent` closes the dangerous default. |
| CERT-033 Commerce managed commission | `PASS — DB-BACKED` | `TestCommissionCommerceRiderGetsOnlyTheAuthorisedShare`, `TestASplitCannotExceedTheFrozenAmount`, `TestTheCustomersFrozenDeliveryAmountIsUnchangedByTheExecutor`. |
| CERT-034 Commerce refund | `PASS — DB-BACKED` | `TestReversalCompensatesWithoutErasingHistory` — a reversal is a compensating journal, never a deletion or an update of the original. `TestIndependentDeliveryRefundStillRecoversFromTheProvider`, `TestDeliveryResponsibilityTransitionConcurrentRetryIsExactlyOnce`. |
| CERT-035 multi-mission Commerce | `PASS — DB-BACKED` | `TestOneOrderCanBeDeliveredByMoreThanOneMission`, `TestTwoMissionsTogetherSettleExactlyTheFrozenAmount`, `TestReplayingAManagedCommerceReleasePaysOnce`, `TestTheExecutorCannotBeChangedAfterTheMoneyMoved`. |
| CERT-036 out-of-order event | `PASS — DB-BACKED` | `TestDispatchLifecycleDuplicateAndStaleVersionsDoNotRegress`, `TestAdvanceParcelStatus_NeverRegresses`, `TestApplyTransition_OutOfOrderCommandsAreRefused`, `TestIngestEventStaleVersionIgnored`, `TestAcceptStatusProjection_RefusesStaleAndDuplicate`. The Section 11 normalization carries a rank per state, so a stale callback cannot walk a mission backwards and an **unmapped** value advances nothing rather than being guessed at. |
| CERT-037 notification loss | `PASS — DB-BACKED` | `TestNotificationDegradedMode_CoreMutationPersistsRetriesDeadLettersAndReplays` — the core mutation persists even when notification delivery is failing, so the state the provider must recover is already durable. Every offer, assignment and case is retrievable by API independently of any push; the apps already poll as the documented fallback. |
| CERT-038 media security | `PASS — DB-BACKED` | `TestUploadAuthorizationIsSingleUse`, `TestUploadAuthorizationIssuesSingleUseGrant`, `TestExpiredUploadAuthorizationIsRefused`, `TestUploadAuthorizationIdempotencyConflict`, `TestBulkMediaAdapterEnforcesOwnershipPurposeSafetyAndReadiness`, `TestRetentionWindowBlocksDeletionUntilItLapses`. Retrieval needs a signed, expiring, object-scoped authorization and `owner_service` must match the authenticated caller. |
| CERT-039 verification failure | `PASS — DB-BACKED` for Dispatch and Taxi; `NOT RUN` for Hauling | Dispatch's typed readiness model gates going online on approved required steps, and corporate membership is evaluated separately from readiness — membership never substitutes for a safety check. **Hauling has no equivalent document-verification/expiry model.** That is GAP-A, unchanged, and it is in ART-008. No documents were invented for it. |
| CERT-040 safety during custody | `PASS — DB-BACKED` (partial) | `TestCreateSOS_EmergencyPriority`, `TestSafetyRouteRequiresIdempotencyKey`, `TestSafetyRoutesDenyWithoutPermission`, `TestSafetyReadPermissionDoesNotGrantWrite`, `TestAcknowledgementLeavesSafetyDeclineIntact`. Goods are not orphaned by construction: custody state is durable and a `FAILED_ATTEMPT` leaves custody exactly where it is (`TestFailedAttemptLeavesCustodyWhereItIs`), and the return path ends back with the sender (`TestReturnEndsBackWithTheSender`). **Corporate context is now recorded**: a provider-raised case snapshots the organization from Fleet over HMAC v2, fail-soft, so an operator can see which organization to contact. An unreachable Fleet never blocks the case. Six tests including the SOS-during-outage path. Still `NOT RUN` as a deployed journey (E2E-029). |

### Summary of CERT outcomes

| Level reached | Count |
|---|---|
| Certified at `DB-BACKED` or stronger | 33 |
| Certified at `UNIT`/`COMPONENT` only | 5 (CERT-005, 006, 018, 020, 022) |
| Carrying an explicit `NOT RUN` sub-case | 7 (CERT-001, 023, 024, 025, 031, 039, 040) |
| `FAIL` | 0 |

## 4. Failure-injection matrix (23.3)

Each injection point is listed with what was actually injected and at what
level. The five verification conditions the section names —
local state correct, retry identity stable, no duplicate posting,
reconciliation surfaces it, convergence after recovery — are reported per row.

| Injection point | Level | Injected | Five conditions |
|---|---|---|---|
| Fleet corporate-context read | `PASS — COMPONENT` | Reader error, reader absent, classified rejection | State unchanged; the operation is refused rather than downgraded; no posting attempted; `FLEET_DEPENDENCY_UNAVAILABLE` is retryable and surfaces as 503; converges when the reader returns. `TestAuthorizeManagedOperation_DependencyFailureIsRetryable`, `TestAuthorizeManagedOperation_NoReaderFailsClosedForManagedOnly`. |
| Fleet completed-work ingest | `PASS — DB-BACKED` | Transport failure, terminal rejection, duplicate | Local completion stays authoritative; the idempotency key is stable across retries (`TestCompletedWorkIdempotencyKeyIsStable`); no duplicate record; terminal rejection dead-letters immediately rather than retrying forever; ambiguity becomes `RECONCILIATION_REQUIRED` and redelivers under the same key. |
| Wallet managed settlement | `PASS — DB-BACKED` | Lost response, concurrent duplicate, conflicting amount | `TestConcurrentSendsOfOneInstructionPostOnce` (6 concurrent → 1 journal); read-back resolves the lost response; a conflicting amount is a conflict, not a second journal; `TestAManagedJobHeldWithNoSettlementIsSurfaced` is the reconciliation surface; `TestAStuckJobThatLaterSettlesIsResolved` is the convergence. |
| Vendor mission callback | `PASS — DB-BACKED` | Stale version, duplicate, unmapped status | No regression (`TestDispatchLifecycleDuplicateStaleAndForeignParcelPostgres`); an unmapped value advances nothing. |
| Media authorization | `PASS — DB-BACKED` | Expired grant, replayed grant, wrong owner | Refused; the evidence reference remains; no partial write. |
| Verification read | `PASS — DB-BACKED` | Missing/unapproved step | Readiness refuses; membership does not bypass it. |
| Notification delivery | `PASS — DB-BACKED` | Delivery failure | Core mutation persists; retries; dead-letters; replays. |
| Redis | `PASS — LIVE LOCAL` | Redis unreachable at boot | Hauling booted and served `/health` 200 with Redis down — optional integrations do not fail liveness. Certified live in the previous phase. |
| Database connection interruption | `NOT RUN` | — | Deliberately not injected. The section says "where safe"; killing connections under a real balanced-journal write on a shared certification database risks leaving the certification data itself in a state that would make every other row in this document unreliable. Lease expiry and crash recovery (CERT-013) cover the observable half of the same failure. |

## 5. Load tests (23.4) and SLO evidence (23.5)

`LOAD-005` was executed. The other six were not, and the reason is the same
for all of them: they are HTTP-level load against a running service, and this
run has no deployed environment to put load on. Running them against a laptop
process would measure the laptop.

`LOAD-005` is the exception because it is the **financially** critical path
and can be driven at the usecase boundary against a real database, which is
where its contention actually lives.

| ID | Level | Result |
|---|---|---|
| LOAD-001 Fleet corporate-context reads | `NOT RUN` | Needs deployed Fleet. |
| LOAD-002 offer / assignment list | `NOT RUN` | Needs deployed execution service. |
| LOAD-003 accept contention | `NOT RUN` as load; correctness certified `PASS — DB-BACKED` | `TestPrivateExecutionPostgres_ConcurrentDuplicateCreatesOneExecution`, `TestMatch_AcceptAtTimeoutBoundaryNotClobbered`. |
| LOAD-004 completed-work ingestion | `NOT RUN` | Needs deployed Fleet. |
| **LOAD-005 managed settlement posting** | **`PASS — DB-BACKED`** | See below. |
| LOAD-006 Commerce mission creation | `NOT RUN` | Needs deployed Vendor + execution authority. |
| LOAD-007 credential verification | `NOT RUN` as load; bound certified `PASS — DB-BACKED` | 5 attempts then LOCKED, 5 regenerations, 45s cooldown — a ceiling of 25 guesses per booking against a six-digit code. |

### LOAD-005 measurements

Environment: throwaway PostgreSQL 16 database `wallet_test_load_s19`, 4 CPUs,
production pool settings (`MaxConns=10`). Test gated behind `WALLET_LOAD_TEST`
so it never runs in an ordinary suite. Each request is a distinct real
instruction posting one balanced journal.

| Concurrency | Requests | Duration | Throughput | p50 | p95 | p99 | max | Errors | Timeouts | Pool empty-acquire waits |
|---|---|---|---|---|---|---|---|---|---|---|
| 4 | 400 | 1.025s | 390.4/s | 5.655ms | 28.215ms | 41.446ms | 54.114ms | 0 (0.00%) | 0 | 3 |
| 8 | 400 | 1.172s | 341.4/s | 5.788ms | 103.158ms | 203.744ms | 227.367ms | 0 (0.00%) | 0 | 7 |
| 16 | 400 | 1.220s | 327.9/s | 24.193ms | 170.465ms | 230.784ms | 325.803ms | 0 (0.00%) | 0 | 399 |

Duplicate storm — 400 replays of one instruction, run at each level:

| Concurrency | Duration | p50 | p95 | p99 | Errors | Journals |
|---|---|---|---|---|---|---|
| 4 | 243ms | 2.247ms | 2.699ms | 5.495ms | 0 | 1 |
| 8 | 320ms | 5.918ms | 8.241ms | 18.485ms | 0 | 1 |
| 16 | 313ms | 11.532ms | 16.754ms | 31.828ms | 0 | 1 |

Raw transcript: `docs/KARRYAM_SECTION19_24_LIVE_EVIDENCE.txt`.

**Reading the numbers, which matter more than the numbers.** Throughput does
not rise with concurrency — it *falls* slightly (390 → 341 → 328/s) across a 4×
increase, while p99 grows 5.6× (41ms → 231ms). That is the signature of a
bottleneck **downstream of the connection pool**: a single PostgreSQL
serializing balanced-journal writes. At concurrency 16, 399 of 400
acquisitions waited for a connection and the pool sat at its maximum of 10 —
so raising `MaxConns` would not add throughput, it would move the same queue
from the pool into the database, where it is harder to see and where it
competes with every other query. The operational guardrail is therefore to
hold settlement concurrency at or below the pool size, and ART-005 records it.

The duplicate storm being ~10× cheaper at p95 than real postings (16.8ms vs
170.5ms at the same concurrency) is the intended shape: a replay
short-circuits on the unique constraint before doing journal work. A retry
storm is survivable, and it produced exactly one journal at every level.

Outbox backlog growth during the test: none — this path posts synchronously
and does not enqueue. Recovery after test: the database was dropped; no
residue. No production data and no real money were involved.

## 6. Live-local evidence recorded this phase

`docs/KARRYAM_SECTION19_24_LIVE_EVIDENCE.txt` holds the raw transcript of the
only new live run in this phase: a Hauling service compiled from this tree,
booted against a throwaway database, answering the metrics endpoint added for
OBS-006/OBS-007 — 401 unauthenticated, 401 on a wrong token, 200 with real
gauges read from the real `hauling_domain_outbox`. It also proves the new route
registration does not collide, which is the exact class of defect the previous
phase found and fixed in this service.

The previous phase's live transcript (`docs/KARRYAM_SECTION15_18_LIVE_EVIDENCE.txt`,
21/21 checks) remains the evidence for the CERT rows labelled
`PASS — LIVE LOCAL` above and is unchanged by this phase.

## 7. What this matrix does not claim

Stated plainly so nobody reads a green table as more than it is:

1. **No scenario was run against a deployed staging environment.** Every
   `PASS — LIVE LOCAL` is a service compiled from this tree and booted on this
   machine.
2. **No persona walked the full offer→settle chain over HTTP in one run.**
   The stages are each certified; the seams between them are certified by
   contract tests, not by a continuous physical run.
3. **Six of seven load tests were not run**, and the one that was is a
   usecase-boundary measurement, not an HTTP-level one.
4. **Database connection interruption was not injected**, by choice.
5. CERT-031's payroll-side guard, CERT-039's Hauling half and CERT-040's
   corporate-context half are open, and are in ART-008 rather than quietly
   absorbed into a PASS.

## 8. E2E-001 … E2E-030 — full-system black-box journeys

Every journey in this section is, by definition, a cross-service run against
deployed staging URLs. There is no deployed staging
(`FINAL_DEPLOYED_STAGING_EVIDENCE.md`), and the decisive detail is that **none
of Fleet, Wallet, Taxi, Dispatch or Hauling has a deploy workflow at all** — so
these are not merely unrun, they are currently unrunnable by any mechanism this
repository defines.

Running them against locally booted binaries and reporting the result here
would be exactly the mislabelling the section forbids, so that was not done.
Where the underlying behaviour is certified at a lower level, the row says so
and names the level; that is a statement about the behaviour, not about the
journey.

| ID | Journey | Deployed status | Underlying behaviour |
|---|---|---|---|
| E2E-001 | Independent Taxi end to end | `NOT RUN` — no environment | `PASS — DB-BACKED` (independent settlement byte-identical) |
| E2E-002 | Corporate Taxi centralized | `NOT RUN` — no environment | `PASS — DB-BACKED` per stage; the seams are contract-tested, not physically run |
| E2E-003 | Corporate Taxi delegated | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-004 | Corporate Dispatch centralized | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-005 | Corporate Dispatch delegated | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-006 | Corporate Hauling | `NOT RUN` — no environment | `PASS — DB-BACKED`; capability declaration additionally `PASS — LIVE LOCAL` |
| E2E-007 | Vendor + Dispatch commerce | `NOT RUN` — no environment | `PASS — DB-BACKED`; credential direction `PASS — LIVE LOCAL` |
| E2E-008 | Vendor + Taxi Goods, `maxPickups=1` | `NOT RUN` — no environment | `PASS — UNIT` (refusal before any partial execution) |
| E2E-009 | Vendor + Hauling, secure bound | `NOT RUN` — no environment | `PASS — LIVE LOCAL` (`maxPickups=10`, `maxSecurePickups=1`, protected 3-pickup refused with `SECURE_PICKUP_LIMIT_EXCEEDED`) |
| E2E-010 | Salaried corporate provider | `NOT RUN` — no environment | `PASS — DB-BACKED` (zero share writes no provider line at all) |
| E2E-011 | Commission provider | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-012 | Per-job provider | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-013 | Mid-period plan change | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-014 | Membership expiry before accept | `NOT RUN` — no environment | `PASS — UNIT` (fails closed; no managed→independent fallback) |
| E2E-015 | Suspension before start | `NOT RUN` — no environment | `PASS — UNIT` + documented policy |
| E2E-016 | Suspension mid-trip | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-017 | Duplicate accept | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-018 | Duplicate completion, five links | `NOT RUN` — no environment | `PASS — DB-BACKED` (6 concurrent sends → 1 journal) |
| E2E-019 | Fleet response lost | `NOT RUN` — no environment | `PASS — DB-BACKED` (UNKNOWN retried under the same key) |
| E2E-020 | Wallet response lost | `NOT RUN` — no environment | `PASS — DB-BACKED` (read-back, never repost) |
| E2E-021 | Restart with pending outbox | `NOT RUN` — no environment | `PASS — DB-BACKED` (lease expiry recovery) |
| E2E-022 | Dead-letter repair | `NOT RUN` — no environment | `PASS — DB-BACKED` + `PASS — LIVE LOCAL` (operator surface: 200/401/403/409/400) |
| E2E-023 | Commerce credential replay | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-024 | Wrong provider's credential | `NOT RUN` — no environment | `PASS — DB-BACKED` (hash binds ProviderID) |
| E2E-025 | Financial privacy across four surfaces | `NOT RUN` — no environment | `PASS — UNIT`/`COMPONENT`; logs closed in the previous phase |
| E2E-026 | Notification loss recovery | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-027 | Media access control | `NOT RUN` — no environment | `PASS — DB-BACKED` |
| E2E-028 | Verification dependency failure | `NOT RUN` — no environment | `PASS — DB-BACKED` for Dispatch/Taxi; Hauling has no readiness model (GAP-A) |
| E2E-029 | SOS during custody | `NOT RUN` — no environment | `PASS — DB-BACKED`; the organization snapshot added this phase is included |
| E2E-030 | Commerce refund / reversal | `NOT RUN` — no environment | `PASS — DB-BACKED` (compensating journal; original immutable) |

**Zero of thirty journeys carry deployed evidence.** That is the honest count,
and it is why Section 23 stays OPEN.

## 9. LOAD-001 … LOAD-008 — deployed load

Unchanged from §5 except for LOAD-008, which this phase's section adds.

| ID | Status |
|---|---|
| LOAD-001 Fleet corporate-context | `NOT RUN` — needs deployed Fleet |
| LOAD-002 offer / assignment list | `NOT RUN` — needs a deployed execution service |
| LOAD-003 accept contention | `NOT RUN` as load; correctness `PASS — DB-BACKED` |
| LOAD-004 completed-work ingest | `NOT RUN` — needs deployed Fleet |
| LOAD-005 managed settlement | `PASS — DB-BACKED` (measured, §5); `NOT RUN` against staging |
| LOAD-006 commerce mission create | `NOT RUN` — needs deployed Vendor + authority |
| LOAD-007 credential verify | `NOT RUN` as load; the bound is `PASS — DB-BACKED` (25 guesses per booking ceiling) |
| LOAD-008 outbox recovery drain | `NOT RUN` — needs two deployed services and a controlled outage between them |

No pass threshold is invented. `LOAD-005`'s measured figures are a baseline,
and the guardrail derived from them — hold settlement concurrency at or below
the pool size — is recorded in the readiness checklist as a proposal, not as a
met SLO.
