-- Sections 21 and 51: warehouse receipts with lines, leased put-away and pick
-- tasks, pick waves, packing units and append-only gate events.
--
-- Stock itself stays in fleet_partner_inventory_items and every quantity
-- change is still an immutable fleet_partner_inventory_movements row; these
-- tables are the operational work that drives those movements.

CREATE TABLE fleet_partner_warehouse_receipts (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 receipt_number text NOT NULL,
 source_type text NOT NULL CHECK(source_type IN('SUPPLIER','CUSTOMER','TRANSFER','RETURN','OTHER')),
 source_reference text NOT NULL DEFAULT '',
 receiving_location_id text NOT NULL REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 quarantine_location_id text REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 yard_visit_id text REFERENCES fleet_partner_yard_visits(id) ON DELETE RESTRICT,
 status text NOT NULL DEFAULT 'EXPECTED' CHECK(status IN('DRAFT','EXPECTED','ARRIVED','RECEIVING','EXCEPTION','RECEIVED','PUTAWAY_IN_PROGRESS','COMPLETED','CANCELLED')),
 expected_at timestamptz, arrived_at timestamptz, received_at timestamptz, completed_at timestamptz,
 version bigint NOT NULL DEFAULT 1, created_by text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,receipt_number)
);
CREATE INDEX idx_fleet_warehouse_receipts_scope ON fleet_partner_warehouse_receipts(organization_id,warehouse_id,status,expected_at);

CREATE TABLE fleet_partner_warehouse_receipt_lines (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 receipt_id text NOT NULL REFERENCES fleet_partner_warehouse_receipts(id) ON DELETE RESTRICT,
 line_number integer NOT NULL CHECK(line_number>0),
 sku text NOT NULL, owner_type text NOT NULL, owner_reference text NOT NULL,
 lot_reference text NOT NULL DEFAULT '', serial_reference text NOT NULL DEFAULT '', unit text NOT NULL,
 expected_quantity numeric(18,3) NOT NULL CHECK(expected_quantity>0),
 received_quantity numeric(18,3) NOT NULL DEFAULT 0 CHECK(received_quantity>=0),
 damaged_quantity numeric(18,3) NOT NULL DEFAULT 0 CHECK(damaged_quantity>=0),
 quarantined_quantity numeric(18,3) NOT NULL DEFAULT 0 CHECK(quarantined_quantity>=0),
 accepted_quantity numeric(18,3) NOT NULL DEFAULT 0 CHECK(accepted_quantity>=0),
 inventory_item_id text REFERENCES fleet_partner_inventory_items(id) ON DELETE RESTRICT,
 quarantine_item_id text REFERENCES fleet_partner_inventory_items(id) ON DELETE RESTRICT,
 putaway_location_id text REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','RECEIVED')),
 discrepancy_status text NOT NULL DEFAULT 'NONE' CHECK(discrepancy_status IN('NONE','OPEN','RESOLVED')),
 discrepancy_resolution text NOT NULL DEFAULT '', discrepancy_note text NOT NULL DEFAULT '',
 received_by text NOT NULL DEFAULT '', received_at timestamptz,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(receipt_id,line_number),
 CHECK(damaged_quantity+quarantined_quantity<=received_quantity),
 CHECK(accepted_quantity=received_quantity-damaged_quantity-quarantined_quantity OR status='PENDING')
);
CREATE INDEX idx_fleet_receipt_lines_receipt ON fleet_partner_warehouse_receipt_lines(organization_id,receipt_id,status,discrepancy_status);

CREATE TABLE fleet_partner_putaway_tasks (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 receipt_id text NOT NULL REFERENCES fleet_partner_warehouse_receipts(id) ON DELETE RESTRICT,
 receipt_line_id text NOT NULL REFERENCES fleet_partner_warehouse_receipt_lines(id) ON DELETE RESTRICT,
 inventory_item_id text NOT NULL REFERENCES fleet_partner_inventory_items(id) ON DELETE RESTRICT,
 from_location_id text NOT NULL REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 suggested_location_id text REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 to_location_id text REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 destination_item_id text REFERENCES fleet_partner_inventory_items(id) ON DELETE RESTRICT,
 quantity numeric(18,3) NOT NULL CHECK(quantity>0),
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','CLAIMED','COMPLETED','CANCELLED')),
 lease_owner text NOT NULL DEFAULT '', client_claim_id text NOT NULL DEFAULT '', lease_expires_at timestamptz,
 completed_by text NOT NULL DEFAULT '', completed_at timestamptz,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(receipt_line_id),
 CHECK(status<>'CLAIMED' OR (lease_owner<>'' AND lease_expires_at IS NOT NULL))
);
CREATE INDEX idx_fleet_putaway_tasks_scope ON fleet_partner_putaway_tasks(organization_id,warehouse_id,status,lease_expires_at);

