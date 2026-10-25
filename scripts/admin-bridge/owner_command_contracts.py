"""Source-owned administrative command routes: one closed dictionary per owner.

Actor, scope, approval objects and resulting states are never request inputs:
HQ submits an intent and an approval identity; the owner decides the outcome.
"""
DOMAINS = {
 "vendor":("vendor-service","vendor.admin-command.v1","VENDOR"),
    "customer": ("customer-service", "customer.admin-command.v1", "CUSTOMER"),
    "taxi": ("driver-taxi-service", "taxi.admin-command.v1", "TAXI"),
    "dispatch": ("driver-dispatch-delivery-service", "dispatch.admin-command.v1", "DISPATCH"),
    "hauling": ("driver-hauling-service", "hauling.admin-command.v1", "HAULING"),
    "fleet": ("fleet-partner-service", "fleet.admin-command.v1", "FLEET_PARTNER"),
    "wallet": ("payment-wallet-service", "wallet.admin-command.v1", "PAYMENT_WALLET"),
    "referral": ("referral-service", "referral.admin-command.v1", "REFERRAL"),
}
ROUTES = [
    # suffix, bridge path, method, permission kind, state, risk
    ("submit", "/{d}/commands", "POST", "execute", "AVAILABLE", "R3"),
    ("readback", "/{d}/commands/{operation_id}", "GET", "read", "READ_ONLY", "R1"),
    ("history", "/{d}/command-history/{subject_kind}/{subject_id}", "GET", "read", "READ_ONLY", "R2"),
    ("approval.propose", "/{d}/command-approvals", "POST", "execute", "AVAILABLE", "R2"),
    ("approval.read", "/{d}/command-approvals/{approval_id}", "GET", "read", "READ_ONLY", "R1"),
    ("approval.decide", "/{d}/command-approvals/{approval_id}/decisions", "POST", "approve", "AVAILABLE", "R3"),
]
SOURCE = {"submit": "POST /internal/{d}/v1/admin/commands", "readback": "GET /internal/{d}/v1/admin/commands/{kind}/{id}/{key}", "history": "GET /internal/{d}/v1/admin/commands/{kind}/{id}", "approval.propose": "Bridge-local two-person approval", "approval.read": "Bridge-local two-person approval", "approval.decide": "Bridge-local two-person approval"}
SEGMENT = {"fleet": "fleet-partner", "wallet": "payment-wallet"}
FLAG = {"vendor":"ADMIN_VENDOR_WRITES_ENABLED","customer": "ADMIN_CUSTOMER_WRITES_ENABLED", "taxi": "ADMIN_TAXI_WRITES_ENABLED", "dispatch": "ADMIN_DISPATCH_RECOVERY_ENABLED", "hauling": "ADMIN_HAULING_RECOVERY_ENABLED", "fleet": "ADMIN_FLEET_WRITES_ENABLED", "wallet": "ADMIN_WALLET_WRITES_ENABLED", "referral": "ADMIN_REFERRAL_WRITES_ENABLED"}
TESTS = ["TestPostgresOwnerCommandTwoPersonApprovalIsBoundSingleUseAndRequesterOnly", "TestPostgresOwnerCommandUncertainOutcomeReconcilesWithoutResubmit", "TestPostgresOwnerCommandRefusalAndSupersessionReleaseOnlyProvenSlots", "TestPostgresOwnerCommandBridgeMTLSRoundTrip"]


def capability_id(domain, suffix):
    return f"{domain}.command.{suffix}"


