-- Section 12B stock operations, policies, variable fulfilment, alerts and
-- bounded bulk jobs. No Order, payment, logistics-execution or dark-store
-- authority is introduced here.

CREATE TABLE shop_inventory_policies (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    listing_id text NOT NULL,
    variant_id text NOT NULL,
    mode text NOT NULL DEFAULT 'NONE' CHECK (mode IN ('NONE','PREORDER','BACKORDER')),
    maximum_outstanding_quantity bigint CHECK (maximum_outstanding_quantity IS NULL OR maximum_outstanding_quantity > 0),
    maximum_per_request bigint CHECK (maximum_per_request IS NULL OR maximum_per_request > 0),
    available_from timestamptz,
    expected_restock_at timestamptz,
    lead_time_days integer CHECK (lead_time_days IS NULL OR lead_time_days BETWEEN 0 AND 365),
    cutoff_at timestamptz,
    allocation_policy text NOT NULL DEFAULT 'FIFO_BY_CREATED_AT'
        CHECK (allocation_policy IN ('FIFO_BY_CREATED_AT','PRIORITY_THEN_FIFO')),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_inventory_policies_listing_fk FOREIGN KEY (listing_id,business_id)
        REFERENCES shop_branch_listings (id,business_id),
    CONSTRAINT shop_inventory_policies_branch_fk FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches (id,business_id),
    CONSTRAINT shop_inventory_policies_variant_listing_fk FOREIGN KEY (branch_id,variant_id)
        REFERENCES shop_branch_listings (branch_id,variant_id),
    UNIQUE (business_id,branch_id,variant_id),
    UNIQUE (id,business_id)
);
CREATE INDEX shop_inventory_policies_active_idx ON shop_inventory_policies (business_id,branch_id,mode,status);

CREATE TABLE shop_inventory_adjustments (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    stock_basis_id text NOT NULL,
    variant_id text NOT NULL,
    quantity_delta bigint NOT NULL CHECK (quantity_delta <> 0),
    reason_code text NOT NULL CHECK (reason_code IN ('FOUND_STOCK','DATA_CORRECTION','DAMAGE','WASTE','SPOILAGE','THEFT_LOSS','EXPIRY','MANUAL_CORRECTION','OTHER_CONTROLLED')),
    note text NOT NULL DEFAULT '' CHECK (length(note) <= 1000),
    expected_revision bigint NOT NULL CHECK (expected_revision >= 0),
    status text NOT NULL DEFAULT 'POSTED' CHECK (status IN ('POSTED','REVERSED')),
    created_by text,
    created_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_inventory_adjustments_basis_fk FOREIGN KEY (stock_basis_id,business_id,branch_id,variant_id)
        REFERENCES shop_inventory_stock_bases (id,business_id,branch_id,variant_id),
    UNIQUE (id,business_id)
);
CREATE INDEX shop_inventory_adjustments_history_idx ON shop_inventory_adjustments (business_id,branch_id,created_at DESC);

CREATE TABLE shop_inventory_quarantines (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    stock_basis_id text NOT NULL,
    variant_id text NOT NULL,
    lot_id text,
    quantity bigint NOT NULL CHECK (quantity > 0),
    reason_code text NOT NULL CHECK (reason_code IN ('QUALITY_CHECK','DAMAGED','RECALL','TEMPERATURE_EXCURSION','SUSPECTED_SPOILAGE','OTHER_CONTROLLED')),
    note text NOT NULL DEFAULT '' CHECK (length(note) <= 1000),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','RELEASED','DISPOSED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_inventory_quarantines_basis_fk FOREIGN KEY (stock_basis_id,business_id,branch_id,variant_id)
        REFERENCES shop_inventory_stock_bases (id,business_id,branch_id,variant_id),
    CONSTRAINT shop_inventory_quarantines_lot_fk FOREIGN KEY (lot_id,business_id,stock_basis_id)
        REFERENCES shop_inventory_lots (id,business_id,stock_basis_id),
    UNIQUE (id,business_id)
);
CREATE INDEX shop_inventory_quarantines_active_idx ON shop_inventory_quarantines (business_id,branch_id,status,created_at);

CREATE TABLE shop_inventory_quarantine_allocations (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    quarantine_id text NOT NULL,
    lot_id text NOT NULL,
    quantity bigint NOT NULL CHECK (quantity > 0),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','RELEASED','DISPOSED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_inventory_quarantine_allocations_quarantine_fk FOREIGN KEY (quarantine_id,business_id)
        REFERENCES shop_inventory_quarantines (id,business_id) ON DELETE CASCADE,
    CONSTRAINT shop_inventory_quarantine_allocations_lot_fk FOREIGN KEY (lot_id,business_id)
        REFERENCES shop_inventory_lots (id,business_id),
    UNIQUE (quarantine_id,lot_id)
);

