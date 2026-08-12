-- Section 23.1: organization communication metadata and notification intent policy.
CREATE TABLE fleet_partner_communication_threads (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 subject_type text NOT NULL CHECK(subject_type IN('CUSTOMER','PROVIDER','ORDER','SHIPMENT','ROUTE','TRIP','INCIDENT','CLAIM','INVOICE','SUPPORT_CASE','QUALITY_CASE','OTHER')),
 subject_reference text NOT NULL, title text NOT NULL,
 visibility text NOT NULL CHECK(visibility IN('INTERNAL','CUSTOMER_VISIBLE','PROVIDER_VISIBLE','RESTRICTED')),
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','PENDING_RESPONSE','RESOLVED','CLOSED','ARCHIVED')),
 created_by text NOT NULL, owner_user_id text NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), closed_at timestamptz
);
CREATE INDEX idx_fleet_communication_threads_scope ON fleet_partner_communication_threads(organization_id,branch_id,status,subject_type,subject_reference);

CREATE TABLE fleet_partner_communication_thread_history (
 id text PRIMARY KEY, organization_id text NOT NULL, thread_id text NOT NULL REFERENCES fleet_partner_communication_threads(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, snapshot jsonb NOT NULL, actor_reference text NOT NULL,
 version bigint NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_communication_thread_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_communication_thread_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_communication_participants (
 thread_id text NOT NULL REFERENCES fleet_partner_communication_threads(id) ON DELETE RESTRICT,
 organization_id text NOT NULL, participant_type text NOT NULL CHECK(participant_type IN('USER','CUSTOMER','PROVIDER','SERVICE')),
 participant_reference text NOT NULL, access_level text NOT NULL DEFAULT 'READ_WRITE' CHECK(access_level IN('READ','READ_WRITE','RESTRICTED')),
 added_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(thread_id,participant_type,participant_reference)
);
CREATE TRIGGER trg_fleet_communication_participants_append_only BEFORE UPDATE OR DELETE ON fleet_partner_communication_participants FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_communication_messages (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 thread_id text NOT NULL REFERENCES fleet_partner_communication_threads(id) ON DELETE RESTRICT,
 sender_type text NOT NULL CHECK(sender_type IN('USER','CUSTOMER','PROVIDER','SYSTEM')),
 sender_reference text NOT NULL, channel text NOT NULL CHECK(channel IN('IN_APP','EMAIL','SMS','PUSH','WHATSAPP','INTERNAL_NOTE')),
 text text NOT NULL, media_file_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
 delivery_operation_reference text NOT NULL DEFAULT '', delivery_status text NOT NULL DEFAULT 'QUEUED' CHECK(delivery_status IN('QUEUED','SENT','DELIVERED','FAILED','NOT_APPLICABLE')),
 reply_to_message_id text REFERENCES fleet_partner_communication_messages(id) ON DELETE RESTRICT,
	 idempotency_key text NOT NULL, source_timestamp timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
	 UNIQUE(organization_id,idempotency_key)
);
CREATE INDEX idx_fleet_communication_messages_thread ON fleet_partner_communication_messages(organization_id,thread_id,created_at);
CREATE TRIGGER trg_fleet_communication_messages_append_only BEFORE UPDATE OR DELETE ON fleet_partner_communication_messages FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_notification_rules (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 event_type text NOT NULL, recipient_scope text NOT NULL, channels jsonb NOT NULL,
 quiet_hours jsonb NOT NULL DEFAULT '{}'::jsonb, timezone text NOT NULL,
 severity_threshold text NOT NULL DEFAULT 'INFO', escalation_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
 enabled boolean NOT NULL DEFAULT true, version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,event_type,recipient_scope)
);

CREATE TABLE fleet_partner_notification_intents (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 source_type text NOT NULL, source_reference text NOT NULL, recipient_type text NOT NULL, recipient_reference text NOT NULL,
 channels jsonb NOT NULL, template_code text NOT NULL, safe_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
 idempotency_key text NOT NULL, status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','SUBMITTING','OUTCOME_UNKNOWN','ACCEPTED','DELIVERED','FAILED','DEAD_LETTERED','NOT_APPLICABLE')),
 delivery_operation_reference text NOT NULL DEFAULT '', attempts integer NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now(),
 last_error_code text NOT NULL DEFAULT '', source_version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,idempotency_key)
);
CREATE INDEX idx_fleet_notification_intents_due ON fleet_partner_notification_intents(status,next_attempt_at) WHERE status IN('PENDING','OUTCOME_UNKNOWN','FAILED');

CREATE TABLE fleet_partner_notification_delivery_attempts (
 id text PRIMARY KEY, organization_id text NOT NULL, intent_id text NOT NULL REFERENCES fleet_partner_notification_intents(id) ON DELETE RESTRICT,
 attempt_number integer NOT NULL, outcome text NOT NULL, operation_reference text NOT NULL DEFAULT '', error_code text NOT NULL DEFAULT '',
 started_at timestamptz NOT NULL, completed_at timestamptz NOT NULL, UNIQUE(intent_id,attempt_number)
);
CREATE TRIGGER trg_fleet_notification_delivery_attempts_append_only BEFORE UPDATE OR DELETE ON fleet_partner_notification_delivery_attempts FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('communications.read','section23','Read authorized communication threads',true),('communications.manage','section23','Manage communication threads',true),
('communications.send','section23','Send organization messages',true),('notification_rules.read','section23','Read notification rules',true),
('notification_rules.manage','section23','Manage notification rules',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('communications.read','communications.manage','communications.send','notification_rules.read','notification_rules.manage')
WHERE r.id IN('rol_system_owner','rol_system_operations') ON CONFLICT DO NOTHING;
