-- Section 15A — Suppliers, Supplier Products & Purchasing Foundation.
--
-- Vendor Service now owns the Vendor-scoped supplier master and the supplier
-- catalogue used to procure stock into Section 14 Inventory.
--
-- This migration DOES NOT create:
--   purchase orders
--   PO approval workflow
--   goods receiving/posting
--   supplier invoices
--   payment authority
--   logistics authority
--
-- Those are later Section 15 phases.
--
-- SupplierProduct maps only to an existing same-business/same-branch generic
-- Inventory item. Purchase conversion is integer-exact into the authoritative
-- Inventory stock unit. Approximate/calibrated/float conversion is not stored.

CREATE TABLE vendor_supplier_categories (
    id text PRIMARY KEY,

    business_id text NOT NULL
        REFERENCES vendor_businesses (id),

    code text NOT NULL
        CHECK (
            char_length(code) BETWEEN 1 AND 40
            AND code = upper(code)
        ),

    name text NOT NULL
        CHECK (char_length(btrim(name)) BETWEEN 1 AND 160),

    description text NOT NULL DEFAULT ''
        CHECK (char_length(description) <= 1000),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE', 'ARCHIVED')),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    UNIQUE (business_id, code)
);

CREATE INDEX vendor_supplier_categories_business_status_idx
ON vendor_supplier_categories (
    business_id,
    status,
    name
);

CREATE TABLE vendor_suppliers (
    id text PRIMARY KEY,

    business_id text NOT NULL
        REFERENCES vendor_businesses (id),

    category_id text,

    code text NOT NULL DEFAULT ''
        CHECK (
            code = ''
            OR (
                char_length(code) BETWEEN 1 AND 80
                AND code = upper(code)
            )
        ),

    name text NOT NULL
        CHECK (char_length(btrim(name)) BETWEEN 1 AND 200),

    legal_name text NOT NULL DEFAULT ''
        CHECK (char_length(legal_name) <= 240),

    default_lead_time_hours integer NOT NULL DEFAULT 0
        CHECK (
            default_lead_time_hours >= 0
            AND default_lead_time_hours <= 8760
        ),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'INACTIVE',
                'SUSPENDED',
                'ARCHIVED'
            )
        ),

    notes text NOT NULL DEFAULT ''
        CHECK (char_length(notes) <= 4000),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_suppliers_category_fk
        FOREIGN KEY (
            category_id,
            business_id
        )
        REFERENCES vendor_supplier_categories (
            id,
            business_id
        ),

    UNIQUE (id, business_id)
);

CREATE UNIQUE INDEX vendor_suppliers_business_code_uq
ON vendor_suppliers (
    business_id,
    code
)
WHERE code <> '';

CREATE INDEX vendor_suppliers_business_status_idx
ON vendor_suppliers (
    business_id,
    status,
    name,
    id
);

CREATE TABLE vendor_supplier_contacts (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    supplier_id text NOT NULL,

    contact_role text NOT NULL DEFAULT 'OTHER'
        CHECK (
            contact_role IN (
                'PRIMARY',
                'SALES',
                'OPERATIONS',
                'FINANCE',
                'OTHER'
            )
        ),

    name text NOT NULL
        CHECK (char_length(btrim(name)) BETWEEN 1 AND 160),

    email text NOT NULL DEFAULT ''
        CHECK (char_length(email) <= 320),

    phone text NOT NULL DEFAULT ''
        CHECK (char_length(phone) <= 80),

    is_primary boolean NOT NULL DEFAULT false,

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE', 'ARCHIVED')),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_supplier_contacts_supplier_fk
        FOREIGN KEY (
            supplier_id,
            business_id
        )
        REFERENCES vendor_suppliers (
            id,
            business_id
        )
        ON DELETE CASCADE,

    UNIQUE (id, business_id, supplier_id)
);

CREATE UNIQUE INDEX vendor_supplier_contacts_one_primary_uq
ON vendor_supplier_contacts (
    business_id,
    supplier_id
)
WHERE is_primary
  AND status = 'ACTIVE';

CREATE INDEX vendor_supplier_contacts_supplier_idx
ON vendor_supplier_contacts (
    business_id,
    supplier_id,
    status,
    name
);

CREATE TABLE vendor_supplier_terms (
    supplier_id text NOT NULL,
    business_id text NOT NULL,

    payment_terms_days integer NOT NULL DEFAULT 0
        CHECK (
            payment_terms_days >= 0
            AND payment_terms_days <= 3650
        ),

    currency_code text NOT NULL
        CHECK (
            char_length(currency_code) = 3
            AND currency_code = upper(currency_code)
        ),

    delivery_terms text NOT NULL DEFAULT ''
        CHECK (char_length(delivery_terms) <= 2000),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (
        supplier_id,
        business_id
    ),

    CONSTRAINT vendor_supplier_terms_supplier_fk
        FOREIGN KEY (
            supplier_id,
            business_id
        )
        REFERENCES vendor_suppliers (
            id,
            business_id
        )
        ON DELETE CASCADE
);

