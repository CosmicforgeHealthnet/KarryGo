-- FLEET-11.1a: let the organization owner read and change its operating mode.
--
-- 000077 seeded fleet_partner_foundation.read and .manage (the permissions on
-- GET/PUT /fleet-partner/operating-mode) but granted them to no role. Every
-- other feature migration gives the owner its new permissions, and a custom
-- role cannot carry a permission its creator does not hold (privilege
-- escalation is refused), so without this grant nobody could ever read or
-- change the operating mode through the API.
INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code
FROM fleet_partner_permissions
WHERE code IN ('fleet_partner_foundation.read', 'fleet_partner_foundation.manage')
ON CONFLICT DO NOTHING;
