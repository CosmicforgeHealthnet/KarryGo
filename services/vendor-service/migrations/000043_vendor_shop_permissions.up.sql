-- Section 12A Shop permissions.
--
-- Catalogue, pricing, measurement, listing, wholesale and fresh authorities
-- are separate grants rather than one "shop.manage". The separation is the
-- point: a stock clerk who will operate Part B's inventory must not inherit
-- wholesale pricing authority because someone once gave them catalogue
-- access, and marketing must never gain price authority by being able to
-- edit a product name.
INSERT INTO vendor_permissions (id, code, description, category, is_high_risk)
VALUES
 ('perm_shop_read', 'vendor.shop.read', 'Read Shop catalogue, measurements, listings and pricing.', 'shop', false),
 ('perm_shop_catalogue_manage', 'vendor.shop.catalogue.manage', 'Manage Shop categories, products, variants, packaging and identifiers.', 'shop', false),
 ('perm_shop_pricing_manage', 'vendor.shop.pricing.manage', 'Establish or change Shop retail prices.', 'shop', true),
 ('perm_shop_wholesale_manage', 'vendor.shop.wholesale.manage', 'Establish or change wholesale prices, quantity breaks and order minimums.', 'shop', true),
 ('perm_shop_measurements_manage', 'vendor.shop.measurements.manage', 'Manage measure definitions, aliases, calibrations and sell units.', 'shop', false),
 ('perm_shop_listings_manage', 'vendor.shop.listings.manage', 'Manage branch listings and branch commercial configuration.', 'shop', false),
 ('perm_shop_fresh_manage', 'vendor.shop.fresh.manage', 'Manage fresh-goods attributes and handling metadata.', 'shop', false)
ON CONFLICT (id) DO NOTHING;

-- Owner: every Shop authority.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_owner', id FROM vendor_permissions WHERE code IN (
 'vendor.shop.read','vendor.shop.catalogue.manage','vendor.shop.pricing.manage',
 'vendor.shop.wholesale.manage','vendor.shop.measurements.manage',
 'vendor.shop.listings.manage','vendor.shop.fresh.manage') ON CONFLICT DO NOTHING;

-- General Manager: business-wide commercial authority, scoped by assignment.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_general_manager', id FROM vendor_permissions WHERE code IN (
 'vendor.shop.read','vendor.shop.catalogue.manage','vendor.shop.pricing.manage',
 'vendor.shop.wholesale.manage','vendor.shop.measurements.manage',
 'vendor.shop.listings.manage','vendor.shop.fresh.manage') ON CONFLICT DO NOTHING;

-- Branch Manager: the branch's own listings, retail pricing and fresh
-- configuration. Deliberately NOT wholesale pricing, and not the shared
-- measurement vocabulary, both of which reach beyond one branch.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_branch_manager', id FROM vendor_permissions WHERE code IN (
 'vendor.shop.read','vendor.shop.listings.manage','vendor.shop.pricing.manage',
 'vendor.shop.fresh.manage') ON CONFLICT DO NOTHING;

-- Shop/Inventory role: catalogue and measurement operation, no price
-- authority of any kind. Part B's stock work will build on this grant.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_shop_inventory', id FROM vendor_permissions WHERE code IN (
 'vendor.shop.read','vendor.shop.catalogue.manage','vendor.shop.measurements.manage',
 'vendor.shop.fresh.manage') ON CONFLICT DO NOTHING;

-- Procurement reads the catalogue it buys against; it sets no sell price.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_procurement', id FROM vendor_permissions WHERE code IN (
 'vendor.shop.read') ON CONFLICT DO NOTHING;

-- Read-only observers.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_analyst_auditor', id FROM vendor_permissions WHERE code IN (
 'vendor.shop.read') ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_marketing', id FROM vendor_permissions WHERE code IN (
 'vendor.shop.read') ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_finance', id FROM vendor_permissions WHERE code IN (
 'vendor.shop.read') ON CONFLICT DO NOTHING;

-- role_kitchen_pastry receives NOTHING here. Managing food production is not
-- a reason to hold Shop authority, and the two domains share a business but
-- never a catalogue.
