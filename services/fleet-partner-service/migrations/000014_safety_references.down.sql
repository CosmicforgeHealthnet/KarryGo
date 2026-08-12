-- Reverses WP6.2 safety references.

DELETE FROM fleet_partner_role_permissions
WHERE permission_code IN ('safety.references.read', 'safety.references.manage');

DELETE FROM fleet_partner_permissions
WHERE code IN ('safety.references.read', 'safety.references.manage');

DROP TRIGGER IF EXISTS trg_fleet_partner_safety_reference_history_append_only
    ON fleet_partner_safety_reference_history;

DROP INDEX IF EXISTS idx_fleet_partner_safety_reference_history;
DROP TABLE IF EXISTS fleet_partner_safety_reference_history;

DROP INDEX IF EXISTS idx_fleet_partner_safety_reference_provider;
DROP INDEX IF EXISTS idx_fleet_partner_safety_reference_trip;
DROP INDEX IF EXISTS idx_fleet_partner_safety_reference_assignment;
DROP INDEX IF EXISTS idx_fleet_partner_safety_reference_org;
DROP INDEX IF EXISTS uq_fleet_partner_safety_reference_idempotency;

DROP TABLE IF EXISTS fleet_partner_safety_references;
