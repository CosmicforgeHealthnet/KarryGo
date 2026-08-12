-- Section 28: generic (non-document) legal hold. Document-level legal hold
-- already exists (fleet_partner_documents.legal_hold, migration 000001) and
-- propagates to media-file-service via documents' own SetRetention action;
-- this table intentionally does not cover DOCUMENT scope for that reason
-- (see internal/features/legalhold/models package doc).
CREATE TABLE IF NOT EXISTS fleet_partner_legal_holds (
    id                     text PRIMARY KEY,
    organization_id        text NOT NULL,
    scope_type             text NOT NULL
                           CHECK (scope_type IN (
                               'ORGANIZATION', 'CUSTOMER', 'PROVIDER', 'TRIP', 'ASSIGNMENT',
                               'INCIDENT', 'SUPPORT_CASE', 'INVOICE', 'EXPORT', 'AUDIT_RANGE'
                           )),
    scope_reference        text NOT NULL,
    reason                 text NOT NULL,
    status                 text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'RELEASED')),
    created_by_actor_type  text NOT NULL,
    created_by_actor_id    text NOT NULL,
    approved_by_actor_id   text,
    step_up_reference      text,
    released_by_actor_id   text,
    release_reason         text,
    created_at             timestamptz NOT NULL DEFAULT now(),
    released_at            timestamptz,
    version                bigint NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_legal_holds_scope
    ON fleet_partner_legal_holds (organization_id, scope_type, scope_reference, status);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_legal_holds_org_created
    ON fleet_partner_legal_holds (organization_id, created_at DESC);

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('privacy.legal_hold.read', 'privacy', 'Read legal holds', true),
    ('privacy.legal_hold.manage', 'privacy', 'Create legal holds', true),
    ('privacy.legal_hold.release', 'privacy', 'Release legal holds', true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p
WHERE r.id IN ('rol_system_owner', 'rol_system_compliance')
  AND p.code IN ('privacy.legal_hold.read', 'privacy.legal_hold.manage', 'privacy.legal_hold.release')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, 'privacy.legal_hold.read' FROM fleet_partner_roles r
WHERE r.id IN ('rol_system_admin', 'rol_system_operations')
ON CONFLICT DO NOTHING;
