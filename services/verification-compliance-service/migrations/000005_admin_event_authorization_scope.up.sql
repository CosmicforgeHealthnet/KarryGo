-- Authorization scope comes only from explicit owner allocations, not native geography.
-- SQL selects a candidate; the Go egress guard MUST verify its signature, expiry,
-- current head, canonical record binding and signed feed purpose before disclosure.
-- Metadata corruption or ambiguity must not break a business transaction.
SELECT pg_advisory_xact_lock(hashtextextended('verification-compliance-service:migration:005', 0));
CREATE OR REPLACE FUNCTION verification_admin_event_scope(p_ref text) RETURNS jsonb
LANGUAGE plpgsql STABLE AS $fn$
DECLARE candidate jsonb; candidates bigint;
BEGIN
    WITH scopes AS (
        SELECT DISTINCT jsonb_build_object('market',h.market,'zone',h.zone,
            'organization',h.organization_ref,'tenant',h.tenant_ref) AS value
        FROM admin_owner_context_heads h JOIN admin_owner_context_history v
          ON v.owner=h.owner AND v.context_id=h.context_id AND v.revision=h.revision AND v.checksum=h.checksum
        WHERE h.owner='verification-compliance-service' AND h.resource_kind='vendor-cases' AND h.resource_ref=p_ref
          AND h.product='karryam' AND 'verification.vendor_cases.read'=ANY(h.capabilities)
          AND h.revision=(SELECT max(x.revision) FROM admin_owner_context_history x WHERE x.owner=h.owner AND x.context_id=h.context_id)
          AND (convert_from(v.envelope,'UTF8')::jsonb->'document'->>'state')='APPROVED'
          AND (convert_from(v.envelope,'UTF8')::jsonb->'document'->>'effective_at')::timestamptz<=now()
          AND (convert_from(v.envelope,'UTF8')::jsonb->'document'->>'expires_at')::timestamptz>now()
    ) SELECT count(*),min(value::text)::jsonb INTO candidates,candidate FROM scopes;
    IF candidates=1 THEN RETURN candidate; END IF;
    RETURN jsonb_build_object('market','','zone','','organization','','tenant','');
EXCEPTION WHEN OTHERS THEN
    RETURN jsonb_build_object('market','','zone','','organization','','tenant','');
END $fn$;


CREATE OR REPLACE FUNCTION verification_emit_vendor_case_admin_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 opaque CONSTANT text := '^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$';
 fmt CONSTANT text := 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"';
 event_id text;
 body jsonb;
BEGIN
 IF TG_OP = 'UPDATE' AND (OLD.status, OLD.source_version, OLD.reason_code, OLD.expires_at, OLD.automation_recommendation, OLD.dimension, OLD.subject_type, OLD.business_id, OLD.updated_at)
    IS NOT DISTINCT FROM (NEW.status, NEW.source_version, NEW.reason_code, NEW.expires_at, NEW.automation_recommendation, NEW.dimension, NEW.subject_type, NEW.business_id, NEW.updated_at) THEN
  RETURN NEW;
 END IF;
 IF NEW.business_id !~ opaque OR NEW.subject_type !~ opaque THEN
  RETURN NEW;
 END IF;
 event_id := gen_random_uuid()::text;
 body := jsonb_build_object(
  'schema', 'karryam.owner-admin-event.v1',
  'event_id', event_id,
  'event_type', 'verification.vendor_case.changed.v1',
  'source_service', 'verification-compliance-service',
  'product', 'karryam',
  'entity_type', 'vendor_verification_case',
  'entity_ref', NEW.id::text,
  'entity_version', NEW.source_version,
  'deleted', false,
  'occurred_at', to_char(now() AT TIME ZONE 'UTC', fmt),
  'emitted_at', to_char(clock_timestamp() AT TIME ZONE 'UTC', fmt),
  'scope', verification_admin_event_scope(NEW.id::text),
  'correlation_id', '',
  'payload', jsonb_build_object(
    'status', NEW.status,
    'dimension', NEW.dimension,
    'subject_type', NEW.subject_type,
    'business_ref', NEW.business_id,
    'automation_recommendation', NEW.automation_recommendation,
    'reason_code', CASE WHEN NEW.reason_code ~ opaque THEN NEW.reason_code END,
    'expires_at', CASE WHEN NEW.expires_at IS NOT NULL THEN to_char(NEW.expires_at AT TIME ZONE 'UTC', fmt) END,
    'updated_at', to_char(NEW.updated_at AT TIME ZONE 'UTC', fmt)));
 INSERT INTO admin_event_outbox(id,event_type,schema_version,aggregate_type,aggregate_id,payload,status,available_at,attempts,created_at)
 VALUES (event_id, 'verification.vendor_case.changed.v1', 1, 'vendor_verification_case', NEW.id::text, body, 'PENDING', now(), 0, now());
 RETURN NEW;
END $$;
