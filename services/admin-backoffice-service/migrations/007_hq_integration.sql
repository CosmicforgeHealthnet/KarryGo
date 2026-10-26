-- Sections 22-30: durable owner-event inbox, minimized projections, HQ outbox,
-- backfill/reconciliation, HQ approvals, campaign orchestration, privacy
-- disposition register and controlled encrypted exports. The Bridge stores
-- orchestration and minimized projections only; owners remain business truth.

CREATE TABLE admin_event_ingress_nonces (
 source_service text NOT NULL,
 nonce_hash text NOT NULL CHECK (nonce_hash ~ '^[0-9a-f]{64}$'),
 expires_at timestamptz NOT NULL,
 PRIMARY KEY (source_service, nonce_hash)
);
CREATE INDEX admin_event_ingress_nonce_expiry ON admin_event_ingress_nonces(expires_at);

-- One row per (source,event): the dedupe key. Outcome is recorded once.
CREATE TABLE admin_event_inbox (
 source_service text NOT NULL,
 event_id uuid NOT NULL,
 event_type text NOT NULL,
 entity_type text NOT NULL,
 entity_ref text NOT NULL,
 entity_version bigint NOT NULL CHECK (entity_version > 0),
 payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
 delivery text NOT NULL CHECK (delivery IN ('INGRESS','BACKFILL','RECONCILIATION')),
 outcome text NOT NULL CHECK (outcome IN ('APPLIED','STALE','QUARANTINED','GAP_APPLIED')),
 received_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (source_service, event_id)
);
CREATE TRIGGER admin_event_inbox_immutable BEFORE UPDATE OR DELETE ON admin_event_inbox FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();
CREATE TRIGGER admin_event_inbox_no_truncate BEFORE TRUNCATE ON admin_event_inbox FOR EACH STATEMENT EXECUTE FUNCTION admin_immutable_evidence();

-- Quarantine keeps only metadata and a hash, never raw bytes: a quarantined
-- event may be exactly the one that would have leaked restricted data.
CREATE TABLE admin_event_quarantine (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 source_service text NOT NULL,
 event_id text NOT NULL,
 event_type text NOT NULL,
 reason text NOT NULL CHECK (reason IN ('UNKNOWN_SCHEMA','INVALID_PAYLOAD','SOURCE_MISMATCH','PRODUCT_MISMATCH')),
 raw_sha256 text NOT NULL CHECK (raw_sha256 ~ '^[0-9a-f]{64}$'),
 received_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (source_service, raw_sha256)
);
CREATE TRIGGER admin_event_quarantine_immutable BEFORE UPDATE OR DELETE ON admin_event_quarantine FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();

CREATE TABLE admin_projections (
 product text NOT NULL CHECK (product='karryam'),
 source_service text NOT NULL,
 entity_type text NOT NULL,
 entity_ref text NOT NULL,
 entity_version bigint NOT NULL CHECK (entity_version > 0),
 export_type text NOT NULL,
 market text NOT NULL DEFAULT '',
 zone text NOT NULL DEFAULT '',
 organization_ref text NOT NULL DEFAULT '',
 tenant_ref text NOT NULL DEFAULT '',
 classification text NOT NULL,
 payload jsonb NOT NULL,
 tombstone boolean NOT NULL DEFAULT false,
 occurred_at timestamptz NOT NULL,
 received_at timestamptz NOT NULL,
 coverage text NOT NULL DEFAULT 'INCREMENTAL' CHECK (coverage IN ('INCREMENTAL','BACKFILLED','RECONCILED')),
 PRIMARY KEY (product, source_service, entity_type, entity_ref),
 CHECK (NOT tombstone OR payload = '{}'::jsonb)
);
CREATE INDEX admin_projections_queue ON admin_projections(source_service, entity_type, received_at DESC);

CREATE TABLE admin_projection_gaps (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 source_service text NOT NULL,
 entity_type text NOT NULL,
 entity_ref text NOT NULL,
 previous_version bigint NOT NULL,
 received_version bigint NOT NULL CHECK (received_version > previous_version + 1),
 detected_at timestamptz NOT NULL DEFAULT now(),
 repaired_at timestamptz
);
CREATE INDEX admin_projection_gaps_open ON admin_projection_gaps(source_service, entity_type) WHERE repaired_at IS NULL;

