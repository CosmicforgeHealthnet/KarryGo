-- Authorization scope comes only from explicit owner allocations, not native geography.
-- SQL selects a candidate; the Go egress guard MUST verify its signature, expiry,
-- current head, canonical record binding and signed feed purpose before disclosure.
-- Metadata corruption or ambiguity must not break a business transaction.
SELECT pg_advisory_xact_lock(hashtextextended('support-dispute-service:migration:010', 0));
CREATE OR REPLACE FUNCTION support_admin_event_scope(p_ref text, p_native_org text) RETURNS jsonb
LANGUAGE plpgsql STABLE AS $fn$
DECLARE candidate jsonb; candidates bigint;
BEGIN
    WITH scopes AS (
        SELECT DISTINCT jsonb_build_object('market',h.market,'zone',h.zone,
            'organization',h.organization_ref,'tenant',h.tenant_ref) AS value
        FROM admin_owner_context_heads h JOIN admin_owner_context_history v
          ON v.owner=h.owner AND v.context_id=h.context_id AND v.revision=h.revision AND v.checksum=h.checksum
        WHERE h.owner='support-dispute-service' AND h.resource_kind='cases' AND h.resource_ref=p_ref
          AND h.product='karryam' AND 'support.cases.read'=ANY(h.capabilities)
          AND h.revision=(SELECT max(x.revision) FROM admin_owner_context_history x WHERE x.owner=h.owner AND x.context_id=h.context_id)
          AND (p_native_org='' OR p_native_org=h.organization_ref)
          AND (convert_from(v.envelope,'UTF8')::jsonb->'document'->>'state')='APPROVED'
          AND (convert_from(v.envelope,'UTF8')::jsonb->'document'->>'effective_at')::timestamptz<=now()
          AND (convert_from(v.envelope,'UTF8')::jsonb->'document'->>'expires_at')::timestamptz>now()
    ) SELECT count(*),min(value::text)::jsonb INTO candidates,candidate FROM scopes;
    IF candidates=1 THEN RETURN candidate; END IF;
    RETURN jsonb_build_object('market','','zone','','organization',p_native_org,'tenant','');
EXCEPTION WHEN OTHERS THEN
    RETURN jsonb_build_object('market','','zone','','organization',p_native_org,'tenant','');
END $fn$;

CREATE OR REPLACE FUNCTION support_admin_case_event(p_complaint_id uuid) RETURNS jsonb
LANGUAGE plpgsql AS $fn$
DECLARE
    c complaints%ROWTYPE;
    j jsonb;
BEGIN
    SELECT * INTO c FROM complaints WHERE id = p_complaint_id;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    j := to_jsonb(c);
    RETURN jsonb_build_object(
        'schema', 'karryam.owner-admin-event.v1',
        'event_id', gen_random_uuid()::text,
        'event_type', 'support.case.changed.v1',
        'source_service', 'support-dispute-service',
        'product', 'karryam',
        'entity_type', 'support_case',
        'entity_ref', c.id::text,
        'entity_version', c.admin_version,
        'deleted', false,
        'occurred_at', support_admin_rfc3339(c.updated_at),
        'emitted_at', support_admin_rfc3339(greatest(clock_timestamp(), c.updated_at)),
        'scope', support_admin_event_scope(c.id::text, COALESCE(support_admin_opaque(j->>'organization_reference'), '')),
        'correlation_id', '',
        'payload', jsonb_build_object(
            'status', upper(c.status::text),
            'priority', CASE lower(j->>'priority') WHEN 'high' THEN 'HIGH' WHEN 'emergency' THEN 'EMERGENCY' ELSE 'NORMAL' END,
            'service_type', c.service_type::text,
            'category', support_admin_opaque(j->>'category'),
            'complainant_type', c.complainant_type::text,
            'context_type', support_admin_opaque(j->>'context_type'),
            'locale', NULL,
            'sla_state', 'POLICY_UNAVAILABLE',
            'consent_classification', CASE WHEN lower(j->>'priority') = 'emergency' THEN 'SAFETY_EVIDENCE' ELSE 'OPERATIONS_MINIMIZED' END,
            'assignee', support_admin_opaque(j->>'assigned_to'),
            'escalation_level', support_admin_opaque(j->>'escalation_level'),
            'mission_ref', support_admin_opaque(j->>'mission_ref'),
            'order_ref', support_admin_opaque(j->>'order_ref'),
            'dispute_ref', (SELECT d.id::text FROM disputes d WHERE d.complaint_id = c.id ORDER BY d.created_at DESC, d.id DESC LIMIT 1),
            'unread_count', (SELECT count(*) FROM support_chat_messages m WHERE m.complaint_id = c.id AND NOT m.is_read AND m.sender_type <> 'admin'),
            'message_count', (SELECT count(*) FROM support_chat_messages m WHERE m.complaint_id = c.id),
            'evidence_count', (SELECT count(*) FROM complaint_evidence e WHERE e.complaint_id = c.id),
            'created_at', support_admin_rfc3339(c.created_at),
            'updated_at', support_admin_rfc3339(c.updated_at)));
END $fn$;
