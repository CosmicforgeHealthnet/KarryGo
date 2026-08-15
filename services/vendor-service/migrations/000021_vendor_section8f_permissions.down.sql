DELETE FROM vendor_role_permissions WHERE permission_id IN
 (SELECT id FROM vendor_permissions WHERE code IN ('vendor.crm.campaigns.execute','vendor.crm.campaigns.runs.read','vendor.crm.campaigns.runs.manage','vendor.crm.campaigns.deliveries.read'));
DELETE FROM vendor_permissions WHERE code IN
 ('vendor.crm.campaigns.execute','vendor.crm.campaigns.runs.read','vendor.crm.campaigns.runs.manage','vendor.crm.campaigns.deliveries.read');
