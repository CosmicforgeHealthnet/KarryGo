"""Independent OpenAPI, JSON Schema, wire sample and deployment source checks."""
import report_contracts
import argparse
import copy
import json
import subprocess
from pathlib import Path

import jsonschema
import yaml
from openapi_spec_validator import validate_spec
from contracts import BASE, OUT, ROOT, main as regenerate_check

def load(path):
    return json.loads(path.read_text(encoding="utf-8"))

def check_compatibility(old, new):
    assert old["security"] == new["security"], "v1 authentication contract changed"
    assert old["components"]["securitySchemes"] == new["components"]["securitySchemes"], "v1 security profile changed"
    for path, methods in old["paths"].items():
        for method, operation in methods.items():
            candidate = new["paths"][path][method]
            assert operation["operationId"] == candidate["operationId"], "operation identity changed"
            assert operation["parameters"] == candidate["parameters"], "v1 scope/query contract changed"
            assert operation.get("requestBody") == candidate.get("requestBody"), "v1 request contract changed"
            assert operation["responses"].keys() <= candidate["responses"].keys(), "response status removed"
            for status, response in operation["responses"].items():
                assert response == candidate["responses"][status], "v1 response shape changed"
    # Conservative: any existing schema semantic change requires a reviewed new
    # version/overlap. New standalone schemas and operations are additive.
    for name, schema in old["components"]["schemas"].items():
        assert schema == new["components"]["schemas"][name], f"breaking v1 schema change: {name}"

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--wire-dir", type=Path)
    parser.add_argument("--base")
    args = parser.parse_args()
    api = load(OUT / "openapi.json")
    validate_spec(api)
    schemas = {p.stem: load(p) for p in (OUT / "schemas").glob("*.json")}
    validators = {}
    for name, schema in schemas.items():
        jsonschema.Draft202012Validator.check_schema(schema)
        validators[name] = jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker())
    matrix = load(OUT / "KARRYGO_ADMIN_CAPABILITY_MATRIX.json")
    for row in matrix:
        validators["Capability"].validate(row)
        operation = api["paths"][BASE + row["bridge_endpoint"]][row["method"].lower()]
        assert operation["x-capability-state"] == row["state"]
        assert ("200" in operation["responses"]) == (row["state"] in ("READ_ONLY", "AVAILABLE"))
    owner_kinds = {"customer": ["review-work-items", "customers", "consents", "booking-references", "review-states"], "taxi": ["providers", "vehicles", "independent-rides", "recoveries"], "dispatch": ["incident-recovery-status", "return-execution-status", "riders", "motorcycles", "missions", "custody-missions", "incidents", "delivery-recoveries", "returns", "handover-states"], "hauling": ["booking-cargo-inputs", "product-eligibility", "providers", "trucks", "missions", "custody-missions", "recoveries", "restrictions", "fit-decisions"], "fleet": ["shipments", "private-assignments", "execution-links"]}
    for domain, kinds in __import__("owner_resource_extensions").kinds_by_domain().items():
        owner_kinds[domain] += kinds
    canonical_resource_owners = {"customer": "customer-service", "taxi": "driver-taxi-service", "dispatch": "driver-dispatch-delivery-service", "hauling": "driver-hauling-service", "fleet": "fleet-partner-service"}
    for row in matrix:
        if ".resources." in row["id"]:
            domain = row["id"].split(".", 1)[0]
            assert row["owner"] == canonical_resource_owners[domain], f"Noncanonical owner in capability contract: {row['id']}"
    resource_ids = {f"{domain}.resources.{kind}.{operation}" for domain, kinds in owner_kinds.items() for kind in kinds for operation in ("list", "detail")}
    assert {r["id"] for r in matrix if r["state"] == "READ_ONLY"} == {"bridge.capabilities", "bridge.health", "analytics.scoped.mission-success.read", "analytics.scoped.mission-success-rate.read", "bridge.operations.read", "customer.review.readback", "taxi.rides.list", "taxi.rides.detail"} | {"customer.review.work_item.readback", "media.case.grant.issue", "media.case.content.read"} | resource_ids | {row["id"] for row in __import__("owner_command_contracts").matrix_entries() if row["state"] == "READ_ONLY"} | {report_contracts.permission(name) for name in report_contracts.REPORTS}
    if args.wire_dir:
        media_grant=args.wire_dir/"media-case-grant.json"
        if media_grant.exists():
            value=load(media_grant)
            validators["Response"].validate(value)
            validators["MediaCaseGrant"].validate(value["result"])
        for schema_name in ("MediaCaseIssueRequest","MediaCaseContentRequest","MediaCaseRevokeRequest"):
            assert schemas[schema_name]["additionalProperties"] is False
        assert "actor" not in schemas["MediaCaseIssueRequest"]["properties"]
        assert "scope" not in schemas["MediaCaseIssueRequest"]["properties"]
        assert "source_capability" not in schemas["MediaCaseIssueRequest"]["properties"]
        for name in report_contracts.REPORTS:
            schema=validators[report_contracts.schema_name(name)]
            for filename,envelope in [(f"scoped-analytics-{name}.json",True),(f"scoped-analytics-owner-{name}.json",False)]:
                path=args.wire_dir/filename
                if path.exists():
                    value=load(path)
                    schema.validate(value["result"] if envelope else value)
    if args.wire_dir:
        for suffix in ("unknown","confirmed"):
            path=args.wire_dir/f"customer-work-item-{suffix}.json"
            if path.exists():validators["CustomerWorkItemOperation"].validate(load(path)["result"])
    command = load(OUT / "fixtures/command.json")
    validators["Command"].validate(command)
    for mutate in [lambda c: c.update(performed_by="forged-admin"), lambda c: c.update(product="healthnet"), lambda c: c["target"].update(expected_version=0), lambda c: c.pop("idempotency_key"), lambda c: c["scope"].update(organization="*")]:
        invalid = copy.deepcopy(command)
        mutate(invalid)
        assert not validators["Command"].is_valid(invalid), "unsafe command fixture accepted"
    validators["StaffAssertion"].validate(load(OUT / "fixtures/staff-assertion.json"))
    validators["Response"].validate(load(OUT / "fixtures/unavailable-response.json"))
    if args.wire_dir:
        expected = {"discovery": "Discovery", "health": "Health", "operation": "Operation", "taxi": "TaxiSnapshot", "resources": "OwnerResourcesSnapshot", "denied": None, "blocked": None, "customer-review-unknown": "CustomerReviewOperation", "customer-review-confirmed": "CustomerReviewOperation", "scoped-analytics": "ScopedMissionSuccessRelease"}
        for name, result_schema in expected.items():
            wire = load(args.wire_dir / (name + ".json"))
            validators["Response"].validate(wire)
            if result_schema:
                assert "error" not in wire
                validators[result_schema].validate(wire["result"])
            else:
                assert wire["result"] is None and "error" in wire
    if args.base:
        result = subprocess.run(["git", "show", f"{args.base}:services/admin-backoffice-service/docs/openapi/openapi.json"], cwd=ROOT, capture_output=True)
        if result.returncode == 0:
            check_compatibility(json.loads(result.stdout), api)
        else:
            # Verify the ref itself exists; a missing ref is not an initial API.
            subprocess.run(["git", "rev-parse", "--verify", args.base], cwd=ROOT, check=True, capture_output=True)
            print("Base predates this initial v1 contract")

    if args.wire_dir:
        rate = load(args.wire_dir / "scoped-analytics-rate.json")
        validators["Response"].validate(rate)
        validators["ScopedMissionSuccessRateRelease"].validate(rate["result"])
        assert rate["result"]["rate"] is None and rate["result"]["coverage"] == "PARTIAL_COVERAGE"
    compose = yaml.safe_load((ROOT / "deploy/platform/compose/admin-backoffice-service.yml").read_text())
    app = compose["services"]["app"]
    assert app["ports"] == ["${ADMIN_PRIVATE_BIND_IP:-127.0.0.1}:8110:8110"]
    assert app["networks"] == ["admin-private"] and compose["networks"]["admin-private"]["internal"] is True
    assert app["read_only"] and app["cap_drop"] == ["ALL"] and app["security_opt"] == ["no-new-privileges:true"]
    assert all("ports" not in service for name, service in compose["services"].items() if name != "app")
    assert compose["services"]["migrate"]["env_file"] != app["env_file"]
    nginx = (ROOT / "deploy/platform/nginx/api-gateway.staging.karryam.com.conf").read_text()
    assert nginx.index("location ~* ^/api/v1/admin") < nginx.index("location /api/v1/")
    # Manual-only releases replace the old broad auto-deploy planner. Keep the
    # isolation invariant for BOTH deployment paths, not a historical grep.
    for name in ("deploy-platform.yml", "deploy-vendor-service.yml"):
        workflow = yaml.load((ROOT / ".github/workflows" / name).read_text(), Loader=yaml.BaseLoader)
        assert set(workflow["on"]) == {"workflow_dispatch"}
        assert set(workflow["on"]["workflow_dispatch"]["inputs"]) == {"release_sha", "services", "restore_run_id"}
        job = workflow["jobs"]["release"]
        assert job["environment"] == "staging"
        steps = job["steps"]
        check = next(i for i, s in enumerate(steps) if "gate.py --kind" in s.get("run", ""))
        deploy = next(i for i, s in enumerate(steps) if s.get("run") == "python scripts/release/execute.py deploy")
        assert check < deploy
        assert "secrets.VPS_" not in str(workflow)
    print(f"OpenAPI 3.1, {len(schemas)} schemas, fixtures, capability states and deployment source checks passed")

if __name__ == "__main__":
    main()
