-- Section 9A permissions.
--
-- Three, split along the two lines that actually matter for a catalogue.
--
-- read/manage is the ordinary editorial split. publish is separate because
-- making a menu ACTIVE is the moment the catalogue becomes the business's
-- public commercial statement, and the person who types in a price is not
-- necessarily the person allowed to release it.
--
-- None is high-risk: editing a menu is not a bank change, an owner transfer or
-- a customer-data export, and marking it so would make step-up meaningless
-- where it genuinely matters.
INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_food_catalogue_read','vendor.food.catalogue.read','View food menus, sections, items, variants and modifiers.','food',false),
 ('perm_vendor_food_catalogue_manage','vendor.food.catalogue.manage','Create and edit food menus, sections, items, variants and modifiers.','food',false),
 ('perm_vendor_food_catalogue_publish','vendor.food.catalogue.publish','Activate, pause or archive a food menu.','food',false)
ON CONFLICT(id) DO NOTHING;

-- Owner and General Manager hold the whole surface business-wide.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_owner','role_general_manager')
  AND p.code IN ('vendor.food.catalogue.read','vendor.food.catalogue.manage','vendor.food.catalogue.publish')
ON CONFLICT DO NOTHING;

-- Branch manager runs their own branch's menu, end to end including publishing
-- it. The grant does not widen reach: scope is resolved per request from the
-- assignment, so a branch-scoped manager can only ever address menus at the
-- branches they are assigned to. Business-level modifier group definitions
-- remain out of reach for a branch-scoped grant — see the service's
-- requireBusinessWide rule — because a shared group edited from one branch
-- would change every other branch's items.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions
WHERE code IN ('vendor.food.catalogue.read','vendor.food.catalogue.manage','vendor.food.catalogue.publish')
ON CONFLICT DO NOTHING;

-- Kitchen/pastry keeps the menu accurate day to day but does not release it.
-- Writing a description and setting a price is the job; deciding that the
-- branch is now open for that menu is not.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_kitchen_pastry',id FROM vendor_permissions
WHERE code IN ('vendor.food.catalogue.read','vendor.food.catalogue.manage')
ON CONFLICT DO NOTHING;

-- Analyst/Auditor reads and nothing else, which is the whole point of the role.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',id FROM vendor_permissions
WHERE code = 'vendor.food.catalogue.read'
ON CONFLICT DO NOTHING;

-- Deliberately unmapped: marketing, CRM/support, finance, procurement,
-- shop/inventory and vendor rider. None of them owns what a restaurant sells,
-- and Section 9C — not this section — is where a read contract for other
-- consumers will be designed.
