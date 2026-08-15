-- Sections 10/11: complete the setup/cleanup buffer hierarchy.
--
-- A reservation snapshots its effective allocation window.  These profile
-- values therefore affect only reservations made after a configuration
-- change; existing operational evidence is never rewritten.
ALTER TABLE production_lines
    ADD COLUMN IF NOT EXISTS setup_buffer_minutes integer NOT NULL DEFAULT 0
        CHECK (setup_buffer_minutes BETWEEN 0 AND 480),
    ADD COLUMN IF NOT EXISTS cleanup_buffer_minutes integer NOT NULL DEFAULT 0
        CHECK (cleanup_buffer_minutes BETWEEN 0 AND 480);

ALTER TABLE production_slots
    ADD COLUMN IF NOT EXISTS archived_at timestamptz;

ALTER TABLE cloud_kitchen_packages
    ADD COLUMN IF NOT EXISTS setup_buffer_minutes integer NOT NULL DEFAULT 0
        CHECK (setup_buffer_minutes BETWEEN 0 AND 480),
    ADD COLUMN IF NOT EXISTS cleanup_buffer_minutes integer NOT NULL DEFAULT 0
        CHECK (cleanup_buffer_minutes BETWEEN 0 AND 480);

ALTER TABLE pastry_products
    ADD COLUMN IF NOT EXISTS setup_buffer_minutes integer NOT NULL DEFAULT 0
        CHECK (setup_buffer_minutes BETWEEN 0 AND 480),
    ADD COLUMN IF NOT EXISTS cleanup_buffer_minutes integer NOT NULL DEFAULT 0
        CHECK (cleanup_buffer_minutes BETWEEN 0 AND 480),
    ADD COLUMN IF NOT EXISTS workload_units_per_item integer NOT NULL DEFAULT 1
        CHECK (workload_units_per_item BETWEEN 1 AND 1000);

-- Standard pastry reservations need an explicit source vocabulary so the
-- engine can resolve the immutable product profile that supplied the buffer.
ALTER TABLE capacity_reservations
    DROP CONSTRAINT IF EXISTS capacity_reservations_source_type_check;
ALTER TABLE capacity_reservations
    ADD CONSTRAINT capacity_reservations_source_type_check
    CHECK (source_type IN ('MANUAL','BULK_PACKAGE','CORPORATE_OCCURRENCE',
                           'PASTRY_PRODUCT','CUSTOM_CAKE_QUOTE','PRODUCTION_JOB','EXTERNAL_ORDER'));

ALTER TABLE production_jobs
    DROP CONSTRAINT IF EXISTS production_jobs_source_type_check;
ALTER TABLE production_jobs
    ADD CONSTRAINT production_jobs_source_type_check
    CHECK (source_type IN ('MANUAL','BULK_PACKAGE','CORPORATE_OCCURRENCE',
                           'PASTRY_PRODUCT','CUSTOM_CAKE_QUOTE','EXTERNAL_ORDER'));

ALTER TABLE capacity_reservation_allocations
    ADD COLUMN IF NOT EXISTS production_line_id text;
ALTER TABLE capacity_reservation_allocations
    DROP CONSTRAINT IF EXISTS capacity_reservation_allocations_line_fk;
ALTER TABLE capacity_reservation_allocations
    ADD CONSTRAINT capacity_reservation_allocations_line_fk
    FOREIGN KEY (production_line_id, business_id)
    REFERENCES production_lines (id, business_id);
CREATE INDEX IF NOT EXISTS capacity_reservation_allocations_line_window_idx
    ON capacity_reservation_allocations (production_line_id, effective_start, effective_end)
    WHERE production_line_id IS NOT NULL;

ALTER TABLE cloud_kitchen_quantity_bands
    ADD COLUMN IF NOT EXISTS capacity_units_per_item integer NOT NULL DEFAULT 1
        CHECK (capacity_units_per_item BETWEEN 1 AND 1000);

