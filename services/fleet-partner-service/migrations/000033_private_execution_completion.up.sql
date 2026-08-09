-- Section 15 blocker closure: production private execution orchestration,
-- source-version projections and operator reconciliation. Additive to 000032.
ALTER TABLE fleet_partner_private_execution_operations
  ADD COLUMN IF NOT EXISTS provider_response_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS vehicle_or_truck_reference text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS correlation_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS causation_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS execution_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS lease_heartbeat_at timestamptz,
  ADD COLUMN IF NOT EXISTS completed_at timestamptz,
  ADD COLUMN IF NOT EXISTS dead_lettered_at timestamptz;

ALTER TABLE fleet_partner_private_execution_operations
  DROP CONSTRAINT IF EXISTS fleet_partner_private_execution_operations_status_check;
ALTER TABLE fleet_partner_private_execution_operations
  ADD CONSTRAINT fleet_partner_private_execution_operations_status_check CHECK(status IN(
    'PENDING','SUBMITTING','OUTCOME_UNKNOWN','CONFIRMED','FAILED',
    'CANCELLATION_PENDING','CANCELLATION_UNKNOWN','CANCELLED','RECONCILIATION_REQUIRED','DEAD_LETTER'));

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_bulk_import_media_once
  ON fleet_partner_bulk_imports(organization_id,media_file_id);

CREATE TABLE IF NOT EXISTS fleet_partner_private_execution_projections(
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  execution_operation_id text NOT NULL REFERENCES fleet_partner_private_execution_operations(id) ON DELETE RESTRICT,
  source_service text NOT NULL CHECK(source_service IN('driver-dispatch-delivery-service','driver-hauling-service')),
  source_aggregate_id text NOT NULL,
  canonical_request_reference text NOT NULL DEFAULT '',
  canonical_trip_reference text NOT NULL DEFAULT '',
  normalized_status text NOT NULL,
  source_version bigint NOT NULL CHECK(source_version>=0),
  freshness_state text NOT NULL CHECK(freshness_state IN('CURRENT','GAP_DETECTED','CONFLICT','UNKNOWN')),
  last_event_id text NOT NULL,
  last_payload_hash text NOT NULL,
  last_event_at timestamptz NOT NULL,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(source_service,source_aggregate_id),
  UNIQUE(execution_operation_id)
);

CREATE TABLE IF NOT EXISTS fleet_partner_private_execution_reconciliation_cases(
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  execution_operation_id text NOT NULL REFERENCES fleet_partner_private_execution_operations(id) ON DELETE RESTRICT,
  projection_id text REFERENCES fleet_partner_private_execution_projections(id) ON DELETE RESTRICT,
  reason_code text NOT NULL,
  status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','PROCESSING','RETRYABLE_FAILURE','RESOLVED','DEAD_LETTER')),
  attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  lease_owner text,
  lease_expires_at timestamptz,
  lease_heartbeat_at timestamptz,
  last_error_code text NOT NULL DEFAULT '',
  correlation_id text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_private_execution_open_reconciliation
  ON fleet_partner_private_execution_reconciliation_cases(execution_operation_id)
  WHERE status IN('PENDING','PROCESSING','RETRYABLE_FAILURE');
CREATE INDEX IF NOT EXISTS idx_fleet_private_execution_reconciliation_claim
  ON fleet_partner_private_execution_reconciliation_cases(next_attempt_at)
  WHERE status IN('PENDING','RETRYABLE_FAILURE');

CREATE TABLE IF NOT EXISTS fleet_partner_private_execution_reconciliation_attempts(
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  reconciliation_case_id text NOT NULL REFERENCES fleet_partner_private_execution_reconciliation_cases(id) ON DELETE RESTRICT,
  execution_operation_id text NOT NULL REFERENCES fleet_partner_private_execution_operations(id) ON DELETE RESTRICT,
  attempt integer NOT NULL CHECK(attempt>0),
  outcome text NOT NULL,
  safe_error_code text NOT NULL DEFAULT '',
  occurred_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(reconciliation_case_id,attempt,outcome)
);

INSERT INTO fleet_partner_permissions(code,category,description,is_active)
VALUES('private_execution.replay','private_execution','Replay dead-lettered private execution operations',true)
ON CONFLICT(code) DO UPDATE SET description=EXCLUDED.description,is_active=true;
INSERT INTO fleet_partner_permissions(code,category,description,is_active)
VALUES('manifests.seal_amend','manifests','Amend the seal of a closed manifest with an audited reason',true)
ON CONFLICT(code) DO UPDATE SET description=EXCLUDED.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
VALUES('rol_system_owner','private_execution.replay'),('rol_system_operations','private_execution.replay'),
('rol_system_owner','manifests.seal_amend'),('rol_system_operations','manifests.seal_amend')
ON CONFLICT DO NOTHING;
