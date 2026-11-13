-- Spec Sections 16.14/45 (operational rescue cases) and 18.12/47.3/48.3
-- (asset lifecycle gaps): provider unassignment, meter readings, tyres,
-- asset defects as a resource, and asset operating-cost history.

-- ---------------------------------------------------------------------------
-- Rescue cases. A case coordinates recovery of company work hit by a
-- breakdown, provider unavailability, delay, failed delivery or an unsafe
-- vehicle. A replacement provider is never forced: the replacement is a
-- normal Section 15 private assignment the provider must accept or decline.
-- ---------------------------------------------------------------------------
CREATE TABLE fleet_partner_rescue_cases (
    id                          text PRIMARY KEY,
    organization_id             text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id                   text NOT NULL REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    trigger_type                text NOT NULL CHECK (trigger_type IN ('BREAKDOWN','PROVIDER_UNAVAILABLE','DELAY','FAILED_DELIVERY','UNSAFE_VEHICLE','ACCIDENT','CARGO_TRANSFER','OTHER')),
    severity                    text NOT NULL CHECK (severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
    status                      text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','REPLACEMENT_PENDING','REPLACEMENT_ACCEPTED','RESOLVED','CANCELLED')),
    -- One current case per subject (trip, else assignment, else breakdown,
    -- else asset) and trigger class.
    subject_key                 text NOT NULL CHECK (subject_key <> ''),
    private_assignment_id       text REFERENCES fleet_partner_private_assignments (id) ON DELETE RESTRICT,
    canonical_trip_reference    text NOT NULL DEFAULT '',
    asset_id                    text REFERENCES fleet_partner_assets (id) ON DELETE RESTRICT,
    breakdown_id                text REFERENCES fleet_partner_breakdowns (id) ON DELETE RESTRICT,
    reason                      text NOT NULL CHECK (length(reason) BETWEEN 1 AND 2000),
    assigned_to_user_id         text NOT NULL DEFAULT '',
    -- The assignment currently carrying the work: the original, or the most
    -- recent replacement offer.
    current_assignment_id       text REFERENCES fleet_partner_private_assignments (id) ON DELETE RESTRICT,
    replacement_assignment_id   text REFERENCES fleet_partner_private_assignments (id) ON DELETE RESTRICT,
    replacement_membership_id   text NOT NULL DEFAULT '',
    replacement_asset_reference text NOT NULL DEFAULT '',
    replacement_attempts        integer NOT NULL DEFAULT 0 CHECK (replacement_attempts >= 0),
    alert_id                    text NOT NULL DEFAULT '',
    resolution_code             text NOT NULL DEFAULT '',
    resolution                  jsonb NOT NULL DEFAULT '{}'::jsonb,
    resolved_by                 text NOT NULL DEFAULT '',
    resolved_at                 timestamptz,
    cancelled_at                timestamptz,
    created_by                  text NOT NULL,
    idempotency_key             text NOT NULL,
    request_fingerprint         text NOT NULL,
    version                     bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, idempotency_key),
    CHECK (private_assignment_id IS NOT NULL OR canonical_trip_reference <> '' OR asset_id IS NOT NULL OR breakdown_id IS NOT NULL),
    CHECK (status <> 'RESOLVED' OR (resolved_at IS NOT NULL AND resolution_code <> ''))
);
CREATE UNIQUE INDEX uq_fleet_rescue_current_case ON fleet_partner_rescue_cases (organization_id, subject_key, trigger_type)
    WHERE status IN ('OPEN','REPLACEMENT_PENDING','REPLACEMENT_ACCEPTED');
CREATE INDEX idx_fleet_rescue_scope ON fleet_partner_rescue_cases (organization_id, branch_id, status, severity, created_at);
CREATE INDEX idx_fleet_rescue_replacement ON fleet_partner_rescue_cases (replacement_assignment_id) WHERE status = 'REPLACEMENT_PENDING';

CREATE TABLE fleet_partner_rescue_case_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    rescue_case_id  text NOT NULL REFERENCES fleet_partner_rescue_cases (id) ON DELETE RESTRICT,
    action          text NOT NULL,
    previous_status text NOT NULL,
    new_status      text NOT NULL,
    actor_id        text NOT NULL,
    reason          text NOT NULL DEFAULT '',
    version         bigint NOT NULL,
    snapshot        jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fleet_rescue_history_case ON fleet_partner_rescue_case_history (organization_id, rescue_case_id, occurred_at);
