"""Validate actual native Vendor HTTP/typed-client output against private contracts."""
import json
import sys
from pathlib import Path
import jsonschema

root=Path(__file__).resolve().parents[2]
wire=Path(sys.argv[1])
schemas=root/"services/admin-backoffice-service/docs/openapi/schemas"
for filename,schema in [("vendor-resources.json","VendorResourcesSnapshot"),("vendor-command-receipt.json","VendorOwnerCommandReceipt")]:
 value=json.loads((wire/filename).read_text(encoding="utf-8"))
 definition=json.loads((schemas/(schema+".json")).read_text(encoding="utf-8"))
 jsonschema.Draft202012Validator(definition,format_checker=jsonschema.FormatChecker()).validate(value)
 assert value["owner"]=="vendor-service"
value=json.loads((wire/"vendor-resources.json").read_text(encoding="utf-8"))
assert len(value["items"])==1 and value["items"][0]["label"], "Actual commercial directory label missing"
assert value["coverage"]=="CONTEXT_SCOPED_PARTIAL"
proof=json.loads((wire/"vendor-case-proof.json").read_text(encoding="utf-8"))
assert proof["owner"]=="vendor-service" and proof["case_kind"]=="cases" and proof["subject_type"]=="CASE"
assert proof["source_contract"]=="vendor.admin-case-evidence.v1" and len(proof["native_fingerprint"])==64
print("Actual signed Vendor directory, command receipt and native case proof validated")
