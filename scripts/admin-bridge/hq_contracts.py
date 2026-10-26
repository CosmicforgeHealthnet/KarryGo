"""Sections 22-30 HQ department capabilities: event feed/acks, projection
health, unified support queue, two-person HQ approvals, campaign intent
orchestration, group finance reports, privacy disposition register and
controlled encrypted exports. Every route is default disabled and requires the
same mTLS + HMAC v2 + delegated staff assertion + signed policy rule naming the
exact owner and source contract (bridge/hq.go hqRoutes)."""

HQ_TESTS = ["TestPostgresEventInboxDedupeVersionFenceGapAndQuarantine", "TestPostgresBackfillReconciliationAndPartialCoverage", "TestPostgresHQBridgeRoutesEndToEnd"]

ROUTES = [
    # id, endpoint, method, owner, contract, state, risk, permission-flag, privacy, idempotent, request, result
    ("hq.events.feed", "/hq/events", "GET", "admin-backoffice-service", "karryam.admin-events.v1", "READ_ONLY", "R1", "ADMIN_HQ_EVENTS_ENABLED", "OPERATIONS_MINIMIZED", False, None, "HQEventFeed"),
    ("hq.events.ack", "/hq/event-acknowledgements", "POST", "admin-backoffice-service", "karryam.admin-events.v1", "AVAILABLE", "R1", "ADMIN_HQ_EVENTS_ENABLED", "OPERATIONS_MINIMIZED", True, "HQEventAck", "HQEventAckResult"),
    ("hq.projections.status", "/hq/projections/status", "GET", "admin-backoffice-service", "karryam.admin-events.v1", "READ_ONLY", "R0", "ADMIN_HQ_EVENTS_ENABLED", "INTERNAL", False, None, "HQProjectionStatus"),
    ("hq.support.queue", "/hq/support/cases", "GET", "support-dispute-service", "karryam.support-queue.v1", "READ_ONLY", "R1", "ADMIN_HQ_SUPPORT_QUEUE_ENABLED", "OPERATIONS_MINIMIZED", False, None, "HQSupportQueue"),
    ("hq.approvals.propose", "/hq/approvals", "POST", "admin-backoffice-service", "karryam.hq-approval.v1", "AVAILABLE", "R2", "ADMIN_HQ_APPROVALS_ENABLED; ADMIN_REMOTE_WRITES_ENABLED", "INTERNAL", True, "HQApprovalProposal", "HQApproval"),
    ("hq.approvals.read", "/hq/approvals/{approval_id}", "GET", "admin-backoffice-service", "karryam.hq-approval.v1", "READ_ONLY", "R1", "ADMIN_HQ_APPROVALS_ENABLED", "INTERNAL", False, None, "HQApproval"),
    ("hq.approvals.decide", "/hq/approvals/{approval_id}/decisions", "POST", "admin-backoffice-service", "karryam.hq-approval.v1", "AVAILABLE", "R3", "ADMIN_HQ_APPROVALS_ENABLED; ADMIN_REMOTE_WRITES_ENABLED", "INTERNAL", False, "HQApprovalDecision", "HQApproval"),
    ("hq.campaigns.submit", "/hq/campaigns", "POST", "notification-service", "karryam.campaign-intent.v1", "AVAILABLE", "R3", "ADMIN_HQ_CAMPAIGNS_ENABLED; ADMIN_HQ_CAMPAIGN_WRITES_ENABLED; ADMIN_REMOTE_WRITES_ENABLED; NOTIFICATION_CAMPAIGNS_ENABLED", "INTERNAL", True, "HQCampaignSubmission", "HQCampaignOperation"),
    ("hq.campaigns.readback", "/hq/campaign-operations/{operation_id}", "GET", "notification-service", "karryam.campaign-intent.v1", "READ_ONLY", "R1", "ADMIN_HQ_CAMPAIGNS_ENABLED", "INTERNAL", False, None, "HQCampaignOperation"),
    ("hq.campaigns.cancel", "/hq/campaign-operations/{operation_id}/cancellations", "POST", "notification-service", "karryam.campaign-intent.v1", "AVAILABLE", "R2", "ADMIN_HQ_CAMPAIGNS_ENABLED; ADMIN_HQ_CAMPAIGN_WRITES_ENABLED; ADMIN_REMOTE_WRITES_ENABLED", "INTERNAL", True, "HQCampaignCancellation", "HQCampaignOperation"),
    ("hq.finance.report", "/hq/finance/reports", "POST", "payment-wallet-service", "karryam.finance-report.v1", "READ_ONLY", "R1", "ADMIN_HQ_FINANCE_ENABLED; PAYMENT_WALLET_ADMIN_FINANCE_REPORT_ENABLED", "FINANCIAL_SENSITIVE", False, "HQFinanceRequest", "FinanceReport"),
    ("hq.privacy.register", "/hq/privacy-requests", "POST", "admin-backoffice-service", "karryam.privacy-register.v1", "AVAILABLE", "R2", "ADMIN_HQ_PRIVACY_ENABLED; ADMIN_REMOTE_WRITES_ENABLED", "RESTRICTED_PII_DIGEST_ONLY", True, "HQPrivacyRegistration", "HQPrivacyRequest"),
    ("hq.privacy.read", "/hq/privacy-requests/{request_id}", "GET", "admin-backoffice-service", "karryam.privacy-register.v1", "READ_ONLY", "R1", "ADMIN_HQ_PRIVACY_ENABLED", "INTERNAL", False, None, "HQPrivacyRequest"),
    ("hq.privacy.fulfilment", "/hq/privacy-requests/{request_id}/fulfilments", "POST", "admin-backoffice-service", "karryam.privacy-register.v1", "AVAILABLE", "R2", "ADMIN_HQ_PRIVACY_ENABLED; ADMIN_REMOTE_WRITES_ENABLED", "INTERNAL", False, "HQPrivacyFulfilment", "HQPrivacyRequest"),
    ("hq.exports.create", "/hq/exports", "POST", "admin-backoffice-service", "karryam.controlled-export.v1", "AVAILABLE", "R3", "ADMIN_HQ_EXPORTS_ENABLED; ADMIN_REMOTE_WRITES_ENABLED", "OPERATIONS_MINIMIZED", True, "HQExportRequest", "HQExport"),
    ("hq.media.legacy-mapping", "/hq/media/legacy-mappings", "POST", "media-file-service", "media.legacy-mapping.v1", "READ_ONLY", "R2", "ADMIN_MEDIA_LEGACY_MAPPING_ENABLED; ADMIN_MEDIA_CASE_READ_ENABLED; MEDIA_LEGACY_MAPPING_LOOKUP_ENABLED", "OPERATIONS_MINIMIZED", False, "HQLegacyMappingRequest", "MediaLegacyMapping"),
    ("hq.exports.download", "/hq/exports/{export_id}/download", "POST", "admin-backoffice-service", "karryam.controlled-export.v1", "READ_ONLY", "R2", "ADMIN_HQ_EXPORTS_ENABLED", "OPERATIONS_MINIMIZED", False, "HQExportDownload", None),
]

