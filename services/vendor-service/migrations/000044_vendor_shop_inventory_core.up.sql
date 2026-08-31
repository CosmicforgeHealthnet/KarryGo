-- Section 12B: authoritative vendor-owned inventory, lots and reservations.
-- Quantities are integer stock-basis units. Approximate catalogue calibrations
-- are never accepted as reservation arithmetic.

CREATE TABLE shop_inventory_stock_bases (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text NOT NULL,
    listing_id text NOT NULL,
    product_id text NOT NULL,
    variant_id text NOT NULL,
    measure_definition_id text NOT NULL,
    measure_owner_key text NOT NULL,
    consumption_mode text NOT NULL CHECK (consumption_mode IN ('SAME_UNIT','EXACT_CONVERSION','VARIABLE_FULFILMENT')),
    lot_policy text NOT NULL DEFAULT 'FIFO' CHECK (lot_policy IN ('FIFO','FEFO')),
    lot_tracking_enabled boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_inventory_stock_bases_branch_fk FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches (id,business_id),
    CONSTRAINT shop_inventory_stock_bases_listing_fk FOREIGN KEY (listing_id,business_id)
        REFERENCES shop_branch_listings (id,business_id),
    CONSTRAINT shop_inventory_stock_bases_variant_fk FOREIGN KEY (variant_id,product_id)
        REFERENCES shop_variants (id,product_id),
    CONSTRAINT shop_inventory_stock_bases_measure_fk FOREIGN KEY (measure_definition_id,measure_owner_key)
        REFERENCES shop_measure_definitions (id,owner_key),
    CONSTRAINT shop_inventory_stock_bases_measure_owner_check CHECK (measure_owner_key IN ('PLATFORM',business_id)),
    UNIQUE (business_id,branch_id,variant_id),
    UNIQUE (id,business_id),
    UNIQUE (id,business_id,branch_id,variant_id)
);

CREATE INDEX shop_inventory_stock_bases_listing_idx ON shop_inventory_stock_bases (business_id,listing_id,status);

CREATE TABLE shop_inventory_balances (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    stock_basis_id text NOT NULL,
    variant_id text NOT NULL,
    on_hand_quantity bigint NOT NULL DEFAULT 0 CHECK (on_hand_quantity >= 0),
    reserved_quantity bigint NOT NULL DEFAULT 0 CHECK (reserved_quantity >= 0),
    quarantined_quantity bigint NOT NULL DEFAULT 0 CHECK (quarantined_quantity >= 0),
    in_transit_quantity bigint NOT NULL DEFAULT 0 CHECK (in_transit_quantity >= 0),
    backorder_quantity bigint NOT NULL DEFAULT 0 CHECK (backorder_quantity >= 0),
    revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_inventory_balances_basis_fk FOREIGN KEY (stock_basis_id,business_id,branch_id,variant_id)
        REFERENCES shop_inventory_stock_bases (id,business_id,branch_id,variant_id),
    CONSTRAINT shop_inventory_balances_physical_check CHECK (
        reserved_quantity + quarantined_quantity <= on_hand_quantity),
    UNIQUE (business_id,branch_id,variant_id),
    UNIQUE (stock_basis_id,business_id)
);

CREATE INDEX shop_inventory_balances_hot_idx ON shop_inventory_balances (business_id,branch_id,variant_id);

CREATE TABLE shop_inventory_revisions (
    business_id text NOT NULL,
    branch_id text NOT NULL,
    revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id,branch_id),
    CONSTRAINT shop_inventory_revisions_branch_fk FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches (id,business_id)
);

CREATE TABLE shop_inventory_lots (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    stock_basis_id text NOT NULL,
    variant_id text NOT NULL,
    lot_number text,
    batch_number text,
    source_ref text,
    received_at timestamptz NOT NULL,
    manufactured_at timestamptz,
    best_before_at timestamptz,
    expires_at timestamptz,
    on_hand_quantity bigint NOT NULL CHECK (on_hand_quantity >= 0),
    reserved_quantity bigint NOT NULL DEFAULT 0 CHECK (reserved_quantity >= 0),
    quarantined_quantity bigint NOT NULL DEFAULT 0 CHECK (quarantined_quantity >= 0),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','DEPLETED','QUARANTINED','EXPIRED','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_inventory_lots_basis_fk FOREIGN KEY (stock_basis_id,business_id,branch_id,variant_id)
        REFERENCES shop_inventory_stock_bases (id,business_id,branch_id,variant_id),
    CONSTRAINT shop_inventory_lots_quantity_check CHECK (reserved_quantity + quarantined_quantity <= on_hand_quantity),
    CONSTRAINT shop_inventory_lots_dates_check CHECK (
        (manufactured_at IS NULL OR manufactured_at <= received_at) AND
        (best_before_at IS NULL OR manufactured_at IS NULL OR best_before_at >= manufactured_at) AND
        (expires_at IS NULL OR expires_at >= received_at)),
    UNIQUE (id,business_id),
    UNIQUE (id,business_id,stock_basis_id)
);

