"""Pin the counts-only, always-partial native scoped Analytics transport."""
import argparse
import json
from pathlib import Path
from contracts import build, string
from openapi_spec_validator import validate_spec

ROOT = Path(__file__).resolve().parents[2]
TARGET = ROOT / "services/analytics-service/docs/openapi/admin-scoped-mission-success.openapi.json"

def document():
    opaque = string(pattern="^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$")
    parameter = lambda name, schema=opaque: {"name": name, "in": "query", "required": True, "schema": schema}
    params = [parameter(key) for key in ("market", "zone", "organization", "tenant", "staff", "role", "purpose", "policy_id", "authority", "product_code")]
    params += [parameter("policy_version", {"type": "integer", "minimum": 1}), parameter("policy_checksum", string(pattern="^[0-9a-f]{64}$"))]
    params += [parameter(key, string(format="date-time")) for key in ("start", "end")]
    params += [parameter("limit", {"const": 1})]
    release = build()["openapi.json"]["components"]["schemas"]["ScopedMissionSuccessRelease"]
    op = {"operationId": "analytics.scoped.mission-success.read", "description": "Default off. Dedicated credential, durable nonce, report-specific permission, five-scope governance, independent producer contexts, suppression/reconstruction journal and original-member reauthorization. This is an incomplete counts-only source: complete membership and rate semantics remain engineering work. Current source always returns PARTIAL_COVERAGE with every value withheld; it cannot publish metrics by turning a flag on.", "parameters": params, "responses": {"200": {"description": "Scoped suppressed counts and explicit coverage. No raw cohort size, member identifiers or producer payload.", "content": {"application/json": {"schema": release}}}, **{str(code): {"description": "Sanitized denial/unavailable; no release values"} for code in (400, 401, 403, 409, 429, 503)}}}
    return {"openapi": "3.1.0", "info": {"title": "Analytics private scoped mission-success counts", "version": "1.0.0"}, "security": [{"ScopedServiceHMAC": []}], "paths": {"/internal/analytics/v1/admin/scoped-network/mission-success": {"get": op}}, "components": {"securitySchemes": {"ScopedServiceHMAC": {"type": "apiKey", "in": "header", "name": "X-Service-Signature", "description": "Dedicated admin-backoffice-service HMAC v2 key, distinct from every legacy Analytics audience. Exact method/path/query/body, name/version/timestamp/nonce headers and durable cross-replica replay protection."}}}}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    data = document()
    validate_spec(data)
    raw = (json.dumps(data, indent=2) + "\n").encode()
    if args.write:
        TARGET.parent.mkdir(parents=True, exist_ok=True)
        TARGET.write_bytes(raw)
    else:
        assert TARGET.read_bytes() == raw, "Scoped Analytics source contract drift"
    print("Scoped suppressed Analytics source OpenAPI validated; no production metrics enabled")

if __name__ == "__main__":
    main()
