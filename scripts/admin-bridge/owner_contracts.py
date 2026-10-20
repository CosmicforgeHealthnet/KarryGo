"""Generate source-owned resource contracts from the pinned Bridge wire schema."""
import argparse
import json
from pathlib import Path
from contracts import build, obj, string
from openapi_spec_validator import validate_spec

ROOT = Path(__file__).resolve().parents[2]
OWNERS = {"customer-service": "customer", "driver-taxi-service": "taxi", "driver-dispatch-delivery-service": "dispatch", "driver-hauling-service": "hauling", "fleet-partner-service": "fleet-partner"}

def document(owner, domain):
    opaque = string(pattern="^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$")
    scope = obj({k: string() for k in ("product", "market", "zone", "organization", "tenant")})
    binding = obj({"fingerprint": string(pattern="^[0-9a-f]{64}$"), "source_contract": opaque, "known_source_dimensions": scope})
    approval = obj({**{k: opaque for k in ("id", "owner", "environment", "state", "resource_kind", "resource_ref", "source_contract", "provenance_ref", "approval_ref")}, "schema_version": {"const": 1}, "revision": {"type": "integer", "minimum": 1}, "binding_fingerprint": string(pattern="^[0-9a-f]{64}$"), "scope": scope, "capabilities": {"type": "array", "items": opaque, "minItems": 1, "maxItems": 32}, **{k: string(format="date-time") for k in ("effective_at", "expires_at")}})
    envelope = obj({"document": approval, "key_id": opaque, "signature": string()})
    schemes = {"ServiceHMAC": {"type": "apiKey", "in": "header", "name": "X-Service-Signature", "description": "HMAC v2 over exact body/path/query, X-Service-Name, X-Service-Timestamp, X-Service-Nonce and X-Service-Signature-Version. Durable nonce protection. Read caller admin-backoffice-service; publisher is the separately credentialed owner-authorization-context-publisher. Publication additionally requires an independent Ed25519 owner signature over the exact document bytes."}}
    base = "/internal/" + domain + "/v1/admin"
    parameter = lambda name, schema=opaque, where="query", required=True: {"name": name, "in": where, "required": required, "schema": schema}
    pathparams = [parameter(k, where="path") for k in ("kind", "id")]
    query = [parameter(k) for k in ("market", "zone", "organization", "tenant", "staff", "role", "policy_id", "purpose")]
    query += [parameter("policy_version", {"type": "integer", "minimum": 1}), parameter("policy_checksum", string(pattern="^[0-9a-f]{64}$"))]
    query += [parameter(k, string(format="date-time")) for k in ("start", "end")]
    query += [parameter("limit", {"type": "integer", "minimum": 1, "maximum": 100}), parameter("cursor", string(maxLength=4096), required=False)]
    def operation(name, parameters, response):
        return {"operationId": domain + ".admin." + name, "description": "Default disabled. Exact owner authorization and approved policy intersection, immutable audit before disclosure. No domain mutation authority.", "parameters": parameters, "responses": {"200": {"description": "Authorized source response", "content": {"application/json": {"schema": response}}}, **{str(code): {"description": "Sanitized denial; no source data"} for code in (400, 401, 403, 404, 409, 413, 429, 503)}}}
    snapshot = build()["openapi.json"]["components"]["schemas"]["OwnerResourcesSnapshot"]
    paths = {base + "/resources/{kind}": {"get": operation("resources.list", pathparams[:1] + query, snapshot)}, base + "/resources/{kind}/{id}": {"get": operation("resources.detail", pathparams + query, snapshot)}, base + "/authorization-contexts/bindings/{kind}/{id}": {"get": operation("contexts.prepare", pathparams + [parameter("purpose")], obj({"state": {"const": "DRAFT_PENDING_OWNER_APPROVAL"}, "binding": binding}))}}
    publish = operation("contexts.publish", [], obj({"status": {"const": "RECORDED"}}))
    publish["requestBody"] = {"required": True, "content": {"application/json": {"schema": envelope}}}
    paths[base + "/authorization-contexts"] = {"post": publish}
    return {"openapi": "3.1.0", "info": {"title": owner + " private Admin resource authority", "version": "1.0.0"}, "security": [{"ServiceHMAC": []}], "paths": paths, "components": {"securitySchemes": schemes}}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    for owner, domain in OWNERS.items():
        doc = document(owner, domain)
        validate_spec(doc)
        target = ROOT / "services" / owner / "docs/openapi/admin-owner-resources.openapi.json"
        content = (json.dumps(doc, indent=2) + "\n").encode()
        if args.write:
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(content)
        else:
            assert target.read_bytes() == content, f"Source contract drift: {owner}"
    print("Five source authority OpenAPI contracts validated")

if __name__ == "__main__":
    main()