CREATE INDEX shop_inventory_lots_allocation_idx
    ON shop_inventory_lots (business_id,stock_basis_id,status,expires_at,received_at,id)
    WHERE on_hand_quantity > 0;
CREATE INDEX shop_inventory_lots_expiry_idx ON shop_inventory_lots (business_id,branch_id,expires_at)
    WHERE expires_at IS NOT NULL AND status='ACTIVE';

CREATE TABLE shop_inventory_receipts (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    source_ref text,
    supplier_ref text,
    notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 2000),
    received_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','POSTED','CANCELLED')),
    posted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_inventory_receipts_branch_fk FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches (id,business_id),
    UNIQUE (id,business_id)
);

CREATE INDEX shop_inventory_receipts_branch_idx ON shop_inventory_receipts (business_id,branch_id,status,received_at DESC);

CREATE TABLE shop_inventory_receipt_lines (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    receipt_id text NOT NULL,
    stock_basis_id text NOT NULL,
    variant_id text NOT NULL,
    quantity bigint NOT NULL CHECK (quantity > 0),
    lot_number text,
    batch_number text,
    manufactured_at timestamptz,
    best_before_at timestamptz,
    expires_at timestamptz,
    unit_cost_minor bigint CHECK (unit_cost_minor IS NULL OR unit_cost_minor >= 0),
    lot_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_inventory_receipt_lines_receipt_fk FOREIGN KEY (receipt_id,business_id)
        REFERENCES shop_inventory_receipts (id,business_id) ON DELETE CASCADE,
    CONSTRAINT shop_inventory_receipt_lines_basis_fk FOREIGN KEY (stock_basis_id,business_id)
        REFERENCES shop_inventory_balances (stock_basis_id,business_id),
    CONSTRAINT shop_inventory_receipt_lines_lot_fk FOREIGN KEY (lot_id,business_id)
        REFERENCES shop_inventory_lots (id,business_id),
    UNIQUE (receipt_id,id)
);

CREATE TABLE shop_inventory_reservations (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    source_type text NOT NULL CHECK (source_type IN ('ORDER','RFQ','MANUAL','INTERNAL_TEST','OTHER_CONTROLLED')),
    source_ref text NOT NULL CHECK (length(source_ref) BETWEEN 1 AND 200),
    request_fingerprint text NOT NULL,
    status text NOT NULL DEFAULT 'HELD' CHECK (status IN ('HELD','CONFIRMED','COMMITTED','RELEASED','CANCELLED','EXPIRED')),
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    committed_at timestamptz,
    released_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (business_id,source_type,source_ref),
    UNIQUE (id,business_id)
);

CREATE INDEX shop_inventory_reservations_expiry_idx ON shop_inventory_reservations (expires_at,id)
    WHERE status='HELD' AND expires_at IS NOT NULL;

