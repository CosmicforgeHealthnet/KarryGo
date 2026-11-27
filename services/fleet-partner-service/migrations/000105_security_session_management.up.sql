-- Section 28/56 (spec 59.10): organization staff-session management.
-- Sessions themselves stay in fleet_partner_corporate_sessions (the auth
-- feature's table); this migration only adds the permission that lets an
-- organization's security managers list and revoke its staff sessions.
-- Granted to owners and administrators only.
INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('security.manage','section56','List and revoke organization staff sessions',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code='security.manage'
WHERE r.id IN('rol_system_owner','rol_system_admin') ON CONFLICT DO NOTHING;
