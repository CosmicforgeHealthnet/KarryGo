ALTER TABLE commerce_payment_legs
    ADD COLUMN allocation_snapshot_fingerprint text NOT NULL DEFAULT '';

INSERT INTO vendor_permissions (id, code, description, category, is_high_risk)
VALUES ('perm_vendor_finance_withdraw', 'vendor.finance.withdraw', 'Withdraw available Vendor payable funds.', 'finance', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT role_id, 'perm_vendor_finance_withdraw'
FROM (VALUES ('role_owner'), ('role_finance')) AS roles(role_id)
ON CONFLICT DO NOTHING;
