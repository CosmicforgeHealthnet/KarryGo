-- Fleet is the policy and RBAC authority for corporate payroll commands. Give
-- its service identity only the Wallet permissions needed to post and reverse
-- provider compensation allocations; this is not an approval or general
-- finance-admin grant.
INSERT INTO finance_roles(id,name,description)
VALUES('frol_fleet_payroll_orchestrator','fleet_payroll_orchestrator','Fleet payroll allocation and compensating-reversal orchestrator')
ON CONFLICT(id) DO UPDATE SET description=excluded.description;

INSERT INTO finance_role_permissions(role_id,permission_code)
SELECT 'frol_fleet_payroll_orchestrator',code
FROM finance_permissions
WHERE code IN('finance.wallet.view','finance.transaction.view','finance.provider_allocation.post','finance.provider_allocation.reverse')
ON CONFLICT DO NOTHING;

INSERT INTO finance_role_grants(id,organization_id,subject_type,subject_id,role_id,granted_by)
VALUES('frgr_fleet_payroll_orchestrator',NULL,'SERVICE','fleet-partner-service','frol_fleet_payroll_orchestrator','migration')
ON CONFLICT DO NOTHING;
