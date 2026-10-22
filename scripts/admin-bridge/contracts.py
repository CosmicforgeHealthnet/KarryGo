"""Validate or regenerate the pinned OpenAPI/JSON Schema bundle. No network needed."""
import work_item_contracts
import report_contracts
import argparse
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "services/admin-backoffice-service/docs/openapi"
BASE = "/internal/karryam-admin/v1"
STATES = ["PENDING", "EXECUTING", "CONFIRMED", "REJECTED", "FAILED_RETRYABLE", "FAILED_TERMINAL", "UNKNOWN_OUTCOME", "CANCELLED_WHEN_SAFE"]
FRESHNESS = ["LIVE", "RECENT", "STALE", "PARTIAL", "UNKNOWN", "UNAVAILABLE"]

def obj(properties, required=None):
    return {"type": "object", "additionalProperties": False, "properties": properties, "required": list(properties) if required is None else required}

def string(**kw):
    return {"type": "string", **kw}

def custody_schema(opaque):
    positive = {"type": "integer", "minimum": 1}
    nonnegative = {"type": "integer", "minimum": 0}
    timestamp = string(format="date-time")
    optional = {"anyOf": [opaque, {"type": "null"}]}
    array = lambda item: {"type": "array", "maxItems": 100, "items": item}
    legacy = obj({"reference": opaque, "version": positive, "pickup_stop": opaque, "package_count": positive, "parcel_references": {"type": "array", "minItems": 1, "maxItems": 100, "items": opaque}, "frozen_at": timestamp})
    stop = obj({"reference": opaque, "type": opaque, "sequence": positive, "status": opaque, "version": positive})
    manifest = obj({"reference": opaque, "manifest_version": positive, "pickup_stop": optional, "authority": opaque, "package_count": positive, "weight_grams": nonnegative, "volume_ml": nonnegative, "required": {"type": "boolean"}, "status": opaque, "custodian": optional, "version": positive})
    package = obj({"reference": opaque, "manifest": opaque, "manifest_version": positive, "package_count": positive, "weight_grams": nonnegative, "volume_ml": nonnegative, "status": opaque})
    holding = obj({"reference": opaque, "manifest": opaque, "manifest_version": positive, "custodian": opaque, "acquired_via": opaque, "acquired_at": timestamp, "release_reason": optional, "released_at": {"type": ["string", "null"], "format": "date-time"}})
    transfer = obj({"operation": opaque, "manifest": opaque, "manifest_version": positive, "from": opaque, "to": opaque, "package_count": positive, "at": timestamp})
    return obj({"contract_version": {"const": "custody.inventory.v1"}, "authority": opaque, "mode": {"enum": ["LEGACY_SINGLE", "MULTI_MANIFEST"]}, "legacy_manifest": {"anyOf": [legacy, {"type": "null"}]}, "stops": array(stop), "manifests": array(manifest), "packages": array(package), "holdings": array(holding), "transfers": array(transfer), "truncated": {"type": "boolean"}})

