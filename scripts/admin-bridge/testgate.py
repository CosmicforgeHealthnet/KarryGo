"""Require real passing test events and prohibit skipped database certification."""
import json
import sys
from pathlib import Path

events = [json.loads(line) for line in Path(sys.argv[1]).read_text(encoding="utf-8").splitlines() if line.strip()]
failures = [e for e in events if e.get("Action") == "fail" or (e.get("Action") == "skip" and e.get("Test"))]
passed = [e for e in events if e.get("Action") == "pass" and e.get("Test")]
required = {"TestPostgresReplayAndMigrationConcurrency", "TestPostgresIdempotencyScopeAndImmutableAudit", "TestPostgresRevocationPrincipalAndRate", "TestRealMTLSBoundary", "TestContractWireSamples"}
assert not failures, f"Failed/skipped tests: {[(e.get('Package'), e.get('Test')) for e in failures]}"
assert required <= {e["Test"] for e in passed}, "Mandatory security/database tests did not execute"
empty_packages = [e for e in events if e.get("Action") == "skip" and not e.get("Test")]
print(f"{len(passed)} test cases passed; zero failed/skipped tests ({len(empty_packages)} packages contain no tests)")
