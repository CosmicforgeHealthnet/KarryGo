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
    case_kinds = {"customer-service": ["review-work-items"], "driver-taxi-service": ["recoveries"], "driver-dispatch-delivery-service": ["incidents", "delivery-recoveries", "returns"], "driver-hauling-service": ["recoveries"]}.get(owner)
    if case_kinds:
        case_base = base + "/case-authority"
        proof = obj({"contract_version": {"const": "karryam.source-case-authorization.v1"}, "owner": {"const": owner}, "case_kind": {"enum": case_kinds}, **{k: opaque for k in ("case_reference", "subject_type", "subject_reference")}, "scope": scope, "source_contract": {"const": domain + ".admin-case-evidence.v1"}, "native_fingerprint": string(pattern="^[0-9a-f]{64}$"), "expires_at": string(format="date-time"), "policy_id": opaque, "policy_version": {"type":"integer", "minimum":1}, "policy_checksum": string(pattern="^[0-9a-f]{64}$")})
        case_query = [p for p in query if p["name"] not in ("limit", "cursor")] + [parameter("limit", {"const": 1})]
        case_parameters = [parameter("kind", {"enum": case_kinds}, "path"), parameter("id", where="path"), parameter("subject", where="path"), parameter("reference", where="path")] + case_query
        case_read = operation("case.subject-proof", case_parameters, proof)
        case_read["description"] = "Default disabled. Fixed Media source-authorizer workload audience and separate credential. Requires a distinct media-case:<kind> source-owner signed allocation, current native case status and exact native subject membership, five-dimensional scope, signed policy and immutable audit. Returns no evidence metadata, URLs or bytes. Expiry is bounded by verified source authorization and policy."
        case_read["security"] = [{"MediaSourceHMAC": []}]
        paths[case_base + "/proofs/{kind}/{id}/{subject}/{reference}"] = {"get": case_read}
        case_prepare = operation("case.contexts.prepare", pathparams + [parameter("purpose")], obj({"state": {"const": "DRAFT_PENDING_OWNER_APPROVAL"}, "binding": binding}))
        paths[case_base + "/authorization-contexts/bindings/{kind}/{id}"] = {"get": case_prepare}
        case_publish = operation("case.contexts.publish", [], obj({"status": {"const": "RECORDED"}}))
        case_publish["requestBody"] = {"required": True, "content": {"application/json": {"schema": envelope}}}
        paths[case_base + "/authorization-contexts"] = {"post": case_publish}
        schemes["MediaSourceHMAC"] = {"type":"apiKey", "in":"header", "name":"X-Service-Signature", "description":"HMAC v2; caller media-file-service-case-authorizer. Durable owner/case-proof nonce namespace, separate from admin-backoffice-service reads and owner publication. No alternate caller accepted."}
    doc = {"openapi": "3.1.0", "info": {"title": owner + " private Admin resource authority", "version": "1.0.0"}, "security": [{"ServiceHMAC": []}], "paths": paths, "components": {"securitySchemes": schemes}}
    if owner == "driver-hauling-service":
        doc["x-source-owned-resource-contracts"]={"product-eligibility":{"source_contract":"hauling.admin-product-eligibility.v1","permission":"hauling.product_eligibility.read","identity":"eligibility UUID:current native organization UUID","canonical_scope":"Matching ACTIVE hauling_organization_provider_links and hauling_organization_truck_links; actual truck/provider ownership; market/zone require independent owner allocation","record_window_usable":"Owning truckcapability.ProductEligibility.UsableAt at read time; does not include all assignment, fit, restriction or custody checks","source_version":None,"authority_source_revision":"Monotonic certification callback counter, not an aggregate mutation version","evidence":"Only inspection/certification reference presence flags; no private reference, object mapping or inspection conclusion","mutation_authority":False}}
        doc["x-source-owned-resource-contracts"]["booking-cargo-inputs"]={"source_contract":"hauling.admin-booking-cargo-inputs.v1","permission":"hauling.cargo_inputs.read","canonical_scope":"Actual booking planning organization/winner consistency; settlement organization is not authorization","source_version":None,"data":"Current recorded booking weight, preferred truck type and fragile-handling input used by native recovery; zero unstated weight is null; normalization reuses cargofit.FromKilograms","limits":"No frozen full requirement snapshot, invented dimensions, temperature, equipment, cargo classes, inspection conclusion or current fit decision","mutation_authority":False}
    if owner == "driver-dispatch-delivery-service":
        doc["x-source-owned-resource-contracts"]={"incident-recovery-status":{"source_contract":"dispatch.admin-incident-recovery-status.v1","permission":"dispatch.incident_recovery.read","canonical_scope":"Actual original Trip organization/tenant; native incident and provider links","source_version":None,"captured_trip_revision":"Captured native Trip revision, not an incident mutation counter","result":"Exact recorded recovery result; UNKNOWN cannot become success or permission to replay","private_fields":"No lease owners, free-text errors, credentials, fingerprints or custody evidence","mutation_authority":False},"return-execution-status":{"source_contract":"dispatch.admin-return-execution.v1","permission":"dispatch.return_execution.read","canonical_scope":"Actual original Trip, recovery action/attempt and native return:<trip> mission/booking/execution-link graph; customer/organization consistency","source_version":None,"assignment_state":"Exact native reconciliation outcome; attempt count is not a full mutation version","private_fields":"No wallet references, lease identity, raw route snapshot, custody secret or private evidence","mutation_authority":False}}
    return doc

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
