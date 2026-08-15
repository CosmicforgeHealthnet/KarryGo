-- Sections 10/11 final integrity and durable operational alert evidence.

ALTER TABLE cloud_kitchen_packages
    ADD CONSTRAINT cloud_kitchen_packages_id_business_uq UNIQUE (id,business_id),
    ADD CONSTRAINT cloud_kitchen_packages_branch_scope_fk
        FOREIGN KEY (branch_id,business_id) REFERENCES vendor_branches(id,business_id),
    ADD CONSTRAINT cloud_kitchen_packages_capacity_pool_scope_fk
        FOREIGN KEY (capacity_pool_id,business_id) REFERENCES production_capacity_pools(id,business_id);

ALTER TABLE cloud_kitchen_settings
    ADD CONSTRAINT cloud_kitchen_settings_branch_scope_fk
        FOREIGN KEY (branch_id,business_id) REFERENCES vendor_branches(id,business_id),
    ADD CONSTRAINT cloud_kitchen_settings_capacity_pool_scope_fk
        FOREIGN KEY (default_capacity_pool_ref,business_id) REFERENCES production_capacity_pools(id,business_id);

ALTER TABLE cloud_kitchen_corporate_plans
    ADD CONSTRAINT cloud_kitchen_corporate_plans_id_business_uq UNIQUE (id,business_id),
    ADD CONSTRAINT cloud_kitchen_corporate_plans_branch_scope_fk
        FOREIGN KEY (branch_id,business_id) REFERENCES vendor_branches(id,business_id),
    ADD CONSTRAINT cloud_kitchen_corporate_plans_package_scope_fk
        FOREIGN KEY (package_id,business_id) REFERENCES cloud_kitchen_packages(id,business_id);

ALTER TABLE pastry_products
    ADD CONSTRAINT pastry_products_id_business_uq UNIQUE (id,business_id),
    ADD CONSTRAINT pastry_products_branch_scope_fk
        FOREIGN KEY (branch_id,business_id) REFERENCES vendor_branches(id,business_id),
    ADD CONSTRAINT pastry_products_capacity_pool_scope_fk
        FOREIGN KEY (capacity_pool_id,business_id) REFERENCES production_capacity_pools(id,business_id);

ALTER TABLE pastry_custom_requests
    ADD CONSTRAINT pastry_custom_requests_id_business_uq UNIQUE (id,business_id),
    ADD CONSTRAINT pastry_custom_requests_branch_scope_fk
        FOREIGN KEY (branch_id,business_id) REFERENCES vendor_branches(id,business_id);

ALTER TABLE pastry_change_requests
    ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE pastry_options
    ADD CONSTRAINT pastry_options_capacity_class_check CHECK (
        capacity_class IS NULL OR capacity_class IN (
            'GENERAL','HOT','COLD','BAKERY','DECORATION','PACKING','CHILLING'
        )
    );

ALTER TABLE production_jobs
    ADD COLUMN IF NOT EXISTS source_version bigint CHECK (source_version IS NULL OR source_version > 0);

ALTER TABLE cloud_kitchen_occurrences
    ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1 CHECK (version > 0);

ALTER TABLE pastry_quotes
    ADD CONSTRAINT pastry_quotes_expiry_after_creation_check CHECK (expires_at > created_at);

ALTER TABLE cloud_kitchen_quantity_bands
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),
    ADD COLUMN IF NOT EXISTS archived_at timestamptz;

CREATE INDEX cloud_kitchen_quantity_bands_active_idx
    ON cloud_kitchen_quantity_bands (package_id,min_quantity)
    WHERE status='ACTIVE';

ALTER TABLE cloud_kitchen_package_components
    DROP CONSTRAINT IF EXISTS cloud_kitchen_package_components_package_id_item_ref_variant_ref_key,
    ADD CONSTRAINT cloud_kitchen_package_components_identity_uq
        UNIQUE NULLS NOT DISTINCT (package_id,item_ref,variant_ref);

CREATE TABLE production_operational_alerts (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    alert_type text NOT NULL CHECK (alert_type IN (
        'PRODUCTION_STARTING_SOON','LATE_PRODUCTION_JOB','CAPACITY_CONFLICT',
        'CUSTOM_CAKE_CHANGE_AWAITING_REVIEW'
    )),
    source_type text NOT NULL CHECK (source_type IN (
        'PRODUCTION_JOB','CORPORATE_OCCURRENCE','PASTRY_CHANGE_REQUEST'
    )),
    source_ref text NOT NULL,
    dedupe_key text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','ACKNOWLEDGED','RESOLVED')),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    detected_at timestamptz NOT NULL DEFAULT now(),
    acknowledged_at timestamptz,
    acknowledged_by text,
    resolved_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT production_operational_alerts_branch_scope_fk
        FOREIGN KEY (branch_id,business_id) REFERENCES vendor_branches(id,business_id),
    CONSTRAINT production_operational_alerts_status_time_check CHECK (
        (status <> 'ACKNOWLEDGED' OR acknowledged_at IS NOT NULL)
        AND (status <> 'RESOLVED' OR resolved_at IS NOT NULL)
    )
);

CREATE INDEX production_operational_alerts_queue_idx
    ON production_operational_alerts (business_id,branch_id,status,detected_at DESC);
