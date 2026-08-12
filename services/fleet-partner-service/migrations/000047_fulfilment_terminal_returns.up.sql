-- Section 21.2: fulfilment, dock, yard, terminal and return operations.
CREATE TABLE fleet_partner_fulfilment_orders (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 customer_reference text NOT NULL, source_order_id text NOT NULL DEFAULT '', priority text NOT NULL DEFAULT 'NORMAL',
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','RELEASED','PICKING','PICKED','PACKING','PACKED','READY_FOR_DISPATCH','DISPATCHED','COMPLETED','CANCELLED','FAILED')),
 requested_dispatch_at timestamptz, assigned_team_reference text NOT NULL DEFAULT '', packing_requirements jsonb NOT NULL DEFAULT '{}'::jsonb,
 proof_policy_reference text NOT NULL DEFAULT '', shipment_reference text NOT NULL DEFAULT '', manifest_reference text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz
);
CREATE INDEX idx_fleet_fulfilment_scope ON fleet_partner_fulfilment_orders(organization_id,warehouse_id,status,requested_dispatch_at);
CREATE TABLE fleet_partner_fulfilment_lines (
 id text PRIMARY KEY, organization_id text NOT NULL, fulfilment_order_id text NOT NULL REFERENCES fleet_partner_fulfilment_orders(id) ON DELETE RESTRICT,
 inventory_item_id text NOT NULL REFERENCES fleet_partner_inventory_items(id) ON DELETE RESTRICT,
 requested_quantity numeric(18,3) NOT NULL CHECK(requested_quantity>0), picked_quantity numeric(18,3) NOT NULL DEFAULT 0 CHECK(picked_quantity>=0),
 package_reference text NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(fulfilment_order_id,inventory_item_id)
);
ALTER TABLE fleet_partner_inventory_reservations ADD CONSTRAINT fk_inventory_reservation_fulfilment FOREIGN KEY(fulfilment_order_id) REFERENCES fleet_partner_fulfilment_orders(id) ON DELETE RESTRICT;
ALTER TABLE fleet_partner_inventory_movements ADD CONSTRAINT fk_inventory_movement_fulfilment FOREIGN KEY(fulfilment_order_id) REFERENCES fleet_partner_fulfilment_orders(id) ON DELETE RESTRICT;

CREATE TABLE fleet_partner_dock_appointments (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 dock_location_id text NOT NULL REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 carrier_reference text NOT NULL DEFAULT '', provider_reference text NOT NULL DEFAULT '', vehicle_or_truck_reference text NOT NULL,
 appointment_type text NOT NULL CHECK(appointment_type IN('LOAD','UNLOAD','CROSS_DOCK')),
 window_start timestamptz NOT NULL, window_end timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'REQUESTED' CHECK(status IN('REQUESTED','CONFIRMED','CHECKED_IN','AT_DOCK','LOADING','UNLOADING','COMPLETED','NO_SHOW','CANCELLED')),
 checked_in_at timestamptz, loading_started_at timestamptz, loading_completed_at timestamptz, checked_out_at timestamptz,
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), CHECK(window_end>window_start)
);
CREATE INDEX idx_fleet_dock_capacity ON fleet_partner_dock_appointments(organization_id,dock_location_id,window_start,window_end,status);

