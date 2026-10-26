-- Section 16: Secure Admin owner boundary for support-dispute-service.
--
-- NOTE ON RE-RUNNING: this service's runner applies every file in migrations/
-- on every start, with no tracker. Every statement below is a no-op the second
-- time (IF NOT EXISTS, guarded DO blocks, CREATE OR REPLACE, and Support DDL
-- that runs only when its object is missing, so a re-run takes no lock on a
-- live case table). The reversal lives in migrations/down/.
--
-- Seeds nothing: no staff grant, no policy, no secret, no approval, no scope
-- mapping. Every runtime switch that uses these tables defaults off.
--
-- What is deliberately absent: an SLA policy (events say POLICY_UNAVAILABLE),
-- a locale column (events carry null), and any Wallet execution. A dispute
-- remedy only links a Wallet operation the Bridge has already confirmed.

-- Concurrent appliers (several service replicas, or parallel test packages)
-- serialize on this file; the whole file is one implicit transaction.
SELECT pg_advisory_xact_lock(hashtextextended('support-dispute-service:migration:009', 0));

-- Admin-attached evidence needs an uploader identity that is not a complainant.
-- The new value is not used anywhere in this file (it cannot be, before commit).
ALTER TYPE complainant_type ADD VALUE IF NOT EXISTS 'admin';

