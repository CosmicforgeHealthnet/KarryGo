-- Section 12B inventory authorities. Stock mutation remains separate from
-- catalogue and pricing authority.
INSERT INTO vendor_permissions (id,code,description,category,is_high_risk) VALUES
 ('perm_shop_inventory_read','vendor.shop.inventory.read','Read branch balances, lots, movements, availability and alerts.','shop_inventory',false),
 ('perm_shop_inventory_receive','vendor.shop.inventory.receive','Create and post vendor-owned stock receipts.','shop_inventory',false),
 ('perm_shop_inventory_adjust','vendor.shop.inventory.adjust','Post reasoned stock adjustments, damage and waste.','shop_inventory',true),
 ('perm_shop_inventory_transfer','vendor.shop.inventory.transfer','Move vendor-owned stock between branches of the same business.','shop_inventory',true),
 ('perm_shop_inventory_stocktake','vendor.shop.inventory.stocktake','Run and post stocktakes and cycle counts.','shop_inventory',true),
 ('perm_shop_inventory_reserve','vendor.shop.inventory.reserve','Hold, confirm, commit and release stock reservations.','shop_inventory',true),
 ('perm_shop_inventory_quarantine','vendor.shop.inventory.quarantine','Quarantine, release or dispose vendor-owned stock.','shop_inventory',true),
 ('perm_shop_inventory_policy','vendor.shop.inventory.manage_policy','Manage preorder, backorder and reorder policy.','shop_inventory',true),
 ('perm_shop_bulk_import','vendor.shop.bulk_import.manage','Validate and apply bounded Shop bulk operations.','shop_inventory',true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT role.id,p.id FROM (VALUES ('role_owner'),('role_general_manager')) role(id)
CROSS JOIN vendor_permissions p WHERE p.code LIKE 'vendor.shop.inventory.%' OR p.code='vendor.shop.bulk_import.manage'
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions WHERE code IN (
 'vendor.shop.inventory.read','vendor.shop.inventory.receive','vendor.shop.inventory.adjust',
 'vendor.shop.inventory.transfer','vendor.shop.inventory.stocktake','vendor.shop.inventory.reserve',
 'vendor.shop.inventory.quarantine','vendor.shop.inventory.manage_policy') ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_shop_inventory',id FROM vendor_permissions WHERE code IN (
 'vendor.shop.inventory.read','vendor.shop.inventory.receive','vendor.shop.inventory.adjust',
 'vendor.shop.inventory.transfer','vendor.shop.inventory.stocktake','vendor.shop.inventory.reserve',
 'vendor.shop.inventory.quarantine','vendor.shop.bulk_import.manage') ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_procurement',id FROM vendor_permissions WHERE code IN (
 'vendor.shop.inventory.read','vendor.shop.inventory.receive') ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',id FROM vendor_permissions WHERE code='vendor.shop.inventory.read'
ON CONFLICT DO NOTHING;

-- Marketing, Finance and Kitchen/Pastry receive no inventory mutation grant.