def build():
    opaque = string(pattern=r"^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$", maxLength=128)
    scope = obj({"market": opaque, "organization": opaque})
    target = obj({"type": opaque, "id": opaque, "expected_version": {"type": "integer", "minimum": 1}})
    command = obj({"contract_version": {"const": "1.0"}, "command_id": string(format="uuid"), "product": {"const": "karryam"}, "action": opaque, "target": target, "scope": scope, "reason_code": opaque, "reason": string(minLength=1, maxLength=512), "case_reference": opaque, "idempotency_key": opaque, "approval_reference": opaque, "requested_at": string(format="date-time")}, ["contract_version", "command_id", "product", "action", "target", "scope", "reason_code", "reason", "case_reference", "idempotency_key", "requested_at"])
    error = obj({"code": string(pattern="^[a-z_]+$"), "message": string(maxLength=128), "retryable": {"type": "boolean"}, "operation_id": string(format="uuid")}, ["code", "message", "retryable"])
    response = obj({"contract_version": {"const": "1.0"}, "product": {"const": "karryam"}, "correlation_id": string(format="uuid"), "as_of": string(format="date-time"), "freshness": {"enum": FRESHNESS}, "result": {"type": ["object", "array", "null"]}, "error": error}, ["contract_version", "product", "correlation_id", "as_of", "freshness", "result"])
    operation = obj({"operation_id": string(format="uuid"), "action": opaque, "target": target, "scope": scope, "source_version": {"type": ["integer", "null"], "minimum": 1}, "status": {"enum": STATES}, "created_at": string(format="date-time"), "as_of": string(format="date-time")})
    timestamp = {"type": "integer", "minimum": 1}
    staff = obj({"iss": string(minLength=1), "sub": opaque, "aud": string(minLength=1), "product": {"const": "karryam"}, **{k: {"type": "array", "minItems": 1, "maxItems": 128, "uniqueItems": True, "items": opaque} for k in ("roles", "permissions", "market_scopes", "organization_scopes")}, "session_id": opaque, "auth_time": timestamp, "acr": string(minLength=1), "jti": opaque, "iat": timestamp, "nbf": timestamp, "exp": timestamp, "request_digest": string(pattern="^[0-9a-f]{64}$")})
    availability = ["AVAILABLE", "READ_ONLY", "DISABLED", "BLOCKED_DEPENDENCY", "UNAVAILABLE"]
    capability = obj({**{k: string() for k in ["id", "version", "owner", "source_endpoint", "bridge_endpoint", "method", "permission", "scope", "privacy", "approval", "evidence", "feature_flag", "blocker", "fallback"]}, "state": {"enum": availability}, "risk": {"enum": ["R0", "R1", "R2", "R3", "R4"]}, **{k: {"type": "boolean"} for k in ["source_version_required", "idempotency_required"]}, "tests": {"type": "array", "items": string(), "minItems": 1}})
    discovery = obj({"contract_version": {"const": "1.0"}, "openapi_sha256": string(pattern="^[0-9a-f]{64}$"), "capabilities": {"type": "array", "items": capability}})
    health = obj({"bridge": {"const": "READY"}, "owners": {"const": "UNAVAILABLE"}, "remote_writes": {"const": "DISABLED"}, "contract_version": {"const": "1.0"}})
    disabled = obj({"contract_version": {"const": "1.0"}, "product": {"const": "karryam"}, "scope": scope, "reason_code": opaque, "idempotency_key": opaque})
    schemas = {"Command": command, "Response": response, "Error": error, "Operation": operation, "StaffAssertion": staff, "Capability": capability, "Discovery": discovery, "Health": health, "DisabledRequest": disabled}
    # Existing schemas remain byte-for-byte compatible. Domain reads use the
    # additional assertion profile, requiring explicit geographic delegation.
    scoped_staff = obj({**staff["properties"], **{key: {"type": "array", "minItems": 1, "maxItems": 128, "uniqueItems": True, "items": opaque} for key in ("zone_scopes", "tenant_scopes")}})
    ride = obj({"id": string(format="uuid"), "status": opaque, "mode": opaque, "source_context": opaque, "source_version": {"type": "null"}, "updated_at": string(format="date-time"), "created_at": string(format="date-time")})
    event = obj({"id": string(format="uuid"), "type": opaque, "at": string(format="date-time")})
    snapshot = obj({"contract_version": {"const": "1.0"}, "scope": obj({"market": opaque, "zone": opaque, "organization": opaque, "tenant": opaque}), "policy_id": opaque, "policy_version": {"type": "integer", "minimum": 1}, "policy_checksum": string(pattern="^[0-9a-f]{64}$"), "as_of": string(format="date-time"), "freshness": {"const": "LIVE"}, "coverage": {"const": "PARTIAL_COVERAGE"}, "items": {"type": "array", "maxItems": 100, "items": ride}, "events": {"type": "array", "maxItems": 200, "items": event}, "history_truncated": {"type": "boolean"}, "next_cursor": string(maxLength=4096)}, ["contract_version", "scope", "policy_id", "policy_version", "policy_checksum", "as_of", "freshness", "coverage", "items", "events", "history_truncated"])
    schemas.update(StaffAssertionZones=scoped_staff, TaxiSnapshot=snapshot)
    nullable_opaque = {"anyOf": [opaque, {"type": "null"}]}
    dimension = obj({"name": opaque, "value": nullable_opaque, "flag": {"type": ["boolean", "null"]}})
    counter = obj({"name": opaque, "value": {"type": ["integer", "null"], "minimum": 0}})
    reference = obj({"kind": opaque, "reference": opaque})
    timeline = obj({"reference": opaque, "type": opaque, "at": {"type": ["string", "null"], "format": "date-time"}})
    resource = obj({"reference": opaque, "kind": opaque, "status": nullable_opaque, "source_version": {"type": ["integer", "null"], "minimum": 1}, "snapshot_etag": string(pattern="^[0-9a-f]{64}$"), "created_at": string(format="date-time"), "updated_at": string(format="date-time"), "dimensions": {"type": "array", "maxItems": 16, "items": dimension}, "counters": {"type": "array", "maxItems": 16, "items": counter}, "references": {"type": "array", "maxItems": 16, "items": reference}, "timeline": {"type": "array", "maxItems": 200, "items": timeline}, "timeline_truncated": {"type": "boolean"}})
    # Detail-only source enrichment, authorized by each owner's admin-custody.v2.
    # Adding this optional shape grants nothing under a prior v1 source rule.
    resource["properties"]["custody"] = custody_schema(opaque)
    resource["allOf"] = [{"if": {"properties": {"kind": {"const": "custody-missions"}}}, "else": {"not": {"required": ["custody"]}}}]
    resources = obj({"contract_version": {"const": "karryam.owner-resources.v1"}, "owner": opaque, "kind": opaque, "scope": obj({"product": {"const": "karryam"}, "market": opaque, "zone": opaque, "organization": opaque, "tenant": opaque}), "policy_id": opaque, "policy_version": {"type": "integer", "minimum": 1}, "policy_checksum": string(pattern="^[0-9a-f]{64}$"), "as_of": string(format="date-time"), "coverage": {"const": "CONTEXT_SCOPED_PARTIAL"}, "items": {"type": "array", "maxItems": 100, "items": resource}, "next_cursor": {"type": ["string", "null"], "maxLength": 4096}})
    schemas.update(OwnerResourcesSnapshot=resources)
    rate = obj({"policy_id": opaque, "policy_version": {"type": "integer", "minimum": 1}, "policy_checksum": string(pattern="^[0-9a-f]{64}$"), "scope": obj({"product": {"const": "karryam"}, **{k: opaque for k in ("market", "zone", "organization", "tenant")}}), "metric": {"const": "mission-success-rate"}, "source_version": opaque, "as_of": string(format="date-time"), "freshness": {"enum": ["LIVE", "STALE"]}, "coverage": {"enum": ["COMPLETE", "PARTIAL_COVERAGE"]}, "formula": {"const": "completed/(completed+failed)"}, "basis": {"const": "DISCLOSED_COMPONENT_COUNTS"}, "rate": {"type": ["number", "null"], "minimum": 0, "maximum": 1}, "suppression": {"enum": ["NONE", "PARTIAL_COVERAGE", "STALE", "SOURCE_SUPPRESSED", "NO_DENOMINATOR"]}})
    rate["allOf"] = [{"if": {"properties": {"suppression": {"const": "NONE"}}}, "then": {"properties": {"rate": {"type": "number"}, "freshness": {"const": "LIVE"}, "coverage": {"const": "COMPLETE"}}}, "else": {"properties": {"rate": {"type": "null"}}}}]
    schemas["ScopedMissionSuccessRateRelease"] = rate

    source_review = json.loads((ROOT / "services/customer-service/docs/openapi/admin-review-requests.openapi.json").read_text(encoding="utf-8"))["paths"]["/internal/customer/v1/admin/account-review-requests"]["post"]
    review_request = source_review["requestBody"]["content"]["application/json"]["schema"]
    review_receipt = source_review["responses"]["200"]["content"]["application/json"]["schema"]
    review_operation = obj({"operation_id": string(format="uuid"), "customer_id": string(format="uuid"), "scope": review_receipt["properties"]["scope"], "status": {"enum": ["EXECUTING", "UNKNOWN_OUTCOME", "CONFIRMED"]}, "created_at": string(format="date-time"), "as_of": string(format="date-time"), "receipt": review_receipt}, ["operation_id", "customer_id", "scope", "status", "created_at", "as_of"])
    review_operation["allOf"] = [{"if": {"properties": {"status": {"const": "CONFIRMED"}}}, "then": {"required": ["receipt"]}, "else": {"not": {"required": ["receipt"]}}}]
    schemas.update(CustomerReviewRequest=review_request, CustomerReviewOperation=review_operation)
    schemas.update(work_item_contracts.build(obj,string,opaque,review_operation))
    released_cell = obj({"key": {"enum": ["completed", "failed", "cancelled", "guarantee_breaches"]}, "value": {"type": ["integer", "null"], "minimum": 0}, "suppression": {"enum": ["NONE", "UNKNOWN", "SMALL_CELL", "COMPLEMENTARY", "PARTIAL_COVERAGE", "STALE"]}})
    released_cell["allOf"] = [{"if": {"properties": {"suppression": {"const": "NONE"}}}, "then": {"properties": {"value": {"type": "integer"}}}, "else": {"properties": {"value": {"type": "null"}}}}]
    released_cells = {"type": "array", "minItems": 4, "maxItems": 4, "items": released_cell, "allOf": [{"contains": {"properties": {"key": {"const": key}}, "required": ["key"]}, "minContains": 1, "maxContains": 1} for key in ("completed", "failed", "cancelled", "guarantee_breaches")]}
    release = obj({"policy_id": opaque, "policy_version": {"type": "integer", "minimum": 1}, "policy_checksum": string(pattern="^[0-9a-f]{64}$"), "scope": review_receipt["properties"]["scope"], "metric": {"const": "mission-success"}, "source_version": opaque, "as_of": string(format="date-time"), "freshness": {"enum": ["LIVE", "STALE"]}, "coverage": {"enum": ["COMPLETE", "PARTIAL_COVERAGE"]}, "cells": released_cells})
    release["allOf"] = [{"if": {"anyOf": [{"properties": {"coverage": {"const": "PARTIAL_COVERAGE"}}}, {"properties": {"freshness": {"const": "STALE"}}}]}, "then": {"properties": {"cells": {"items": {"properties": {"value": {"type": "null"}, "suppression": {"not": {"const": "NONE"}}}}}}}}]
    schemas["ScopedMissionSuccessRelease"] = release
    schemas.update(report_contracts.schemas(release))
    matrix = json.loads((OUT / "KARRYGO_ADMIN_CAPABILITY_MATRIX.json").read_text())
    paths = {}
    error_response = {"description": "Sanitized error envelope. Never retry a domain command without authoritative outcome resolution.", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Response"}}}}
    for c in matrix:
        params = [{"name": k, "in": "query", "required": True, "schema": opaque} for k in ("market", "organization", "purpose")]
        if c["permission"] == "taxi.rides.read" or ".resources." in c["id"] or c["id"].startswith("customer.review.") or c["id"].startswith("analytics.scoped."):
            params += [{"name": key, "in": "query", "required": True, "schema": opaque} for key in ("zone", "tenant")]
            params += [{"name": k, "in": "query", "required": True, "schema": string(format="date-time")} for k in ("start", "end")]
            params += [{"name": "limit", "in": "query", "required": True, "schema": {"type": "integer", "minimum": 1, "maximum": 100}}, {"name": "cursor", "in": "query", "required": False, "schema": string(maxLength=4096)}]
        if c["id"].startswith("analytics.scoped."):
            params = [p for p in params if p["name"] not in ("limit", "cursor")]
            params += [{"name": key, "in": "query", "required": True, "schema": opaque} for key in ("authority", "product_code")]
        for segment in c["bridge_endpoint"].split("/"):
            if segment.startswith("{"):
                params.append({"name": segment[1:-1], "in": "path", "required": True, "schema": opaque})
        op = {"operationId": c["id"], "summary": c["id"], "description": c["blocker"] or "Functional bridge metadata; scoped and audited.", "x-capability-state": c["state"], "x-owner": c["owner"], "x-required-permission": c["permission"], "parameters": params, "responses": {str(status): error_response for status in [400, 401, 403, 404, 409, 413, 415, 429, 503]}}
        if c["state"] in ("READ_ONLY", "AVAILABLE"):
            result = "ScopedMissionSuccessRateRelease" if c["id"] == "analytics.scoped.mission-success-rate.read" else "ScopedMissionSuccessRelease" if c["id"] == "analytics.scoped.mission-success.read" else report_contracts.schema_name(c["bridge_endpoint"].rsplit("/",1)[-1]) if c["id"].startswith("analytics.scoped.") else "CustomerWorkItemOperation" if c["id"].startswith("customer.review.work_item.") else "CustomerReviewOperation" if c["id"].startswith("customer.review.") else "OwnerResourcesSnapshot" if ".resources." in c["id"] else {"bridge.capabilities": "Discovery", "bridge.health": "Health", "bridge.operations.read": "Operation", "taxi.rides.list": "TaxiSnapshot", "taxi.rides.detail": "TaxiSnapshot"}[c["id"]]
            success_schema = {**response, "properties": {**response["properties"], "result": {"$ref": f"#/components/schemas/{result}"}}, "not": {"required": ["error"]}}
            if ".resources." in c["id"] and c["id"].endswith(".list"):
                metadata = {"properties": {"items": {"items": {"not": {"required": ["custody"]}, "properties": {"timeline": {"maxItems": 0}, "timeline_truncated": {"const": False}}}}}}
                success_schema["properties"]["result"] = {"allOf": [{"$ref": f"#/components/schemas/{result}"}, metadata]}
            if c["id"] in ("dispatch.resources.custody-missions.detail", "hauling.resources.custody-missions.detail"):
                success_schema["properties"]["result"] = {"allOf": [{"$ref": f"#/components/schemas/{result}"}, {"properties": {"items": {"items": {"required": ["custody"]}}}}]}
            op["responses"]["200"] = {"description": "Authorized owner snapshot." if c["permission"] == "taxi.rides.read" else "Authorized local metadata.", "content": {"application/json": {"schema": success_schema}}}
        if c["method"] == "POST":
            op["requestBody"] = {"required": True, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/" + ("CustomerWorkItemCommand" if c["id"]=="customer.review.work_item.submit" else "CustomerReviewRequest" if c["id"] == "customer.review.submit" else "Command" if c["id"] == "bridge.operations.submit" else "DisabledRequest")}}}}
        paths.setdefault(BASE + c["bridge_endpoint"], {})[c["method"].lower()] = op
    api = {"openapi": "3.1.0", "info": {"title": "Karryam private HQ bridge", "version": "1.0.0", "description": "Private transport plus mTLS, HMAC v2 and delegated Ed25519 JWT are all mandatory. Owner review intake is compiled and default off; all production access requires approved governance and current source authorization."}, "paths": paths, "security": [{"WorkloadMTLS": [], "ServiceHMAC": [], "StaffAssertion": []}], "components": {"securitySchemes": {"WorkloadMTLS": {"type": "mutualTLS"}, "ServiceHMAC": {"type": "apiKey", "in": "header", "name": "X-Service-Signature", "description": "HMAC v2 profile in signing-profile.md; requires all X-Service-* headers and X-HQ-Key-ID."}, "StaffAssertion": {"type": "http", "scheme": "bearer", "bearerFormat": "EdDSA JWT"}}, "schemas": schemas}}
    files = {"openapi.json": api}
    files.update({f"schemas/{name}.json": {"$schema": "https://json-schema.org/draft/2020-12/schema", **schema} for name, schema in schemas.items()})
    return files

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    files = build()
    manifest = []
    for name, data in files.items():
        content = (json.dumps(data, indent=2, ensure_ascii=False) + "\n").encode()
        p = OUT / name
        if args.write:
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_bytes(content)
        elif not p.exists() or p.read_bytes() != content:
            raise SystemExit(f"Contract drift: {name}; regenerate with --write and review version policy")
        manifest.append(f"{hashlib.sha256(content).hexdigest()}  {name}")
    matrix = OUT / "KARRYGO_ADMIN_CAPABILITY_MATRIX.json"
    manifest.append(f"{hashlib.sha256(matrix.read_bytes()).hexdigest()}  {matrix.name}")
    for fixture in sorted((OUT / "fixtures").glob("*.json")):
        manifest.append(f"{hashlib.sha256(fixture.read_bytes()).hexdigest()}  fixtures/{fixture.name}")
    profile = OUT / "signing-profile.md"
    manifest.append(f"{hashlib.sha256(profile.read_bytes()).hexdigest()}  {profile.name}")
    content = ("\n".join(manifest) + "\n").encode()
    if args.write:
        (OUT / "SHA256SUMS").write_bytes(content)
    elif (OUT / "SHA256SUMS").read_bytes() != content:
        raise SystemExit("Contract checksum drift")
    print(f"Contract deterministic generation and checksums verified ({len(manifest)} artifacts)")

if __name__ == "__main__":
    main()
