-- Section 17: marketing campaign fulfilment (owner side of the
-- karryam.campaign-intent.v1 contract, shared/go/admincampaign).
--
-- An accepted intent is an immutable revision. Recipients are planned from
-- customer-service's consent authority, re-checked immediately before each
-- send, and recorded with a truthful outcome: SENT only for a real provider
-- acceptance; LOGGED_ONLY for development logging senders; never DELIVERED
-- (no provider delivery receipts exist).
--
-- This service re-applies every migration on each MIGRATION=true boot, so
-- every statement is idempotent.

-- Template content revision. Advances only when the rendered content changes,
-- so a campaign bound to (key, locale, revision, content hash) cannot be sent
-- with different copy.
ALTER TABLE notification_templates ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 1;
CREATE OR REPLACE FUNCTION notification_template_revision_advance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.title IS DISTINCT FROM OLD.title OR NEW.body IS DISTINCT FROM OLD.body THEN
  NEW.revision := OLD.revision + 1;
 ELSE
  NEW.revision := OLD.revision;
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS notification_template_revision ON notification_templates;
CREATE TRIGGER notification_template_revision BEFORE UPDATE ON notification_templates FOR EACH ROW EXECUTE FUNCTION notification_template_revision_advance();

CREATE TABLE IF NOT EXISTS campaigns (
 campaign_id uuid NOT NULL,
 revision bigint NOT NULL CHECK (revision >= 1),
 intent jsonb NOT NULL,
 intent_digest text NOT NULL CHECK (intent_digest ~ '^[0-9a-f]{64}$'),
 idempotency_key text NOT NULL UNIQUE,
 state text NOT NULL CHECK (state IN ('ACCEPTED_BY_KARRYGO','PLANNED','QUEUED','SENT','DELIVERED','COMPLETED','REJECTED','FAILED','SUPPRESSED','CANCELLED')),
 rejection_code text CHECK (rejection_code IS NULL OR rejection_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 requested_by text NOT NULL,
 approvals jsonb NOT NULL,
 -- Content snapshot validated against the template revision and hash; the
 -- send path uses exactly this copy.
 content_title text,
 content_body text,
 plan_cursor jsonb NOT NULL DEFAULT '{}'::jsonb,
 planned_at timestamptz,
 audience_count bigint NOT NULL DEFAULT 0 CHECK (audience_count >= 0),
 eligible_count bigint NOT NULL DEFAULT 0 CHECK (eligible_count >= 0),
 suppressed_count bigint NOT NULL DEFAULT 0 CHECK (suppressed_count >= 0),
 queued_count bigint NOT NULL DEFAULT 0 CHECK (queued_count >= 0),
 sent_count bigint NOT NULL DEFAULT 0 CHECK (sent_count >= 0),
 failed_count bigint NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
 logged_only_count bigint NOT NULL DEFAULT 0 CHECK (logged_only_count >= 0),
 suppression_reasons jsonb NOT NULL DEFAULT '{}'::jsonb,
 next_run_at timestamptz NOT NULL DEFAULT now(),
 lease_owner text,
 lease_expires_at timestamptz,
 version bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (campaign_id, revision),
 CHECK ((state = 'REJECTED') = (rejection_code IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS campaigns_due ON campaigns (next_run_at)
 WHERE state IN ('ACCEPTED_BY_KARRYGO','PLANNED','QUEUED','SENT');

-- The accepted intent of a revision never changes.
CREATE OR REPLACE FUNCTION reject_campaign_intent_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'DELETE' THEN RAISE EXCEPTION 'Campaign revisions are never deleted'; END IF;
 IF NEW.intent IS DISTINCT FROM OLD.intent OR NEW.intent_digest IS DISTINCT FROM OLD.intent_digest
    OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key OR NEW.approvals IS DISTINCT FROM OLD.approvals
    OR NEW.requested_by IS DISTINCT FROM OLD.requested_by THEN
  RAISE EXCEPTION 'Campaign revision intent is immutable';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS campaigns_intent_immutable ON campaigns;
CREATE TRIGGER campaigns_intent_immutable BEFORE UPDATE OR DELETE ON campaigns FOR EACH ROW EXECUTE FUNCTION reject_campaign_intent_mutation();

CREATE TABLE IF NOT EXISTS campaign_recipients (
 campaign_id uuid NOT NULL,
 revision bigint NOT NULL,
 recipient_id text NOT NULL,
 channel text NOT NULL CHECK (channel IN ('in_app','push','email')),
 market text NOT NULL,
 consent_version bigint NOT NULL CHECK (consent_version >= 1),
 state text NOT NULL CHECK (state IN ('PLANNED','QUEUED','SENT','LOGGED_ONLY','FAILED','SUPPRESSED','CANCELLED')),
 suppression_reason text CHECK (suppression_reason IS NULL OR suppression_reason ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 message_id uuid,
 queued_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (campaign_id, revision, recipient_id, channel),
 FOREIGN KEY (campaign_id, revision) REFERENCES campaigns(campaign_id, revision),
 CHECK ((state = 'SUPPRESSED') = (suppression_reason IS NOT NULL)),
 CHECK (state NOT IN ('QUEUED','SENT','LOGGED_ONLY','FAILED') OR message_id IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS campaign_recipients_planned ON campaign_recipients (campaign_id, revision, channel, recipient_id) WHERE state = 'PLANNED';
CREATE INDEX IF NOT EXISTS campaign_recipients_queued ON campaign_recipients (campaign_id, revision) WHERE state = 'QUEUED';
-- Frequency cap: messages a recipient was given across ALL campaigns.
CREATE INDEX IF NOT EXISTS campaign_recipients_frequency ON campaign_recipients (recipient_id, queued_at) WHERE queued_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS campaign_cancellations (
 cancellation_key text PRIMARY KEY,
 campaign_id uuid NOT NULL,
 revision bigint NOT NULL,
 requested_by text NOT NULL,
 reason_code text NOT NULL CHECK (reason_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (campaign_id, revision) REFERENCES campaigns(campaign_id, revision)
);

-- Durable single-use nonces for the private campaign and snapshot routes.
CREATE TABLE IF NOT EXISTS campaign_request_nonces (
 caller text NOT NULL,
 nonce_hash text NOT NULL,
 seen_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (caller, nonce_hash)
);
CREATE INDEX IF NOT EXISTS campaign_request_nonces_seen ON campaign_request_nonces (seen_at);

-- Section 22 owner event outbox (shared/go/adminevents.OutboxSchema, verbatim).
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