CREATE TABLE fleet_partner_pick_waves (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 wave_number text NOT NULL,
 status text NOT NULL DEFAULT 'RELEASED' CHECK(status IN('RELEASED','COMPLETED','CANCELLED')),
 created_by text NOT NULL, completed_at timestamptz,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,wave_number)
);
CREATE INDEX idx_fleet_pick_waves_scope ON fleet_partner_pick_waves(organization_id,warehouse_id,status);

CREATE TABLE fleet_partner_pick_tasks (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 pick_wave_id text NOT NULL REFERENCES fleet_partner_pick_waves(id) ON DELETE RESTRICT,
 fulfilment_order_id text NOT NULL REFERENCES fleet_partner_fulfilment_orders(id) ON DELETE RESTRICT,
 fulfilment_line_id text NOT NULL REFERENCES fleet_partner_fulfilment_lines(id) ON DELETE RESTRICT,
 inventory_item_id text NOT NULL REFERENCES fleet_partner_inventory_items(id) ON DELETE RESTRICT,
 from_location_id text NOT NULL REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 quantity numeric(18,3) NOT NULL CHECK(quantity>0),
 picked_quantity numeric(18,3) NOT NULL DEFAULT 0 CHECK(picked_quantity>=0 AND picked_quantity<=quantity),
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','CLAIMED','COMPLETED','SHORT','CANCELLED')),
 lease_owner text NOT NULL DEFAULT '', client_claim_id text NOT NULL DEFAULT '', lease_expires_at timestamptz,
 completed_by text NOT NULL DEFAULT '', completed_at timestamptz,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(status<>'CLAIMED' OR (lease_owner<>'' AND lease_expires_at IS NOT NULL))
);
-- A fulfilment line is picked by at most one live task.
CREATE UNIQUE INDEX uq_fleet_pick_task_live_line ON fleet_partner_pick_tasks(fulfilment_line_id) WHERE status IN('OPEN','CLAIMED','COMPLETED','SHORT');
CREATE INDEX idx_fleet_pick_tasks_scope ON fleet_partner_pick_tasks(organization_id,pick_wave_id,status,lease_expires_at);

CREATE TABLE fleet_partner_packing_units (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 fulfilment_order_id text NOT NULL REFERENCES fleet_partner_fulfilment_orders(id) ON DELETE RESTRICT,
 packing_code text NOT NULL,
 status text NOT NULL DEFAULT 'PACKED' CHECK(status IN('PACKED','SEALED','STAGED','LOADED')),
 weight_grams bigint CHECK(weight_grams IS NULL OR weight_grams>0),
 dimensions_mm jsonb NOT NULL DEFAULT '{}'::jsonb,
 seal_number text NOT NULL DEFAULT '',
 contents jsonb NOT NULL DEFAULT '[]'::jsonb,
 packed_by text NOT NULL, packed_at timestamptz NOT NULL DEFAULT now(),
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,packing_code)
);
CREATE INDEX idx_fleet_packing_units_order ON fleet_partner_packing_units(organization_id,fulfilment_order_id,status);

CREATE TABLE fleet_partner_gate_events (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 yard_visit_id text NOT NULL REFERENCES fleet_partner_yard_visits(id) ON DELETE RESTRICT,
 sequence_no integer NOT NULL CHECK(sequence_no>0),
 event_type text NOT NULL CHECK(event_type IN('ENTRY','SECURITY_CHECK','SEAL_CHECK','PARKING','EXIT')),
 gate_code text NOT NULL, actor_reference text NOT NULL,
 evidence_file_ids jsonb NOT NULL DEFAULT '[]'::jsonb, notes text NOT NULL DEFAULT '',
 occurred_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(yard_visit_id,sequence_no)
);
CREATE INDEX idx_fleet_gate_events_visit ON fleet_partner_gate_events(organization_id,yard_visit_id,sequence_no);
CREATE TRIGGER trg_fleet_gate_events_append_only BEFORE UPDATE OR DELETE ON fleet_partner_gate_events FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('warehouse_receipts.read','warehouse_receipts','Read warehouse receipts',true),
('warehouse_receipts.manage','warehouse_receipts','Create, receive and resolve warehouse receipts',true),
('warehouse_tasks.read','warehouse_tasks','Read put-away and pick tasks',true),
('warehouse_tasks.execute','warehouse_tasks','Claim and complete put-away and pick tasks',true),
('pick_waves.manage','warehouse_tasks','Create pick waves',true),
('packing.manage','warehouse_tasks','Create packing units',true),
('yard.gate','yard','Record yard gate events',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('warehouse_receipts.read','warehouse_receipts.manage','warehouse_tasks.read','warehouse_tasks.execute','pick_waves.manage','packing.manage','yard.gate')
WHERE r.id IN('rol_system_owner','rol_system_operations') ON CONFLICT DO NOTHING;
