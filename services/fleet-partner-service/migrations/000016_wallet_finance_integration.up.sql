CREATE TABLE fleet_partner_finance_operations (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id),
 subject_type text NOT NULL, subject_id text NOT NULL, operation_type text NOT NULL,
 payload jsonb NOT NULL, payload_version integer NOT NULL DEFAULT 1 CHECK(payload_version>0),
 expected_version bigint NOT NULL DEFAULT 0, idempotency_key text NOT NULL,
 request_fingerprint text NOT NULL, wallet_reference text, status text NOT NULL
 CHECK(status IN ('PENDING','PROCESSING','UNKNOWN','RECONCILING','SUCCEEDED','RETRYABLE_FAILURE','PERMANENT_FAILURE','DEAD_LETTER')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0), next_attempt_at timestamptz NOT NULL DEFAULT now(),
 lease_owner text, lease_expires_at timestamptz, safe_error_code text NOT NULL DEFAULT '',
 correlation_id text NOT NULL, causation_id text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 UNIQUE(organization_id,idempotency_key)
);
CREATE INDEX idx_fleet_finance_operations_due ON fleet_partner_finance_operations(status,next_attempt_at) WHERE status IN ('PENDING','RETRYABLE_FAILURE','UNKNOWN','RECONCILING');
CREATE INDEX idx_fleet_finance_operations_lease ON fleet_partner_finance_operations(lease_expires_at) WHERE lease_owner IS NOT NULL;
CREATE INDEX idx_fleet_finance_operations_subject ON fleet_partner_finance_operations(organization_id,subject_type,subject_id,operation_type);

CREATE TABLE fleet_partner_finance_projections (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id),
 projection_type text NOT NULL CHECK(projection_type IN ('WALLET','EARNING_ACCOUNT','SUBSCRIPTION','INVOICE','PAYMENT_VERIFICATION','HOLD','COMMISSION','EARNING','SETTLEMENT','REFUND','REVERSAL')),
 subject_type text NOT NULL, subject_id text NOT NULL, wallet_reference text NOT NULL,
 amount_minor bigint, currency char(3), safe_status text NOT NULL,
 source_service text NOT NULL DEFAULT 'payment-wallet-service', source_version bigint NOT NULL CHECK(source_version>0),
 freshness_state text NOT NULL CHECK(freshness_state IN ('CURRENT','STALE','GAP_DETECTED','UNKNOWN','RECONCILING')),
 last_event_id text, last_synced_at timestamptz NOT NULL DEFAULT now(), version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,projection_type,wallet_reference), CHECK((amount_minor IS NULL AND currency IS NULL) OR (amount_minor>=0 AND currency IS NOT NULL))
);
CREATE INDEX idx_fleet_finance_projection_subject ON fleet_partner_finance_projections(organization_id,subject_type,subject_id,projection_type);
CREATE INDEX idx_fleet_finance_projection_reconcile ON fleet_partner_finance_projections(freshness_state,last_synced_at) WHERE freshness_state<>'CURRENT';

CREATE TABLE fleet_partner_finance_source_versions (
 aggregate_type text NOT NULL, aggregate_id text NOT NULL, source_version bigint NOT NULL,
 last_event_id text NOT NULL, payload_hash text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(aggregate_type,aggregate_id)
);
CREATE TABLE fleet_partner_finance_operation_history (
 id text PRIMARY KEY, organization_id text NOT NULL, operation_id text NOT NULL REFERENCES fleet_partner_finance_operations(id),
 previous_status text NOT NULL, new_status text NOT NULL, safe_reason_code text NOT NULL DEFAULT '',
 actor_type text NOT NULL, actor_id text NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fleet_finance_history_operation ON fleet_partner_finance_operation_history(operation_id,occurred_at);
