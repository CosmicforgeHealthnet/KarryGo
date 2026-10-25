"""Require Referral's PostgreSQL certification evidence; skips fail CI."""

import json
import sys
from pathlib import Path


events = [
    json.loads(line)
    for line in Path(sys.argv[1]).read_text(encoding="utf-8-sig").splitlines()
    if line.strip()
]
assert events, "No Referral certification evidence"
failed_or_skipped = [
    event
    for event in events
    if event.get("Action") == "fail"
    or (event.get("Action") == "skip" and event.get("Test"))
]
assert not failed_or_skipped, (
    "Failed or skipped Referral certification: "
    + repr([(event.get("Package"), event.get("Test")) for event in failed_or_skipped])
)

passed = {event["Test"] for event in events if event.get("Action") == "pass" and event.get("Test")}
required = {
    "TestMigrationsReplayAndRollback",
    "TestReferralSecureAdminOwnerBoundary",
    "TestReferralAdminResourceDictionaryReadsNativeRows",
    "TestConcurrentDuplicateDeliveries",
    "TestAdminActionsOverHTTP",
    "TestResolvingFlags",
    "TestTakeBackWaitsForTheRewardItUndoes",
    "TestConcurrentSettlementAndReversal",
    "TestReconcileWithWallet",
    "TestSharedBankAccountStopsCashOut",
}
assert required <= passed, f"Missing Referral certification scenarios: {sorted(required - passed)}"
print(f"{len(passed)} Referral tests passed; required PostgreSQL/security evidence present, zero skipped tests")
