-- Section 28: audit-log read permissions. The append-only audit table and
-- its BEFORE UPDATE OR DELETE trigger have existed since migration 000001;
-- this is the first migration to grant any role the ability to read it
-- through the API (internal/features/audit/http).
--
-- Two tiers, deliberately separate: audit.read yields actor/action/resource
-- summaries; audit.read_sensitive additionally unlocks the redacted
-- before/after value summaries and network metadata a record carries.
-- audit.read_sensitive is granted narrowly (owner + compliance only),
-- matching the documents.download_restricted precedent from migration
-- 000057 (RESTRICTED/COMPANY_CONFIDENTIAL documents are not extended to
-- rol_system_admin either).
INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('audit.read', 'audit', 'Read audit-log actor, action and resource summaries', true),
    ('audit.read_sensitive', 'audit', 'Read audit-log before/after value summaries and network metadata', true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, 'audit.read' FROM fleet_partner_roles r
WHERE r.id IN ('rol_system_owner', 'rol_system_admin', 'rol_system_operations', 'rol_system_compliance')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, 'audit.read_sensitive' FROM fleet_partner_roles r
WHERE r.id IN ('rol_system_owner', 'rol_system_compliance')
ON CONFLICT DO NOTHING;
