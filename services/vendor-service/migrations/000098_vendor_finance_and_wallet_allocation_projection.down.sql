DELETE FROM vendor_role_permissions WHERE permission_id = 'perm_vendor_finance_withdraw';
DELETE FROM vendor_permissions WHERE id = 'perm_vendor_finance_withdraw';
ALTER TABLE commerce_payment_legs DROP COLUMN IF EXISTS allocation_snapshot_fingerprint;