CREATE TABLE shop_inventory_transfers (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    source_branch_id text NOT NULL,
    destination_branch_id text NOT NULL,
    status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','APPROVED','IN_TRANSIT','PARTIALLY_RECEIVED','RECEIVED','CANCELLED')),
    notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 2000),
    dispatched_at timestamptz,
    received_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_inventory_transfers_source_fk FOREIGN KEY (source_branch_id,business_id)
        REFERENCES vendor_branches (id,business_id),
    CONSTRAINT shop_inventory_transfers_destination_fk FOREIGN KEY (destination_branch_id,business_id)
        REFERENCES vendor_branches (id,business_id),
    CONSTRAINT shop_inventory_transfers_distinct_branch CHECK (source_branch_id <> destination_branch_id),
    UNIQUE (id,business_id)
);
CREATE INDEX shop_inventory_transfers_branch_idx ON shop_inventory_transfers (business_id,source_branch_id,destination_branch_id,status,created_at DESC);

CREATE TABLE shop_inventory_transfer_lines (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    transfer_id text NOT NULL,
    source_stock_basis_id text NOT NULL,
    destination_stock_basis_id text NOT NULL,
    variant_id text NOT NULL,
    sent_quantity bigint NOT NULL CHECK (sent_quantity > 0),
    received_quantity bigint NOT NULL DEFAULT 0 CHECK (received_quantity >= 0),
    damaged_quantity bigint NOT NULL DEFAULT 0 CHECK (damaged_quantity >= 0),
    missing_quantity bigint NOT NULL DEFAULT 0 CHECK (missing_quantity >= 0),
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','IN_TRANSIT','PARTIALLY_RECEIVED','RECEIVED','CANCELLED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_inventory_transfer_lines_transfer_fk FOREIGN KEY (transfer_id,business_id)
        REFERENCES shop_inventory_transfers (id,business_id) ON DELETE CASCADE,
    CONSTRAINT shop_inventory_transfer_lines_source_basis_fk FOREIGN KEY (source_stock_basis_id,business_id)
        REFERENCES shop_inventory_balances (stock_basis_id,business_id),
    CONSTRAINT shop_inventory_transfer_lines_destination_basis_fk FOREIGN KEY (destination_stock_basis_id,business_id)
        REFERENCES shop_inventory_balances (stock_basis_id,business_id),
    CONSTRAINT shop_inventory_transfer_lines_received_check CHECK (
        received_quantity + damaged_quantity + missing_quantity <= sent_quantity),
    UNIQUE (transfer_id,variant_id),
    UNIQUE (id,business_id)
);

CREATE TABLE shop_inventory_stocktakes (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    stocktake_type text NOT NULL CHECK (stocktake_type IN ('FULL_STOCKTAKE','CYCLE_COUNT')),
    snapshot_revision bigint NOT NULL CHECK (snapshot_revision >= 0),
    status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','COUNTING','READY','POSTED','CANCELLED','STALE')),
    started_at timestamptz NOT NULL DEFAULT now(),
    posted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_inventory_stocktakes_branch_fk FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches (id,business_id),
    UNIQUE (id,business_id)
);
CREATE UNIQUE INDEX shop_inventory_stocktakes_active_uq ON shop_inventory_stocktakes (business_id,branch_id,stocktake_type)
    WHERE status IN ('DRAFT','COUNTING','READY');

CREATE TABLE shop_inventory_stocktake_lines (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    stocktake_id text NOT NULL,
    stock_basis_id text NOT NULL,
    variant_id text NOT NULL,
    expected_quantity bigint NOT NULL CHECK (expected_quantity >= 0),
    counted_quantity bigint CHECK (counted_quantity IS NULL OR counted_quantity >= 0),
    variance_quantity bigint,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_inventory_stocktake_lines_stocktake_fk FOREIGN KEY (stocktake_id,business_id)
        REFERENCES shop_inventory_stocktakes (id,business_id) ON DELETE CASCADE,
    CONSTRAINT shop_inventory_stocktake_lines_basis_fk FOREIGN KEY (stock_basis_id,business_id)
        REFERENCES shop_inventory_balances (stock_basis_id,business_id),
    UNIQUE (stocktake_id,stock_basis_id),
    UNIQUE (id,business_id)
);

