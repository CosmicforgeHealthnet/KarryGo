-- Sections 19 (19.5, 19.12, 19.13), 20 (20.3, 20.13, 20.14), 49 and 50.
--
-- 1. Generic chain of custody. fleet_partner_custody_events (000028) was the
--    package-only custody chain. It is generalised in place rather than
--    duplicated: a package event keeps package_id/shipment_id exactly as
--    before (subject_type defaults to PACKAGE, so existing rows and the
--    existing package writer are unchanged), while a shipment, order, asset or
--    specialized-record handover stores its subject in subject_reference.
--    subject_key is generated so both kinds share one chain index without an
--    UPDATE of the append-only rows.
ALTER TABLE fleet_partner_custody_events
    ALTER COLUMN package_id DROP NOT NULL,
    ALTER COLUMN shipment_id DROP NOT NULL,
    ADD COLUMN subject_type text NOT NULL DEFAULT 'PACKAGE'
        CHECK (subject_type IN ('PACKAGE','SHIPMENT','ORDER','ASSET','SPECIALIZED_RECORD')),
    ADD COLUMN subject_reference text,
    ADD COLUMN evidence_media_ids jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(evidence_media_ids) = 'array' AND jsonb_array_length(evidence_media_ids) <= 10);
ALTER TABLE fleet_partner_custody_events
    ADD COLUMN subject_key text GENERATED ALWAYS AS (COALESCE(subject_reference, package_id)) STORED;
ALTER TABLE fleet_partner_custody_events
    ADD CONSTRAINT chk_fleet_custody_events_subject CHECK (
        (subject_type = 'PACKAGE' AND package_id IS NOT NULL AND shipment_id IS NOT NULL AND subject_reference IS NULL)
     OR (subject_type <> 'PACKAGE' AND package_id IS NULL AND subject_reference IS NOT NULL AND subject_reference <> ''));
CREATE INDEX idx_fleet_custody_events_subject
    ON fleet_partner_custody_events (organization_id, subject_type, subject_key, occurred_at, id);

-- 2. Incident lifecycle (spec 58.11). The existing statuses keep their
--    meaning (OPEN = reported, INVESTIGATING = under_investigation,
--    CORRECTIVE_ACTION_REQUIRED = corrective_action); TRIAGED and ESCALATED
--    are added, together with the facts each transition must record.
ALTER TABLE fleet_partner_incidents DROP CONSTRAINT fleet_partner_incidents_status_check;
ALTER TABLE fleet_partner_incidents ADD CONSTRAINT fleet_partner_incidents_status_check
    CHECK (status IN ('OPEN','TRIAGED','CONTAINED','INVESTIGATING','CORRECTIVE_ACTION_REQUIRED','RESOLVED','CLOSED','ESCALATED'));
ALTER TABLE fleet_partner_incidents
    ADD COLUMN immediate_actions text NOT NULL DEFAULT '',
    ADD COLUMN triaged_by text NOT NULL DEFAULT '',
    ADD COLUMN triaged_at timestamptz,
    ADD COLUMN containment_summary text NOT NULL DEFAULT '',
    ADD COLUMN contained_by text NOT NULL DEFAULT '',
    ADD COLUMN contained_at timestamptz,
    ADD COLUMN escalation_reason text NOT NULL DEFAULT '',
    ADD COLUMN escalation_trigger text NOT NULL DEFAULT ''
        CHECK (escalation_trigger IN ('','SEVERITY','REGULATORY','CUSTOMER','SAFETY_CRITICAL','OTHER')),
    ADD COLUMN pre_escalation_status text NOT NULL DEFAULT '',
    ADD COLUMN escalated_by text NOT NULL DEFAULT '',
    ADD COLUMN escalated_at timestamptz,
    ADD COLUMN closure_reason text NOT NULL DEFAULT '',
    ADD COLUMN closed_by text NOT NULL DEFAULT '';

