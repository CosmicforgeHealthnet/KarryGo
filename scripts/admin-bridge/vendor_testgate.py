"""Require Section 11 owner behavior in canonical isolated Vendor evidence."""
import json
import sys
from pathlib import Path

required = {
 "TestVendorSecureAdminDeliveryReadsPreserveAllocationAndOwnerAuthority",
 "TestVendorSecureAdminCheaperSubstitutionReadPreservesSignedPriceDifference",
 "TestVendorSecureAdminPauseBlocksApprovedQuoteButPreservesCommittedOrder",
 "TestVendorCustomerStandingSignedPostgresGuardAndCommittedReadback",
 "TestVendorCustomerStandingLiftIsNotCached",
 "TestVendorStandingNeverFollowsCredentialRedirect",
 "TestVendorAdminCredentialsNeverReuseStandingOrApplicationAudiences",
 "TestVendorSecureAdminCasesAndNativeMutations",
 "TestVendorSecureAdminNativeRowsAndBranchAuthorization",
 "TestVendorSecureAdminAuditFailureRollsBackNativeEffect",
 "TestVendorSecureAdminListingModerationPreservesStockAndPrice",
 "TestVendorSecureAdminProductionRecoveryNeverCompletesOrInventsCapacity",
 "TestVendorSecureAdminCapacityRescheduleUsesNativeCapacityFence",
 "TestVendorSecureAdminOrderAllocationGraph",
 "TestVendorSecureAdminNativeCaseMembershipForMedia",
 "TestVendorSecureAdminProcessesOnlyCustomerApprovedSubstitutionAtomically",
 "TestVendorSecureAdminReplacementAdapterJoinsRealStockTransaction",
 "TestVendorSecureAdminStockReadsUseCanonicalExpiryAwareAvailability",
 "TestVendorSecureAdminEligibilityUsesNativeVerificationReducer",
 "TestVendorCheckoutFailsClosedForExpiredVerificationOnApprovedQuote",
 "TestVendorCheckoutRejectsSuspendedMerchantAndClosedBranch",
 "TestVendorCheckoutRejectsTakenDownRealListing",
 "TestSubstitution_DeadlineRequiredAndExpiredApprovalPersistsTerminalState",
 "TestSubstitution_PendingLiabilityRemainsDurableUntilWalletApplies",
}

def verify(directory):
 files = sorted(Path(directory).glob("*.jsonl"))
 assert files, "Missing canonical Vendor runner evidence"
 passed = set()
 for path in files:
  events = [json.loads(line) for line in path.read_text(encoding="utf-8-sig").splitlines() if line.strip()]
  assert events, f"Empty Vendor evidence: {path.name}"
  assert not any(e.get("Action") == "fail" or e.get("Action") == "skip" and e.get("Test") for e in events), f"Failed/skipped Vendor tests: {path.name}"
  passed.update(e["Test"] for e in events if e.get("Action") == "pass" and e.get("Test"))
 assert required <= passed, f"Missing required Vendor owner tests: {sorted(required - passed)}"
 return len(passed)

if __name__ == "__main__":
 print(f"{verify(sys.argv[1])} distinct Vendor tests passed; Section 11 owner scenarios present")
