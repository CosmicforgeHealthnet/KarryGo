-- Default permission catalogue and system role families for Section 5.
-- Deterministic ids (perm_<code with dots as underscores>, role_<code>) so
-- this file can be safely re-run (ON CONFLICT DO NOTHING) and referenced by
-- other seed statements in the same migration.

INSERT INTO vendor_permissions (id, code, description, category, is_high_risk) VALUES
    ('perm_vendor_business_read', 'vendor.business.read', 'View vendor business settings.', 'business', false),
    ('perm_vendor_business_manage', 'vendor.business.manage', 'Manage vendor business settings.', 'business', false),
    ('perm_vendor_branches_read', 'vendor.branches.read', 'View vendor branches.', 'business', false),
    ('perm_vendor_branches_manage', 'vendor.branches.manage', 'Manage vendor branches.', 'business', false),
    ('perm_vendor_staff_read', 'vendor.staff.read', 'View vendor staff.', 'iam', false),
    ('perm_vendor_staff_manage', 'vendor.staff.manage', 'Provision, invite and update vendor staff.', 'iam', false),
    ('perm_vendor_staff_suspend', 'vendor.staff.suspend', 'Suspend or reactivate vendor staff.', 'iam', false),
    ('perm_vendor_roles_read', 'vendor.roles.read', 'View vendor roles and permission catalogue.', 'iam', false),
    ('perm_vendor_roles_manage', 'vendor.roles.manage', 'Create, edit or deactivate vendor roles.', 'iam', false),
    ('perm_vendor_roles_assign', 'vendor.roles.assign', 'Assign or remove staff role assignments.', 'iam', false),
    ('perm_vendor_orders_read', 'vendor.orders.read', 'View vendor orders.', 'commerce', false),
    ('perm_vendor_orders_manage', 'vendor.orders.manage', 'Accept, reject or manage vendor orders.', 'commerce', false),
    ('perm_vendor_inventory_read', 'vendor.inventory.read', 'View inventory/stock levels.', 'inventory', false),
    ('perm_vendor_inventory_manage', 'vendor.inventory.manage', 'Manage inventory records.', 'inventory', false),
    ('perm_vendor_inventory_receive', 'vendor.inventory.receive', 'Receive stock into inventory.', 'inventory', false),
    ('perm_vendor_inventory_transfer', 'vendor.inventory.transfer', 'Transfer stock between branches.', 'inventory', false),
    ('perm_vendor_inventory_stocktake', 'vendor.inventory.stocktake', 'Perform inventory stocktakes.', 'inventory', false),
    ('perm_vendor_purchasing_read', 'vendor.purchasing.read', 'View purchase orders and suppliers.', 'purchasing', false),
    ('perm_vendor_purchasing_manage', 'vendor.purchasing.manage', 'Manage purchase orders and suppliers.', 'purchasing', false),
    ('perm_vendor_purchasing_approve', 'vendor.purchasing.approve', 'Approve purchase orders.', 'purchasing', false),
    ('perm_vendor_production_read', 'vendor.production.read', 'View production/kitchen queue.', 'production', false),
    ('perm_vendor_production_manage', 'vendor.production.manage', 'Manage production/kitchen workflow.', 'production', false),
    ('perm_vendor_crm_read', 'vendor.crm.read', 'View vendor CRM records.', 'crm', false),
    ('perm_vendor_crm_manage', 'vendor.crm.manage', 'Manage vendor CRM records.', 'crm', false),
    ('perm_vendor_marketing_read', 'vendor.marketing.read', 'View promotions/marketing.', 'marketing', false),
    ('perm_vendor_marketing_manage', 'vendor.marketing.manage', 'Manage promotions/marketing.', 'marketing', false),
    ('perm_vendor_finance_read', 'vendor.finance.read', 'View sales, payouts and cost reports.', 'finance', false),
    ('perm_vendor_finance_refund', 'vendor.finance.refund', 'Issue vendor-side refunds.', 'finance', false),
    ('perm_vendor_finance_bank_change', 'vendor.finance.bank_change', 'Change vendor bank/payout account.', 'finance', true),
    ('perm_vendor_delivery_read', 'vendor.delivery.read', 'View delivery/fulfilment status.', 'delivery', false),
    ('perm_vendor_delivery_manage', 'vendor.delivery.manage', 'Manage vendor-owned delivery.', 'delivery', false),
    ('perm_vendor_delivery_execute_assigned', 'vendor.delivery.execute_assigned', 'Execute deliveries assigned to the acting rider.', 'delivery', false),
    ('perm_vendor_reports_read', 'vendor.reports.read', 'View vendor operational reports.', 'reports', false),
    ('perm_vendor_audit_read', 'vendor.audit.read', 'View the vendor security/IAM audit trail.', 'audit', false),
    ('perm_vendor_owner_transfer', 'vendor.owner.transfer', 'Transfer vendor business ownership.', 'iam', true),
    ('perm_vendor_data_export_sensitive', 'vendor.data.export_sensitive', 'Export sensitive vendor data.', 'data', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vendor_roles (id, vendor_id, code, name, description, is_system) VALUES
    ('role_owner', NULL, 'owner', 'Owner / Account Controller', 'Full business control, including ownership transfer and bank changes (still step-up gated).', true),
    ('role_general_manager', NULL, 'general_manager', 'General Manager', 'Operational business scope excluding owner-only controls.', true),
    ('role_branch_manager', NULL, 'branch_manager', 'Branch Manager', 'Scoped to the branch(es) explicitly assigned.', true),
    ('role_kitchen_pastry', NULL, 'kitchen_pastry', 'Kitchen / Pastry', 'Production queue and readiness operations.', true),
    ('role_shop_inventory', NULL, 'shop_inventory', 'Shop / Inventory', 'Picking, stock, receiving, transfers, stocktakes.', true),
    ('role_procurement', NULL, 'procurement', 'Procurement', 'Suppliers, purchase orders, receiving follow-up.', true),
    ('role_crm_support', NULL, 'crm_support', 'CRM / Support', 'Vendor customer profiles, follow-ups, issue coordination.', true),
    ('role_marketing', NULL, 'marketing', 'Marketing', 'Promotions, coupons, merchandising.', true),
    ('role_finance', NULL, 'finance', 'Finance', 'Sales, payouts, refunds, cost reports. Bank changes require step-up.', true),
    ('role_vendor_rider', NULL, 'vendor_rider', 'Vendor Rider', 'Assigned vendor-owned delivery responsibilities only. Not a KarryGo marketplace provider.', true),
    ('role_analyst_auditor', NULL, 'analyst_auditor', 'Analyst / Auditor', 'Read-only scoped access.', true)
ON CONFLICT (id) DO NOTHING;

-- Owner: every permission.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_owner', id FROM vendor_permissions
ON CONFLICT DO NOTHING;

-- General Manager: everything except owner-only controls (ownership
-- transfer, bank change, role definition management, sensitive data export).
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_general_manager', id FROM vendor_permissions
WHERE code NOT IN ('vendor.owner.transfer', 'vendor.finance.bank_change', 'vendor.roles.manage', 'vendor.data.export_sensitive')
ON CONFLICT DO NOTHING;

-- Branch Manager includes vendor.staff.manage (not just .read) per the
-- specification's "branch staff where permitted" — combined with
-- branch-scoped assignment (branch_id set on the role assignment, not a
-- separate permission), this lets a Branch Manager invite/manage staff
-- within their own branch, never vendor-wide and never another branch
-- (enforced by the delegation/branch-scope check in
-- staffusecases.assertActorMayDelegateRole, not by this permission grant
-- alone).
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_branch_manager', id FROM vendor_permissions
WHERE code IN ('vendor.orders.read', 'vendor.orders.manage', 'vendor.inventory.read', 'vendor.inventory.manage',
               'vendor.staff.read', 'vendor.staff.manage', 'vendor.delivery.read', 'vendor.delivery.manage',
               'vendor.delivery.execute_assigned', 'vendor.reports.read')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_kitchen_pastry', id FROM vendor_permissions
WHERE code IN ('vendor.production.read', 'vendor.production.manage')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_shop_inventory', id FROM vendor_permissions
WHERE code IN ('vendor.inventory.read', 'vendor.inventory.manage', 'vendor.inventory.receive',
               'vendor.inventory.transfer', 'vendor.inventory.stocktake')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_procurement', id FROM vendor_permissions
WHERE code IN ('vendor.purchasing.read', 'vendor.purchasing.manage', 'vendor.purchasing.approve')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_crm_support', id FROM vendor_permissions
WHERE code IN ('vendor.crm.read', 'vendor.crm.manage')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_marketing', id FROM vendor_permissions
WHERE code IN ('vendor.marketing.read', 'vendor.marketing.manage')
ON CONFLICT DO NOTHING;

-- Finance: bank_change permission is granted but remains high_risk, so using
-- it still requires a fresh step-up grant regardless of role membership.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_finance', id FROM vendor_permissions
WHERE code IN ('vendor.finance.read', 'vendor.finance.refund', 'vendor.finance.bank_change', 'vendor.reports.read')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_vendor_rider', id FROM vendor_permissions
WHERE code = 'vendor.delivery.execute_assigned'
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_analyst_auditor', id FROM vendor_permissions
WHERE code LIKE '%.read' OR code = 'vendor.audit.read'
ON CONFLICT DO NOTHING;
