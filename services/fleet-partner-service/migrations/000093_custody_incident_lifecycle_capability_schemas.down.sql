DELETE FROM fleet_partner_role_permissions
 WHERE permission_code IN ('incidents.triage','incidents.investigate','incidents.approve');
DELETE FROM fleet_partner_permissions
 WHERE code IN ('incidents.triage','incidents.investigate','incidents.approve');

DROP INDEX IF EXISTS idx_fleet_specialized_records_subject;
ALTER TABLE fleet_partner_specialized_records DROP COLUMN IF EXISTS schema_version;

DROP TABLE IF EXISTS fleet_partner_compliance_checks;
DROP TABLE IF EXISTS fleet_partner_incident_investigations;

-- Statuses introduced by 000093 have no equivalent in the 000044 constraint:
-- TRIAGED returns to OPEN and ESCALATED to the status it escalated from.
UPDATE fleet_partner_incidents
   SET status = CASE WHEN status = 'TRIAGED' THEN 'OPEN'
                     WHEN pre_escalation_status IN ('OPEN','INVESTIGATING','CONTAINED','CORRECTIVE_ACTION_REQUIRED') THEN pre_escalation_status
                     ELSE 'OPEN' END
 WHERE status IN ('TRIAGED','ESCALATED');
ALTER TABLE fleet_partner_incidents DROP CONSTRAINT fleet_partner_incidents_status_check;
ALTER TABLE fleet_partner_incidents ADD CONSTRAINT fleet_partner_incidents_status_check
    CHECK (status IN ('OPEN','INVESTIGATING','CONTAINED','CORRECTIVE_ACTION_REQUIRED','RESOLVED','CLOSED'));
ALTER TABLE fleet_partner_incidents
    DROP COLUMN IF EXISTS immediate_actions,
    DROP COLUMN IF EXISTS triaged_by,
    DROP COLUMN IF EXISTS triaged_at,
    DROP COLUMN IF EXISTS containment_summary,
    DROP COLUMN IF EXISTS contained_by,
    DROP COLUMN IF EXISTS contained_at,
    DROP COLUMN IF EXISTS escalation_reason,
    DROP COLUMN IF EXISTS escalation_trigger,
    DROP COLUMN IF EXISTS pre_escalation_status,
    DROP COLUMN IF EXISTS escalated_by,
    DROP COLUMN IF EXISTS escalated_at,
    DROP COLUMN IF EXISTS closure_reason,
    DROP COLUMN IF EXISTS closed_by;

-- Non-package custody events cannot be represented by the package-only
-- 000028 shape; the append-only guard is lifted only for this rollback.
ALTER TABLE fleet_partner_custody_events DISABLE TRIGGER trg_fleet_custody_events_append_only;
DELETE FROM fleet_partner_custody_events WHERE subject_type <> 'PACKAGE';
ALTER TABLE fleet_partner_custody_events ENABLE TRIGGER trg_fleet_custody_events_append_only;
DROP INDEX IF EXISTS idx_fleet_custody_events_subject;
ALTER TABLE fleet_partner_custody_events DROP CONSTRAINT IF EXISTS chk_fleet_custody_events_subject;
ALTER TABLE fleet_partner_custody_events
    DROP COLUMN IF EXISTS subject_key,
    DROP COLUMN IF EXISTS evidence_media_ids,
    DROP COLUMN IF EXISTS subject_reference,
    DROP COLUMN IF EXISTS subject_type;
ALTER TABLE fleet_partner_custody_events
    ALTER COLUMN package_id SET NOT NULL,
    ALTER COLUMN shipment_id SET NOT NULL;