-- shared/go/admincontext/schema.sql, verbatim.
-- shared-template-sha256: f99f214835f16f9bc15336399a7e9cf00be8fea045bef30ff50b5d379dadac41
-- Install only through the consuming source owner's normal migration runner.
-- Each owner keeps these records in its own database; Admin never connects.
CREATE TABLE IF NOT EXISTS admin_owner_context_heads (
 owner text NOT NULL,context_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),
 checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),resource_kind text NOT NULL,resource_ref text NOT NULL,
 product text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 capabilities text[] NOT NULL,
 PRIMARY KEY(owner,context_id)
);
CREATE INDEX IF NOT EXISTS admin_owner_context_resource ON admin_owner_context_heads(owner,resource_kind,resource_ref);
CREATE INDEX IF NOT EXISTS admin_owner_context_scope ON admin_owner_context_heads(owner,resource_kind,product,market,zone,organization_ref,tenant_ref,resource_ref);
CREATE TABLE IF NOT EXISTS admin_owner_context_history (
 owner text NOT NULL,context_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),
 checksum text NOT NULL,envelope bytea NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner,context_id,revision)
);
CREATE OR REPLACE FUNCTION reject_admin_owner_context_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Source-owner authorization history is append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_context_history_immutable ON admin_owner_context_history;
CREATE TRIGGER admin_owner_context_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_history FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_context_history_mutation();
-- shared/go/adminowner/schema.sql, verbatim.
-- shared-template-sha256: fb1e87880df5e72d40bdbffb2fd932ce1b6f53647ef6548ecd09d5fed8d6319b
CREATE TABLE IF NOT EXISTS admin_owner_policy_revisions (
 policy_id text PRIMARY KEY,version bigint NOT NULL CHECK(version>0),checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),accepted_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS admin_owner_read_nonces (
 owner text NOT NULL,nonce_hash text NOT NULL,expires_at timestamptz NOT NULL,
 PRIMARY KEY(owner,nonce_hash)
);
CREATE INDEX IF NOT EXISTS admin_owner_read_nonce_expiry ON admin_owner_read_nonces(expires_at);
CREATE TABLE IF NOT EXISTS admin_owner_context_preparation_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,
 workload_principal text NOT NULL,purpose_code text NOT NULL,binding_fingerprint text NOT NULL,source_contract text NOT NULL,
 known_source_dimensions jsonb NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now()
);
DROP TRIGGER IF EXISTS admin_owner_context_preparation_immutable ON admin_owner_context_preparation_audit;
CREATE TRIGGER admin_owner_context_preparation_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_preparation_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_context_history_mutation();
CREATE TABLE IF NOT EXISTS admin_owner_read_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,
 staff_ref text NOT NULL,purpose_code text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 policy_id text NOT NULL,policy_version bigint NOT NULL,policy_checksum text NOT NULL,as_of timestamptz NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now(),outcome text NOT NULL DEFAULT 'AUTHORIZED'
);
CREATE OR REPLACE FUNCTION reject_admin_owner_read_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Owner administrative read audit is append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_read_audit_immutable ON admin_owner_read_audit;
CREATE TRIGGER admin_owner_read_audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_read_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_read_audit_mutation();
-- shared/go/admincommand/schema.sql, verbatim.
-- shared-template-sha256: 6b244aa0dce15e2f7300c9b461c4393b0dc0d81d7c25cb9f24d3030e09bd0f29
-- Source-owned administrative command decision record. Install only through
-- the consuming owner's normal migration runner; each owner keeps its own
-- immutable copy in its own database. The Admin Bridge never connects here.
CREATE TABLE IF NOT EXISTS admin_owner_command_decisions (
 idempotency_key text PRIMARY KEY,
 operation_id uuid NOT NULL UNIQUE,
 decision_id uuid NOT NULL UNIQUE,
 owner text NOT NULL,
 contract text NOT NULL,
 subject_kind text NOT NULL,
 subject_id text NOT NULL,
 case_ref text NOT NULL DEFAULT '',
 action text NOT NULL,
 version_scope text NOT NULL CHECK (version_scope IN ('SUBJECT','CASE')),
 version_key text NOT NULL,
 recorded_version bigint NOT NULL CHECK (recorded_version > 0),
 operation_digest text NOT NULL CHECK (operation_digest ~ '^[0-9a-f]{64}$'),
 intent_digest text NOT NULL CHECK (intent_digest ~ '^[0-9a-f]{64}$'),
 approval_id uuid UNIQUE,
 approver_ref text,
 staff_ref text NOT NULL,
 purpose_code text NOT NULL,
 product text NOT NULL,
 market text NOT NULL,
 zone text NOT NULL,
 organization_ref text NOT NULL,
 tenant_ref text NOT NULL,
 policy_id text NOT NULL,
 policy_version bigint NOT NULL CHECK (policy_version > 0),
 policy_checksum text NOT NULL CHECK (policy_checksum ~ '^[0-9a-f]{64}$'),
 receipt jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((approval_id IS NULL) = (approver_ref IS NULL)),
 CHECK (approver_ref IS NULL OR approver_ref <> staff_ref),
 UNIQUE (owner, subject_kind, subject_id, version_scope, version_key, recorded_version)
);
CREATE INDEX IF NOT EXISTS admin_owner_command_decisions_subject ON admin_owner_command_decisions(owner, subject_kind, subject_id, created_at DESC);
CREATE OR REPLACE FUNCTION reject_admin_owner_command_decision_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Owner administrative command decisions are append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_command_decisions_immutable ON admin_owner_command_decisions;
CREATE TRIGGER admin_owner_command_decisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_command_decisions FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_command_decision_mutation();

-- shared/go/adminevents.OutboxSchema, verbatim.
CREATE TABLE IF NOT EXISTS admin_event_outbox (
 id text PRIMARY KEY,
 event_type text NOT NULL,
 schema_version int NOT NULL CHECK (schema_version > 0),
 aggregate_type text NOT NULL,
 aggregate_id text NOT NULL,
 payload jsonb NOT NULL,
 status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PUBLISHED','DEAD_LETTERED')),
 available_at timestamptz NOT NULL DEFAULT now(),
 attempts int NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 lease_owner text,
 lease_expires_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 published_at timestamptz,
 last_error_redacted text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS admin_event_outbox_due_idx ON admin_event_outbox (status, available_at) WHERE status = 'PENDING';

-- ---------------------------------------------------------------------------
-- Lock-free re-runs
-- ---------------------------------------------------------------------------
-- ALTER TABLE / CREATE INDEX / CREATE TRIGGER take heavy table locks even when
-- IF [NOT] EXISTS makes them no-ops. Because this file re-runs on every boot
-- while other replicas serve traffic, each such statement below runs only when
-- its object is actually missing, so a re-run takes no table lock at all and
-- cannot deadlock with live case writes. A changed trigger definition
-- therefore needs a new migration; function bodies are replaced every run.
CREATE OR REPLACE FUNCTION support_admin_ddl_if(missing boolean, ddl text) RETURNS void
LANGUAGE plpgsql AS $fn$
BEGIN
    IF missing THEN
        EXECUTE ddl;
    END IF;
END $fn$;

CREATE OR REPLACE FUNCTION support_admin_lacks_column(tbl text, col text) RETURNS boolean
LANGUAGE sql STABLE AS $fn$
    SELECT NOT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid = to_regclass(tbl) AND attname = col AND NOT attisdropped)
