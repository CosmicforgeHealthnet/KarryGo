DELETE FROM vendor_role_permissions
WHERE permission_id IN (
    SELECT id
    FROM vendor_permissions
    WHERE code IN (
        'vendor.inventory.adjust',
        'vendor.inventory.reserve',
        'vendor.inventory.quarantine',
        'vendor.inventory.policy'
    )
);

DELETE FROM vendor_permissions
WHERE code IN (
    'vendor.inventory.adjust',
    'vendor.inventory.reserve',
    'vendor.inventory.quarantine',
    'vendor.inventory.policy'
);

DROP TABLE IF EXISTS inventory_alert_event_emissions;
DROP TABLE IF EXISTS vendor_job_controls;