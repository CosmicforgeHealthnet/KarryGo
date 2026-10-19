"""Require actual owner migrations and authorization tests; skips fail CI."""
import json
import sys
from pathlib import Path

events = [json.loads(line) for line in Path(sys.argv[1]).read_text(encoding="utf-8-sig").splitlines() if line.strip()]
assert events, "No owner authority test evidence"
fixture_packages = {"cosmicforge/logistics/shared/go/admincontext/testfixture", "cosmicforge/logistics/shared/go/adminowner/testfixture"}
assert not [e for e in events if e["Action"] == "fail" or e["Action"] == "skip" and (e.get("Test") or e["Package"] not in fixture_packages)], "Failed or skipped owner authority certification"
passed = {(e["Package"], e["Test"]) for e in events if e["Action"] == "pass" and e.get("Test")}
prefix = "cosmicforge/logistics/"
required = {
    (prefix + "services/customer-service/internal/features/adminreview", "TestPostgresCustomerReviewRuntimeRequiresDistinctKeysAndNativeSchema"),
    (prefix + "services/customer-service/internal/features/adminreview", "TestPostgresCustomerReviewRecordsOwnerRequestWithImmutableReadback"),
    (prefix + "services/customer-service/internal/features/adminreview", "TestPostgresCustomerReviewConcurrentIdentityAndFreshKeysCannotDuplicate"),
    (prefix + "services/customer-service/internal/features/adminreview", "TestPostgresCustomerReviewAuditFailureRollsBackAndCurrentAccessIsRechecked"),
    (prefix + "services/customer-service/internal/features/adminreview", "TestPostgresCustomerReviewSignedBoundaryAndReadbackKeySeparation"),
    (prefix + "services/customer-service/internal/features/adminreview", "TestPostgresCustomerReviewActualTypedClientUsesOriginalReadback"),
    (prefix + "services/customer-service/internal/features/adminreview", "TestCustomerReviewRuntimeOffDoesNotReadSecretFiles"),
    (prefix + "shared/go/customerreview", "TestCustomerReviewClientRejectsUnboundProofAndNeverRepostsReadback"),

    (prefix + "services/hauling-service/internal/features/adminresources", "TestHaulingTruckCapabilitiesPreserveUnknownAndRequireNewContract"),
    (prefix + "services/hauling-service/internal/features/adminresources", "TestHaulingRecoveryReadsBindActualPlanningTenantAndMission"),
    (prefix + "services/dispatch-delivery-service/internal/features/adminresources", "TestDispatchWorkflowReadsBindCanonicalTripAndRejectCrossLinks"),
    (prefix + "services/hauling-service/internal/features/adminresources", "TestCustodyLegacyInventoryUsesExactFrozenManifestAndOmitsRawParcels"),
    (prefix + "services/hauling-service/internal/features/adminresources", "TestCustodyInventoryUsesActualScopedMultiManifestRecords"),
    (prefix + "services/dispatch-delivery-service/internal/features/adminresources", "TestCustodyLegacyInventoryUsesExactFrozenManifestAndOmitsRawParcels"),
    (prefix + "services/dispatch-delivery-service/internal/features/adminresources", "TestCustodyInventoryUsesActualScopedMultiManifestRecords"),
    (prefix + "services/dispatch-delivery-service/internal/features/adminresources", "TestCustodyAuthorizationBindsAuthorityModeAndCanonicalTenant"),
    (prefix + "services/customer-service/internal/features/adminresources", "TestCustomerProjectionCannotAuthorizeOrphanCustomerReference"),
    (prefix + "shared/go/admincontext", "TestSourceOwnerContextRequiresIndependentAuthenticApproval"),
    (prefix + "shared/go/admincontext", "TestPostgresOwnerContextHistoryRevocationCanonicalLinksAndPolicyIntersection"),
    (prefix + "shared/go/adminowner", "TestPostgresOwnerReadScopedPaginationRevocationAndMinimization"),
    (prefix + "shared/go/adminowner", "TestPostgresOwnerReadNonceAndAuditFailureAcrossReplicas"),
    (prefix + "shared/go/adminowner", "TestPostgresOwnerContextPublisherCannotSelfApproveOrOverrideCanonicalTenant"),
    (prefix + "shared/go/adminowner", "TestPostgresContextPreparationRequiresOwnerWorkloadAndImmutableAudit"),
    (prefix + "shared/go/adminowner", "TestOwnerResourcesDefaultDisabledWithoutCredentialAccess"),
    (prefix + "services/hauling-service/internal/features/adminresources", "TestHaulingPlanningTenantCannotUseSettlementOrConflictingOwner"),
}
for module in ("customer-service", "taxi-service", "dispatch-delivery-service", "hauling-service", "fleet-partner-service"):
    required.add((prefix + "services/" + module + "/internal/features/adminresources", "TestActualOwnerMigrationsAndSourceDictionary"))
assert required <= passed, f"Missing source authority scenarios: {sorted(required - passed)}"
print(f"{len(passed)} source authority scenarios passed; all five actual owner migration suites executed, zero skips")