BLOCKER = {
    "notification-service": "Default disabled. Requires NOTIFICATION_CAMPAIGNS_ENABLED, a dedicated campaign credential and customer-service marketing consent records; delivered/viewed/converted/billed are reported only with provider receipts (currently UNAVAILABLE).",
    "payment-wallet-service": "Default disabled. Aggregate reporting only; no revenue recognition policy is approved, so fee figures are labelled SOURCE_FIGURE_RECOGNITION_POLICY_PENDING. Never ledger access or mutation.",
    "support-dispute-service": "Default disabled. Minimized projection of owner events; detail and actions remain owner-context scoped resource reads and owner commands.",
    "media-file-service": "Default disabled. Returns only opaque ids and the owner attestation digest for owner-attested adoptions; unadopted, document_file and signature legacy assets have no authoritative association and answer not found.",
    "admin-backoffice-service": "Default disabled. Bridge-local orchestration/projection metadata; owners remain authoritative.",
}


def is_hq(identifier):
    return identifier.startswith("hq.")


def matrix_entries():
    rows = []
    for (cid, endpoint, method, owner, contract, state, risk, flag, privacy, idem, request, result) in ROUTES:
        rows.append({
            "id": cid, "version": "1.0", "owner": owner,
            "source_endpoint": {"media-file-service": "POST /internal/media/v1/legacy-mappings", "notification-service": "POST/GET /internal/notification/v1/admin/campaigns", "payment-wallet-service": "GET /internal/payment-wallet/v1/admin/finance-reports"}.get(owner, "Bridge-local " + contract),
            "bridge_endpoint": endpoint, "method": method, "state": state, "permission": cid,
            "scope": "karryam + exact market + zone + organization + tenant; delegated staff; signed policy rule naming owner and " + contract,
            "risk": risk, "privacy": privacy, "source_version_required": False, "idempotency_required": idem,
            "approval": "Two-person HQ approval bound to the exact intent digest (campaign submit, export create); approver must differ from requester." if cid in ("hq.campaigns.submit", "hq.exports.create") else "Signed governance policy required; no production approval supplied.",
            "evidence": "Immutable Bridge audit; durable orchestration rows; owner read-back for owner effects.",
            "feature_flag": flag, "tests": HQ_TESTS, "blocker": BLOCKER[owner],
            "fallback": "Fail closed. Unknown owner outcomes are read back by original identity, never resubmitted; stale or partial data is labelled, never zero-filled.",
        })
    return rows


