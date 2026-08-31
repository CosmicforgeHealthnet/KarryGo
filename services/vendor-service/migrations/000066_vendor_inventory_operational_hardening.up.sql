-- Section 14H
-- Workers, IAM, internal APIs, events and operational hardening.
--
-- This extends the existing vendor-service recurring jobs architecture.
-- vendor_job_controls stores worker coordination only; it is not a second
-- business task queue.

CREATE TABLE vendor_job_controls (
    job_name                text PRIMARY KEY,
    status                  text NOT NULL DEFAULT 'READY',
    next_run_at             timestamptz NOT NULL DEFAULT now(),
    lease_owner             text,
    lease_expires_at        timestamptz,
    heartbeat_at            timestamptz,
    attempt_count           integer NOT NULL DEFAULT 0,
    max_attempts            integer NOT NULL DEFAULT 5,
    replay_count            integer NOT NULL DEFAULT 0,
    last_error_redacted     text NOT NULL DEFAULT '',
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_job_controls_status_check
        CHECK (
            status IN (
                'READY',
                'RUNNING',
                'RETRY',
                'DEAD_LETTER'
            )
        ),

    CONSTRAINT vendor_job_controls_attempt_check
        CHECK (
            attempt_count >= 0
            AND max_attempts BETWEEN 1 AND 25
            AND replay_count >= 0
        )
);

CREATE INDEX vendor_job_controls_due_idx
    ON vendor_job_controls (
        next_run_at,
        job_name
    )
    WHERE status IN ('READY', 'RETRY');

CREATE INDEX vendor_job_controls_lease_idx
    ON vendor_job_controls (
        lease_expires_at,
        job_name
    )
    WHERE status = 'RUNNING';

-- Exactly-once low-stock event evidence.
CREATE TABLE inventory_alert_event_emissions (
    business_id     text NOT NULL,
    alert_id        text NOT NULL,
    event_type      text NOT NULL,
    emitted_at      timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (
        business_id,
        alert_id,
        event_type
    )
);

-- Existing generic Inventory permissions are reused where their semantics are
-- already correct. New permissions are only added for authorities where the
-- broad vendor.inventory.manage permission is insufficiently precise.

INSERT INTO vendor_permissions (
    id,
    code,
    description,
    category,
    is_high_risk
)
VALUES
    (
        'perm_vendor_inventory_adjust',
        'vendor.inventory.adjust',
        'Post reasoned Inventory adjustments, damage and waste.',
        'inventory',
        true
    ),
    (
        'perm_vendor_inventory_reserve',
        'vendor.inventory.reserve',
        'Hold, confirm, commit and release Inventory reservations.',
        'inventory',
        false
    ),
    (
        'perm_vendor_inventory_quarantine',
        'vendor.inventory.quarantine',
        'Quarantine, release or dispose Inventory stock.',
        'inventory',
        true
    ),
    (
        'perm_vendor_inventory_policy',
        'vendor.inventory.policy',
        'Manage Inventory reorder and stock policy.',
        'inventory',
        true
    )
ON CONFLICT (id) DO NOTHING;

INSERT INTO vendor_role_permissions (
    role_id,
    permission_id
)
SELECT
    r.id,
    p.id
FROM vendor_roles r
CROSS JOIN vendor_permissions p
WHERE r.id IN (
    'role_owner',
    'role_general_manager'
)
AND p.code IN (
    'vendor.inventory.adjust',
    'vendor.inventory.reserve',
    'vendor.inventory.quarantine',
    'vendor.inventory.policy'
)
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (
    role_id,
    permission_id
)
SELECT
    'role_branch_manager',
    id
FROM vendor_permissions
WHERE code IN (
    'vendor.inventory.read',
    'vendor.inventory.manage',
    'vendor.inventory.receive',
    'vendor.inventory.adjust',
    'vendor.inventory.reserve',
    'vendor.inventory.transfer',
    'vendor.inventory.stocktake',
    'vendor.inventory.quarantine',
    'vendor.inventory.policy'
)
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (
    role_id,
    permission_id
)
SELECT
    'role_shop_inventory',
    id
FROM vendor_permissions
WHERE code IN (
    'vendor.inventory.read',
    'vendor.inventory.manage',
    'vendor.inventory.receive',
    'vendor.inventory.adjust',
    'vendor.inventory.reserve',
    'vendor.inventory.transfer',
    'vendor.inventory.stocktake',
    'vendor.inventory.quarantine',
    'vendor.inventory.policy'
)
ON CONFLICT DO NOTHING;

-- Recipe/ingredient authority remains Section 16.
INSERT INTO vendor_role_permissions (
    role_id,
    permission_id
)
SELECT
    'role_kitchen_pastry',
    id
FROM vendor_permissions
WHERE code = 'vendor.inventory.read'
ON CONFLICT DO NOTHING;

-- Supplier/PO authority remains Section 15.
INSERT INTO vendor_role_permissions (
    role_id,
    permission_id
)
SELECT
    'role_procurement',
    id
FROM vendor_permissions
WHERE code IN (
    'vendor.inventory.read',
    'vendor.inventory.receive'
)
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (
    role_id,
    permission_id
)
SELECT
    'role_analyst_auditor',
    id
FROM vendor_permissions
WHERE code = 'vendor.inventory.read'
ON CONFLICT DO NOTHING;