DELETE FROM fleet_partner_role_permissions WHERE permission_code IN (
    'marketplace.read', 'marketplace.accept', 'marketplace.decline',
    'marketplace.assign', 'marketplace.cancel', 'marketplace.reassign');

DELETE FROM fleet_partner_permissions WHERE code IN (
    'marketplace.read', 'marketplace.accept', 'marketplace.decline',
    'marketplace.assign', 'marketplace.cancel', 'marketplace.reassign');