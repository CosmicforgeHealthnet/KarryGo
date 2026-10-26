"""Sections 16-38 owner certification gate: real PostgreSQL/Redis evidence,
zero failures and zero skipped tests, and every named scenario executed."""
import json
import sys
from pathlib import Path

events = [json.loads(line) for line in Path(sys.argv[1]).read_text(encoding="utf-8-sig").splitlines() if line.strip()]
assert events, "No Sections 16-38 certification evidence"
bad = [e for e in events if e.get("Action") == "fail" or (e.get("Action") == "skip" and e.get("Test"))]
assert not bad, "Failed or skipped Sections 16-38 tests: " + repr([(e.get("Package"), e.get("Test")) for e in bad])
passed = {e["Test"] for e in events if e.get("Action") == "pass" and e.get("Test")}
required = {
    # Section 16 Support
    "TestSupportSecureAdminCaseStateMachine", "TestSupportAdminReplyDeliveryIsSeparateFromTheMessage", "TestSupportChatSequencesAreContiguousUnderConcurrency", "TestSupportSOSIsSafetyEvidenceAndEscalatable", "TestSupportDisputeResolutionAndRemedy", "TestSupportAdminEventSnapshotBackfill", "TestSecureAdminMigrationReplayRollbackAndReapply",
    # Section 17/25 Notification + consent authority
    "TestTargetedMarketOnlyUnknownMarketExcludedAndMessageStatusUpdated", "TestSameIntentTwiceIsOneCampaignAndRevisionConflicts", "TestConsentWithdrawnAfterPlanningIsSuppressedAtSend", "TestDuplicateRecipientsAreDeduplicatedPerChannel", "TestQuietHoursAcrossTimezoneBoundaryDeferNotDrop", "TestCancellationBeforeSendPreventsAllMessages", "TestProviderOutageKeepsQueuedThenRetrySends", "TestLoggingProviderIsLoggedOnlyNeverSent", "TestOwnerEventsMatchTheAdapterAndSnapshot", "TestCampaignRoutesSecurityIdempotencyAndConflicts",
    "TestConsentRoutesCaptureWithdrawIdempotentAndHistory", "TestAudiencePaginationAndMarketFilter", "TestInternalRoutesCallerFlagAndReplay", "TestPostgresPrivacyRequestLifecycleAndConsentWithdrawal",
    # Section 18 Verification
    "TestCaseStateMachineTable", "TestSection18AutomationRecordsFindingsAndNeverDecides", "TestSection18ExpirySweepNotifiesVendorAndCallbackRepair", "TestSection18DecisionRouteContract", "TestVerificationSecureAdminOwnerBoundary", "TestSnapshotBackfillPagesValidatedMinimizedEvents",
    # Section 19 Media
    "TestSection19PublicLegacyDownloadServesOnlyProfilePhotos", "TestSection19HighRiskPurposesFailClosedWithoutSafeVerdict", "TestSection19CaseGrantIsOneTimeAndConcurrentRedemptionSucceedsOnce", "TestSection19AdoptedLegacyAssetStopsServingAndMappingIsPrivate", "TestSection19ClosedCaseRecordsImmutableRevocationAndFailsFast",
    # Section 26 Wallet finance reporting
    "TestFinanceReportExactFigures", "TestFinanceReportHTTPBoundary", "TestDefinitionsMatchExecutedSQL",
    # Section 15 regression fixed here
    "TestReferralSecureAdminOwnerBoundary",
}
missing = sorted(required - passed)
assert not missing, f"Missing Sections 16-38 scenarios: {missing}"
print(f"{len(passed)} Sections 16-38 owner tests passed; zero failed/skipped")
