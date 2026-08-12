ALTER TABLE fleet_partner_finance_projections
 ADD COLUMN reconciliation_status text NOT NULL DEFAULT 'IDLE'
  CHECK (reconciliation_status IN ('IDLE','PENDING','PROCESSING','RETRYABLE_FAILURE','UNKNOWN','DEAD_LETTER')),
 ADD COLUMN reconciliation_attempts integer NOT NULL DEFAULT 0 CHECK (reconciliation_attempts >= 0),
 ADD COLUMN reconciliation_next_attempt_at timestamptz NOT NULL DEFAULT now(),
 ADD COLUMN reconciliation_lease_owner text,
 ADD COLUMN reconciliation_lease_expires_at timestamptz,
 ADD COLUMN reconciliation_safe_error_code text NOT NULL DEFAULT '';

CREATE INDEX idx_fleet_finance_projection_reconciliation_due
 ON fleet_partner_finance_projections(reconciliation_status, reconciliation_next_attempt_at)
 WHERE reconciliation_status IN ('PENDING','RETRYABLE_FAILURE','UNKNOWN');

CREATE INDEX idx_fleet_finance_projection_reconciliation_lease
 ON fleet_partner_finance_projections(reconciliation_lease_expires_at)
 WHERE reconciliation_lease_owner IS NOT NULL;

CREATE TABLE fleet_partner_finance_projection_history (
 id text PRIMARY KEY,
 organization_id text NOT NULL,
 projection_id text NOT NULL REFERENCES fleet_partner_finance_projections(id),
 previous_freshness_state text NOT NULL,
 new_freshness_state text NOT NULL,
 previous_reconciliation_status text NOT NULL,
 new_reconciliation_status text NOT NULL,
 safe_reason_code text NOT NULL DEFAULT '',
 actor_type text NOT NULL,
 actor_id text NOT NULL,
 occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_fleet_finance_projection_history_projection
 ON fleet_partner_finance_projection_history(projection_id, occurred_at);
