"""Require real passing test events and prohibit skipped database certification."""
import json
import sys
import argparse
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("path")
parser.add_argument("--suite", choices=["admin", "gateway"], default="admin")
args = parser.parse_args()
events = [json.loads(line) for line in Path(args.path).read_text(encoding="utf-8-sig").splitlines() if line.strip()]
failures = [e for e in events if e.get("Action") == "fail" or (e.get("Action") == "skip" and e.get("Test"))]
passed = [e for e in events if e.get("Action") == "pass" and e.get("Test")]
required = {"TestPostgresReplayAndMigrationConcurrency", "TestPostgresIdempotencyScopeAndImmutableAudit", "TestPostgresRevocationPrincipalAndRate", "TestRealMTLSBoundary", "TestRealPostgresMTLSIntegration", "TestRuntimeRoleGuardAndGrants", "TestMigrationFailureRollsBackAndDownIsIrreversible", "TestContractWireSamples"}
required |= {"TestTaxiOwnerProcessPostgresMTLS", "TestTaxiReadDelegationAndOwnerBoundary", "TestOwnerResourcesWireContract", "TestOwnerResourcesDelegation"}
required |= {"TestOwnerPolicyCannotInventCanonicalTenant", "TestOwnerPolicyIntersectionAndUnmappedReferences", "TestDraftRepositoryPolicyAndMissingCanonicalMappingsDeny", "TestPolicyApprovalAuthenticityEnvironmentAndDraft", "TestPostgresDisclosureAuditFrozenReleaseAndReconstructionGuard", "TestPostgresDisclosureBudgetAndAuditFailureDenyData", "TestMissingApprovalAndUnmappedSourceCannotDisclose", "TestTenAnalyticsContractsUseFixedSignedOwnerPaths", "TestRolloutFlagsCannotCreateOwnerCapabilities"}
required |= {"TestPostgresMissionSourceCountsDistinctApprovedNativeUnitsAndWithholdsIncompleteCoverage", "TestPostgresAnalyticsLineageUsesCanonicalEventsAndIndependentOwnerContexts", "TestPostgresAnalyticsLineageNeverCountsAggregateBucketsAsSubjects", "TestAnalyticsLineageDictionaryMatchesAllTenReports"}
required |= {"TestPostgresCustomerReviewBridgeConcurrentSubmissionPostsOnce", "TestPostgresCustomerReviewBridgeUnknownOutcomeOnlyReadsOriginal", "TestPostgresCustomerReviewBridgeCrashFenceAndMismatchedProof", "TestPostgresCustomerReviewBridgeScopeAndAuditFailureFence", "TestPostgresCustomerReviewBridgeTypedOwnerBoundaryAndRecovery", "TestCustomerReviewConfigurationSeparatesAudiencesAndReadback"}
required |= {"TestPostgresFrozenReleaseRequiresOriginalMembershipWithoutResampling", "TestPostgresLegacyFrozenReleaseWithoutMembershipFailsClosed", "TestPostgresDisclosureMembershipAndReleaseRollBackTogetherOnAuditFailure"}
required |= {"TestPostgresScopedAnalyticsBridgeMTLSFiveScopeAndSuppressedWire", "TestScopedAnalyticsConfigurationNeedsBothFlagsAndDistinctCredential", "TestPostgresScopedAnalyticsNonceStrictQueryAndPolicyBoundary", "TestPostgresScopedAnalyticsRuntimeCredentialSeparationAndNoInventedApproval", "TestScopedAnalyticsRuntimeOffDoesNotReadFiles", "TestScopedAnalyticsClientNeverUsesLegacyPathsOrRawCohorts"}
required |= {"TestAnalyticsProducerPreparationReturnsOnlyDraftNativeBinding", "TestAnalyticsProducerPublisherClientUsesFixedOwnerAndNoAutomaticRetry", "TestPostgresAnalyticsProducerPublicationBindsNativeFactsAndRevokes", "TestPostgresAnalyticsPublisherRuntimeIsIndependentAndSeparatesCredentials"}
required |= {"TestMediaCasePolicyNeedsExplicitSignedValuesWithoutProductionDefaults", "TestPostgresVendorReadinessPublicationUsesIndependentNativeBranchBinding"}
required |= {"TestMissionRateUsesOnlyDisclosedCountsAndNeverGuessesUnknownValues", "TestPostgresScopedMissionRateRequiresBothPermissionsAndPreservesSuppression"}
if args.suite == "gateway":
    required = {"TestPublicAdminIsolation", "TestForgedHQHeadersAreStrippedFromPublicRoutes", "TestPostgresRefreshRotationAndMigrationReplay"}
assert not failures, f"Failed/skipped tests: {[(e.get('Package'), e.get('Test')) for e in failures]}"
assert required <= {e["Test"] for e in passed}, "Mandatory security/database tests did not execute"
empty_packages = [e for e in events if e.get("Action") == "skip" and not e.get("Test")]
print(f"{len(passed)} test cases passed; zero failed/skipped tests ({len(empty_packages)} packages contain no tests)")