CREATE TRIGGER trg_fleet_rescue_case_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_rescue_case_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Asset-safety and rescue alerts raised through the Section 16 alert port
-- were rejected by the original alert_type list; widen it.
ALTER TABLE fleet_partner_operational_alerts DROP CONSTRAINT fleet_partner_operational_alerts_alert_type_check;
ALTER TABLE fleet_partner_operational_alerts ADD CONSTRAINT fleet_partner_operational_alerts_alert_type_check CHECK (alert_type IN (
    'ASSIGNMENT_EXPIRED','PROVIDER_DECLINED','NO_ELIGIBLE_PROVIDER','ROUTE_DELAY','ROUTE_DEVIATION','GEOFENCE_ENTRY','GEOFENCE_EXIT',
    'TRACKING_STALE','TRACKING_GAP','VEHICLE_OR_TRUCK_UNAVAILABLE','SHIPMENT_AT_RISK','SLA_AT_RISK','EXECUTION_FAILED',
    'CANCELLATION_REQUIRES_RECONCILIATION',
    'RESCUE_REQUIRED','ASSET_BREAKDOWN','CRITICAL_INSPECTION','CRITICAL_MAINTENANCE_OVERDUE','PART_REORDER_REQUIRED',
    'UNSAFE_VEHICLE_DECLINE','CRITICAL_DEFECT','METER_READING_ANOMALY'));

-- ---------------------------------------------------------------------------
-- Provider-to-asset unassignment is recorded on the assignment itself.
-- ---------------------------------------------------------------------------
ALTER TABLE fleet_partner_asset_assignments
    ADD COLUMN ended_by text NOT NULL DEFAULT '',
    ADD COLUMN end_reason text NOT NULL DEFAULT '';

-- ---------------------------------------------------------------------------
-- Meter readings (odometer, engine hours). Append-only and monotonic; an
-- implausible jump is recorded with an anomaly flag for review, never
-- silently rejected.
-- ---------------------------------------------------------------------------
CREATE TABLE fleet_partner_meter_readings (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    asset_id         text NOT NULL REFERENCES fleet_partner_assets (id) ON DELETE RESTRICT,
    meter_type       text NOT NULL CHECK (meter_type IN ('ODOMETER','ENGINE_HOURS')),
    reading          numeric(18,3) NOT NULL CHECK (reading >= 0),
    previous_reading numeric(18,3) NOT NULL CHECK (previous_reading >= 0),
    source           text NOT NULL CHECK (source IN ('MANUAL','INSPECTION','FUEL_RECORD','TELEMATICS')),
    source_reference text NOT NULL DEFAULT '',
    recorded_at      timestamptz NOT NULL,
    anomaly          boolean NOT NULL DEFAULT false,
    anomaly_reasons  jsonb NOT NULL DEFAULT '[]'::jsonb,
    recorded_by      text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (reading >= previous_reading)
);
CREATE INDEX idx_fleet_meter_readings_asset ON fleet_partner_meter_readings (organization_id, asset_id, meter_type, recorded_at DESC);
CREATE TRIGGER trg_fleet_meter_readings_append_only BEFORE UPDATE OR DELETE ON fleet_partner_meter_readings
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Asset defects: reported from inspections, safety declines, tyre readings or
-- manual reports. A critical defect blocks the asset in the same transaction
-- and can never be deferred; only an authorized return to service releases
-- the asset.
-- ---------------------------------------------------------------------------
CREATE TABLE fleet_partner_asset_defects (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id        text NOT NULL REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    asset_id         text NOT NULL REFERENCES fleet_partner_assets (id) ON DELETE RESTRICT,
    source_type      text NOT NULL CHECK (source_type IN ('INSPECTION','SAFETY_DECLINE','TYRE_READING','MANUAL_REPORT','PROVIDER_REPORT')),
    source_reference text NOT NULL DEFAULT '',
    item_code        text NOT NULL DEFAULT '',
    description      text NOT NULL CHECK (length(description) BETWEEN 1 AND 2000),
    severity         text NOT NULL CHECK (severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
    critical         boolean GENERATED ALWAYS AS (severity = 'CRITICAL') STORED,
    status           text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','DEFERRED','REPAIRING','RESOLVED')),
    work_order_id    text REFERENCES fleet_partner_work_orders (id) ON DELETE RESTRICT,
    deferred_until   timestamptz,
    deferral_reason  text NOT NULL DEFAULT '',
    resolution_notes text NOT NULL DEFAULT '',
    resolved_by      text NOT NULL DEFAULT '',
    resolved_at      timestamptz,
    reported_by      text NOT NULL,
    version          bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (NOT (status = 'DEFERRED' AND severity = 'CRITICAL')),
    CHECK (status <> 'DEFERRED' OR deferred_until IS NOT NULL),
    CHECK (status <> 'RESOLVED' OR resolved_at IS NOT NULL)
);
CREATE UNIQUE INDEX uq_fleet_asset_defect_source_item ON fleet_partner_asset_defects (organization_id, source_type, source_reference, item_code)
    WHERE source_type IN ('INSPECTION','SAFETY_DECLINE','TYRE_READING');
CREATE INDEX idx_fleet_asset_defects_scope ON fleet_partner_asset_defects (organization_id, asset_id, status, severity);

CREATE TABLE fleet_partner_asset_defect_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    defect_id       text NOT NULL REFERENCES fleet_partner_asset_defects (id) ON DELETE RESTRICT,
    previous_status text NOT NULL,
    new_status      text NOT NULL,
    actor_id        text NOT NULL,
    reason          text NOT NULL DEFAULT '',
    version         bigint NOT NULL,
    occurred_at     timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_asset_defect_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_asset_defect_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Tyres: positions, install/remove/rotate and tread readings. At most one
-- installed tyre per asset position.
-- ---------------------------------------------------------------------------
CREATE TABLE fleet_partner_tyres (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id           text NOT NULL REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    serial_number       text NOT NULL CHECK (length(serial_number) BETWEEN 1 AND 120),
    brand               text NOT NULL DEFAULT '',
    size                text NOT NULL DEFAULT '',
    status              text NOT NULL DEFAULT 'IN_STOCK' CHECK (status IN ('IN_STOCK','INSTALLED','REMOVED','SCRAPPED')),
    asset_id            text REFERENCES fleet_partner_assets (id) ON DELETE RESTRICT,
    position            text NOT NULL DEFAULT '',
    installed_at        timestamptz,
    last_tread_depth_mm numeric(6,2) CHECK (last_tread_depth_mm IS NULL OR last_tread_depth_mm >= 0),
    min_tread_depth_mm  numeric(6,2) NOT NULL DEFAULT 1.6 CHECK (min_tread_depth_mm >= 0),
    version             bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, serial_number),
    CHECK ((status = 'INSTALLED') = (asset_id IS NOT NULL AND position <> ''))
);
CREATE UNIQUE INDEX uq_fleet_tyre_installed_position ON fleet_partner_tyres (asset_id, position) WHERE status = 'INSTALLED';
CREATE INDEX idx_fleet_tyres_scope ON fleet_partner_tyres (organization_id, branch_id, status);