$fn$;

CREATE OR REPLACE FUNCTION support_admin_lacks_trigger(tbl text, trg text) RETURNS boolean
LANGUAGE sql STABLE AS $fn$
    SELECT NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = to_regclass(tbl) AND tgname = trg)
$fn$;

-- ---------------------------------------------------------------------------
-- Native administrative versions, escalation and chat ordering
-- ---------------------------------------------------------------------------
SELECT support_admin_ddl_if(support_admin_lacks_column('complaints', 'admin_version'), 'ALTER TABLE complaints ADD COLUMN admin_version bigint NOT NULL DEFAULT 1');
SELECT support_admin_ddl_if(support_admin_lacks_column('complaints', 'admin_version_txid'), 'ALTER TABLE complaints ADD COLUMN admin_version_txid bigint');
SELECT support_admin_ddl_if(support_admin_lacks_column('complaints', 'escalation_level'), 'ALTER TABLE complaints ADD COLUMN escalation_level text');
SELECT support_admin_ddl_if(support_admin_lacks_column('complaints', 'message_seq'), 'ALTER TABLE complaints ADD COLUMN message_seq bigint NOT NULL DEFAULT 0');
DO $$ BEGIN
IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'complaints'::regclass AND conname = 'complaints_admin_version_chk') THEN
    ALTER TABLE complaints ADD CONSTRAINT complaints_admin_version_chk CHECK (admin_version > 0);
END IF;
IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'complaints'::regclass AND conname = 'complaints_message_seq_chk') THEN
    ALTER TABLE complaints ADD CONSTRAINT complaints_message_seq_chk CHECK (message_seq >= 0);
END IF;
IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'complaints'::regclass AND conname = 'complaints_escalation_level_chk') THEN
    ALTER TABLE complaints ADD CONSTRAINT complaints_escalation_level_chk CHECK (escalation_level IS NULL OR escalation_level IN
        ('OPERATIONS_REVIEW', 'SAFETY_REVIEW', 'DISPUTE_REVIEW', 'SAFETY_LEAD', 'EMERGENCY_LIAISON'));
END IF;
END $$;

SELECT support_admin_ddl_if(support_admin_lacks_column('disputes', 'admin_version'), 'ALTER TABLE disputes ADD COLUMN admin_version bigint NOT NULL DEFAULT 1');
SELECT support_admin_ddl_if(support_admin_lacks_column('disputes', 'admin_version_txid'), 'ALTER TABLE disputes ADD COLUMN admin_version_txid bigint');
SELECT support_admin_ddl_if(support_admin_lacks_column('disputes', 'remedy_state'), 'ALTER TABLE disputes ADD COLUMN remedy_state text NOT NULL DEFAULT ''NONE''');
SELECT support_admin_ddl_if(support_admin_lacks_column('disputes', 'remedy_wallet_operation_id'), 'ALTER TABLE disputes ADD COLUMN remedy_wallet_operation_id uuid');
SELECT support_admin_ddl_if(support_admin_lacks_column('disputes', 'remedy_amount_minor'), 'ALTER TABLE disputes ADD COLUMN remedy_amount_minor bigint');
SELECT support_admin_ddl_if(support_admin_lacks_column('disputes', 'remedy_recorded_at'), 'ALTER TABLE disputes ADD COLUMN remedy_recorded_at timestamptz');
SELECT support_admin_ddl_if(support_admin_lacks_column('disputes', 'remedy_recorded_by'), 'ALTER TABLE disputes ADD COLUMN remedy_recorded_by text');
DO $$ BEGIN
IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'disputes'::regclass AND conname = 'disputes_admin_version_chk') THEN
    ALTER TABLE disputes ADD CONSTRAINT disputes_admin_version_chk CHECK (admin_version > 0);
