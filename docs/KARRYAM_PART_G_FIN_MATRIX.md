# KARRYAM Part G — FIN-01 … FIN-35 certification matrix

One row per invariant. Every row names where the behaviour lives, where it is
proved, and what the evidence actually is. No row says "covered by tests"
without naming the test.

**Status legend.** PASS: the invariant holds and a named test proves it. BLOCKED:
the behaviour exists but something outside this work prevents certification.
NOT_APPLICABLE: the invariant addresses a capability this platform does not have.

Paths are repository-relative. `wallet` is `services/payment-wallet-service`,
`fleet` is `services/fleet-partner-service`.

---

| ID | Invariant | Implementation | Test | Runtime | Status |
| --- | --- | --- | --- | --- | --- |
| FIN-01 | One completed work produces one settlement posting | `wallet/internal/features/wallets/repositories/managed_settlement_repository.go` `PostManagedSettlement` | `wallet/internal/integration/managed_settlement_test.go` `TestManagedSettlementPostsOneBalancedJournalForACommissionProvider` | Part E: "Fleet posts a managed settlement" | PASS |
| FIN-02 | A salaried provider is not paid per job | `fleet/.../compensation_calculation.go` `ResolveWorkCompensation`; `wallet/.../managed_settlement_repository.go` provider-line guard | `fleet/.../compensation_calculation_test.go` `TestPureSalaryResolvesToZeroProviderJobShare`; `wallet/internal/integration/managed_settlement_test.go` `TestSalariedProviderReceivesNoPerJobCredit` | — | PASS |
| FIN-03 | A commission provider receives exactly the resolved share | `fleet/.../compensation_calculation.go` `CalculateRuleShare` | `fleet/.../compensation_calculation_test.go` (FIN-03 block) | — | PASS |
| FIN-04 | A per-job provider receives the configured amount | `fleet/.../compensation_calculation.go` | `wallet/internal/integration/managed_settlement_test.go` `TestPerJobProviderReceivesExactlyTheConfiguredAmount` | — | PASS |
| FIN-05 | Every settlement journal balances | `wallet/.../managed_settlement_repository.go` `postLedgerTransaction` | `wallet/internal/integration/managed_settlement_test.go` `TestManagedSettlementPostsOneBalancedJournalForACommissionProvider` | Part E end-to-end journal | PASS |
| FIN-06 | Platform + organization + provider equals what the customer paid | `wallet/.../managed_settlement_repository.go` allocation check against the intent | `wallet/internal/integration/managed_settlement_test.go` `TestUnderAndOverAllocationAreRefused` | — | PASS |
| FIN-07 | No residual is dropped and none invented | same as FIN-06, restated against the intent rather than the instruction | `TestUnderAndOverAllocationAreRefused` | — | PASS |
| FIN-08 | The platform share is Wallet's and cannot be overridden | `wallet/.../managed_settlement_repository.go` platform-share comparison | `wallet/internal/integration/managed_settlement_test.go` `TestInstructionPlatformShareCannotOverrideTheAuthorizedFee` | — | PASS |
| FIN-09 | Fleet decides compensation; Wallet does not recalculate | instruction carries resolved amounts only | `fleet/internal/features/financeops/managed_settlement_test.go` `TestSubmissionCarriesFleetsResolvedFiguresVerbatim` (asserts no `rate_bps`, `commission_basis` or `rules` on the wire) | — | PASS |
| FIN-10 | Execution services do not calculate compensation | `taxi/.../booking/settlement`, `dispatch/.../tripsettlement`, `hauling/.../booking/settlement` are read-only | `taxi/.../settlement/managed_handoff_test.go`, `dispatch/.../tripsettlement/managed_handoff_test.go`, `hauling/.../settlement/managed_handoff_test.go` | Part E: "only Fleet may instruct a settlement" (403 for a signed Taxi) | PASS |
| FIN-11 | Vendor does not calculate provider compensation | `vendor/internal/integrations/logistics` carries no provider field | `vendor/internal/integrations/logistics/consistency_test.go` `TestNoVendorMissionRequestCarriesProviderSelectionAuthority`, with `TestTheProviderSelectionBanIsNotVacuous` proving the walker would catch one | — | PASS |
| FIN-12 | One instruction identity posts one journal | unique `(instruction_id, instruction_version)` + request fingerprint | `wallet/internal/integration/managed_settlement_test.go` `TestResendingTheSameInstructionPostsOnce` | Part E: "resending the instruction posts once" and the retry returning the original journal | PASS |
| FIN-13 | Concurrent sends of one instruction post once | row lock on the intent before the fingerprint check | `wallet/internal/integration/managed_settlement_test.go` `TestConcurrentSendsOfOneInstructionPostOnce` (six goroutines, one journal) | — | PASS |
| FIN-14 | The same identity with different amounts is a conflict, not a repeat | request fingerprint comparison | `wallet/internal/integration/managed_settlement_test.go` `TestSameIdentityWithDifferentAmountsIsAConflict` | Part E: "a changed instruction is a conflict" (409) | PASS |
| FIN-15 | A rule applies only within its effective dates | `fleet/.../models.go` `RuleApplies` | `fleet/.../compensation_calculation_test.go` (FIN-15/16 block) | — | PASS |
| FIN-16 | An ended rule stops applying | `RuleApplies` `EffectiveTo` check | same | — | PASS |
| FIN-17 | A rule applies only to its services | `RuleApplies` service check | `fleet/.../compensation_calculation_test.go` (FIN-17/18 block) | — | PASS |
| FIN-18 | A rule applies only to its roles | `RuleApplies` role check | same | — | PASS |
| FIN-19 | Work-level and payroll-level components never overlap | `fleet/.../settlement_strategy.go`; `fleet/.../organization_share.go` `CheckEconomicConservation` | `fleet/.../settlement_strategy_test.go` `TestEveryRuleSettlesInExactlyOnePlace`; `organization_share_test.go` `TestARuleSettledInTwoPlacesIsRefused` | — | PASS |
| FIN-20 | Immediate work compensation is never re-paid in payroll | `fleet/.../repository.go` `RecordWorkSettledComponents`; `fleet/internal/features/financeops/payroll.go` exclusion guard | `fleet/internal/integration/cmp027_double_pay_test.go` `TestCMP027_PayrollDoesNotRepayWorkSettledComponent` | — | PASS |
| FIN-21 | Payroll-accrued compensation is never paid immediately | `fleet/.../compensation_calculation.go` payroll-level short circuit | `fleet/.../settlement_strategy_test.go` `TestAccruedCommissionIsNotPaidWithTheJob` | — | PASS |
| FIN-22 | Salary is prorated deterministically in integer minor units | `fleet/.../salary_proration.go` | `fleet/.../salary_proration_test.go` (13 tests incl. `TestProrationIsDeterministic`, `TestTheRemainderIsRecordedAndTheRoundingIsVisible`) | — | PASS |
| FIN-23 | Every non-salary payroll component carries its provenance | `fleet/.../payroll_components.go` | `fleet/.../payroll_components_test.go` (allowance/overtime/bonus/deduction provenance and refusal tables) | — | PASS |
| FIN-24 | Overtime is paid only from an approved record | `fleet/.../payroll_components.go` `CalculateOvertime` | `fleet/.../payroll_components_test.go` `TestOvertimeWithoutAValidApprovalIsRefused`, `TestAnOvertimeApprovalCannotBeReusedInAnotherPeriod` | — | PASS |
| FIN-25 | A deduction never exceeds the liability it recovers | `fleet/.../payroll_components.go` `CalculateDeduction` | `fleet/.../payroll_components_test.go` `TestADeductionCannotExceedWhatIsOwed`, `TestInstalmentsRecoverALiabilityAndNoMore` | — | PASS |
| FIN-26 | Amounts in different currencies are never combined | `fleet/.../compensation_calculation.go` currency checks; `fleet/.../organization_share.go` `ResolveMultiServicePayroll` | `fleet/.../compensation_calculation_test.go` (FIN-26 block); `wallet/internal/integration/managed_settlement_test.go` `TestInstructionInAnotherCurrencyIsRefused`; `organization_share_test.go` `TestComponentsInAnotherCurrencyAreRefused` | — | PASS |
| FIN-27 | A timeout is never read as a confirmed failure | `shared/go/managedsettlement` Outcome; `fleet/.../financeops` `OUTCOME_UNKNOWN` read-back | `shared/go/managedsettlement/managedsettlement_test.go` `TestUnreachableWalletIsUnknownNotUnsettled`; the three services' handoff tests | Part E: "a timeout is Unknown, never a confirmed failure"; "an unreachable Wallet is Unknown, not unsettled" | PASS |
| FIN-28 | The legacy direct-payout path refuses managed work | `wallet/.../managed_settlement_repository.go` settlement-mode marker and settlement-row check | `wallet/internal/integration/managed_settlement_test.go` `TestLegacyCompleteJobRefusesManagedWork`, `TestLegacyCompleteJobStillWorksForIndependentProviders` | Part E: "the legacy payout path pays nothing for managed work" (0 journals) | PASS |
| FIN-29 | A provider cannot see organization or platform economics | `shared/go/providercompat/visibility.go` `AlwaysConcealed`; `provider_views.go` | `shared/go/providercompat/section14_visibility_test.go`; `provider_views_test.go` `TestAJobViewNeverCarriesOrganizationOrPlatformEconomics` | — | PASS |
| FIN-30 | Corrections are reversals; history is never rewritten | `wallet/.../managed_settlement_repository.go` `ReverseManagedSettlement` | `wallet/internal/integration/managed_settlement_test.go` `TestReversalCompensatesWithoutErasingHistory` | Part E: "Fleet may reverse a settlement"; "a reversed settlement reads back as unpaid" | PASS |
| FIN-31 | One job is never paid at both work and payroll level | `fleet/.../repository.go` + `financeops/payroll.go` exclusion | `fleet/internal/integration/cmp027_double_pay_test.go` (two tests, with a control assignment so a payroll pass that stopped producing items would not pass) | — | PASS |
| FIN-32 | A managed Commerce refund recovers from the parties that were paid | `wallet/.../commerce_delivery_recovery.go`; `wallet/.../commerce.go` organization recovery | `wallet/internal/integration/commerce_delivery_recovery_test.go` (17 tests incl. `TestASalariedRidersRefundTakesNothingFromTheRider`, `TestARefundAndADisputeShareOneRecoveryPool`) | Part E: dispute and recovery-plan routes reachable and caller-narrowed | PASS |
| FIN-33 | Wallet provider reads cannot bypass the visibility policy | `wallet/internal/features/wallets/http/routes.go` provider group; caller narrowing on the internal group | `wallet/.../http/provider_visibility_audit_test.go` (5 tests, enumerating the real router) | Part E: "the provider surface refuses an unauthenticated read"; "no settlement route exists on the provider surface" | PASS |
| FIN-34 | Notifications do not leak concealed financial fields | `shared/go/notifications/notifications.go` `Validate`; `shared/go/providercompat/payload_audit.go` | `shared/go/notifications/visibility_test.go` (7 tests incl. nested and template-data cases) | — | PASS |
| FIN-35 | A multi-service provider is paid once for period components | `fleet/.../organization_share.go` `ResolveMultiServicePayroll` | `fleet/.../organization_share_test.go` `TestOneSalaryIsPaidOncePerProviderNotOncePerService`, `TestAFixedAllowanceIsAlsoPaidOncePerProvider`, `TestPerServiceEarningsAccumulateAcrossServices` | — | PASS |

---

## Runtime certification

The "Runtime" column names the check in
`docs/KARRYAM_PART_E_LIVE_WIRING_EVIDENCE.txt`, produced by
`docs/liveprobe` against a running `payment-wallet-service` with real HMAC
signing. Rows with `—` are invariants of pure logic or of a database
transaction, where a live HTTP round trip would add nothing a
PostgreSQL-backed test does not already prove.

## Summary

PASS 35 · FAIL 0 · BLOCKED 0 · NOT_APPLICABLE 0
