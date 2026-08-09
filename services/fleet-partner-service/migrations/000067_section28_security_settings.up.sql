-- Section 28: per-organization security settings. One versioned row per
-- organization; a missing row means the organization has never overridden
-- anything and reads as platform defaults (see
-- internal/features/securitysettings/models.Defaults) rather than a
-- separate "unset" state in the database.
CREATE TABLE IF NOT EXISTS fleet_partner_security_settings (
    organization_id                      text PRIMARY KEY,
    mfa_required_for_sensitive_actions   boolean NOT NULL DEFAULT true,
    step_up_freshness_seconds            integer NOT NULL DEFAULT 300,
    session_revocation_policy            text NOT NULL DEFAULT 'IMMEDIATE'
                                         CHECK (session_revocation_policy IN ('IMMEDIATE', 'STANDARD')),
    allowed_provider_access_policy       text NOT NULL DEFAULT 'ACTIVE_MEMBERSHIP_ONLY'
                                         CHECK (allowed_provider_access_policy IN ('ACTIVE_MEMBERSHIP_ONLY', 'ASSIGNED_ONLY')),
    sensitive_export_threshold_kobo      bigint NOT NULL DEFAULT 0,
    restricted_document_step_up_required boolean NOT NULL DEFAULT true,
    audit_capture_policy                 text NOT NULL DEFAULT 'FULL'
                                         CHECK (audit_capture_policy IN ('FULL', 'NONE')),
    retention_preference_days            integer NOT NULL DEFAULT 0,
    version                              bigint NOT NULL DEFAULT 1,
    created_at                           timestamptz NOT NULL DEFAULT now(),
    updated_at                           timestamptz NOT NULL DEFAULT now(),
    CHECK (step_up_freshness_seconds BETWEEN 300 AND 3600),
    CHECK (sensitive_export_threshold_kobo >= 0),
    CHECK (retention_preference_days >= 0)
);

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('security.settings.read', 'security', 'Read organization security settings', true),
    ('security.settings.manage', 'security', 'Change organization security settings', true),
    ('security.sessions.revoke', 'security', 'Revoke a corporate user session', true),
    ('security.credentials.manage', 'security', 'Manage service-to-service credentials', true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p
WHERE r.id IN ('rol_system_owner', 'rol_system_admin')
  AND p.code IN ('security.settings.read', 'security.settings.manage', 'security.sessions.revoke', 'security.credentials.manage')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, 'security.settings.read' FROM fleet_partner_roles r
WHERE r.id IN ('rol_system_compliance', 'rol_system_operations')
ON CONFLICT DO NOTHING;
