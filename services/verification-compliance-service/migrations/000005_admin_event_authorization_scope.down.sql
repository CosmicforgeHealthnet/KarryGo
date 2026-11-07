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
  'scope', jsonb_build_object('market', '', 'zone', '', 'organization', '', 'tenant', ''),
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

DROP FUNCTION IF EXISTS verification_admin_event_scope(text);
