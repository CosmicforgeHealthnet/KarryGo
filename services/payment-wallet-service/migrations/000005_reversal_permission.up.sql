-- Migration 000005: Reversal permission
-- Adds the finance.reversal.post permission to the permission catalogue and
-- grants it to the finance_admin role only.  Fleet (finance_operator) does
-- NOT hold this permission; it submits reversal commands via the durable
-- queue and the Wallet service's own identity (finance_admin) executes them.

INSERT INTO finance_permissions (code, description, sensitive) VALUES
    ('finance.reversal.post', 'Post a compensating reversal journal entry', true)
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, sensitive = EXCLUDED.sensitive;

INSERT INTO finance_role_permissions (role_id, permission_code)
SELECT 'frol_finance_admin', 'finance.reversal.post'
ON CONFLICT DO NOTHING;