END IF;
-- A remedy is either absent, or a complete, positive, recorded link. There is
-- no "pending" remedy: Support never asks Wallet to move money.
IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'disputes'::regclass AND conname = 'disputes_remedy_chk') THEN
    ALTER TABLE disputes ADD CONSTRAINT disputes_remedy_chk CHECK (
        (remedy_state = 'NONE' AND remedy_wallet_operation_id IS NULL AND remedy_amount_minor IS NULL AND remedy_recorded_at IS NULL)
        OR (remedy_state = 'RECORDED' AND remedy_wallet_operation_id IS NOT NULL AND remedy_amount_minor > 0
            AND remedy_recorded_at IS NOT NULL AND remedy_recorded_by IS NOT NULL
            AND outcome IN ('favour_complainant', 'split')));
END IF;
END $$;
-- One Wallet operation can remedy at most one dispute.
SELECT support_admin_ddl_if(to_regclass('uq_disputes_remedy_wallet_operation') IS NULL, 'CREATE UNIQUE INDEX uq_disputes_remedy_wallet_operation
    ON disputes (remedy_wallet_operation_id) WHERE remedy_wallet_operation_id IS NOT NULL');

SELECT support_admin_ddl_if(support_admin_lacks_column('support_chat_messages', 'client_message_id'), 'ALTER TABLE support_chat_messages ADD COLUMN client_message_id uuid');
SELECT support_admin_ddl_if(support_admin_lacks_column('support_chat_messages', 'sequence'), 'ALTER TABLE support_chat_messages ADD COLUMN sequence bigint');
-- Backfill: number pre-existing messages per case in their historical order.
-- After the first run there is nothing NULL left, so both statements no-op.
UPDATE support_chat_messages m SET sequence = n.base + n.rn
FROM (SELECT s.id,
             row_number() OVER (PARTITION BY s.complaint_id ORDER BY s.created_at, s.id) AS rn,
             COALESCE((SELECT max(x.sequence) FROM support_chat_messages x WHERE x.complaint_id = s.complaint_id), 0) AS base
      FROM support_chat_messages s WHERE s.sequence IS NULL) n
WHERE m.id = n.id;
UPDATE complaints c SET message_seq = s.top
FROM (SELECT complaint_id, max(sequence) AS top FROM support_chat_messages GROUP BY complaint_id) s
WHERE s.complaint_id = c.id AND c.message_seq < s.top;
SELECT support_admin_ddl_if(NOT (SELECT attnotnull FROM pg_attribute WHERE attrelid = to_regclass('support_chat_messages') AND attname = 'sequence'),
    'ALTER TABLE support_chat_messages ALTER COLUMN sequence SET NOT NULL');
SELECT support_admin_ddl_if(to_regclass('uq_support_chat_messages_sequence') IS NULL, 'CREATE UNIQUE INDEX uq_support_chat_messages_sequence
    ON support_chat_messages (complaint_id, sequence)');
SELECT support_admin_ddl_if(to_regclass('uq_support_chat_messages_client_message') IS NULL, 'CREATE UNIQUE INDEX uq_support_chat_messages_client_message
    ON support_chat_messages (complaint_id, client_message_id) WHERE client_message_id IS NOT NULL');

-- ---------------------------------------------------------------------------
-- Native Support staff grants (none seeded)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS support_admin_staff_grants (
 id text PRIMARY KEY,
 staff_ref text NOT NULL,
 permission text NOT NULL,
 granted_by text NOT NULL,
 approval_ref text NOT NULL,
 expires_at timestamptz,
 revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(staff_ref, permission)
);
SELECT support_admin_ddl_if(to_regclass('support_admin_staff_grants_current') IS NULL, 'CREATE INDEX support_admin_staff_grants_current
    ON support_admin_staff_grants (staff_ref, permission) WHERE revoked_at IS NULL');

-- ---------------------------------------------------------------------------
-- Admin reply notification delivery: separate from the persisted message.
-- The message row is the truth; this only tracks the attempt to tell someone.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS support_admin_reply_deliveries (
 message_id uuid PRIMARY KEY REFERENCES support_chat_messages(id) ON DELETE CASCADE,
 complaint_id uuid NOT NULL REFERENCES complaints(id) ON DELETE CASCADE,
 state text NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING', 'SENT', 'FAILED')),
 attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 last_attempt_at timestamptz,
 delivered_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
SELECT support_admin_ddl_if(to_regclass('support_admin_reply_deliveries_due') IS NULL, 'CREATE INDEX support_admin_reply_deliveries_due
    ON support_admin_reply_deliveries (next_attempt_at) WHERE state = ''PENDING''');

-- One emitted event per (case, version), however many statements touched the
-- case inside the transaction that produced that version.
CREATE TABLE IF NOT EXISTS support_admin_case_emissions (
 complaint_id uuid NOT NULL REFERENCES complaints(id) ON DELETE CASCADE,
 entity_version bigint NOT NULL CHECK (entity_version > 0),
 event_id text NOT NULL UNIQUE,
 emitted_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (complaint_id, entity_version)
);

-- ---------------------------------------------------------------------------
-- Functions
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION support_admin_opaque(value text) RETURNS text
LANGUAGE sql IMMUTABLE AS $fn$
    SELECT CASE WHEN value ~ '^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$' THEN value END
$fn$;

CREATE OR REPLACE FUNCTION support_admin_rfc3339(value timestamptz) RETURNS text
LANGUAGE sql IMMUTABLE AS $fn$
    SELECT to_char(value AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
$fn$;

-- Bumps admin_version at most once per transaction, whenever the row changes
-- or a caller explicitly asks for a bump (admin_version = admin_version + 1).
-- Any explicitly supplied version is normalized: it can never jump or rewind.
CREATE OR REPLACE FUNCTION support_admin_version_update() RETURNS trigger
LANGUAGE plpgsql AS $fn$
BEGIN
    IF OLD.admin_version_txid IS NOT DISTINCT FROM txid_current() THEN
        NEW.admin_version := OLD.admin_version;
        NEW.admin_version_txid := OLD.admin_version_txid;
    ELSIF NEW.admin_version IS DISTINCT FROM OLD.admin_version
       OR (to_jsonb(NEW) - 'admin_version' - 'admin_version_txid')
          IS DISTINCT FROM (to_jsonb(OLD) - 'admin_version' - 'admin_version_txid') THEN
        NEW.admin_version := OLD.admin_version + 1;
        NEW.admin_version_txid := txid_current();
    ELSE
        NEW.admin_version := OLD.admin_version;
        NEW.admin_version_txid := OLD.admin_version_txid;
    END IF;
    RETURN NEW;
END $fn$;

CREATE OR REPLACE FUNCTION support_admin_version_insert() RETURNS trigger
LANGUAGE plpgsql AS $fn$
BEGIN
    NEW.admin_version := 1;
    NEW.admin_version_txid := txid_current();
    RETURN NEW;
END $fn$;

-- A child change (message, evidence, dispute, read state) is a change to the
-- case: its counters and references move, so its version must too.
CREATE OR REPLACE FUNCTION support_admin_touch_case() RETURNS trigger
LANGUAGE plpgsql AS $fn$
BEGIN
    UPDATE complaints SET updated_at = now(), admin_version = admin_version + 1 WHERE id = NEW.complaint_id;
    RETURN NULL;
END $fn$;

-- Assigns the next per-case sequence under the case row lock, so concurrent
-- writers get distinct, contiguous numbers and the case version moves too.
CREATE OR REPLACE FUNCTION support_admin_message_sequence() RETURNS trigger
LANGUAGE plpgsql AS $fn$
DECLARE next_sequence bigint;
BEGIN
    UPDATE complaints SET message_seq = message_seq + 1, updated_at = now()
    WHERE id = NEW.complaint_id RETURNING message_seq INTO next_sequence;
    IF next_sequence IS NULL THEN
        RAISE EXCEPTION 'support chat message references an unknown case' USING ERRCODE = '23503';
    END IF;
    NEW.sequence := next_sequence;
    RETURN NEW;
END $fn$;

-- Builds the exact adminevents.SourceEvent for one case. Optional columns are
-- read through to_jsonb so a rolled-back sibling migration degrades a field to
-- null instead of failing every case write. Free text (subject, description,
-- resolution note, chat), identity snapshots and incident coordinates are
-- never read into the payload.
CREATE OR REPLACE FUNCTION support_admin_case_event(p_complaint_id uuid) RETURNS jsonb
LANGUAGE plpgsql AS $fn$
DECLARE
    c complaints%ROWTYPE;
    j jsonb;
BEGIN
    SELECT * INTO c FROM complaints WHERE id = p_complaint_id;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    j := to_jsonb(c);
    RETURN jsonb_build_object(
        'schema', 'karryam.owner-admin-event.v1',
        'event_id', gen_random_uuid()::text,
        'event_type', 'support.case.changed.v1',
        'source_service', 'support-dispute-service',
        'product', 'karryam',
        'entity_type', 'support_case',
        'entity_ref', c.id::text,
        'entity_version', c.admin_version,
        'deleted', false,
        'occurred_at', support_admin_rfc3339(c.updated_at),
        'emitted_at', support_admin_rfc3339(greatest(clock_timestamp(), c.updated_at)),
        'scope', jsonb_build_object('market', '', 'zone', '',
            'organization', COALESCE(support_admin_opaque(j->>'organization_reference'), ''), 'tenant', ''),
        'correlation_id', '',
        'payload', jsonb_build_object(
            'status', upper(c.status::text),
            'priority', CASE lower(j->>'priority') WHEN 'high' THEN 'HIGH' WHEN 'emergency' THEN 'EMERGENCY' ELSE 'NORMAL' END,
            'service_type', c.service_type::text,
            'category', support_admin_opaque(j->>'category'),
            'complainant_type', c.complainant_type::text,
            'context_type', support_admin_opaque(j->>'context_type'),
            'locale', NULL,
            'sla_state', 'POLICY_UNAVAILABLE',
            'consent_classification', CASE WHEN lower(j->>'priority') = 'emergency' THEN 'SAFETY_EVIDENCE' ELSE 'OPERATIONS_MINIMIZED' END,
            'assignee', support_admin_opaque(j->>'assigned_to'),
            'escalation_level', support_admin_opaque(j->>'escalation_level'),
            'mission_ref', support_admin_opaque(j->>'mission_ref'),
            'order_ref', support_admin_opaque(j->>'order_ref'),
            'dispute_ref', (SELECT d.id::text FROM disputes d WHERE d.complaint_id = c.id ORDER BY d.created_at DESC, d.id DESC LIMIT 1),
            'unread_count', (SELECT count(*) FROM support_chat_messages m WHERE m.complaint_id = c.id AND NOT m.is_read AND m.sender_type <> 'admin'),
            'message_count', (SELECT count(*) FROM support_chat_messages m WHERE m.complaint_id = c.id),
            'evidence_count', (SELECT count(*) FROM complaint_evidence e WHERE e.complaint_id = c.id),
            'created_at', support_admin_rfc3339(c.created_at),
            'updated_at', support_admin_rfc3339(c.updated_at)));
END $fn$;

-- Deferred to commit: reads the case's final state for this transaction and
-- appends it to the outbox at most once per (case, version).
CREATE OR REPLACE FUNCTION support_admin_emit_case_event() RETURNS trigger
LANGUAGE plpgsql AS $fn$
DECLARE ev jsonb;
BEGIN
    ev := support_admin_case_event(NEW.id);
    IF ev IS NULL THEN
        RETURN NULL;
    END IF;
    INSERT INTO support_admin_case_emissions (complaint_id, entity_version, event_id)
    VALUES (NEW.id, (ev->>'entity_version')::bigint, ev->>'event_id') ON CONFLICT DO NOTHING;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    INSERT INTO admin_event_outbox (id, event_type, schema_version, aggregate_type, aggregate_id, payload, status, available_at, attempts, created_at)
    VALUES (ev->>'event_id', 'support.case.changed.v1', 1, 'support_case', NEW.id::text, ev, 'PENDING', now(), 0, now());
    RETURN NULL;
END $fn$;

-- ---------------------------------------------------------------------------
-- Triggers
-- ---------------------------------------------------------------------------
SELECT support_admin_ddl_if(support_admin_lacks_trigger('complaints', 'support_admin_complaints_version_insert'),
    'CREATE TRIGGER support_admin_complaints_version_insert BEFORE INSERT ON complaints
    FOR EACH ROW EXECUTE FUNCTION support_admin_version_insert()');
SELECT support_admin_ddl_if(support_admin_lacks_trigger('complaints', 'support_admin_complaints_version_update'),
    'CREATE TRIGGER support_admin_complaints_version_update BEFORE UPDATE ON complaints
    FOR EACH ROW EXECUTE FUNCTION support_admin_version_update()');
SELECT support_admin_ddl_if(support_admin_lacks_trigger('disputes', 'support_admin_disputes_version_insert'),
    'CREATE TRIGGER support_admin_disputes_version_insert BEFORE INSERT ON disputes
    FOR EACH ROW EXECUTE FUNCTION support_admin_version_insert()');
SELECT support_admin_ddl_if(support_admin_lacks_trigger('disputes', 'support_admin_disputes_version_update'),
    'CREATE TRIGGER support_admin_disputes_version_update BEFORE UPDATE ON disputes
    FOR EACH ROW EXECUTE FUNCTION support_admin_version_update()');

SELECT support_admin_ddl_if(support_admin_lacks_trigger('support_chat_messages', 'support_admin_messages_sequence'),
    'CREATE TRIGGER support_admin_messages_sequence BEFORE INSERT ON support_chat_messages
    FOR EACH ROW EXECUTE FUNCTION support_admin_message_sequence()');
SELECT support_admin_ddl_if(support_admin_lacks_trigger('support_chat_messages', 'support_admin_messages_read_touch'),
    'CREATE TRIGGER support_admin_messages_read_touch AFTER UPDATE OF is_read ON support_chat_messages
    FOR EACH ROW WHEN (OLD.is_read IS DISTINCT FROM NEW.is_read) EXECUTE FUNCTION support_admin_touch_case()');
SELECT support_admin_ddl_if(support_admin_lacks_trigger('complaint_evidence', 'support_admin_evidence_touch'),
    'CREATE TRIGGER support_admin_evidence_touch AFTER INSERT ON complaint_evidence
    FOR EACH ROW EXECUTE FUNCTION support_admin_touch_case()');
SELECT support_admin_ddl_if(support_admin_lacks_trigger('disputes', 'support_admin_disputes_touch'),
    'CREATE TRIGGER support_admin_disputes_touch AFTER INSERT OR UPDATE ON disputes
    FOR EACH ROW EXECUTE FUNCTION support_admin_touch_case()');

SELECT support_admin_ddl_if(support_admin_lacks_trigger('complaints', 'support_admin_case_event_emit'),
    'CREATE CONSTRAINT TRIGGER support_admin_case_event_emit AFTER INSERT OR UPDATE ON complaints
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION support_admin_emit_case_event()');
