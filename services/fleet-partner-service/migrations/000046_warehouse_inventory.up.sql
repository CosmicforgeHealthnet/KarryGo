-- Section 21.1: warehouse hierarchy, customer stock and immutable movements.
CREATE TABLE fleet_partner_warehouses (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 code text NOT NULL, name text NOT NULL, site_reference text NOT NULL DEFAULT '', address_reference text NOT NULL,
 timezone text NOT NULL, operating_hours jsonb NOT NULL DEFAULT '{}'::jsonb,
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','ACTIVE','INACTIVE','ARCHIVED')),
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), archived_at timestamptz,
 UNIQUE(organization_id,code)
);
CREATE INDEX idx_fleet_warehouses_scope ON fleet_partner_warehouses(organization_id,branch_id,status);

CREATE TABLE fleet_partner_storage_locations (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 parent_location_id text REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 code text NOT NULL, name text NOT NULL,
 location_type text NOT NULL CHECK(location_type IN('ZONE','AISLE','RACK','SHELF','BIN','COLD_ROOM','QUARANTINE','STAGING','DOCK','YARD_BAY','SECURE_AREA')),
 capacity numeric(18,3) CHECK(capacity IS NULL OR capacity>=0), capacity_unit text NOT NULL DEFAULT '',
 restrictions jsonb NOT NULL DEFAULT '{}'::jsonb,
 status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','INACTIVE','ARCHIVED')),
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(warehouse_id,code), CHECK(parent_location_id IS NULL OR parent_location_id<>id)
);
CREATE INDEX idx_fleet_storage_location_tree ON fleet_partner_storage_locations(organization_id,warehouse_id,parent_location_id,status);

CREATE TABLE fleet_partner_inventory_items (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 warehouse_id text NOT NULL REFERENCES fleet_partner_warehouses(id) ON DELETE RESTRICT,
 location_id text NOT NULL REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 owner_type text NOT NULL, owner_reference text NOT NULL, sku text NOT NULL,
 batch_reference text NOT NULL DEFAULT '', lot_reference text NOT NULL DEFAULT '', serial_reference text NOT NULL DEFAULT '',
 quantity_projection numeric(18,3) NOT NULL DEFAULT 0 CHECK(quantity_projection>=0),
 reserved_quantity numeric(18,3) NOT NULL DEFAULT 0 CHECK(reserved_quantity>=0 AND reserved_quantity<=quantity_projection),
 unit text NOT NULL, status text NOT NULL DEFAULT 'EXPECTED' CHECK(status IN('EXPECTED','AVAILABLE','RESERVED','PICKING','PACKED','QUARANTINED','DAMAGED','EXPIRED','DISPATCHED','RETURNED')),
 expiry_at timestamptz, condition text NOT NULL DEFAULT 'GOOD',
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,warehouse_id,location_id,owner_type,owner_reference,sku,batch_reference,lot_reference,serial_reference)
);
CREATE INDEX idx_fleet_inventory_lookup ON fleet_partner_inventory_items(organization_id,warehouse_id,location_id,sku,status,expiry_at);

CREATE TABLE fleet_partner_inventory_movements (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 inventory_item_id text NOT NULL REFERENCES fleet_partner_inventory_items(id) ON DELETE RESTRICT,
 movement_type text NOT NULL CHECK(movement_type IN('RECEIPT','PUT_AWAY','PICK','PACK','TRANSFER','ADJUSTMENT','QUARANTINE','RELEASE','DISPATCH','RETURN','DAMAGE','EXPIRY')),
 from_location_id text REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 to_location_id text REFERENCES fleet_partner_storage_locations(id) ON DELETE RESTRICT,
 quantity numeric(18,3) NOT NULL CHECK(quantity>0), reason_code text NOT NULL, actor_reference text NOT NULL,
 fulfilment_order_id text, terminal_transfer_id text, return_id text,
 correlation_id text NOT NULL, projection_before numeric(18,3) NOT NULL, projection_after numeric(18,3) NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(organization_id,correlation_id), CHECK(projection_before>=0 AND projection_after>=0)
);
CREATE INDEX idx_fleet_inventory_movement_item ON fleet_partner_inventory_movements(organization_id,inventory_item_id,created_at,id);
CREATE TRIGGER trg_fleet_inventory_movements_append_only BEFORE UPDATE OR DELETE ON fleet_partner_inventory_movements FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_inventory_reservations (
 id text PRIMARY KEY, organization_id text NOT NULL, inventory_item_id text NOT NULL REFERENCES fleet_partner_inventory_items(id) ON DELETE RESTRICT,
 fulfilment_order_id text NOT NULL, quantity numeric(18,3) NOT NULL CHECK(quantity>0),
 status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','PICKED','RELEASED','EXPIRED')),
 expires_at timestamptz NOT NULL, version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,inventory_item_id,fulfilment_order_id)
);
CREATE INDEX idx_fleet_inventory_reservations_due ON fleet_partner_inventory_reservations(organization_id,status,expires_at);

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('warehouses.read','warehouses','Read warehouses',true),('warehouses.manage','warehouses','Manage warehouses',true),
('warehouse_locations.read','warehouse_locations','Read storage locations',true),('warehouse_locations.manage','warehouse_locations','Manage storage locations',true),
('warehouse_inventory.read','warehouse_inventory','Read warehouse stock',true),('warehouse_inventory.receive','warehouse_inventory','Receive warehouse stock',true),
('warehouse_inventory.move','warehouse_inventory','Move warehouse stock',true),('warehouse_inventory.adjust','warehouse_inventory','Adjust warehouse stock',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p WHERE r.id IN('rol_system_owner','rol_system_operations') AND p.category IN('warehouses','warehouse_locations','warehouse_inventory') ON CONFLICT DO NOTHING;
