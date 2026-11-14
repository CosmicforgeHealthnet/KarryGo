-- Reverse 000099. The permissions themselves belong to 000077 and stay.
DELETE FROM fleet_partner_role_permissions
WHERE role_id = 'rol_system_owner'
  AND permission_code IN ('fleet_partner_foundation.read', 'fleet_partner_foundation.manage');
