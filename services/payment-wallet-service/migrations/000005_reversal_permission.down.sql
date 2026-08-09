-- Rollback: remove the reversal permission and its role grants.
DELETE FROM finance_role_permissions WHERE permission_code = 'finance.reversal.post';
DELETE FROM finance_permissions WHERE code = 'finance.reversal.post';
