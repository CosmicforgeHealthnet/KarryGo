DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM fleet_partner_admin_investigations
    WHERE subject_kind NOT IN ('shipments', 'private-assignments', 'execution-links')
  ) THEN
    RAISE EXCEPTION 'cannot remove expanded Fleet investigation subjects while cases exist';
  END IF;
END $$;

ALTER TABLE fleet_partner_admin_investigations
    DROP CONSTRAINT fleet_partner_admin_investigation_subject_ck,
    DROP CONSTRAINT fleet_partner_admin_investigations_category_check,
    DROP CONSTRAINT fleet_partner_admin_investigations_subject_kind_check;

ALTER TABLE fleet_partner_admin_investigations
    DROP COLUMN sla_measurement_id,
    DROP COLUMN route_optimization_id,
    DROP COLUMN route_plan_id,
    DROP COLUMN incident_id,
    DROP COLUMN contract_id,
    DROP COLUMN asset_id,
    DROP COLUMN organization_subject_id;

ALTER TABLE fleet_partner_admin_investigations
    ADD CONSTRAINT fleet_partner_admin_investigations_subject_kind_check CHECK (subject_kind IN ('shipments', 'private-assignments', 'execution-links')),
    ADD CONSTRAINT fleet_partner_admin_investigations_category_check CHECK (category IN ('CARGO_DAMAGE', 'CARGO_LOSS', 'SAFETY', 'CONDUCT', 'SERVICE_FAILURE')),
    ADD CONSTRAINT fleet_partner_admin_investigation_subject_ck CHECK (
        (subject_kind = 'shipments' AND shipment_id = subject_id AND private_assignment_id IS NULL AND execution_link_id IS NULL)
     OR (subject_kind = 'private-assignments' AND private_assignment_id = subject_id AND shipment_id IS NULL AND execution_link_id IS NULL)
     OR (subject_kind = 'execution-links' AND execution_link_id = subject_id AND shipment_id IS NULL AND private_assignment_id IS NULL));