-- Append-only minimized HQ feed; sequence is the HQ cursor.
CREATE TABLE admin_hq_outbox (
 sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id uuid NOT NULL UNIQUE,
 export_type text NOT NULL,
 market text NOT NULL DEFAULT '',
 zone text NOT NULL DEFAULT '',
 organization_ref text NOT NULL DEFAULT '',
 tenant_ref text NOT NULL DEFAULT '',
 export jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER admin_hq_outbox_immutable BEFORE UPDATE OR DELETE ON admin_hq_outbox FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();
CREATE TRIGGER admin_hq_outbox_no_truncate BEFORE TRUNCATE ON admin_hq_outbox FOR EACH STATEMENT EXECUTE FUNCTION admin_immutable_evidence();

CREATE TABLE admin_hq_consumer_cursors (
 consumer text PRIMARY KEY,
 acknowledged_sequence bigint NOT NULL CHECK (acknowledged_sequence >= 0),
 acknowledged_by text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION admin_hq_cursor_monotonic() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'consumer cursors are durable'; END IF;
 IF NEW.acknowledged_sequence < OLD.acknowledged_sequence THEN RAISE EXCEPTION 'consumer cursor cannot move backwards'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER admin_hq_cursor_monotonic BEFORE UPDATE OR DELETE ON admin_hq_consumer_cursors FOR EACH ROW EXECUTE FUNCTION admin_hq_cursor_monotonic();

CREATE TABLE admin_backfill_runs (
 id uuid PRIMARY KEY,
 source_service text NOT NULL,
 entity_type text NOT NULL,
 status text NOT NULL CHECK (status IN ('RUNNING','COMPLETE','PARTIAL_COVERAGE','FAILED')),
 watermark timestamptz,
 cursor text NOT NULL DEFAULT '',
 pages integer NOT NULL DEFAULT 0,
 events integer NOT NULL DEFAULT 0,
 lease_owner text,
 lease_expires_at timestamptz,
 last_error_code text NOT NULL DEFAULT '',
 started_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz
);
CREATE UNIQUE INDEX admin_backfill_one_running ON admin_backfill_runs(source_service, entity_type) WHERE status='RUNNING';

CREATE TABLE admin_reconciliation_runs (
 id uuid PRIMARY KEY,
 source_service text NOT NULL,
 entity_type text NOT NULL,
 checked integer NOT NULL,
 drift integer NOT NULL,
 repaired integer NOT NULL,
 missing_at_source integer NOT NULL,
 completed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER admin_reconciliation_runs_immutable BEFORE UPDATE OR DELETE ON admin_reconciliation_runs FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();

-- Generic HQ two-person approvals for non-owner-command intents (campaigns,
-- exports). Same invariants as admin_command_approvals.
CREATE TABLE admin_hq_approvals (
 id uuid PRIMARY KEY,
 kind text NOT NULL CHECK (kind IN ('CAMPAIGN','EXPORT')),
 subject_ref text NOT NULL,
 intent_digest text NOT NULL CHECK (intent_digest ~ '^[0-9a-f]{64}$'),
 requester_ref text NOT NULL,
 purpose_code text NOT NULL,
 market text NOT NULL,
 zone text NOT NULL,
 organization_ref text NOT NULL,
 tenant_ref text NOT NULL,
 status text NOT NULL CHECK (status IN ('PENDING','APPROVED','REJECTED','CONSUMED')),
 approver_ref text,
 decided_at timestamptz,
 consumed_by text UNIQUE,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK (approver_ref IS NULL OR approver_ref <> requester_ref),
 CHECK ((status='PENDING') = (approver_ref IS NULL AND decided_at IS NULL)),
 CHECK ((status='CONSUMED') = (consumed_by IS NOT NULL))
);
CREATE FUNCTION admin_hq_approval_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP <> 'UPDATE' THEN RAISE EXCEPTION 'HQ approval identity is durable'; END IF;
 IF (to_jsonb(OLD)-ARRAY['status','approver_ref','decided_at','consumed_by','updated_at']) IS DISTINCT FROM
    (to_jsonb(NEW)-ARRAY['status','approver_ref','decided_at','consumed_by','updated_at']) THEN
  RAISE EXCEPTION 'HQ approval intent is immutable';
 END IF;
 IF NOT ((OLD.status='PENDING' AND NEW.status IN ('APPROVED','REJECTED')) OR (OLD.status='APPROVED' AND NEW.status='CONSUMED')) THEN
  RAISE EXCEPTION 'illegal HQ approval transition';
 END IF;
 IF OLD.status<>'PENDING' AND (NEW.approver_ref IS DISTINCT FROM OLD.approver_ref OR NEW.decided_at IS DISTINCT FROM OLD.decided_at) THEN
  RAISE EXCEPTION 'HQ approval decision is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER admin_hq_approval_guard BEFORE UPDATE OR DELETE ON admin_hq_approvals FOR EACH ROW EXECUTE FUNCTION admin_hq_approval_guard();
CREATE TRIGGER admin_hq_approval_no_truncate BEFORE TRUNCATE ON admin_hq_approvals FOR EACH STATEMENT EXECUTE FUNCTION admin_hq_approval_guard();

CREATE TABLE admin_campaign_operations (
 id uuid PRIMARY KEY,
 campaign_id uuid NOT NULL,
 revision bigint NOT NULL CHECK (revision > 0),
 idempotency_key text NOT NULL UNIQUE,
 intent_digest text NOT NULL CHECK (intent_digest ~ '^[0-9a-f]{64}$'),
 approval_id uuid NOT NULL UNIQUE,
 staff_ref text NOT NULL,
 status text NOT NULL CHECK (status IN ('PENDING','CONFIRMED','REJECTED','UNKNOWN_OUTCOME')),
 owner_status jsonb,
 rejection_code text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (campaign_id, revision)
);

CREATE TABLE admin_privacy_requests (
 id uuid PRIMARY KEY,
 subject_kind text NOT NULL CHECK (subject_kind IN ('CUSTOMER','PROVIDER','VENDOR_STAFF')),
 subject_digest text NOT NULL CHECK (subject_digest ~ '^[0-9a-f]{64}$'),
 request_type text NOT NULL CHECK (request_type IN ('ACCESS_EXPORT','ERASURE','RECTIFICATION','CONSENT_WITHDRAWAL','RESTRICT_PROCESSING')),
 received_on date NOT NULL,
 registered_by text NOT NULL,
 legal_hold boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER admin_privacy_requests_immutable BEFORE UPDATE OR DELETE ON admin_privacy_requests FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();
-- Append-only disposition history; the latest row per (request,owner) is current.
CREATE TABLE admin_privacy_dispositions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 request_id uuid NOT NULL REFERENCES admin_privacy_requests(id),
 owner text NOT NULL,
 disposition text NOT NULL CHECK (disposition IN ('PENDING_OWNER_ACTION','POLICY_REQUIRED','LEGAL_HOLD','FULFILLED_BY_OWNER','RETAINED_LEGAL_OBLIGATION','NOT_HELD_BY_OWNER','PROJECTION_TOMBSTONED')),
 evidence_ref text NOT NULL DEFAULT '',
 recorded_by text NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now(),
 CHECK (disposition NOT IN ('FULFILLED_BY_OWNER','RETAINED_LEGAL_OBLIGATION') OR evidence_ref <> '')
);
CREATE TRIGGER admin_privacy_dispositions_immutable BEFORE UPDATE OR DELETE ON admin_privacy_dispositions FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();

CREATE TABLE admin_exports (
 id uuid PRIMARY KEY,
 kind text NOT NULL CHECK (kind IN ('SUPPORT_CASE_QUEUE')),
 requested_by text NOT NULL,
 approval_id uuid NOT NULL UNIQUE,
 purpose_code text NOT NULL,
 market text NOT NULL,
 zone text NOT NULL,
 organization_ref text NOT NULL,
 tenant_ref text NOT NULL,
 row_limit integer NOT NULL CHECK (row_limit BETWEEN 1 AND 10000),
 period_start timestamptz NOT NULL,
 period_end timestamptz NOT NULL CHECK (period_end > period_start),
 row_count integer NOT NULL CHECK (row_count >= 0),
 key_id text NOT NULL,
 nonce bytea NOT NULL,
 ciphertext bytea NOT NULL,
 plaintext_sha256 text NOT NULL CHECK (plaintext_sha256 ~ '^[0-9a-f]{64}$'),
 token_hash text NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK (period_end - period_start <= interval '93 days')
);
CREATE TRIGGER admin_exports_immutable BEFORE UPDATE ON admin_exports FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();
CREATE TABLE admin_export_downloads (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 export_id uuid NOT NULL REFERENCES admin_exports(id),
 staff_ref text NOT NULL,
 outcome text NOT NULL CHECK (outcome IN ('DELIVERED','EXPIRED','DENIED')),
 recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX admin_export_single_delivery ON admin_export_downloads(export_id) WHERE outcome='DELIVERED';
CREATE TRIGGER admin_export_downloads_immutable BEFORE UPDATE OR DELETE ON admin_export_downloads FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();

REVOKE ALL ON admin_event_ingress_nonces,admin_event_inbox,admin_event_quarantine,admin_projections,admin_projection_gaps,admin_hq_outbox,admin_hq_consumer_cursors,admin_backfill_runs,admin_reconciliation_runs,admin_hq_approvals,admin_campaign_operations,admin_privacy_requests,admin_privacy_dispositions,admin_exports,admin_export_downloads FROM PUBLIC;
