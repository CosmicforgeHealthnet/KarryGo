-- Section 16: company geofences, deduplicated triggers and operational alerts.
CREATE TABLE IF NOT EXISTS fleet_partner_geofences (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
  branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT, name text NOT NULL,
  geometry_type text NOT NULL CHECK(geometry_type IN('CIRCLE','POLYGON')),
  geometry jsonb NOT NULL, geometry_reference text NOT NULL DEFAULT '', event_rules jsonb NOT NULL DEFAULT '[]'::jsonb,
  active_status text NOT NULL DEFAULT 'INACTIVE' CHECK(active_status IN('ACTIVE','INACTIVE')),
  created_by text NOT NULL, idempotency_key text NOT NULL, request_fingerprint text NOT NULL,
  version bigint NOT NULL DEFAULT 1 CHECK(version>0), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(organization_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_fleet_geofences_scope ON fleet_partner_geofences(organization_id,branch_id,active_status);
CREATE TABLE IF NOT EXISTS fleet_partner_geofence_events (
  id text PRIMARY KEY, organization_id text NOT NULL, geofence_id text NOT NULL REFERENCES fleet_partner_geofences(id) ON DELETE RESTRICT,
  canonical_trip_reference text NOT NULL, event_rule text NOT NULL CHECK(event_rule IN('ENTER','EXIT','DWELL','ROUTE_DEVIATION','RESTRICTED_AREA')),
  source_version bigint NOT NULL, confidence text NOT NULL CHECK(confidence IN('HIGH','LOW','SUPPRESSED_STALE')),
  occurred_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(geofence_id,canonical_trip_reference,event_rule,source_version)
);
CREATE TABLE IF NOT EXISTS fleet_partner_operational_alerts (
  id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
  branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
  alert_type text NOT NULL CHECK(alert_type IN('ASSIGNMENT_EXPIRED','PROVIDER_DECLINED','NO_ELIGIBLE_PROVIDER','ROUTE_DELAY','ROUTE_DEVIATION','GEOFENCE_ENTRY','GEOFENCE_EXIT','TRACKING_STALE','TRACKING_GAP','VEHICLE_OR_TRUCK_UNAVAILABLE','SHIPMENT_AT_RISK','SLA_AT_RISK','EXECUTION_FAILED','CANCELLATION_REQUIRES_RECONCILIATION')),
  severity text NOT NULL CHECK(severity IN('INFO','LOW','MEDIUM','HIGH','CRITICAL')),
  source_type text NOT NULL, source_id text NOT NULL, route_plan_id text REFERENCES fleet_partner_route_plans(id) ON DELETE RESTRICT,
  assignment_id text NOT NULL DEFAULT '', shipment_id text NOT NULL DEFAULT '', canonical_trip_reference text NOT NULL DEFAULT '',
  owner_user_id text NOT NULL DEFAULT '', status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','ACKNOWLEDGED','IN_PROGRESS','RESOLVED','DISMISSED')),
  condition_fingerprint text NOT NULL, sla_due_at timestamptz NOT NULL, acknowledged_at timestamptz, resolved_at timestamptz,
  resolution_code text NOT NULL DEFAULT '', resolution_reason text NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 1 CHECK(version>0),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_alert_open_condition ON fleet_partner_operational_alerts(organization_id,condition_fingerprint) WHERE status IN('OPEN','ACKNOWLEDGED','IN_PROGRESS');
CREATE INDEX IF NOT EXISTS idx_fleet_alerts_scope ON fleet_partner_operational_alerts(organization_id,branch_id,status,severity,sla_due_at);
CREATE TABLE IF NOT EXISTS fleet_partner_operational_alert_history (
  id text PRIMARY KEY, organization_id text NOT NULL, alert_id text NOT NULL REFERENCES fleet_partner_operational_alerts(id) ON DELETE RESTRICT,
  previous_status text NOT NULL, new_status text NOT NULL, actor_id text NOT NULL, reason text NOT NULL DEFAULT '', version bigint NOT NULL,
  occurred_at timestamptz NOT NULL DEFAULT now()
);
