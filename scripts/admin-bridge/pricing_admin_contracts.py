"""Finite Secure Admin Section 14 Pricing contracts."""

OWNER = "pricing-service"
SOURCE = "/internal/pricing/v1/admin"


def _row(capability, source, bridge, method, permission, risk, state="AVAILABLE", version=False, idem=False, tests=None):
    write = method in ("POST", "PUT") and capability not in ("pricing.simulate", "pricing.resolve")
    return {
        "id": capability, "version": "1.0", "owner": OWNER,
        "source_endpoint": SOURCE + source, "bridge_endpoint": bridge,
        "method": method, "state": state, "permission": permission,
        "scope": "karryam + exact market + zone + organization + tenant; delegated staff and signed Pricing-owner policy",
        "risk": risk, "privacy": "MINIMIZED_PRICING_POLICY_METADATA",
        "source_version_required": version, "idempotency_required": idem,
        "approval": "Pricing-owned four-eyes approval is mandatory for activation; signed owner policy and Bridge authorization remain independent gates",
        "evidence": "Pricing-native immutable policy version, approval, activation, rollout or market-freeze row plus owner and Bridge audit",
        "feature_flag": "ADMIN_PRICING_READ_ENABLED" + ("; ADMIN_REMOTE_WRITES_ENABLED; ADMIN_PRICING_WRITES_ENABLED" if write else ""),
        "tests": tests or ["TestPricingAdminBridgeUsesFixedSignedRoutesAndDeniesUnsupportedActions", "TestPricingPolicyLifecycleRequiresFourEyesAndImmutableActivation"],
        "blocker": "Disabled by default; no production policy values, grants, thresholds or activation authority are installed.",
        "fallback": "Fail closed; no public route, arbitrary proxy, source-state setter or quote mutation.",
    }


def matrix_entries():
    return [
        _row("pricing.policy.versions.read", "/policies/{id}/versions", "/pricing/policies/{id}/versions", "GET", "pricing.policy.read", "R3", "READ_ONLY"),
        _row("pricing.policy.version.read", "/policy-versions/{id}", "/pricing/policy-versions/{id}", "GET", "pricing.policy.read", "R3", "READ_ONLY"),
        _row("pricing.policy.draft.create", "/policies/drafts", "/pricing/policies/drafts", "POST", "pricing.policy.edit", "R3", idem=True),
        _row("pricing.policy.draft.update", "/policy-versions/{id}", "/pricing/policy-versions/{id}", "PUT", "pricing.policy.edit", "R3", version=True, idem=True),
        _row("pricing.policy.submit", "/policy-versions/{id}/submit", "/pricing/policy-versions/{id}/submit", "POST", "pricing.policy.edit", "R3", version=True, idem=True),
        _row("pricing.policy.approve", "/policy-versions/{id}/approve", "/pricing/policy-versions/{id}/approve", "POST", "pricing.policy.approve", "R4", version=True, idem=True),
        _row("pricing.policy.reject", "/policy-versions/{id}/reject", "/pricing/policy-versions/{id}/reject", "POST", "pricing.policy.approve", "R4", version=True, idem=True),
        _row("pricing.policy.activate", "/policy-versions/{id}/activate", "/pricing/policy-versions/{id}/activate", "POST", "pricing.policy.activate", "R4", version=True, idem=True),
        _row("pricing.simulate", "/simulate", "/pricing/simulate", "POST", "pricing.simulate", "R2"),
        _row("pricing.resolve", "/resolve", "/pricing/resolve", "POST", "pricing.resolve", "R2"),
        _row("pricing.approvals.read", "/approvals", "/pricing/approvals", "GET", "pricing.approval.read", "R3", "READ_ONLY"),
        _row("pricing.rollouts.read", "/rollouts", "/pricing/rollouts", "GET", "pricing.rollout.read", "R2", "READ_ONLY"),
        _row("pricing.rollouts.manage", "/rollouts", "/pricing/rollouts", "POST", "pricing.rollout.manage", "R4", idem=True),
        _row("pricing.market.freeze.read", "/markets/{code}/freeze", "/pricing/markets/{code}/freeze", "GET", "pricing.market_freeze.read", "R3", "READ_ONLY"),
        _row("pricing.market.freeze", "/markets/{code}/freeze", "/pricing/markets/{code}/freeze", "POST", "pricing.market_freeze.manage", "R4", idem=True),
        _row("pricing.market.unfreeze", "/markets/{code}/unfreeze", "/pricing/markets/{code}/unfreeze", "POST", "pricing.market_freeze.manage", "R4", idem=True),
    ]


def schemas(obj, string, opaque):
    request = obj({
        "idempotency_key": opaque,
        "expected_body_hash": {"type": "string", "pattern": "^[0-9a-f]{64}$"},
        "kind": string, "geography_level": string, "geography_ref": string,
        "schema_version": {"type": "integer", "minimum": 1},
        "body": {"type": "object", "additionalProperties": True}, "clone_from": opaque,
        "effective_from": {"type": "string", "format": "date-time"}, "rollback": {"type": "boolean"},
        "version_id": opaque, "commission_version_id": opaque, "scope": string,
        "channel": string, "class": string, "distance_meters": {"type": "integer", "minimum": 0},
        "duration_seconds": {"type": "integer", "minimum": 0}, "traffic_duration_seconds": {"type": "integer", "minimum": 0},
        "route_confidence": string, "route_degraded": {"type": "boolean"}, "package_size": string,
        "service_tier": string, "weight_grams": {"type": "integer", "minimum": 0}, "helpers": {"type": "integer", "minimum": 0},
        "zone_id": opaque, "market_id": opaque, "state_id": opaque, "country_code": string, "as_of": {"type": "string", "format": "date-time"},
        "capability": string, "market_code": string, "mode": string,
    }, [])
    return {"PricingAdminRequest": request, "PricingAdminResult": {"type": "object", "additionalProperties": True}}


if __name__ == "__main__":
    import json
    from pathlib import Path
    path = Path(__file__).resolve().parents[2] / "services/admin-backoffice-service/docs/openapi/KARRYGO_ADMIN_CAPABILITY_MATRIX.json"
    matrix = json.loads(path.read_text(encoding="utf-8"))
    ids = {row["id"] for row in matrix_entries()}
    matrix = [row for row in matrix if row["id"] not in ids] + matrix_entries()
    path.write_bytes((json.dumps(matrix, indent=2, ensure_ascii=False) + "\n").encode("utf-8"))
    print(f"{len(matrix)} capability entries")
