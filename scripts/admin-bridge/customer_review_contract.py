"""Pin the Customer-owned review-intake API; with a separate scoped Bridge orchestration contract."""
import work_item_contracts
import copy
import argparse
import json
from pathlib import Path
from contracts import obj, string
from openapi_spec_validator import validate_spec
import jsonschema

ROOT = Path(__file__).resolve().parents[2]
TARGET = ROOT / "services/customer-service/docs/openapi/admin-review-requests.openapi.json"

def build():
    opaque = string(pattern="^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$")
    identifier = string(format="uuid", pattern="^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
    segment = string(pattern="^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$")
    scope = obj({"product": {"const": "karryam"}, **{k: opaque for k in ("market", "zone", "organization", "tenant")}})
    request = obj({"contract_version": {"const": "customer.admin-review-request.v1"}, "request_id": identifier, "customer_id": identifier, "expected_review_revision": {"type": "integer", "minimum": 1, "maximum": 9223372036854775806}, "idempotency_key": segment, "reason_code": opaque, "requested_at": string(format="date-time")})
    receipt = obj({"contract_version": {"const": "customer.admin-review-request.v1"}, "status": {"const": "REVIEW_REQUEST_RECORDED"}, "review_request_id": identifier, "request_id": identifier, "customer_id": identifier, "idempotency_key": segment, "intent_digest": string(pattern="^[0-9a-f]{64}$"), "recorded_review_revision": {"type": "integer", "minimum": 2}, "scope": scope, "recorded_at": string(format="date-time")})
    parameter = lambda key, schema=opaque, where="query": {"name": key, "in": where, "required": True, "schema": schema}
    query = [parameter(k) for k in ("market", "zone", "organization", "tenant", "staff", "role", "purpose", "policy_id")]
    query += [parameter("policy_version", {"type": "integer", "minimum": 1}), parameter("policy_checksum", string(pattern="^[0-9a-f]{64}$"))]
    query += [parameter(k, string(format="date-time")) for k in ("start", "end")]
    query += [parameter("limit", {"type": "integer", "minimum": 1, "maximum": 100})]
    errors = {str(status): {"description": "Sanitized denial or unavailable outcome. A 404 on readback is not terminal absence proof; never automatically repost."} for status in (400, 401, 403, 404, 409, 413, 415, 503)}
    success = {"description": "Immutable proof that Customer recorded an owner review work item. No account, verification, restriction, privacy fulfillment, Trip or Wallet outcome is asserted.", "content": {"application/json": {"schema": receipt}}}
    base = "/internal/customer/v1/admin/account-review-requests"
    post = {"operationId": "customer.admin.review.request", "description": "Default off. Distinct write key, approved staff permission customer.review.request, source-owner allocation and native review revision guard. The revision versions only review intake. Original identity is durable and audit failure rolls back work. Bridge orchestration uses a separate durable no-retry fence and original-identity GET recovery.", "parameters": query, "requestBody": {"required": True, "content": {"application/json": {"schema": request}}}, "responses": {"200": success, **errors}}
    get = {"operationId": "customer.admin.review.readback", "description": "Default off. Distinct readback key and current customer.review.read permission/allocation; immutable historical receipt under the exact original scope. Write kill switch does not disable readback. No missing or malformed proof permits another POST.", "parameters": query + [parameter("customer", identifier, "path"), parameter("key", segment, "path")], "responses": {"200": success, **errors}}
    result={"openapi": "3.1.0", "info": {"title": "Customer private owner review intake", "version": "1.0.0"}, "security": [{"ServiceHMAC": []}], "paths": {base: {"post": post}, base + "/{customer}/{key}": {"get": get}}, "components": {"securitySchemes": {"ServiceHMAC": {"type": "apiKey", "in": "header", "name": "X-Service-Signature", "description": "Dedicated admin-backoffice-service HMAC v2 identity. Full service version/name/timestamp/nonce/signature headers, body and exact query; durable cross-replica nonce claims. Write and readback keys are distinct from each other and the resource-read key."}}}}
    schemas=work_item_contracts.build(obj,string,opaque,{"properties":{"receipt":{}}})
    work_post=copy.deepcopy(post);work_post["operationId"]="customer.admin.review.work_item.transition";work_post["description"]="Default-disabled source-owned queue ACKNOWLEDGE/CLOSE only; actual case/customer/original scope, expected work-item version, immutable transition audit and idempotency. No account-standing, consent or privacy fulfillment effect."
    work_post["requestBody"]["content"]["application/json"]["schema"]=schemas["CustomerWorkItemCommand"]
    work_post["responses"]["200"]["content"]["application/json"]["schema"]=schemas["CustomerWorkItemReceipt"]
    work_get=copy.deepcopy(get);work_get["operationId"]="customer.admin.review.work_item.readback";work_get["description"]="Current owner-authorized original immutable work-item transition receipt. GET only; does not resubmit an uncertain action."
    work_get["responses"]["200"]["content"]["application/json"]["schema"]=schemas["CustomerWorkItemReceipt"]
    work_base="/internal/customer/v1/admin/review-work-item-transitions"
    result["paths"][work_base]={"post":work_post};result["paths"][work_base+"/{customer}/{key}"]={"get":work_get}
    return result

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--write", action="store_true")
    parser.add_argument("--wire-dir", type=Path)
    args = parser.parse_args()
    data = build()
    validate_spec(data)
    content = (json.dumps(data, indent=2) + "\n").encode()
    if args.write:
        TARGET.write_bytes(content)
    else:
        assert TARGET.read_bytes() == content, "Customer review contract drift"
    if args.wire_dir:
        op = data["paths"]["/internal/customer/v1/admin/account-review-requests"]["post"]
        for filename, schema in (("customer-review-request.json", op["requestBody"]["content"]["application/json"]["schema"]), ("customer-review-receipt.json", op["responses"]["200"]["content"]["application/json"]["schema"])):
            actual = json.loads((args.wire_dir / filename).read_text(encoding="utf-8"))
            jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker()).validate(actual)
    if args.wire_dir:
        work=data["paths"]["/internal/customer/v1/admin/review-work-item-transitions"]["post"]
        for filename,schema in (("customer-work-item-command.json",work["requestBody"]["content"]["application/json"]["schema"]),("customer-work-item-receipt.json",work["responses"]["200"]["content"]["application/json"]["schema"])):
            jsonschema.Draft202012Validator(schema,format_checker=jsonschema.FormatChecker()).validate(json.loads((args.wire_dir/filename).read_text(encoding="utf-8")))
    print("Customer review request/readback OpenAPI and supplied actual wire output validate")

if __name__ == "__main__":
    main()
