"""Validate real owner custody HTTP output against its independent schema."""
import json
import sys
from pathlib import Path

import jsonschema

root = Path(__file__).resolve().parents[2]
schema = json.loads((root / "services/admin-backoffice-service/docs/openapi/schemas/OwnerResourcesSnapshot.json").read_text(encoding="utf-8"))
for filename, owner in (("custody.json", "driver-dispatch-delivery-service"), ("hauling-custody.json", "driver-hauling-service")):
    wire = json.loads((Path(sys.argv[1]) / filename).read_text(encoding="utf-8"))
    jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker()).validate(wire)
    assert len(wire["items"]) == 1 and wire["items"][0].get("custody"), "Real custody detail did not execute"
    assert wire["owner"] == owner and wire["kind"] == "custody-missions"
print("Real PostgreSQL Dispatch and Hauling custody HTTP output satisfies the pinned typed contract")

for filename, kind in (("hauling-capability-resource.json", "trucks"), ("hauling-recovery-resource.json", "recoveries")):
    resource = json.loads((Path(sys.argv[1]) / filename).read_text(encoding="utf-8"))
    jsonschema.Draft202012Validator(schema["properties"]["items"]["items"], format_checker=jsonschema.FormatChecker()).validate(resource)
    assert resource["kind"] == kind
print("Actual Hauling capability/recovery SQL projections satisfy the pinned resource contract")

for filename in ("taxi-passenger-recovery-resource.json", "taxi-goods-recovery-resource.json"):
    resource = json.loads((Path(sys.argv[1]) / filename).read_text(encoding="utf-8"))
    jsonschema.Draft202012Validator(schema["properties"]["items"]["items"], format_checker=jsonschema.FormatChecker()).validate(resource)
    assert resource["kind"] == "recoveries" and resource["source_version"] > 0
print("Actual Taxi passenger/goods recovery SQL projections satisfy the pinned resource contract")

release = json.loads((Path(sys.argv[1]) / "scoped-analytics-owner.json").read_text(encoding="utf-8"))
release_schema = json.loads((root / "services/admin-backoffice-service/docs/openapi/schemas/ScopedMissionSuccessRelease.json").read_text(encoding="utf-8"))
jsonschema.Draft202012Validator(release_schema, format_checker=jsonschema.FormatChecker()).validate(release)
assert release["coverage"] == "PARTIAL_COVERAGE" and all(cell["value"] is None for cell in release["cells"])
print("Actual native Analytics scoped HTTP output satisfies the suppressed release schema")

preparation = json.loads((Path(sys.argv[1]) / "scoped-analytics-binding.json").read_text(encoding="utf-8"))
source_contract = json.loads((root / "services/analytics-service/docs/openapi/admin-scoped-mission-success.openapi.json").read_text(encoding="utf-8"))
prepare_schema = source_contract["paths"]["/internal/analytics/v1/admin/source-contexts/driver-taxi-service/bindings/{kind}/{id}"]["get"]["responses"]["200"]["content"]["application/json"]["schema"]
jsonschema.Draft202012Validator(prepare_schema, format_checker=jsonschema.FormatChecker()).validate(preparation)
assert preparation["state"] == "DRAFT_PENDING_OWNER_APPROVAL"
assert preparation["binding"]["known_source_dimensions"] == {"product": "karryam", "market": "", "zone": "", "organization": "", "tenant": ""}
print("Actual Analytics producer preparation is draft-only and supplies no geographic/tenant mapping")

handover = json.loads((Path(sys.argv[1]) / "dispatch-handover-state.json").read_text(encoding="utf-8"))
jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker()).validate(handover)
assert handover["kind"] == "handover-states" and len(handover["items"]) == 1
item = handover["items"][0]
assert item["source_version"] is None and {d["name"] for d in item["dimensions"]} == {"stage", "notification"}
assert item["counters"] == [] and item["timeline"] == [] and {r["kind"] for r in item["references"]} == {"trip"}
assert "PRIVATE_" not in json.dumps(handover)
print("Actual native Dispatch handover wire contains only minimized stored metadata and no complete revision")

restriction = json.loads((Path(sys.argv[1]) / "hauling-restriction.json").read_text(encoding="utf-8"))
jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker()).validate(restriction)
assert restriction["kind"] == "restrictions" and restriction["owner"] == "driver-hauling-service" and len(restriction["items"]) == 1
item = restriction["items"][0]
assert item["source_version"] == 1 and item["status"] == "APPLIED"
assert {d["name"] for d in item["dimensions"]} == {"subject_type", "reason"}
assert "PRIVATE_" not in json.dumps(restriction)
print("Actual Hauling restriction HTTP output preserves native revision and excludes private provenance")

fit = json.loads((Path(sys.argv[1]) / "hauling-fit-decision.json").read_text(encoding="utf-8"))
jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker()).validate(fit)
assert fit["kind"] == "fit-decisions" and fit["owner"] == "driver-hauling-service" and len(fit["items"]) == 1
item = fit["items"][0]
assert item["source_version"] is None and item["status"] == "ELIGIBLE"
assert {d["name"] for d in item["dimensions"]} == {"shadow", "policy_revision"}
assert item["counters"] == [{"name": "captured_capability_revision", "value": 7}]
assert {r["kind"] for r in item["references"]} == {"booking", "provider", "truck"}
assert item["timeline"] == [] and "PRIVATE_" not in json.dumps(fit) and "987654" not in json.dumps(fit)
print("Actual historical Hauling fit-decision HTTP output excludes private evidence and does not assert current eligibility")