def matrix_entries():
    rows = []
    for d, (owner, contract, prefix) in DOMAINS.items():
        for suffix, path, method, kind, state, risk in ROUTES:
            permission = f"{d}.admin.command.{kind}"
            flag = f"ADMIN_{prefix}_COMMAND_WRITES_ENABLED; {FLAG[d]}; ADMIN_REMOTE_WRITES_ENABLED" if method == "POST" else f"ADMIN_{prefix}_COMMAND_READBACK_ENABLED"
            rows.append({
                "id": capability_id(d, suffix), "version": "1.0", "owner": owner,
                "source_endpoint": SOURCE[suffix].replace("{d}", SEGMENT.get(d, d)), "bridge_endpoint": path.replace("{d}", d), "method": method,
                "state": state, "permission": permission,
                "scope": "product,market,zone,organization,tenant,staff,purpose; subject bound by current signed owner context",
                "risk": risk, "privacy": "MINIMIZED_DECISION_RECORD",
                "source_version_required": suffix == "submit", "idempotency_required": suffix in ("submit", "approval.propose"),
                "approval": "Owner-declared actions require a second, independently delegated staff approval bound to the exact intent digest; signed governance policy required; no production approval supplied.",
                "evidence": "Durable original identity, approval binding/consumption, owner version fence, immutable owner decision ledger, atomic Bridge audit.",
                "feature_flag": flag, "tests": TESTS,
                "blocker": f"Default disabled / BLOCKED_POLICY. Closed {contract} dictionary only: named owner transitions, never a generic state setter, trip/fare/custody/financial override.",
                "fallback": "Use original operation GET readback. UNKNOWN_OUTCOME is never retried; only an owner refusal or version supersession releases the slot.",
            })
    return rows


def schemas(obj, string, opaque, response_scope):
    identifier = string(format="uuid")
    token = string(pattern="^[A-Z][A-Z0-9_]{0,63}$")
    digest = string(pattern="^[0-9a-f]{64}$")
    contract = {"enum": [c for d, (_, c, _) in DOMAINS.items() if d not in ("vendor", "referral")]}
    params = {"type": "object", "maxProperties": 16, "propertyNames": {"pattern": "^[a-z][a-z0-9_]{0,63}$"}, "additionalProperties": string(minLength=1, maxLength=512)}
    version = {"type": "integer", "minimum": 0}
    submission = obj({"contract_version": contract, "operation_id": identifier, "subject_kind": opaque, "subject_id": opaque, "case_ref": {"anyOf": [opaque, {"const": ""}]}, "action": token, "expected_version": version, "idempotency_key": opaque, "reason_code": token, "params": params, "approval_id": {"anyOf": [identifier, {"const": ""}]}, "requested_at": string(format="date-time")})
    proposal = obj({"contract_version": contract, "approval_id": identifier, "subject_kind": opaque, "subject_id": opaque, "case_ref": {"anyOf": [opaque, {"const": ""}]}, "action": token, "expected_version": version, "reason_code": token, "params": params})
    decision = obj({"decision": {"enum": ["APPROVE", "REJECT"]}, "reason_code": token})
    receipt = obj({"contract_version": contract, "owner": opaque, "operation_id": identifier, "decision_id": identifier, "subject_kind": opaque, "subject_id": opaque, "case_ref": {"anyOf": [opaque, {"const": ""}]}, "action": token, "idempotency_key": opaque, "intent_digest": digest, "approval_id": {"anyOf": [identifier, {"const": ""}]}, "prior_state": token, "resulting_state": token, "recorded_version": {"type": "integer", "minimum": 1}, "outcome": token, "scope": response_scope, "recorded_at": string(format="date-time")})
    operation = obj({"operation_id": identifier, "owner": opaque, "action": token, "subject_kind": opaque, "subject_id": opaque, "case_ref": {"anyOf": [opaque, {"const": ""}]}, "expected_version": version, "approval_id": identifier, "scope": response_scope, "status": {"enum": ["EXECUTING", "UNKNOWN_OUTCOME", "CONFIRMED", "REJECTED"]}, "rejection_code": {"enum": ["owner_refused", "superseded_by_owner_version"]}, "created_at": string(format="date-time"), "as_of": string(format="date-time"), "receipt": receipt}, ["operation_id", "owner", "action", "subject_kind", "subject_id", "case_ref", "expected_version", "scope", "status", "created_at", "as_of"])
    operation["allOf"] = [
        {"if": {"properties": {"status": {"const": "CONFIRMED"}}}, "then": {"required": ["receipt"]}, "else": {"not": {"required": ["receipt"]}}},
        {"if": {"properties": {"status": {"const": "REJECTED"}}}, "then": {"required": ["rejection_code"]}, "else": {"not": {"required": ["rejection_code"]}}},
    ]
    history = obj({"contract_version": contract, "owner": opaque, "subject_kind": opaque, "subject_id": opaque, "current_version": version, "current_state": token, "decisions": {"type": "array", "maxItems": 50, "items": receipt}, "truncated": {"type": "boolean"}, "scope": response_scope, "as_of": string(format="date-time")})
    approval = obj({"approval_id": identifier, "owner": opaque, "action": token, "subject_kind": opaque, "subject_id": opaque, "case_ref": {"anyOf": [opaque, {"const": ""}]}, "expected_version": version, "intent_digest": digest, "status": {"enum": ["PENDING", "APPROVED", "REJECTED", "CONSUMED", "EXPIRED"]}, "scope": response_scope, "expires_at": string(format="date-time"), "decided_at": string(format="date-time"), "created_at": string(format="date-time")}, ["approval_id", "owner", "action", "subject_kind", "subject_id", "case_ref", "expected_version", "intent_digest", "status", "scope", "expires_at", "created_at"])
    base = {"OwnerCommandSubmission": submission, "OwnerCommandProposal": proposal, "OwnerCommandApprovalDecision": decision, "OwnerCommandReceipt": receipt, "OwnerCommandOperation": operation, "OwnerCommandHistory": history, "OwnerCommandApproval": approval}
    # New owner domains are additive contract families: do not widen the
    # existing owners' pinned v1 schema enums or alter their wire promises.
    import copy
    def specialize(value, domain):
        if isinstance(value, dict):
            if value == contract:
                value["enum"] = [DOMAINS[domain][1]]
            else:
                for child in value.values(): specialize(child, domain)
        elif isinstance(value, list):
            for child in value: specialize(child, domain)
    additions = {}
    for domain, prefix in (("vendor", "Vendor"), ("referral", "Referral")):
        family = copy.deepcopy(base)
        specialize(family, domain)
        additions.update({prefix + name: value for name, value in family.items()})
    return {**base, **additions}


