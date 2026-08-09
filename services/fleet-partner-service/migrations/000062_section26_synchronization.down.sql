DELETE FROM fleet_partner_role_permissions WHERE permission_code LIKE 'synchronization.%';
DELETE FROM fleet_partner_permissions WHERE code LIKE 'synchronization.%';
DROP TABLE IF EXISTS fleet_partner_synchronization_dead_letters;
DROP TRIGGER IF EXISTS fleet_synchronization_attempts_append_only ON fleet_partner_synchronization_attempts;
DROP TABLE IF EXISTS fleet_partner_synchronization_attempts;
DROP TABLE IF EXISTS fleet_partner_synchronization_cases;
DROP TABLE IF EXISTS fleet_partner_synchronization_projections;
