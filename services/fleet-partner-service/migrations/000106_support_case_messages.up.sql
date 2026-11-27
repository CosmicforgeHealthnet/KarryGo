-- Spec 59.10: POST /support-cases/{id}/messages (support.manage), event
-- support_message.created.
--
-- support-dispute-service is the one store of platform support conversations:
-- it keeps the message text, its files and its visibility. This table is only
-- Fleet's submission ledger. It holds no message text: just what Fleet needs
-- to submit a message exactly once per client message id, read back an
-- ambiguous outcome instead of resending blindly, and announce
-- support_message.created once.
CREATE TABLE fleet_partner_support_case_messages (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 projection_id text NOT NULL REFERENCES fleet_partner_support_case_projections(id) ON DELETE RESTRICT,
 support_service_case_id text NOT NULL,
 client_message_id uuid NOT NULL,
 request_hash text NOT NULL,
 visibility text NOT NULL CHECK (visibility IN ('SHARED','INTERNAL')),
 file_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(file_ids) = 'array' AND jsonb_array_length(file_ids) <= 10),
 status text NOT NULL DEFAULT 'SUBMITTING' CHECK (status IN ('SUBMITTING','OUTCOME_UNKNOWN','CONFIRMED','REJECTED')),
 support_message_id text NOT NULL DEFAULT '',
 attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 last_error_code text NOT NULL DEFAULT '',
 created_by text NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 confirmed_at timestamptz,
 UNIQUE (organization_id, support_service_case_id, client_message_id),
 CHECK ((status = 'CONFIRMED') = (confirmed_at IS NOT NULL AND support_message_id <> ''))
);
CREATE INDEX idx_fleet_support_messages_unknown ON fleet_partner_support_case_messages (organization_id, updated_at)
 WHERE status = 'OUTCOME_UNKNOWN';

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('support.manage','section23','Post messages on organization platform support cases',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
-- The roles that may already open platform support cases may also write on them.
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code='support.manage'
WHERE r.id IN('rol_system_owner','rol_system_operations','rol_system_compliance') ON CONFLICT DO NOTHING;
