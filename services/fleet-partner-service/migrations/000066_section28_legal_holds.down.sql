DELETE FROM fleet_partner_role_permissions WHERE permission_code IN
    ('privacy.legal_hold.read', 'privacy.legal_hold.manage', 'privacy.legal_hold.release');
DELETE FROM fleet_partner_permissions WHERE code IN
    ('privacy.legal_hold.read', 'privacy.legal_hold.manage', 'privacy.legal_hold.release');

DROP TABLE IF EXISTS fleet_partner_legal_holds;
