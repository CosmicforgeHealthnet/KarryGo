"""Require real Media object/case enforcement evidence; skipped tests fail CI."""
import json
import sys
from pathlib import Path

events = [json.loads(line) for line in Path(sys.argv[1]).read_text(encoding="utf-8-sig").splitlines() if line.strip()]
assert events, "Missing Media test evidence"
assert not [e for e in events if e["Action"] == "fail" or e["Action"] == "skip" and e.get("Test")], "Failed/skipped Media tests"
passed = {e["Test"] for e in events if e["Action"] == "pass" and e.get("Test")}
required = {
 "TestPostgresMediaVendorCaseEvidenceThroughExistingCaseServices",
 "TestPostgresMediaNativeCustomerCaseMembershipAndModernEvidence",
 "TestSourceCaseClientPinsOwnerMembershipScopePolicyAndAudience",
 "TestOwnerCaseEnrollmentRequiresFiniteOwnersAndDistinctAudienceKeys",
 "TestPostgresMediaCaseGrantCannotOutliveSignedDisclosurePolicy",
 "TestPrivateMediaCaseClientUsesFixedPathsAndRejectsUnboundOrRawResponses",
 "TestPostgresMediaCasePrivateHTTPAndTypedClientEnforceActorScopeAndReplay",
 "TestPostgresMediaCaseRuntimeSeparatesKeysAndCannotInventAdaptersOrPolicy",
 "TestMediaCaseRuntimeOffReadsNoCredentialOrPolicyFiles",
 "TestNativeMediaDownloadGrantCannotOutliveEvidenceRevocation",
 "TestExistingDownloadGrantRechecksCurrentSafetyPolicy",
 "TestPostgresMediaCaseGrantBindsStaffCaseSubjectScopeAndNativeObject",
 "TestPostgresMediaCaseCurrentNativeSafetyBindingAndRevocationFloor",
 "TestPostgresMediaCaseAuditAtomicityImmutabilityAndRevocationAfterKill",
 "TestPostgresMediaCaseExpiryAndPolicyRollbackCannotRestoreAccess",
 "TestPostgresMediaCaseRedemptionAndRevocationSerializeOnOriginalGrant",
 "TestMediaCaseAccessOffOrMissingAuthorityCannotMintGrant",
 "TestEvidenceMigrationPreservesLegacyRows",
 "TestMigrationsReplayAndRollback",
}
assert required <= passed, "Mandatory actual Media storage/case/migration tests did not run"
print(f"{len(passed)} Media cases passed, zero failed/skipped tests; native owner-case integration verified, activation remains disabled")
