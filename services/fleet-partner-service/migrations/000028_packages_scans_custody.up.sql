-- Section 15 (WP14), slices A5-A6: packages, immutable state/scan history,
-- and physical chain of custody. Fleet owns these operational records; media
-- evidence remains an opaque media-file-service reference.

CREATE TABLE IF NOT EXISTS fleet_partner_packages (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
    shipment_id text NOT NULL REFERENCES fleet_partner_shipments(id) ON DELETE RESTRICT,
    package_code text NOT NULL,
    barcode_value text NOT NULL,
    qr_value text NOT NULL,
    package_type text NOT NULL DEFAULT 'PARCEL',
    length numeric(12,3) NOT NULL CHECK (length > 0),
    width numeric(12,3) NOT NULL CHECK (width > 0),
    height numeric(12,3) NOT NULL CHECK (height > 0),
    dimension_unit text NOT NULL CHECK (dimension_unit IN ('CM','M','IN')),
    weight numeric(12,3) NOT NULL CHECK (weight > 0),
    weight_unit text NOT NULL CHECK (weight_unit IN ('KG','G','LB')),
    declared_value_amount_minor bigint NOT NULL DEFAULT 0 CHECK (declared_value_amount_minor >= 0),
    declared_value_currency char(3) NOT NULL CHECK (declared_value_currency ~ '^[A-Z]{3}$'),
    handling_instructions text NOT NULL DEFAULT '',
    condition text NOT NULL DEFAULT 'GOOD' CHECK (condition IN ('GOOD','DAMAGED','UNKNOWN')),
    status text NOT NULL DEFAULT 'CREATED' CHECK (status IN
      ('CREATED','RECEIVED','SORTED','LOADED','IN_TRANSIT','OUT_FOR_DELIVERY','DELIVERED',
       'FAILED_ATTEMPT','RETURNING','RETURNED','DAMAGED','LOST','QUARANTINED')),
    version bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by text NOT NULL,
    updated_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    UNIQUE (organization_id, package_code),
    UNIQUE (organization_id, barcode_value),
    UNIQUE (organization_id, qr_value),
    UNIQUE (organization_id, id)
);
CREATE INDEX IF NOT EXISTS idx_fleet_packages_shipment
  ON fleet_partner_packages(organization_id, shipment_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_fleet_packages_status
  ON fleet_partner_packages(organization_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS fleet_partner_package_history (
    id text PRIMARY KEY,
    organization_id text NOT NULL,
    package_id text NOT NULL REFERENCES fleet_partner_packages(id) ON DELETE RESTRICT,
    previous_status text NOT NULL,
    new_status text NOT NULL,
    reason_code text NOT NULL DEFAULT '',
    safe_note text NOT NULL DEFAULT '',
    actor_type text NOT NULL,
    actor_reference text NOT NULL,
    source text NOT NULL,
    request_id text NOT NULL DEFAULT '',
    correlation_id text NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_fleet_package_history
  ON fleet_partner_package_history(organization_id, package_id, occurred_at, id);
CREATE TRIGGER trg_fleet_package_history_append_only
  BEFORE UPDATE OR DELETE ON fleet_partner_package_history
  FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE IF NOT EXISTS fleet_partner_package_scans (
    id text PRIMARY KEY,
    organization_id text NOT NULL,
    package_id text NOT NULL REFERENCES fleet_partner_packages(id) ON DELETE RESTRICT,
    scan_type text NOT NULL,
    barcode_value text NOT NULL DEFAULT '',
    qr_value text NOT NULL DEFAULT '',
    location_reference text NOT NULL DEFAULT '',
    device_reference text NOT NULL DEFAULT '',
    client_scan_id text NOT NULL,
    request_fingerprint text NOT NULL,
    disposition text NOT NULL CHECK (disposition IN ('APPLIED','REVIEW_REQUIRED')),
    actor_type text NOT NULL,
    actor_reference text NOT NULL,
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, client_scan_id)
);
CREATE INDEX IF NOT EXISTS idx_fleet_package_scans
  ON fleet_partner_package_scans(organization_id, package_id, occurred_at, received_at);
CREATE TRIGGER trg_fleet_package_scans_append_only
  BEFORE UPDATE OR DELETE ON fleet_partner_package_scans
  FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE IF NOT EXISTS fleet_partner_custody_events (
    id text PRIMARY KEY,
    organization_id text NOT NULL,
    package_id text NOT NULL REFERENCES fleet_partner_packages(id) ON DELETE RESTRICT,
    shipment_id text NOT NULL REFERENCES fleet_partner_shipments(id) ON DELETE RESTRICT,
    manifest_id text,
    from_actor_type text NOT NULL,
    from_actor_reference text NOT NULL,
    to_actor_type text NOT NULL,
    to_actor_reference text NOT NULL,
    from_location_reference text NOT NULL DEFAULT '',
    to_location_reference text NOT NULL DEFAULT '',
    condition text NOT NULL,
    evidence_media_id text NOT NULL DEFAULT '',
    correction_of_event_id text REFERENCES fleet_partner_custody_events(id) ON DELETE RESTRICT,
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    source text NOT NULL,
    correlation_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_fleet_custody_events
  ON fleet_partner_custody_events(organization_id, package_id, occurred_at, id);
CREATE TRIGGER trg_fleet_custody_events_append_only
  BEFORE UPDATE OR DELETE ON fleet_partner_custody_events
  FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code, category, description, is_active) VALUES
 ('packages.read','packages','Read packages, scans and history',true),
 ('packages.manage','packages','Create and transition packages',true),
 ('packages.scan','packages','Record package scans',true),
 ('custody.read','custody','Read package custody history',true),
 ('custody.record','custody','Record package handovers',true)
ON CONFLICT(code) DO UPDATE SET category=EXCLUDED.category, description=EXCLUDED.description, is_active=true;

INSERT INTO fleet_partner_role_permissions(role_id, permission_code)
SELECT 'rol_system_owner', code FROM fleet_partner_permissions
WHERE code IN ('packages.read','packages.manage','packages.scan','custody.read','custody.record')
ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id, permission_code)
SELECT 'rol_system_operations', code FROM fleet_partner_permissions
WHERE code IN ('packages.read','packages.manage','packages.scan','custody.read','custody.record')
ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id, permission_code)
SELECT 'rol_system_analyst', code FROM fleet_partner_permissions
WHERE code IN ('packages.read','custody.read') ON CONFLICT DO NOTHING;
