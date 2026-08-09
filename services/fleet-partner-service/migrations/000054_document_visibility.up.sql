-- Section 25 gap-close: document visibility and archival.
--
-- internal/features/documents (WP10/Section 10) already is Section 25's
-- generic file-reference concept — Media-backed, zero bytes, reconciliation
-- workers with lease/heartbeat already built. Retention is already covered
-- by retention_class/retention_until/legal_hold. The two real gaps against
-- the Section 25 spec are visibility tiers and an explicit archive action,
-- both closed here additively; nothing about the existing lifecycle,
-- verification or deletion state machines changes.

ALTER TABLE fleet_partner_documents
    ADD COLUMN IF NOT EXISTS visibility text NOT NULL DEFAULT 'INTERNAL'
        CHECK (visibility IN ('INTERNAL', 'PROVIDER_VISIBLE', 'CUSTOMER_VISIBLE', 'RESTRICTED',
                               'COMPANY_CONFIDENTIAL')),
    ADD COLUMN IF NOT EXISTS archived_at timestamptz,
    ADD COLUMN IF NOT EXISTS archived_by_actor_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_visibility
    ON fleet_partner_documents (organization_id, visibility);

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------
--
-- Ordinary documents.download already gates download for INTERNAL,
-- PROVIDER_VISIBLE and CUSTOMER_VISIBLE files. RESTRICTED and
-- COMPANY_CONFIDENTIAL additionally require this permission — a caller who
-- can download ordinary documents does not automatically gain access to the
-- most sensitive tier. Deliberately narrow: the owner and the compliance and
-- safety manager (the role accountable for the organization's most sensitive
-- records) get it; the company administrator's existing documents.download
-- does not extend to it.

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('documents.download_restricted', 'documents',
     'Download RESTRICTED and COMPANY_CONFIDENTIAL documents', true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, p.code
FROM fleet_partner_roles r
JOIN fleet_partner_permissions p ON p.code = 'documents.download_restricted'
WHERE r.id IN ('rol_system_owner', 'rol_system_compliance')
ON CONFLICT DO NOTHING;
