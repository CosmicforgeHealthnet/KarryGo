-- Section 28/29/30: organization-configurable retention policy overrides,
-- one row per (organization, retention_class) that has ever been explicitly
-- set. A missing row means the class sits at the platform minimum (see
-- internal/features/retentionpolicy/models.PlatformMinimumDays) rather than
-- a separate "unset" state in the database. Distinct from
-- fleet_partner_legal_holds (migration 000066): a legal hold blocks deletion
-- outright regardless of what this table says; this table only governs the
-- ordinary, non-held retention duration.
CREATE TABLE IF NOT EXISTS fleet_partner_retention_policies (
    organization_id  text NOT NULL,
    retention_class  text NOT NULL
                     CHECK (retention_class IN (
                         'TEMPORARY', 'OPERATIONAL', 'CUSTOMER_RECORD', 'PROVIDER_RECORD',
                         'TRIP_EXECUTION', 'SAFETY_EVIDENCE', 'INCIDENT_EVIDENCE', 'COMPLIANCE',
                         'FINANCIAL', 'PAYROLL', 'SUPPORT', 'AUDIT', 'LEGAL_HOLD', 'COMPANY_CONFIDENTIAL'
                     )),
    retention_days   integer NOT NULL CHECK (retention_days >= 0),
    version          bigint NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, retention_class)
);

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('privacy.retention.read', 'privacy', 'Read organization retention policy', true),
    ('privacy.retention.manage', 'privacy', 'Change organization retention policy', true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p
WHERE r.id IN ('rol_system_owner', 'rol_system_compliance')
  AND p.code IN ('privacy.retention.read', 'privacy.retention.manage')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, 'privacy.retention.read' FROM fleet_partner_roles r
WHERE r.id IN ('rol_system_admin', 'rol_system_operations')
ON CONFLICT DO NOTHING;
