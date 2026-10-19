"""Require actual Fleet PostgreSQL receipt/isolation evidence, with no skips."""
import json
import sys
from pathlib import Path

events = [json.loads(line) for line in Path(sys.argv[1]).read_text(encoding="utf-8-sig").splitlines() if line.strip()]
assert events, "No receipt evidence"
assert not any(e["Action"] == "fail" or e["Action"] == "skip" and e.get("Test") for e in events), "Failed or skipped receipt tests"
passed = {e["Test"] for e in events if e["Action"] == "pass" and e.get("Test")}
required = {"TestProviderOfferReceiptExactImmutableDecisionAndIsolation", "TestProviderOfferReceiptFailureRollsBackNativeAcceptance", "TestProviderOfferLegacyMutableDecisionCannotInventReceipt", "TestProviderDecisionReceiptPrivateAssignmentOriginalKeyAndCanonicalProvider", "TestProviderDecisionReceiptHistoricalIdentityAndIsolation", "TestProviderDecisionReceiptHistoricalIdentityAndIsolation/same-key-cannot-change-decision", "TestProviderDecisionReceiptHistoricalIdentityAndIsolation/tenant-injection", "TestProviderDecisionReceiptHistoricalIdentityAndIsolation/unsigned", "TestProviderDecisionReceiptHistoricalIdentityAndIsolation/non-owner-service"}
assert required <= passed, f"Missing receipt cases: {sorted(required - passed)}"
print(f"{len(passed)} Fleet receipt scenarios passed; zero failed or skipped tests")
