-- Reverses WP6.1 assignment acknowledgement.

DROP TRIGGER IF EXISTS trg_fleet_partner_acknowledgements_append_only
    ON fleet_partner_assignment_acknowledgements;

DROP INDEX IF EXISTS idx_fleet_partner_acknowledgements_org;
DROP INDEX IF EXISTS uq_fleet_partner_acknowledgement_idempotency;

DROP TABLE IF EXISTS fleet_partner_assignment_acknowledgements;

ALTER TABLE fleet_partner_organization_settings
    DROP COLUMN IF EXISTS require_assignment_acknowledgement;

DROP INDEX IF EXISTS idx_fleet_partner_assignments_unacknowledged;

ALTER TABLE fleet_partner_provider_assignments
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_assignment_acknowledgement_state;

ALTER TABLE fleet_partner_provider_assignments
    DROP COLUMN IF EXISTS acknowledged_by_actor_id;

ALTER TABLE fleet_partner_provider_assignments
    DROP COLUMN IF EXISTS acknowledged_by_actor_type;

ALTER TABLE fleet_partner_provider_assignments
    DROP COLUMN IF EXISTS acknowledged_at;

ALTER TABLE fleet_partner_provider_assignments
    DROP COLUMN IF EXISTS acknowledgement_state;