CREATE TABLE shop_inventory_reservation_lines (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    reservation_id text NOT NULL,
    branch_id text NOT NULL,
    stock_basis_id text NOT NULL,
    listing_id text NOT NULL,
    variant_id text NOT NULL,
    requested_quantity bigint NOT NULL CHECK (requested_quantity > 0),
    physical_quantity bigint NOT NULL CHECK (physical_quantity >= 0),
    backorder_quantity bigint NOT NULL DEFAULT 0 CHECK (backorder_quantity >= 0),
    consumption_mode text NOT NULL CHECK (consumption_mode IN ('SAME_UNIT','EXACT_CONVERSION','VARIABLE_FULFILMENT')),
    status text NOT NULL DEFAULT 'HELD' CHECK (status IN ('HELD','COMMITTED','RELEASED','EXPIRED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_inventory_reservation_lines_reservation_fk FOREIGN KEY (reservation_id,business_id)
        REFERENCES shop_inventory_reservations (id,business_id) ON DELETE CASCADE,
    CONSTRAINT shop_inventory_reservation_lines_basis_fk FOREIGN KEY (stock_basis_id,business_id,branch_id,variant_id)
        REFERENCES shop_inventory_stock_bases (id,business_id,branch_id,variant_id),
    CONSTRAINT shop_inventory_reservation_lines_listing_fk FOREIGN KEY (listing_id,business_id)
        REFERENCES shop_branch_listings (id,business_id),
    CONSTRAINT shop_inventory_reservation_lines_total_check CHECK (physical_quantity + backorder_quantity = requested_quantity),
    UNIQUE (reservation_id,branch_id,variant_id),
    UNIQUE (id,business_id)
);

CREATE TABLE shop_inventory_reservation_allocations (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    reservation_line_id text NOT NULL,
    lot_id text NOT NULL,
    quantity bigint NOT NULL CHECK (quantity > 0),
    status text NOT NULL DEFAULT 'HELD' CHECK (status IN ('HELD','COMMITTED','RELEASED','EXPIRED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_inventory_reservation_allocations_line_fk FOREIGN KEY (reservation_line_id,business_id)
        REFERENCES shop_inventory_reservation_lines (id,business_id) ON DELETE CASCADE,
    CONSTRAINT shop_inventory_reservation_allocations_lot_fk FOREIGN KEY (lot_id,business_id)
        REFERENCES shop_inventory_lots (id,business_id),
    UNIQUE (reservation_line_id,lot_id)
);

CREATE TABLE shop_inventory_movements (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    stock_basis_id text NOT NULL,
    variant_id text NOT NULL,
    lot_id text,
    movement_type text NOT NULL CHECK (movement_type IN (
        'RECEIPT','RESERVATION','RESERVATION_RELEASE','SALE_COMMIT','ADJUSTMENT_IN','ADJUSTMENT_OUT',
        'TRANSFER_OUT','TRANSFER_IN','RETURN','DAMAGE','WASTE','EXPIRY','QUARANTINE_IN','QUARANTINE_RELEASE',
        'STOCKTAKE_CORRECTION','PREORDER_ALLOCATION')),
    quantity bigint NOT NULL CHECK (quantity > 0),
    on_hand_delta bigint NOT NULL DEFAULT 0,
    reserved_delta bigint NOT NULL DEFAULT 0,
    quarantined_delta bigint NOT NULL DEFAULT 0,
    in_transit_delta bigint NOT NULL DEFAULT 0,
    source_type text NOT NULL,
    source_ref text NOT NULL,
    reason_code text,
    actor_ref text,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    inventory_revision bigint NOT NULL CHECK (inventory_revision > 0),
    CONSTRAINT shop_inventory_movements_basis_fk FOREIGN KEY (stock_basis_id,business_id,branch_id,variant_id)
        REFERENCES shop_inventory_stock_bases (id,business_id,branch_id,variant_id),
    CONSTRAINT shop_inventory_movements_lot_fk FOREIGN KEY (lot_id,business_id,stock_basis_id)
        REFERENCES shop_inventory_lots (id,business_id,stock_basis_id),
    CONSTRAINT shop_inventory_movements_effect_check CHECK (
        on_hand_delta <> 0 OR reserved_delta <> 0 OR quarantined_delta <> 0 OR in_transit_delta <> 0)
);

CREATE INDEX shop_inventory_movements_history_idx ON shop_inventory_movements (business_id,branch_id,occurred_at DESC,id);
CREATE INDEX shop_inventory_movements_variant_idx ON shop_inventory_movements (business_id,branch_id,variant_id,occurred_at DESC);
CREATE INDEX shop_inventory_movements_source_idx ON shop_inventory_movements (business_id,source_type,source_ref);

CREATE FUNCTION shop_inventory_movements_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'shop inventory movement ledger is append-only' USING ERRCODE='55000';
END $$;
CREATE TRIGGER shop_inventory_movements_immutable_trigger
    BEFORE UPDATE OR DELETE ON shop_inventory_movements
    FOR EACH ROW EXECUTE FUNCTION shop_inventory_movements_immutable();

