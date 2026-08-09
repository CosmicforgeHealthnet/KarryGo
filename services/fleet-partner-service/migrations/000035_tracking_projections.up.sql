-- Section 16: last-known projections from authoritative Dispatch/Hauling events.
CREATE TABLE IF NOT EXISTS fleet_partner_tracking_event_inbox (
  event_id text PRIMARY KEY,
  organization_id text NOT NULL,
  source_service text NOT NULL CHECK(source_service IN('driver-dispatch-delivery-service','driver-hauling-service')),
  canonical_trip_reference text NOT NULL,
  source_version bigint NOT NULL CHECK(source_version>0),
  payload_hash text NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS fleet_partner_tracking_projections (
  organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
  source_service text NOT NULL CHECK(source_service IN('driver-dispatch-delivery-service','driver-hauling-service')),
  canonical_trip_reference text NOT NULL,
  assignment_id text NOT NULL DEFAULT '',
  route_plan_id text REFERENCES fleet_partner_route_plans(id) ON DELETE RESTRICT,
  provider_id text NOT NULL DEFAULT '',
  vehicle_or_truck_reference text NOT NULL DEFAULT '',
  latitude numeric(9,6), longitude numeric(9,6), heading numeric(6,2), speed numeric(9,3), accuracy numeric(9,3),
  operational_status text NOT NULL DEFAULT 'UNKNOWN',
  source_timestamp timestamptz NOT NULL,
  received_at timestamptz NOT NULL,
  source_version bigint NOT NULL CHECK(source_version>0),
  payload_hash text NOT NULL,
  freshness_status text NOT NULL CHECK(freshness_status IN('FRESH','AGING','STALE','UNKNOWN','GAPPED')),
  gap_status text NOT NULL CHECK(gap_status IN('CURRENT','GAP_DETECTED','CONFLICT','RECONCILING')),
  last_reconciled_at timestamptz,
  retention_expires_at timestamptz NOT NULL,
  PRIMARY KEY(source_service,canonical_trip_reference),
  CHECK((latitude IS NULL AND longitude IS NULL) OR (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180))
);
CREATE INDEX IF NOT EXISTS idx_fleet_tracking_scope ON fleet_partner_tracking_projections(organization_id,route_plan_id,freshness_status,gap_status);
CREATE INDEX IF NOT EXISTS idx_fleet_tracking_retention ON fleet_partner_tracking_projections(retention_expires_at);
CREATE TABLE IF NOT EXISTS fleet_partner_tracking_projection_conflicts (
  id text PRIMARY KEY, organization_id text NOT NULL, source_service text NOT NULL,
  canonical_trip_reference text NOT NULL, source_version bigint NOT NULL,
  existing_payload_hash text NOT NULL, received_payload_hash text NOT NULL,
  status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','RECONCILING','RESOLVED','DEAD_LETTER')),
  attempts integer NOT NULL DEFAULT 0, lease_owner text, lease_expires_at timestamptz, lease_heartbeat_at timestamptz,
  next_attempt_at timestamptz NOT NULL DEFAULT now(), last_error_code text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), resolved_at timestamptz,
  UNIQUE(source_service,canonical_trip_reference,source_version,received_payload_hash)
);
CREATE INDEX IF NOT EXISTS idx_fleet_tracking_conflicts_claim ON fleet_partner_tracking_projection_conflicts(status,next_attempt_at) WHERE status IN('OPEN','RECONCILING');
