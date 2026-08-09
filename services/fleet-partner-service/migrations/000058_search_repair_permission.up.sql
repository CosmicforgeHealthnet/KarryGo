-- Section 25 gap-close: an operator-facing repair permission for the search
-- projection. search.read only lets a caller query the index; repairing a
-- drifted or missing entry (force reindex, mark unavailable) is a distinct,
-- narrower-granted capability, matching this service's existing convention
-- of separating "read" from "act" permissions everywhere else (documents,
-- reconciliation cases, deletion requests).

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('search.repair', 'search', 'Manually reindex or repair a search projection entry', true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, p.code
FROM fleet_partner_roles r
JOIN fleet_partner_permissions p ON p.code = 'search.repair'
WHERE r.id IN ('rol_system_owner', 'rol_system_admin', 'rol_system_operations')
ON CONFLICT DO NOTHING;