def schemas(obj, string, opaque):
    uid = string(format="uuid")
    ts = string(format="date-time")
    digest = string(pattern="^[0-9a-f]{64}$")
    count = {"type": "integer", "minimum": 0}
    nullable_count = {"type": ["integer", "null"], "minimum": 0}
    token = string(pattern="^[A-Z][A-Z0-9_]{0,63}$")
    export_event = obj({"schema": {"const": "cosmicforge.product-admin-event.v1"}, "event_id": uid, "sequence": {"type": "integer", "minimum": 1}, "event_type": opaque, "source_service": opaque, "product": {"const": "karryam"}, "entity_type": opaque, "entity_ref": opaque, "entity_version": {"type": "integer", "minimum": 1}, "tombstone": {"type": "boolean"}, "occurred_at": ts, "emitted_at": ts, "market": string(maxLength=128), "scope": obj({"zone": string(maxLength=128), "organization": string(maxLength=128), "tenant": string(maxLength=128)}), "classification": {"enum": ["PUBLIC", "INTERNAL", "OPERATIONS_MINIMIZED", "FINANCIAL_SENSITIVE", "SAFETY_EVIDENCE"]}, "correlation_id": string(maxLength=128), "payload": {"type": "object", "maxProperties": 32}})
    feed = obj({"events": {"type": "array", "maxItems": 500, "items": export_event}, "next_sequence": count, "head_sequence": count, "as_of": ts})
    ack = obj({"consumer": opaque, "sequence": count})
    ack_result = obj({"consumer": opaque, "acknowledged_sequence": count})
    source = obj({"source_service": opaque, "entity_type": opaque, "projections": count, "last_received_at": {"type": ["string", "null"], "format": "date-time"}, "lag_seconds": nullable_count, "open_gaps": count, "freshness": {"enum": ["LIVE", "STALE", "UNKNOWN"]}, "coverage": {"enum": ["COMPLETE", "PARTIAL_COVERAGE"]}, "last_backfill_status": {"enum": ["NONE", "RUNNING", "COMPLETE", "PARTIAL_COVERAGE", "FAILED"]}})
    status = obj({"as_of": ts, "freshness_threshold_seconds": count, "sources": {"type": "array", "items": source}, "quarantined": count, "head_sequence": count, "consumer_lag": {"type": "object", "additionalProperties": count}})
    case = obj({"product": {"const": "karryam"}, "source_service": opaque, "source_case_id": opaque, "composite_id": string(pattern="^karryam:[A-Za-z0-9._-]+:[A-Za-z0-9._:@/-]+$"), "source_version": {"type": "integer", "minimum": 1}, "classification": opaque, "organization": string(maxLength=128), "fields": {"type": "object", "maxProperties": 32}, "received_at": ts, "coverage": {"enum": ["INCREMENTAL", "BACKFILLED", "RECONCILED"]}})
    queue = obj({"items": {"type": "array", "maxItems": 100, "items": case}, "next_cursor": string(maxLength=128), "as_of": ts, "freshness": {"enum": ["LIVE", "STALE", "UNKNOWN"]}, "coverage": {"enum": ["COMPLETE", "PARTIAL_COVERAGE"]}})
    approval = obj({"approval_id": uid, "kind": {"enum": ["CAMPAIGN", "EXPORT"]}, "subject_ref": opaque, "intent_digest": digest, "status": {"enum": ["PENDING", "APPROVED", "REJECTED", "CONSUMED"]}, "expires_at": ts, "decided_at": ts}, ["approval_id", "kind", "subject_ref", "intent_digest", "status", "expires_at"])
    quiet = {"anyOf": [obj({"start": string(pattern="^([01][0-9]|2[0-3]):[0-5][0-9]$"), "end": string(pattern="^([01][0-9]|2[0-3]):[0-5][0-9]$"), "timezone": string(minLength=1, maxLength=64)}), {"type": "null"}]}
    markets = {"type": "array", "minItems": 1, "maxItems": 16, "uniqueItems": True, "items": opaque}
    approvals = {"type": "array", "maxItems": 4, "items": obj({"approval_id": uid, "approver_ref": opaque, "approved_at": ts})}
    intent = obj({"contract_version": {"const": "karryam.campaign-intent.v1"}, "campaign_id": uid, "revision": {"type": "integer", "minimum": 1}, "idempotency_key": opaque, "product": {"const": "karryam"}, "requesting_department": opaque, "requested_by": opaque, "approvals": approvals, "category": {"enum": ["MARKETING", "PRODUCT_ANNOUNCEMENT"]}, "markets": markets, "zones": {"type": "array", "maxItems": 64, "items": opaque}, "audience": {"const": "CUSTOMERS_OPTED_IN"}, "audience_definition_sha256": digest, "legal_basis": {"const": "CONSENT"}, "channels": {"type": "array", "minItems": 1, "maxItems": 3, "uniqueItems": True, "items": {"enum": ["in_app", "push", "email"]}}, "template_key": opaque, "template_locale": opaque, "template_revision": {"type": "integer", "minimum": 1}, "content_sha256": digest, "start_at": ts, "end_at": ts, "max_per_recipient_per_day": {"type": "integer", "minimum": 1, "maximum": 10}, "max_messages": {"type": "integer", "minimum": 1}, "budget_minor": count, "currency": string(maxLength=3), "priority": {"enum": ["low", "normal"]}, "quiet_hours": quiet, "correlation_id": string(maxLength=128)})
    campaign_status = obj({"contract_version": {"const": "karryam.campaign-intent.v1"}, "campaign_id": uid, "revision": {"type": "integer", "minimum": 1}, "intent_digest": digest, "state": {"enum": ["ACCEPTED_BY_KARRYGO", "PLANNED", "QUEUED", "SENT", "DELIVERED", "COMPLETED", "REJECTED", "FAILED", "SUPPRESSED", "CANCELLED"]}, "rejection_code": string(maxLength=64), "audience_count": count, "eligible_count": count, "suppressed_count": count, "suppression_reasons": {"type": "object", "additionalProperties": count}, "queued_count": count, "sent_count": count, "logged_only_count": count, "failed_count": count, "delivered_count": nullable_count, "delivery_confirmation": {"enum": ["UNAVAILABLE", "PROVIDER_RECEIPTS"]}, "conversion_reporting": {"enum": ["UNAVAILABLE"]}, "as_of": ts}, ["contract_version", "campaign_id", "revision", "intent_digest", "state", "audience_count", "eligible_count", "suppressed_count", "suppression_reasons", "queued_count", "sent_count", "logged_only_count", "failed_count", "delivered_count", "delivery_confirmation", "conversion_reporting", "as_of"])
    campaign_status["allOf"] = [{"if": {"properties": {"delivery_confirmation": {"const": "UNAVAILABLE"}}}, "then": {"properties": {"delivered_count": {"type": "null"}, "state": {"not": {"const": "DELIVERED"}}}}}]
    campaign_op = obj({"operation_id": uid, "campaign_id": uid, "revision": {"type": "integer", "minimum": 1}, "intent_digest": digest, "status": {"enum": ["PENDING", "CONFIRMED", "REJECTED", "UNKNOWN_OUTCOME"]}, "rejection_code": string(maxLength=64), "owner_status": {"anyOf": [campaign_status, {"type": "null"}]}, "as_of": ts}, ["operation_id", "campaign_id", "revision", "intent_digest", "status", "owner_status", "as_of"])
    definition = obj({"measure": opaque, "source_field": string(maxLength=512), "calculation": string(maxLength=1024), "recognition": {"enum": ["NOT_REVENUE", "SOURCE_FIGURE_RECOGNITION_POLICY_PENDING", "RECOGNIZED_BY_APPROVED_POLICY"]}, "correction": string(maxLength=1024)})
    figure = obj({"measure": opaque, "currency": string(pattern="^[A-Z]{3}$"), "amount_minor": {"type": ["integer", "null"]}, "count": nullable_count, "completeness": {"enum": ["COMPLETE", "PARTIAL_COVERAGE"]}, "unknown_reason": string(maxLength=128)}, ["measure", "currency", "amount_minor", "count", "completeness"])
    report = obj({"contract_version": {"const": "karryam.finance-report.v1"}, "owner": {"const": "payment-wallet-service"}, "period_start": ts, "period_end": ts, "timezone": string(minLength=1, maxLength=64), "data_as_of": ts, "source_watermark": ts, "aggregation_version": string(minLength=1, maxLength=64), "unit": {"const": "MINOR_UNITS"}, "definitions": {"type": "array", "items": definition}, "figures": {"type": "array", "items": figure}, "completeness": {"enum": ["COMPLETE", "PARTIAL_COVERAGE"]}, "error_reason": string(maxLength=128)}, ["contract_version", "owner", "period_start", "period_end", "timezone", "data_as_of", "source_watermark", "aggregation_version", "unit", "definitions", "figures", "completeness"])
    disposition = obj({"owner": opaque, "disposition": {"enum": ["PENDING_OWNER_ACTION", "POLICY_REQUIRED", "LEGAL_HOLD", "FULFILLED_BY_OWNER", "RETAINED_LEGAL_OBLIGATION", "NOT_HELD_BY_OWNER", "PROJECTION_TOMBSTONED"]}, "evidence_ref": string(maxLength=160), "recorded_at": ts})
    privacy = obj({"request_id": uid, "subject_kind": {"enum": ["CUSTOMER", "PROVIDER", "VENDOR_STAFF"]}, "request_type": {"enum": ["ACCESS_EXPORT", "ERASURE", "RECTIFICATION", "CONSENT_WITHDRAWAL", "RESTRICT_PROCESSING"]}, "received_on": string(format="date"), "legal_hold": {"type": "boolean"}, "dispositions": {"type": "array", "items": disposition}, "complete": {"type": "boolean"}})
    export = obj({"export_id": uid, "row_count": count, "download_token": string(pattern="^[A-Za-z0-9_-]{43}$"), "expires_at": ts, "plaintext_sha256": digest}, ["export_id", "row_count", "expires_at", "plaintext_sha256"])
    return {
        "HQExportEvent": export_event, "HQEventFeed": feed, "HQEventAck": ack, "HQEventAckResult": ack_result, "HQProjectionStatus": status, "HQSupportQueue": queue,
        "HQApproval": approval,
        "HQApprovalProposal": obj({"approval_id": uid, "kind": {"enum": ["CAMPAIGN", "EXPORT"]}, "intent": {"anyOf": [intent, {"type": "null"}]}, "row_limit": {"type": "integer", "minimum": 0, "maximum": 10000}}, ["approval_id", "kind"]),
        "HQApprovalDecision": obj({"decision": {"enum": ["APPROVE", "REJECT"]}}),
        "CampaignIntent": intent, "CampaignStatus": campaign_status, "HQCampaignOperation": campaign_op,
        "HQCampaignSubmission": obj({"approval_id": uid, "intent": intent}),
        "HQCampaignCancellation": obj({"cancellation_key": opaque, "reason_code": token}),
        "HQFinanceRequest": obj({"timezone": string(minLength=1, maxLength=64)}), "FinanceReport": report,
        "HQPrivacyRegistration": obj({"request_id": uid, "subject_kind": {"enum": ["CUSTOMER", "PROVIDER", "VENDOR_STAFF"]}, "subject_ref": opaque, "request_type": {"enum": ["ACCESS_EXPORT", "ERASURE", "RECTIFICATION", "CONSENT_WITHDRAWAL", "RESTRICT_PROCESSING"]}, "received_on": string(format="date"), "legal_hold": {"type": "boolean"}}),
        "HQPrivacyFulfilment": obj({"owner": {"const": "customer-service"}, "operation_id": uid}), "HQPrivacyRequest": privacy,
        "HQExportRequest": obj({"approval_id": uid, "row_limit": {"type": "integer", "minimum": 1, "maximum": 10000}}), "HQExport": export,
        "HQLegacyMappingRequest": obj({"legacy_asset_id": uid}),
        "MediaLegacyMapping": obj({"contract_version": {"const": "media.legacy-mapping.v1"}, "legacy_asset_id": uid, "media_object_id": opaque, "attestation_digest": digest}),
        "HQExportDownload": obj({"token": string(pattern="^[A-Za-z0-9_-]{43}$")}),
    }


def request_schema(identifier):
    for row in ROUTES:
        if row[0] == identifier:
            return row[10]


def result_schema(identifier):
    for row in ROUTES:
        if row[0] == identifier:
            return row[11]


if __name__ == "__main__":
    import json
    from pathlib import Path
    path = Path(__file__).resolve().parents[2] / "services/admin-backoffice-service/docs/openapi/KARRYGO_ADMIN_CAPABILITY_MATRIX.json"
    matrix = json.loads(path.read_text(encoding="utf-8"))
    matrix = [row for row in matrix if not is_hq(row["id"])] + matrix_entries()
    path.write_bytes((json.dumps(matrix, indent=2, ensure_ascii=False) + "\n").encode("utf-8"))
    print(f"{len(matrix)} capability entries")
