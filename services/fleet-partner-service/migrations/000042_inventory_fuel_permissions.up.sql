-- Section 18.3: operational parts stock and fuel/energy records (never a financial ledger).
CREATE TABLE fleet_partner_parts(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 sku text NOT NULL,name text NOT NULL,stock_unit text NOT NULL,current_quantity_projection numeric(18,3) NOT NULL DEFAULT 0,reorder_level numeric(18,3) NOT NULL DEFAULT 0,
 warranty_information text NOT NULL DEFAULT '',status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','INACTIVE','ARCHIVED')),version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,sku),CHECK(current_quantity_projection>=0 AND reorder_level>=0)
);
CREATE TABLE fleet_partner_stock_movements(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,part_id text NOT NULL REFERENCES fleet_partner_parts(id) ON DELETE RESTRICT,
 location_reference text NOT NULL,movement_type text NOT NULL CHECK(movement_type IN('RECEIPT','ISSUE','RETURN','TRANSFER_IN','TRANSFER_OUT','ADJUSTMENT')),quantity numeric(18,3) NOT NULL CHECK(quantity>0),
 reason text NOT NULL,work_order_id text REFERENCES fleet_partner_work_orders(id) ON DELETE RESTRICT,correlation_id text NOT NULL,created_by text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,correlation_id)
);
CREATE TRIGGER trg_fleet_stock_movements_append_only BEFORE UPDATE OR DELETE ON fleet_partner_stock_movements FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
CREATE TABLE fleet_partner_fuel_records(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,asset_id text NOT NULL REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
 provider_membership_id text NOT NULL,fuel_or_energy_type text NOT NULL CHECK(fuel_or_energy_type IN('PETROL','DIESEL','ELECTRIC','GAS','OTHER')),quantity numeric(18,3) NOT NULL CHECK(quantity>0),unit text NOT NULL,
 odometer numeric(18,3),operating_hours numeric(18,3),receipt_media_id text NOT NULL DEFAULT '',supplier_reference text NOT NULL DEFAULT '',recorded_at timestamptz NOT NULL,
 approval_status text NOT NULL DEFAULT 'PENDING' CHECK(approval_status IN('PENDING','APPROVED','REJECTED')),approved_by text NOT NULL DEFAULT '',version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,asset_id,supplier_reference,recorded_at),CHECK(odometer IS NULL OR odometer>=0),CHECK(operating_hours IS NULL OR operating_hours>=0)
);
CREATE TABLE fleet_partner_fuel_approval_history(
 id text PRIMARY KEY,organization_id text NOT NULL,fuel_record_id text NOT NULL REFERENCES fleet_partner_fuel_records(id) ON DELETE RESTRICT,previous_status text NOT NULL,new_status text NOT NULL,
 actor_id text NOT NULL,reason text NOT NULL,version bigint NOT NULL,occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_fuel_approval_append_only BEFORE UPDATE OR DELETE ON fleet_partner_fuel_approval_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('workforce.read','workforce','Read workforce profiles',true),('workforce.manage','workforce','Manage workforce profiles',true),
('shifts.read','shifts','Read shifts',true),('shifts.manage','shifts','Manage shifts',true),('shifts.assign','shifts','Assign shifts',true),('shifts.check_in','shifts','Check in',true),('shifts.check_out','shifts','Check out',true),
('leave.read','leave','Read leave',true),('leave.manage','leave','Manage leave',true),('leave.approve','leave','Approve leave',true),
('qualifications.read','qualifications','Read qualifications',true),('qualifications.manage','qualifications','Manage qualifications',true),('qualifications.verify','qualifications','Verify qualifications',true),
('training.read','training','Read training',true),('training.manage','training','Manage training',true),('training.record_completion','training','Record training completion',true),('performance.read','performance','Read performance snapshots',true),
('assets.read','assets','Read assets',true),('assets.manage','assets','Manage assets',true),('assets.assign','assets','Assign assets',true),('assets.archive','assets','Archive assets',true),('assets.retire','assets','Retire assets',true),
('inspections.read','inspections','Read inspections',true),('inspections.manage','inspections','Manage inspections',true),('inspections.complete','inspections','Complete inspections',true),
('maintenance.read','maintenance','Read maintenance',true),('maintenance.manage','maintenance','Manage maintenance',true),('maintenance.approve','maintenance','Approve maintenance',true),('maintenance.complete','maintenance','Complete maintenance',true),('maintenance.return_to_service','maintenance','Approve return to service',true),
('breakdowns.read','breakdowns','Read breakdowns',true),('breakdowns.manage','breakdowns','Manage breakdowns',true),
('inventory.read','inventory','Read inventory',true),('inventory.manage','inventory','Manage inventory',true),('inventory.adjust','inventory','Adjust inventory',true),
('fuel.read','fuel','Read fuel records',true),('fuel.manage','fuel','Manage fuel records',true),('fuel.approve','fuel','Approve fuel records',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p WHERE r.id IN('rol_system_owner','rol_system_operations') AND p.category IN('workforce','shifts','leave','qualifications','training','performance','assets','inspections','maintenance','breakdowns','inventory','fuel') ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT 'rol_system_dispatcher',code FROM fleet_partner_permissions WHERE code IN('workforce.read','shifts.read','shifts.assign','shifts.check_in','shifts.check_out','leave.read','qualifications.read','training.read','performance.read','assets.read','assets.assign','inspections.read','inspections.manage','breakdowns.read','breakdowns.manage') ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT 'rol_system_compliance',code FROM fleet_partner_permissions WHERE code IN('workforce.read','leave.read','qualifications.read','qualifications.verify','training.read','inspections.read','inspections.complete','maintenance.read','maintenance.return_to_service','assets.read') ON CONFLICT DO NOTHING;