CREATE TABLE IF NOT EXISTS cloud_kitchen_package_versions (
    id text PRIMARY KEY,
    package_id text NOT NULL REFERENCES cloud_kitchen_packages(id) ON DELETE CASCADE,
    version bigint NOT NULL CHECK (version > 0),
    definition jsonb NOT NULL,
    reason text NOT NULL CHECK (reason IN ('CREATED','CONFIG_UPDATED','COMPONENT_UPDATED','BAND_UPDATED','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (package_id, version)
);
CREATE INDEX IF NOT EXISTS cloud_kitchen_package_versions_lookup_idx
    ON cloud_kitchen_package_versions (package_id, version DESC);

INSERT INTO cloud_kitchen_package_versions(id,package_id,version,definition,reason)
SELECT 'ckpv_' || p.id || '_' || p.version,
       p.id,
       p.version,
       jsonb_build_object(
           'package', to_jsonb(p),
           'components', COALESCE((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.position,c.id)
                                    FROM cloud_kitchen_package_components c WHERE c.package_id=p.id),'[]'::jsonb),
           'bands', COALESCE((SELECT jsonb_agg(to_jsonb(b) ORDER BY b.min_quantity,b.id)
                              FROM cloud_kitchen_quantity_bands b WHERE b.package_id=p.id),'[]'::jsonb)
       ),
       'CONFIG_UPDATED'
FROM cloud_kitchen_packages p
ON CONFLICT (package_id,version) DO NOTHING;

ALTER TABLE cloud_kitchen_corporate_plans
    ADD COLUMN IF NOT EXISTS reservation_horizon_days integer
        CHECK (reservation_horizon_days BETWEEN 1 AND 366);

ALTER TABLE pastry_quotes
    ADD COLUMN IF NOT EXISTS production_start timestamptz,
    ADD COLUMN IF NOT EXISTS production_end timestamptz,
    ADD COLUMN IF NOT EXISTS approval_required boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS approved_by text,
    ADD COLUMN IF NOT EXISTS approved_at timestamptz,
    ADD COLUMN IF NOT EXISTS vendor_terms text NOT NULL DEFAULT '' CHECK (char_length(vendor_terms) <= 4000),
    ADD COLUMN IF NOT EXISTS negotiation_reason text NOT NULL DEFAULT '' CHECK (char_length(negotiation_reason) <= 80),
    ADD COLUMN IF NOT EXISTS negotiation_note text NOT NULL DEFAULT '' CHECK (char_length(negotiation_note) <= 1000),
    ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD CONSTRAINT pastry_quotes_production_window_check
        CHECK (production_start IS NULL OR production_end IS NULL OR production_end > production_start);
ALTER TABLE pastry_quotes DROP CONSTRAINT IF EXISTS pastry_quotes_status_check;
ALTER TABLE pastry_quotes ADD CONSTRAINT pastry_quotes_status_check
    CHECK (status IN ('DRAFT','PENDING_APPROVAL','APPROVED','SUPERSEDED','ACCEPTED','DECLINED','EXPIRED','CANCELLED'));

CREATE TABLE IF NOT EXISTS pastry_quote_capacity_requirements (
    quote_id text NOT NULL REFERENCES pastry_quotes(id) ON DELETE CASCADE,
    capacity_pool_id text NOT NULL,
    production_line_id text,
    quantity integer NOT NULL CHECK (quantity BETWEEN 1 AND 1000000),
    PRIMARY KEY (quote_id,capacity_pool_id),
    FOREIGN KEY (capacity_pool_id) REFERENCES production_capacity_pools(id),
    FOREIGN KEY (production_line_id) REFERENCES production_lines(id)
);

ALTER TABLE pastry_accepted_snapshots
    ADD COLUMN IF NOT EXISTS capacity_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS vendor_terms text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS accepted_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE pastry_custom_requests
    ADD COLUMN IF NOT EXISTS corporate_profile_ref text,
    ADD COLUMN IF NOT EXISTS change_policy jsonb NOT NULL DEFAULT '{}'::jsonb;
UPDATE pastry_custom_requests SET status='ACCEPTED_PENDING_ORDER' WHERE status='ACCEPTED';
ALTER TABLE pastry_custom_requests DROP CONSTRAINT IF EXISTS pastry_custom_requests_status_check;
ALTER TABLE pastry_custom_requests ADD CONSTRAINT pastry_custom_requests_status_check
    CHECK (status IN ('DRAFT','SUBMITTED','UNDER_REVIEW','QUOTE_PENDING','QUOTED','ACCEPTED_PENDING_ORDER','DECLINED','CANCELLED','EXPIRED','CONVERTED','COMPLETED'));

CREATE TABLE IF NOT EXISTS pastry_product_versions (
    id text PRIMARY KEY,
    product_id text NOT NULL REFERENCES pastry_products(id) ON DELETE CASCADE,
    version bigint NOT NULL CHECK (version > 0),
    definition jsonb NOT NULL,
    reason text NOT NULL CHECK (reason IN ('CREATED','CONFIG_UPDATED','OPTIONS_UPDATED','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id,version)
);
CREATE TABLE IF NOT EXISTS pastry_product_price_history (
    id text PRIMARY KEY,
    product_id text NOT NULL REFERENCES pastry_products(id) ON DELETE CASCADE,
    product_version bigint NOT NULL CHECK (product_version > 0),
    amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
    currency text NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id,product_version)
);
INSERT INTO pastry_product_versions(id,product_id,version,definition,reason)
SELECT 'ppv_'||p.id||'_'||p.version,p.id,p.version,jsonb_build_object(
    'product',to_jsonb(p),
    'option_groups',COALESCE((SELECT jsonb_agg(jsonb_build_object('attachment',to_jsonb(a),'group',to_jsonb(g),'options',COALESCE((SELECT jsonb_agg(to_jsonb(o) ORDER BY o.position,o.id) FROM pastry_options o WHERE o.group_id=g.id),'[]'::jsonb)) ORDER BY a.position,g.id) FROM pastry_product_option_groups a JOIN pastry_option_groups g ON g.id=a.group_id WHERE a.product_id=p.id),'[]'::jsonb)
),'CONFIG_UPDATED' FROM pastry_products p ON CONFLICT(product_id,version) DO NOTHING;
INSERT INTO pastry_product_price_history(id,product_id,product_version,amount_minor,currency)
SELECT 'pph_'||p.id||'_'||p.version,p.id,p.version,p.base_price_minor,p.currency
FROM pastry_products p ON CONFLICT(product_id,product_version) DO NOTHING;