CREATE TABLE vendor_supplier_products (
    id text PRIMARY KEY,

    business_id text NOT NULL,
    supplier_id text NOT NULL,

    branch_id text NOT NULL,
    inventory_item_id text NOT NULL,

    supplier_sku text NOT NULL DEFAULT ''
        CHECK (char_length(supplier_sku) <= 160),

    purchase_unit_code text NOT NULL
        CHECK (
            char_length(purchase_unit_code) BETWEEN 1 AND 80
            AND purchase_unit_code = upper(purchase_unit_code)
        ),

    purchase_unit_name text NOT NULL
        CHECK (
            char_length(btrim(purchase_unit_name))
            BETWEEN 1 AND 160
        ),

    -- Purchase quantity is converted into the existing Inventory item's
    -- authoritative stock quantity using integer arithmetic only.
    conversion_mode text NOT NULL
        CHECK (
            conversion_mode IN (
                'SAME_AS_INVENTORY',
                'EXACT_INTEGER_MULTIPLE'
            )
        ),

    inventory_units_per_purchase_unit bigint NOT NULL
        CHECK (inventory_units_per_purchase_unit > 0),

    minimum_order_quantity bigint NOT NULL DEFAULT 1
        CHECK (minimum_order_quantity > 0),

    purchase_multiple bigint NOT NULL DEFAULT 1
        CHECK (purchase_multiple > 0),

    typical_cost_minor bigint
        CHECK (
            typical_cost_minor IS NULL
            OR typical_cost_minor >= 0
        ),

    -- Read-only from the 15A management API. Later goods-receiving authority
    -- updates this from an actual posted receipt.
    last_cost_minor bigint
        CHECK (
            last_cost_minor IS NULL
            OR last_cost_minor >= 0
        ),

    currency_code text NOT NULL
        CHECK (
            char_length(currency_code) = 3
            AND currency_code = upper(currency_code)
        ),

    lead_time_hours integer
        CHECK (
            lead_time_hours IS NULL
            OR (
                lead_time_hours >= 0
                AND lead_time_hours <= 8760
            )
        ),

    -- Snapshot of the exact Inventory basis at mapping time. It is evidence,
    -- not a duplicate source of measurement truth.
    inventory_measure_ref text NOT NULL,
    inventory_measure_owner_key text NOT NULL,

    inventory_unit_kind text NOT NULL
        CHECK (
            inventory_unit_kind IN (
                'COUNT',
                'MASS',
                'VOLUME',
                'CONFIGURED_MEASURE'
            )
        ),

    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE', 'ARCHIVED')),

    version bigint NOT NULL DEFAULT 1
        CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendor_supplier_products_supplier_fk
        FOREIGN KEY (
            supplier_id,
            business_id
        )
        REFERENCES vendor_suppliers (
            id,
            business_id
        ),

    CONSTRAINT vendor_supplier_products_branch_fk
        FOREIGN KEY (
            branch_id,
            business_id
        )
        REFERENCES vendor_branches (
            id,
            business_id
        ),

    CONSTRAINT vendor_supplier_products_inventory_item_fk
        FOREIGN KEY (
            inventory_item_id,
            business_id,
            branch_id
        )
        REFERENCES inventory_items (
            id,
            business_id,
            branch_id
        ),

    CONSTRAINT vendor_supplier_products_conversion_ck
        CHECK (
            (
                conversion_mode = 'SAME_AS_INVENTORY'
                AND inventory_units_per_purchase_unit = 1
            )
            OR
            (
                conversion_mode = 'EXACT_INTEGER_MULTIPLE'
                AND inventory_units_per_purchase_unit > 0
            )
        ),

    UNIQUE (
        id,
        business_id,
        supplier_id
    ),

    UNIQUE (
        business_id,
        supplier_id,
        branch_id,
        inventory_item_id,
        purchase_unit_code
    )
);

CREATE UNIQUE INDEX vendor_supplier_products_supplier_sku_uq
ON vendor_supplier_products (
    business_id,
    supplier_id,
    supplier_sku
)
WHERE supplier_sku <> ''
  AND status = 'ACTIVE';

CREATE INDEX vendor_supplier_products_inventory_idx
ON vendor_supplier_products (
    business_id,
    branch_id,
    inventory_item_id,
    status
);

CREATE INDEX vendor_supplier_products_supplier_idx
ON vendor_supplier_products (
    business_id,
    supplier_id,
    branch_id,
    status,
    id
);