CREATE TABLE fleet_partner_tyre_events (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    tyre_id         text NOT NULL REFERENCES fleet_partner_tyres (id) ON DELETE RESTRICT,
    event_type      text NOT NULL CHECK (event_type IN ('REGISTERED','INSTALLED','REMOVED','ROTATED','TREAD_READING','SCRAPPED')),
    asset_id        text NOT NULL DEFAULT '',
    from_position   text NOT NULL DEFAULT '',
    to_position     text NOT NULL DEFAULT '',
    tread_depth_mm  numeric(6,2),
    odometer        numeric(18,3),
    reason          text NOT NULL DEFAULT '',
    actor_id        text NOT NULL,
    occurred_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fleet_tyre_events_tyre ON fleet_partner_tyre_events (organization_id, tyre_id, occurred_at);
CREATE TRIGGER trg_fleet_tyre_events_append_only BEFORE UPDATE OR DELETE ON fleet_partner_tyre_events
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Asset operating-cost history (maintenance, fuel, parts, tyres) in integer
-- minor units with an ISO currency. Organization data only; never a wallet
-- or ledger record.
-- ---------------------------------------------------------------------------
CREATE TABLE fleet_partner_asset_cost_entries (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    asset_id         text NOT NULL REFERENCES fleet_partner_assets (id) ON DELETE RESTRICT,
    cost_type        text NOT NULL CHECK (cost_type IN ('MAINTENANCE','FUEL','PARTS','TYRES','OTHER')),
    source_type      text NOT NULL CHECK (source_type IN ('WORK_ORDER','FUEL_RECORD','STOCK_MOVEMENT','TYRE','MANUAL')),
    source_reference text NOT NULL DEFAULT '',
    amount_minor     bigint NOT NULL CHECK (amount_minor > 0),
    currency         text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    incurred_at      timestamptz NOT NULL,
    description      text NOT NULL DEFAULT '',
    recorded_by      text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (source_type = 'MANUAL' OR source_reference <> '')
);
CREATE INDEX idx_fleet_asset_costs_asset ON fleet_partner_asset_cost_entries (organization_id, asset_id, incurred_at DESC);
CREATE TRIGGER trg_fleet_asset_cost_entries_append_only BEFORE UPDATE OR DELETE ON fleet_partner_asset_cost_entries
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Permissions.
-- ---------------------------------------------------------------------------
INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('rescue.read','dispatch','Read operational rescue cases',true),
('rescue.manage','dispatch','Open, coordinate replacement for and resolve rescue cases',true),
('assets.costs.read','assets','Read asset operating-cost history',true),
('assets.costs.manage','assets','Record asset operating costs',true)
ON CONFLICT(code) DO UPDATE SET category=EXCLUDED.category,description=EXCLUDED.description,is_active=true;

INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r CROSS JOIN (VALUES ('rescue.read'),('rescue.manage')) AS p(code)
WHERE r.id IN ('rol_system_owner','rol_system_operations','rol_system_dispatcher') ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r CROSS JOIN (VALUES ('assets.costs.read'),('assets.costs.manage')) AS p(code)
WHERE r.id IN ('rol_system_owner','rol_system_operations') ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
VALUES ('rol_system_finance','assets.costs.read') ON CONFLICT DO NOTHING;
