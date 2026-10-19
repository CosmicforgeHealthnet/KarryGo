"""Require executed owner safety scenarios and reject skipped certification."""
import argparse
import json
from pathlib import Path

p = argparse.ArgumentParser()
p.add_argument("path", type=Path)
p.add_argument("--custody", action="store_true")
args = p.parse_args()
events = [json.loads(line) for line in args.path.read_text(encoding="utf-8-sig").splitlines() if line.strip()]
assert events, "No test evidence"
assert not [e for e in events if e["Action"] in ("fail", "skip") and (e.get("Test") or e["Action"] == "fail")], "Failed or skipped owner test"
passed = {e["Test"] for e in events if e["Action"] == "pass" and e.get("Test")}
required = {"TestRelayWorkerUnknownOutcomeIsNotRedelivered", "TestRelayLeaseRecoveryDeadLetterAndReplay", "TestConcurrentRelayWorkersDeliverEachDecisionOnce", "TestPostgresRepositoryTripFoundation", "TestRealRedisHandoverLimitAcrossClients"}
required |= {"TestImmediateRelayClaimFencesDuplicateRequestsAndWorker", "TestImmediateDeliveryPersistenceFailureStaysPending"}
required.add("TestFencedLegacyDecisionCannotBeRecreatedWithFreshKey")
required |= {"TestRelayAuthoritativeReadbackNeverRepeatsUnknownMutation", "TestRelayReadbackDeniesMissingOrMismatchedDecision", "TestRelayReadbackExpiredReaderCannotFinishNewLease", "TestRelayReadbackAuditFailureCannotConfirm"}
if args.custody:
    required = {"TestSecureDeliveryPhoneOnlyReceiverFailsBeforeChallenge", "TestSecureDeliveryDifferentReceiverAccountCannotBeReadBySender", "TestSecureHandoverOTPAndQRRaceConsumesOnce", "TestSecureDeliveryEvidenceGatesCompletionAndIsAppendOnly", "TestRelayUncertaintyUpgradePreservesLegacyOutcomes"}
assert required <= passed, f"Mandatory scenarios missing: {sorted(required - passed)}"
print(f"{len(passed)} owner scenarios passed; zero failed or skipped tests")
