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
