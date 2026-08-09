-- WP8: finance operator access.
--
-- Adds the accountability columns an operator route needs — who asked for a
-- financial operation and which branch it belongs to — and seeds the finance
-- permission catalogue. Without the requester on the row there is no way to
-- enforce separation of duties later, and no way to answer "who did this?"
-- from the record itself.

ALTER TABLE fleet_partner_finance_operations
    ADD COLUMN IF NOT EXISTS requested_by_actor_type text NOT NULL DEFAULT 'SERVICE',
    ADD COLUMN IF NOT EXISTS requested_by_actor_id   text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS branch_id               text;

ALTER TABLE fleet_partner_finance_projections
    ADD COLUMN IF NOT EXISTS branch_id text;

CREATE INDEX IF NOT EXISTS idx_fleet_finance_operations_branch
    ON fleet_partner_finance_operations (organization_id, branch_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_finance_operations_requester
    ON fleet_partner_finance_operations (organization_id, requested_by_actor_id);
CREATE INDEX IF NOT EXISTS idx_fleet_finance_projections_branch
    ON fleet_partner_finance_projections (organization_id, branch_id);

-- Reading a financial operation, replaying one, and asking for money to move
-- are three different capabilities. Someone who may investigate a stuck payout
-- is not automatically someone who may re-run it, and neither of them is
-- automatically someone who may request a refund.
INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('finance.operations.read',        'finance', 'Read durable wallet operations and finance projections', true),
    ('finance.operations.replay',      'finance', 'Replay a dead-lettered wallet operation', true),
    ('finance.reconciliation.read',    'finance', 'Read finance reconciliation cases', true),
    ('finance.reconciliation.manage',  'finance', 'Replay reconciliation cases and trigger authoritative read-back', true),
    ('finance.settlements.request',    'finance', 'Request a provider or organization settlement', true),
    ('finance.refunds.request',        'finance', 'Request a refund', true),
    ('finance.reversals.request',      'finance', 'Request a reversal of a posted financial entry', true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code
FROM fleet_partner_permissions
WHERE code IN ('finance.operations.read', 'finance.operations.replay',
               'finance.reconciliation.read', 'finance.reconciliation.manage',
               'finance.settlements.request', 'finance.refunds.request',
               'finance.reversals.request')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_finance', code
FROM fleet_partner_permissions
WHERE code IN ('finance.operations.read', 'finance.operations.replay',
               'finance.reconciliation.read', 'finance.reconciliation.manage',
               'finance.settlements.request', 'finance.refunds.request',
               'finance.reversals.request')
ON CONFLICT DO NOTHING;

-- Administration, operations and compliance investigate; they do not move
-- money.
INSERT INTO fleet_partner_role_permissions (role_id, permission_code) VALUES
    ('rol_system_admin',      'finance.operations.read'),
    ('rol_system_admin',      'finance.reconciliation.read'),
    ('rol_system_operations', 'finance.operations.read'),
    ('rol_system_compliance', 'finance.operations.read'),
    ('rol_system_compliance', 'finance.reconciliation.read'),
    ('rol_system_analyst',    'finance.operations.read')
ON CONFLICT DO NOTHING;
