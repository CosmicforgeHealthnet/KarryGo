"""Require every owner command certification scenario; skips or failures fail CI."""
import json
import sys
from pathlib import Path

events = [json.loads(line) for line in Path(sys.argv[1]).read_text(encoding="utf-8-sig").splitlines() if line.strip()]
assert events, "No owner command test evidence"
assert not [e for e in events if e["Action"] == "fail" or (e["Action"] == "skip" and e.get("Test"))], "Failed or skipped owner command certification"
passed = {(e["Package"], e["Test"]) for e in events if e["Action"] == "pass" and e.get("Test")}
p = "cosmicforge/logistics/"
required = {
    "shared/go/admincommand": ["TestOwnerCommandDictionaryRejectsUnknownActionsParamsAndUnboundApproval", "TestPostgresOwnerCommandApprovalVersionFenceIdempotencyAndImmutableLedger", "TestPostgresOwnerCommandSignedHTTPBoundaryAndClientProof"],
    "services/customer-service/internal/features/admincommands": ["TestPostgresLoginBlockRevokesSessionsAndRefusesOTPAndRefresh", "TestPostgresBookingHoldIsAuthoritativeOnInternalContextRoute", "TestPostgresRemediationRevokesSessionsAndRequiresRealCompletion", "TestPostgresPrivacyRequestLifecycleAndConsentWithdrawal", "TestPostgresSupportDecisionsAreVersionedCaseRecords", "TestPostgresConcurrentRestrictionsCommitExactlyOnce", "TestPostgresCrossTenantSubjectIsDeniedWithoutEffect", "TestPostgresSignedCommandBoundaryOnRealRouteRegistration", "TestRuntimeDefaultsOffAndRefusesUnreadableWrites"],
    "services/taxi-service/internal/features/admininterventions": ["TestLedgerMigrationEmbedsSharedTemplateVerbatim", "TestCanonicalExpiryIsEndOfDayInLagos", "TestIncidentLifecycleAndIllegalTransitions", "TestDocumentDecisionCanonicalExpiryAndGating", "TestRestrictProviderSuspendOnlineRequiresApprovalAndReallyBlocks", "TestSuspensionConvergesOnHeartbeatWhenLivePoolWasNotReachable", "TestNoNewAssignmentsKeepsOnlineButBlocksAssignment", "TestVehicleOutOfServiceAndReverification", "TestConcurrentConflictingCommandsCommitExactlyOnce", "TestCrossTenantSubjectDenied", "TestSignedHTTPBoundaryReadbackAndHistory"],
    "services/taxi-service/internal/features/adminresources": ["TestTaxiIncidentReadsAndCaseEvidenceAuthority"],
    "services/dispatch-delivery-service/internal/features/adminintervention": ["TestDispatchInterventionMigrationEmbedsSharedLedger", "TestPostgresReassignmentRetryEscalationThroughRealRecoveryEngine", "TestPostgresReassignmentRefusedForCustodyAndUncertainOutcome", "TestPostgresConcurrentReassignmentCommitsExactlyOnce", "TestPostgresCrossTenantSubjectDenied", "TestPostgresReturnAssignmentRetryRearmsRealWorker", "TestPostgresDeliveryReplanAwaitsCustomerApproval", "TestPostgresSignedHTTPBoundary"],
    "services/dispatch-delivery-service/internal/features/legacymediaadoption": ["TestPostgresDispatchAttestsHubCheckpointLegacyMediaForItsRecoveryCase", "TestDispatchLegacyAdoptionWorkerSignedClientAndDefaultOff"],
    "services/hauling-service/internal/features/admincommands": ["TestPostgresSignedCommandBoundaryOnTheRealRegistration", "TestPostgresInspectionLifecycleFailRestrictsTruckAndServesCaseAuthority", "TestPostgresInspectionCrossTenantAndRevokedOwnership", "TestPostgresCargoSuitabilityUsesCurrentInputsAndBlocksAssignment", "TestPostgresRestrictLiftOwnershipApprovalConcurrencyAndReplay", "TestPostgresLegacyInspectionMappingIsExactSameTruckConcludedAndSingle", "TestPostgresRecoveryReplacementRequiresCurrentOwnerValidation", "TestCommandLedgerMigrationEmbedsSharedTemplateVerbatim", "TestPostgresAdminMigrationsRollBackOnlyWhenEmptyAndReapply"],
    "services/fleet-partner-service/internal/features/admininvestigations": ["TestFleetLedgerMigrationEmbedsSharedTemplateVerbatim", "TestPostgresFleetInvestigationLifecycleApprovalAndRealEffects", "TestPostgresFleetInvestigationCrossTenantAndConcurrency", "TestPostgresFleetInvestigationSignedHTTPBoundary", "TestPostgresFleetInvestigationCaseAuthorityProof"],
    "services/taxi-service/internal/features/analyticsproducer": ["TestMigrationEmbedsSharedProducerSchemaVerbatim", "TestPostgresTaxiProducerUsesDeliveredIdentityScopedContextsAndRoster"],
    "services/dispatch-delivery-service/internal/features/analyticsproducer": ["TestMigrationEmbedsSharedProducerSchemaVerbatim", "TestPostgresDispatchProducerAttestsScopedTripsAndRiderRoster"],
    "services/hauling-service/internal/features/analyticsproducer": ["TestMigrationEmbedsSharedProducerSchemaVerbatim", "TestPostgresHaulingProducerAttestsScopedFactsAndIndividualRoster"],
    "shared/go/analyticsadmin": ["TestManifestDigestBindsEveryMemberAndRejectsAggregates", "TestVerifyManifestRejectsForgeryTamperAndIdentitySwap", "TestPublishManifestFixedPathAndOutcomes", "TestPostgresProducerAttestsScopedMembersIdempotentlyAndRetriesUnknownOutcome", "TestPostgresRosterRecorderRecordsIndividualMembersOncePerBucket"],
    "shared/go/networkanalytics": ["TestSupplyRosterMemberIsIndividualAndDeterministic", "TestSameSlotMeanTrustsOnlyMinimumHistory"],
}
missing = sorted((pkg, name) for pkg, names in required.items() for name in names if (p + pkg, name) not in passed)
assert not missing, f"Missing owner command scenarios: {missing}"
print(f"{len(passed)} owner command scenarios passed; zero failed or skipped tests")