-- The organization's own incident investigations. Distinct from the platform
-- operator's admin investigations (000083/000084), which are an operator
-- support tool and never part of the company's safety record.
CREATE TABLE fleet_partner_incident_investigations (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    incident_id text NOT NULL REFERENCES fleet_partner_incidents(id) ON DELETE RESTRICT,
    investigator_user_id text NOT NULL,
    scope text NOT NULL CHECK (btrim(scope) <> ''),
    evidence_preservation text NOT NULL CHECK (btrim(evidence_preservation) <> ''),
    status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','FINDINGS_SUBMITTED','APPROVED','CLOSED')),
    findings text NOT NULL DEFAULT '',
    root_cause text NOT NULL DEFAULT '',
    contributing_factors jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(contributing_factors) = 'array'),
    findings_submitted_at timestamptz,
    approved_by text NOT NULL DEFAULT '',
    approved_at timestamptz,
    closed_at timestamptz,
    opened_by text NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (status = 'OPEN' OR (btrim(findings) <> '' AND btrim(root_cause) <> '' AND findings_submitted_at IS NOT NULL)),
    CHECK (status NOT IN ('APPROVED','CLOSED') OR (approved_at IS NOT NULL AND approved_by <> '' AND approved_by <> investigator_user_id))
);
CREATE UNIQUE INDEX uq_fleet_incident_investigation_active
    ON fleet_partner_incident_investigations (organization_id, incident_id) WHERE status <> 'CLOSED';
CREATE INDEX idx_fleet_incident_investigations_scope
    ON fleet_partner_incident_investigations (organization_id, incident_id, created_at);

-- 3. Compliance checks (spec 49.1): one immutable run of every applicable
--    requirement for one subject, linking the evaluations it produced.
CREATE TABLE fleet_partner_compliance_checks (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    subject_type text NOT NULL CHECK (subject_type IN ('ORGANIZATION','PROVIDER','ASSET','ORDER','SHIPMENT','CARGO','WAREHOUSE','LOCATION')),
    subject_reference text NOT NULL,
    source_type text NOT NULL,
    source_reference text NOT NULL DEFAULT '',
    source_version bigint NOT NULL CHECK (source_version >= 0),
    overall_state text NOT NULL CHECK (overall_state IN ('UNKNOWN','PENDING','COMPLIANT','DUE_SOON','EXPIRED','NON_COMPLIANT','RESTRICTED','NOT_APPLICABLE')),
    governing_requirement_id text NOT NULL DEFAULT '',
    requirement_count integer NOT NULL CHECK (requirement_count >= 0),
    results jsonb NOT NULL CHECK (jsonb_typeof(results) = 'array'),
    restriction_references jsonb NOT NULL DEFAULT '[]'::jsonb,
    requested_by text NOT NULL,
    correlation_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fleet_compliance_checks_subject
    ON fleet_partner_compliance_checks (organization_id, subject_type, subject_reference, created_at DESC);
CREATE TRIGGER trg_fleet_compliance_checks_append_only BEFORE UPDATE OR DELETE ON fleet_partner_compliance_checks
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- 4. Specialized extension schema versions (spec 50.2). Rows written before
--    this migration were validated by the original presence-only rules and
--    stay on schema version 1; the typed, closed version 2 schema is the
--    default for new writes.
ALTER TABLE fleet_partner_specialized_records
    ADD COLUMN schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version IN (1, 2));
CREATE INDEX idx_fleet_specialized_records_subject
    ON fleet_partner_specialized_records (organization_id, subject_type, subject_reference, capability_code);

-- 5. Permissions for the lifecycle commands. Each is granted to the system
--    roles that already hold the matching incidents permissions (000044).
INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
 ('incidents.triage','incidents','Triage, contain and escalate incidents',true),
 ('incidents.investigate','incidents','Open incident investigations and record findings',true),
 ('incidents.approve','incidents','Approve investigation findings, resolve and close incidents',true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;
INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p
 WHERE r.id IN ('rol_system_owner','rol_system_operations','rol_system_compliance')
   AND p.code IN ('incidents.triage','incidents.investigate','incidents.approve')
ON CONFLICT DO NOTHING;
