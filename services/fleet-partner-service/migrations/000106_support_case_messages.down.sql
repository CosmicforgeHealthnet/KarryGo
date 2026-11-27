DELETE FROM fleet_partner_role_permissions WHERE permission_code='support.manage';
DELETE FROM fleet_partner_permissions WHERE code='support.manage';
DROP TABLE IF EXISTS fleet_partner_support_case_messages;
