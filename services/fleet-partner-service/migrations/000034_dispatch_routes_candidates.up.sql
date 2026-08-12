-- Section 16: corporate dispatch planning, route planning and explainable candidates.
CREATE TABLE IF NOT EXISTS fleet_partner_dispatch_plans (
  id text PRIMARY KEY,
  organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
  branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
  plan_date date NOT NULL,
  work_window_start timestamptz NOT NULL,
  work_window_end timestamptz NOT NULL,
  timezone text NOT NULL,
  status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','READY','APPROVED','PUBLISHED','IN_PROGRESS','COMPLETED','CANCELLED')),
  planner_user_id text NOT NULL,
  approved_by text,
  approved_at timestamptz,
  published_at timestamptz,
  idempotency_key text NOT NULL,
  request_fingerprint text NOT NULL,
  version bigint NOT NULL DEFAULT 1 CHECK(version>0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK(work_window_end>work_window_start),
  UNIQUE(organization_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_fleet_dispatch_plans_scope ON fleet_partner_dispatch_plans(organization_id,branch_id,plan_date,status);

CREATE TABLE IF NOT EXISTS fleet_partner_route_plans (
  id text PRIMARY KEY,
  organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
  branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
  dispatch_plan_id text REFERENCES fleet_partner_dispatch_plans(id) ON DELETE RESTRICT,
  replacement_for_route_id text REFERENCES fleet_partner_route_plans(id) ON DELETE RESTRICT,
  route_reference text NOT NULL,
  route_type text NOT NULL CHECK(route_type IN('DELIVERY','HAULING','WAREHOUSE_TRANSFER','RETURN','MIXED','PASSENGER')),
  provider_membership_id text NOT NULL DEFAULT '',
  provider_id text NOT NULL DEFAULT '',
  vehicle_or_truck_reference text NOT NULL DEFAULT '',
  execution_service text NOT NULL CHECK(execution_service IN('DISPATCH','HAULING','TAXI')),
  status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','OPTIMIZATION_PENDING','OPTIMIZED','READY','APPROVED','PUBLISHED','IN_PROGRESS','COMPLETED','PARTIALLY_COMPLETED','FAILED','CANCELLED')),
  planned_distance numeric(18,3) NOT NULL DEFAULT 0 CHECK(planned_distance>=0),
  distance_unit text NOT NULL DEFAULT 'KM' CHECK(distance_unit IN('KM','MI')),
  planned_duration_seconds integer NOT NULL DEFAULT 0 CHECK(planned_duration_seconds>=0),
  optimization_reference text NOT NULL DEFAULT '',
  optimization_version bigint NOT NULL DEFAULT 0 CHECK(optimization_version>=0),
  constraint_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
  approved_by text,
  approved_at timestamptz,
  published_at timestamptz,
  idempotency_key text NOT NULL,
  request_fingerprint text NOT NULL,
  version bigint NOT NULL DEFAULT 1 CHECK(version>0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(organization_id,route_reference),
  UNIQUE(organization_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_fleet_route_plans_scope ON fleet_partner_route_plans(organization_id,branch_id,status,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_route_plans_dispatch ON fleet_partner_route_plans(dispatch_plan_id,status);

CREATE TABLE IF NOT EXISTS fleet_partner_route_stops (
  id text PRIMARY KEY,
  organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
  route_plan_id text NOT NULL REFERENCES fleet_partner_route_plans(id) ON DELETE RESTRICT,
  sequence integer NOT NULL CHECK(sequence>0),
  stop_type text NOT NULL CHECK(stop_type IN('PICKUP','DELIVERY','PASSENGER_PICKUP','PASSENGER_DROPOFF','WAREHOUSE_TRANSFER','RETURN','DEPOT_START','DEPOT_END')),
  order_id text,
  shipment_id text,
  package_id text,
  manifest_id text,
  source_order_stop_id text,
  location_reference text NOT NULL DEFAULT '',
  latitude numeric(9,6),
  longitude numeric(9,6),
  time_window_start timestamptz,
  time_window_end timestamptz,
  service_duration_seconds integer NOT NULL DEFAULT 0 CHECK(service_duration_seconds>=0),
  planned_arrival_at timestamptz,
  planned_departure_at timestamptz,
  actual_arrival_at timestamptz,
  actual_departure_at timestamptz,
  status text NOT NULL DEFAULT 'PLANNED' CHECK(status IN('PLANNED','ASSIGNED','EN_ROUTE','ARRIVED','SERVICE_STARTED','COMPLETED','FAILED','SKIPPED','CANCELLED')),
  version bigint NOT NULL DEFAULT 1 CHECK(version>0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(route_plan_id,sequence),
  CHECK((latitude IS NULL AND longitude IS NULL) OR (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180)),
  CHECK(time_window_end IS NULL OR time_window_start IS NULL OR time_window_end>time_window_start),
  CHECK(planned_departure_at IS NULL OR planned_arrival_at IS NULL OR planned_departure_at>=planned_arrival_at)
);
CREATE INDEX IF NOT EXISTS idx_fleet_route_stops_route ON fleet_partner_route_stops(route_plan_id,sequence);
CREATE INDEX IF NOT EXISTS idx_fleet_route_stops_work ON fleet_partner_route_stops(organization_id,order_id,shipment_id,status);

CREATE TABLE IF NOT EXISTS fleet_partner_assignment_candidates (
  id text PRIMARY KEY,
  organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
  branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
  source_type text NOT NULL CHECK(source_type IN('ORDER','SHIPMENT','ROUTE','ROUTE_STOP')),
  source_id text NOT NULL,
  provider_id text NOT NULL,
  provider_membership_id text NOT NULL,
  vehicle_or_truck_reference text NOT NULL DEFAULT '',
  execution_service text NOT NULL CHECK(execution_service IN('DISPATCH','HAULING')),
  eligibility_status text NOT NULL CHECK(eligibility_status IN('ELIGIBLE','INELIGIBLE','UNKNOWN','STALE')),
  total_score integer NOT NULL CHECK(total_score BETWEEN 0 AND 100),
  score_components jsonb NOT NULL DEFAULT '{}'::jsonb,
  eligibility_reasons jsonb NOT NULL DEFAULT '[]'::jsonb,
  warnings jsonb NOT NULL DEFAULT '[]'::jsonb,
  hard_blocks jsonb NOT NULL DEFAULT '[]'::jsonb,
  data_freshness text NOT NULL CHECK(data_freshness IN('FRESH','AGING','STALE','UNKNOWN','GAPPED')),
  generated_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  source_versions jsonb NOT NULL DEFAULT '{}'::jsonb,
  input_fingerprint text NOT NULL,
  CHECK(expires_at>generated_at)
);
CREATE INDEX IF NOT EXISTS idx_fleet_candidates_source ON fleet_partner_assignment_candidates(organization_id,source_type,source_id,expires_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_candidates_snapshot ON fleet_partner_assignment_candidates(organization_id,source_type,source_id,provider_membership_id,vehicle_or_truck_reference,input_fingerprint);

CREATE TABLE IF NOT EXISTS fleet_partner_dispatch_route_history (
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  aggregate_type text NOT NULL CHECK(aggregate_type IN('DISPATCH_PLAN','ROUTE_PLAN','ROUTE_STOP')),
  aggregate_id text NOT NULL,
  previous_status text NOT NULL DEFAULT '',
  new_status text NOT NULL,
  reason_code text NOT NULL DEFAULT '',
  actor_id text NOT NULL,
  version bigint NOT NULL,
  snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
  occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_fleet_dispatch_route_history ON fleet_partner_dispatch_route_history(organization_id,aggregate_type,aggregate_id,version);

CREATE TABLE IF NOT EXISTS fleet_partner_route_publication_snapshots (
  id text PRIMARY KEY,
  organization_id text NOT NULL,
  route_plan_id text NOT NULL REFERENCES fleet_partner_route_plans(id) ON DELETE RESTRICT,
  route_version bigint NOT NULL,
  assignment_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
  override_reason text NOT NULL DEFAULT '',
  snapshot jsonb NOT NULL,
  published_by text NOT NULL,
  published_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(route_plan_id,route_version)
);

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('dispatch.read','dispatch','Read dispatch board and plans',true),('dispatch.plan','dispatch','Create and edit dispatch plans',true),
('dispatch.approve','dispatch','Approve dispatch plans',true),('dispatch.publish','dispatch','Publish dispatch plans',true),
('dispatch.override_warning','dispatch','Override an allowed dispatch warning with a reason',true),
('routes.read','routes','Read route plans and stops',true),('routes.manage','routes','Manage draft route plans and stops',true),
('routes.optimize','routes','Request route optimization',true),('routes.approve','routes','Approve route plans',true),
('routes.publish','routes','Publish route plans',true),('routes.replan','routes','Create controlled route replacements',true),
('control_tower.read','dispatch','Read control tower projections',true),('tracking.read','tracking','Read tracking projections',true),
('geofences.read','tracking','Read geofences',true),('geofences.manage','tracking','Manage geofences',true),
('alerts.read','dispatch','Read operational alerts',true),('alerts.acknowledge','dispatch','Acknowledge operational alerts',true),
('alerts.resolve','dispatch','Resolve operational alerts',true),('alerts.replay','dispatch','Replay Section 16 dead-lettered work',true)
ON CONFLICT(code) DO UPDATE SET description=EXCLUDED.description,is_active=true;

INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT fleet_partner_roles.id,p.code FROM fleet_partner_roles CROSS JOIN (VALUES
('dispatch.read'),('dispatch.plan'),('dispatch.approve'),('dispatch.publish'),('dispatch.override_warning'),
('routes.read'),('routes.manage'),('routes.optimize'),('routes.approve'),('routes.publish'),('routes.replan'),
('control_tower.read'),('tracking.read'),('geofences.read'),('geofences.manage'),
('alerts.read'),('alerts.acknowledge'),('alerts.resolve'),('alerts.replay')) AS p(code)
WHERE id IN('rol_system_owner','rol_system_operations') ON CONFLICT DO NOTHING;