CREATE TABLE fleet_partner_yard_visits (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 provider_reference text NOT NULL, vehicle_or_truck_reference text NOT NULL, gate_reference text NOT NULL DEFAULT '', bay_reference text NOT NULL DEFAULT '', seal_reference text NOT NULL DEFAULT '',
 security_status text NOT NULL DEFAULT 'PENDING' CHECK(security_status IN('PENDING','CLEARED','HOLD')),
 status text NOT NULL DEFAULT 'EXPECTED' CHECK(status IN('EXPECTED','GATE_CHECK_IN','SECURITY_HOLD','IN_YARD','AT_BAY','RELEASED','CHECKED_OUT','CANCELLED')),
 checked_in_at timestamptz, entered_yard_at timestamptz, exited_yard_at timestamptz, checked_out_at timestamptz,
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fleet_yard_visits_scope ON fleet_partner_yard_visits(organization_id,warehouse_id,status,created_at);
CREATE TABLE fleet_partner_yard_visit_history (
 id text PRIMARY KEY, organization_id text NOT NULL, yard_visit_id text NOT NULL REFERENCES fleet_partner_yard_visits(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, previous_seal text NOT NULL, new_seal text NOT NULL,
 reason text NOT NULL, actor_reference text NOT NULL, version bigint NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_yard_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_yard_visit_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_terminal_transfers (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 origin_warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 destination_warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 manifest_id text NOT NULL, route_reference text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','READY','DEPARTED','IN_TRANSIT','ARRIVED','RECONCILING','COMPLETED','DISCREPANCY','CANCELLED')),
 departed_at timestamptz, arrived_at timestamptz,
 origin_snapshot_reference text NOT NULL DEFAULT '', destination_snapshot_reference text NOT NULL DEFAULT '',
 reconciliation_status text NOT NULL DEFAULT 'PENDING' CHECK(reconciliation_status IN('PENDING','MATCHED','DISCREPANCY','RESOLVED')),
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(origin_warehouse_id<>destination_warehouse_id)
);
CREATE TABLE fleet_partner_terminal_transfer_snapshots (
 id text PRIMARY KEY, organization_id text NOT NULL, transfer_id text NOT NULL REFERENCES fleet_partner_terminal_transfers(id) ON DELETE RESTRICT,
 snapshot_type text NOT NULL CHECK(snapshot_type IN('ORIGIN','DESTINATION')), package_snapshot jsonb NOT NULL, inventory_snapshot jsonb NOT NULL,
 custody_snapshot jsonb NOT NULL, captured_by text NOT NULL, captured_at timestamptz NOT NULL DEFAULT now(), UNIQUE(transfer_id,snapshot_type)
);
CREATE TRIGGER trg_fleet_terminal_snapshots_append_only BEFORE UPDATE OR DELETE ON fleet_partner_terminal_transfer_snapshots FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
CREATE TABLE fleet_partner_terminal_discrepancies (
 id text PRIMARY KEY, organization_id text NOT NULL, transfer_id text NOT NULL REFERENCES fleet_partner_terminal_transfers(id) ON DELETE RESTRICT,
 discrepancy_type text NOT NULL, expected_snapshot jsonb NOT NULL, actual_snapshot jsonb NOT NULL,
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','RESOLVED')), resolution_reason text NOT NULL DEFAULT '', resolved_by text NOT NULL DEFAULT '', resolved_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE fleet_partner_returns (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 source_order_id text NOT NULL DEFAULT '', source_shipment_id text NOT NULL DEFAULT '', package_id text REFERENCES fleet_partner_packages(id) ON DELETE RESTRICT,
 reason_code text NOT NULL, condition text NOT NULL DEFAULT '', disposition text NOT NULL DEFAULT '' CHECK(disposition IN('','RESTOCK','QUARANTINE','REPAIR','RETURN_TO_OWNER','DISPOSE','INVESTIGATE')),
 financial_reference text NOT NULL DEFAULT '', status text NOT NULL DEFAULT 'REQUESTED' CHECK(status IN('REQUESTED','RECEIVED','INSPECTING','DECIDED','COMPLETED','CANCELLED')),
 inventory_item_id text REFERENCES fleet_partner_inventory_items(id) ON DELETE RESTRICT,
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz
);
CREATE INDEX idx_fleet_returns_scope ON fleet_partner_returns(organization_id,warehouse_id,status,created_at);
ALTER TABLE fleet_partner_inventory_movements ADD CONSTRAINT fk_inventory_movement_terminal FOREIGN KEY(terminal_transfer_id) REFERENCES fleet_partner_terminal_transfers(id) ON DELETE RESTRICT;
ALTER TABLE fleet_partner_inventory_movements ADD CONSTRAINT fk_inventory_movement_return FOREIGN KEY(return_id) REFERENCES fleet_partner_returns(id) ON DELETE RESTRICT;

-- Immutable per-attempt evidence for every shared durable worker, including
-- Sections 19-21 handlers. The mutable job row remains the claim projection.
CREATE TABLE fleet_partner_job_attempts (
 id text PRIMARY KEY, job_id text NOT NULL REFERENCES fleet_partner_jobs(id) ON DELETE RESTRICT,
 organization_id text, attempt_number integer NOT NULL CHECK(attempt_number>0), worker_owner text NOT NULL,
 status text NOT NULL CHECK(status IN('RUNNING','SUCCEEDED','FAILED','DEAD_LETTERED','LEASE_LOST')),
 error_redacted text NOT NULL DEFAULT '', started_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 UNIQUE(job_id,attempt_number)
);
CREATE TRIGGER trg_fleet_job_attempts_append_only BEFORE UPDATE OR DELETE ON fleet_partner_job_attempts FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('fulfilment.read','fulfilment','Read fulfilment orders',true),('fulfilment.manage','fulfilment','Manage fulfilment orders',true),('fulfilment.release','fulfilment','Release fulfilment orders',true),('fulfilment.pick','fulfilment','Pick stock',true),('fulfilment.pack','fulfilment','Pack orders',true),('fulfilment.dispatch','fulfilment','Dispatch fulfilment orders',true),
('dock.read','dock','Read dock appointments',true),('dock.manage','dock','Manage dock appointments',true),
('yard.read','yard','Read yard visits',true),('yard.manage','yard','Manage yard visits',true),
('terminal.read','terminal','Read terminal transfers',true),('terminal.manage','terminal','Manage terminal transfers',true),('terminal.reconcile','terminal','Reconcile terminal transfers',true),
('returns.read','returns','Read returns',true),('returns.manage','returns','Manage returns',true),('returns.decide','returns','Decide return disposition',true),
('operations.replay','operations','Replay dead-lettered operational jobs',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p WHERE r.id IN('rol_system_owner','rol_system_operations') AND p.category IN('fulfilment','dock','yard','terminal','returns','operations') ON CONFLICT DO NOTHING;