def result_schema(identifier):
    suffix = identifier.split(".command.", 1)[1]
    prefix = "Vendor" if identifier.startswith("vendor.") else "Referral" if identifier.startswith("referral.") else ""
    return prefix + {"submit": "OwnerCommandOperation", "readback": "OwnerCommandOperation", "history": "OwnerCommandHistory", "approval.propose": "OwnerCommandApproval", "approval.read": "OwnerCommandApproval", "approval.decide": "OwnerCommandApproval"}[suffix]


def request_schema(identifier):
    suffix = identifier.split(".command.", 1)[1]
    prefix = "Vendor" if identifier.startswith("vendor.") else "Referral" if identifier.startswith("referral.") else ""
    return prefix + {"submit": "OwnerCommandSubmission", "approval.propose": "OwnerCommandProposal", "approval.decide": "OwnerCommandApprovalDecision"}[suffix]


def is_command(identifier):
    return ".command." in identifier and identifier.split(".", 1)[0] in DOMAINS


if __name__ == "__main__":
    import json
    from pathlib import Path
    path = Path(__file__).resolve().parents[2] / "services/admin-backoffice-service/docs/openapi/KARRYGO_ADMIN_CAPABILITY_MATRIX.json"
    matrix = json.loads(path.read_text(encoding="utf-8"))
    matrix = [row for row in matrix if not is_command(row["id"])] + matrix_entries()
    path.write_bytes((json.dumps(matrix, indent=2, ensure_ascii=False) + "\n").encode("utf-8"))
    print(f"{len(matrix)} capability entries")
