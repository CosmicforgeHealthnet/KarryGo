-- Section 16: provider-neutral optimization request/result ledger. Execution is performed by generic durable jobs.
CREATE TABLE IF NOT EXISTS fleet_partner_route_optimization_requests (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
  route_plan_id text NOT NULL REFERENCES fleet_partner_route_plans(id) ON DELETE RESTRICT, route_version bigint NOT NULL,
  optimization_version bigint NOT NULL, status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','RUNNING','SUCCEEDED','FAILED','CANCELLED','RECONCILIATION_REQUIRED')),
  provider_reference text NOT NULL DEFAULT '', provider_job_reference text NOT NULL DEFAULT '', input_fingerprint text NOT NULL,
  input_snapshot jsonb NOT NULL, result_snapshot jsonb, constraint_violations jsonb NOT NULL DEFAULT '[]'::jsonb,
  unserved_stops jsonb NOT NULL DEFAULT '[]'::jsonb, warnings jsonb NOT NULL DEFAULT '[]'::jsonb,
  estimated_distance numeric(18,3) NOT NULL DEFAULT 0, estimated_duration_seconds integer NOT NULL DEFAULT 0,
  result_version bigint NOT NULL DEFAULT 0, idempotency_key text NOT NULL, request_fingerprint text NOT NULL,
  created_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(route_plan_id,route_version,input_fingerprint), UNIQUE(organization_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_fleet_route_optimization_status ON fleet_partner_route_optimization_requests(organization_id,status,updated_at);
CREATE TABLE IF NOT EXISTS fleet_partner_route_optimization_attempts (
  id text PRIMARY KEY, organization_id text NOT NULL, optimization_request_id text NOT NULL REFERENCES fleet_partner_route_optimization_requests(id) ON DELETE RESTRICT,
  attempt integer NOT NULL CHECK(attempt>0), outcome text NOT NULL, safe_error_code text NOT NULL DEFAULT '',
  occurred_at timestamptz NOT NULL DEFAULT now(), UNIQUE(optimization_request_id,attempt,outcome)
);