CREATE TABLE shop_inventory_reorder_rules (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    stock_basis_id text NOT NULL,
    variant_id text NOT NULL,
    minimum_quantity bigint NOT NULL CHECK (minimum_quantity >= 0),
    target_quantity bigint NOT NULL CHECK (target_quantity > minimum_quantity),
    expiring_soon_hours integer CHECK (expiring_soon_hours IS NULL OR expiring_soon_hours BETWEEN 1 AND 8760),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_inventory_reorder_rules_basis_fk FOREIGN KEY (stock_basis_id,business_id,branch_id,variant_id)
        REFERENCES shop_inventory_stock_bases (id,business_id,branch_id,variant_id),
    UNIQUE (business_id,branch_id,variant_id),
    UNIQUE (id,business_id)
);

CREATE TABLE shop_inventory_alerts (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    stock_basis_id text,
    alert_type text NOT NULL CHECK (alert_type IN ('LOW_STOCK','OUT_OF_STOCK','EXPIRING_SOON','EXPIRED','RESERVATION_CONFLICT','QUARANTINED','TRANSFER_DISCREPANCY','STOCKTAKE_VARIANCE','BACKORDER_OUTSTANDING')),
    severity text NOT NULL DEFAULT 'WARNING' CHECK (severity IN ('INFO','WARNING','CRITICAL')),
    dedupe_key text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','ACKNOWLEDGED','RESOLVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_inventory_alerts_branch_fk FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches (id,business_id),
    UNIQUE (business_id,dedupe_key,status)
);
CREATE INDEX shop_inventory_alerts_open_idx ON shop_inventory_alerts (business_id,branch_id,status,created_at DESC);

CREATE TABLE shop_fresh_fulfilment_measurements (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    reservation_line_id text NOT NULL,
    requested_quantity bigint NOT NULL CHECK (requested_quantity > 0),
    requested_measure_id text NOT NULL,
    actual_quantity bigint NOT NULL CHECK (actual_quantity > 0),
    actual_measure_id text NOT NULL,
    tolerance_result text NOT NULL CHECK (tolerance_result IN ('WITHIN_TOLERANCE','UNDER_TOLERANCE','OVER_TOLERANCE','REQUIRES_REVIEW')),
    measured_by text,
    measured_at timestamptz NOT NULL,
    source_ref text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT shop_fresh_fulfilment_line_fk FOREIGN KEY (reservation_line_id,business_id)
        REFERENCES shop_inventory_reservation_lines (id,business_id),
    CONSTRAINT shop_fresh_fulfilment_branch_fk FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches (id,business_id),
    UNIQUE (business_id,source_ref),
    UNIQUE (id,business_id)
);

CREATE TABLE shop_bulk_operation_jobs (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text,
    operation_type text NOT NULL CHECK (operation_type IN ('CATALOGUE_IMPORT','INITIAL_BALANCE','RECEIPT','STOCK_ADJUSTMENT','PRICE_UPDATE','LISTING_UPDATE')),
    dry_run boolean NOT NULL DEFAULT true,
    request_fingerprint text NOT NULL,
    expected_catalogue_revision bigint,
    expected_inventory_revision bigint,
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','VALIDATED','APPLYING','COMPLETED','FAILED','DEAD_LETTER')),
    row_count integer NOT NULL CHECK (row_count BETWEEN 1 AND 1000),
    error_count integer NOT NULL DEFAULT 0 CHECK (error_count >= 0),
    lease_owner text,
    lease_expires_at timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 20),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    CONSTRAINT shop_bulk_operation_jobs_branch_fk FOREIGN KEY (branch_id,business_id)
        REFERENCES vendor_branches (id,business_id),
    UNIQUE (business_id,operation_type,request_fingerprint),
    UNIQUE (id,business_id)
);
CREATE INDEX shop_bulk_operation_jobs_worker_idx ON shop_bulk_operation_jobs (status,lease_expires_at,created_at);

CREATE TABLE shop_bulk_operation_rows (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    job_id text NOT NULL,
    row_number integer NOT NULL CHECK (row_number > 0),
    payload jsonb NOT NULL,
    validation_errors jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','VALID','APPLIED','ERROR')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_bulk_operation_rows_job_fk FOREIGN KEY (job_id,business_id)
        REFERENCES shop_bulk_operation_jobs (id,business_id) ON DELETE CASCADE,
    UNIQUE (job_id,row_number)
);
