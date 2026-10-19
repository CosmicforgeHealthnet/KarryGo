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
