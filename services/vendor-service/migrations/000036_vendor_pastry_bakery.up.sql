-- Section 11: first-class Pastry/Bakery catalogue and custom-cake workflow.
CREATE TABLE pastry_products (
    id text PRIMARY KEY, business_id text NOT NULL, branch_id text NOT NULL, name text NOT NULL, description text NOT NULL DEFAULT '', product_type text NOT NULL CHECK(product_type IN ('CAKE','CUPCAKE','BREAD','DOUGHNUT','PIE','COOKIE','SMALL_CHOPS','SNACK','BOX','TRAY','PARTY_PACK','OTHER')), base_price_minor bigint NOT NULL CHECK(base_price_minor>=0), currency text NOT NULL DEFAULT 'NGN', customizable boolean NOT NULL DEFAULT false, production_class text, capacity_pool_id text, prep_profile jsonb NOT NULL DEFAULT '{}'::jsonb, logistics jsonb NOT NULL DEFAULT '{}'::jsonb, status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN ('DRAFT','ACTIVE','ARCHIVED')), version bigint NOT NULL DEFAULT 1, position integer NOT NULL DEFAULT 0, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX pastry_products_branch_idx ON pastry_products(business_id,branch_id,status,position);
CREATE TABLE pastry_option_groups (
    id text PRIMARY KEY, business_id text NOT NULL, name text NOT NULL, group_type text NOT NULL CHECK(group_type IN ('FLAVOUR','SIZE','WEIGHT','LAYERS','FILLING','ICING','SERVING_COUNT','PACKAGING','SHAPE','COLOUR_SCHEME','DECORATION_STYLE')), min_select integer NOT NULL DEFAULT 0 CHECK(min_select>=0), max_select integer NOT NULL DEFAULT 1 CHECK(max_select>=min_select), status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN ('ACTIVE','ARCHIVED')), version bigint NOT NULL DEFAULT 1, UNIQUE(business_id,name)
);
CREATE TABLE pastry_options (
    id text PRIMARY KEY, group_id text NOT NULL REFERENCES pastry_option_groups(id) ON DELETE CASCADE, name text NOT NULL, code text NOT NULL, position integer NOT NULL DEFAULT 0, price_delta_minor bigint NOT NULL DEFAULT 0 CHECK(price_delta_minor>=0), workload_weight integer NOT NULL DEFAULT 1 CHECK(workload_weight>0), capacity_class text, status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN ('ACTIVE','ARCHIVED')), version bigint NOT NULL DEFAULT 1, UNIQUE(group_id,code)
);
CREATE TABLE pastry_product_option_groups (
    product_id text NOT NULL REFERENCES pastry_products(id) ON DELETE CASCADE, group_id text NOT NULL REFERENCES pastry_option_groups(id), position integer NOT NULL DEFAULT 0, required boolean NOT NULL DEFAULT false, PRIMARY KEY(product_id,group_id)
);
CREATE TABLE pastry_custom_requests (
    id text PRIMARY KEY, business_id text NOT NULL, branch_id text NOT NULL, customer_ref text, title text NOT NULL, notes text NOT NULL DEFAULT '', event_at timestamptz, status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN ('DRAFT','SUBMITTED','QUOTED','ACCEPTED','DECLINED','CANCELLED','COMPLETED')), change_cutoff_at timestamptz, logistics jsonb NOT NULL DEFAULT '{}'::jsonb, risk_flags jsonb NOT NULL DEFAULT '[]'::jsonb, version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE pastry_specification_versions (
    id text PRIMARY KEY, request_id text NOT NULL REFERENCES pastry_custom_requests(id) ON DELETE CASCADE, revision integer NOT NULL CHECK(revision>0), specification jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(request_id,revision)
);
CREATE TABLE pastry_change_requests (
    id text PRIMARY KEY, request_id text NOT NULL REFERENCES pastry_custom_requests(id) ON DELETE CASCADE, from_revision integer NOT NULL, to_revision integer, reason text NOT NULL, status text NOT NULL DEFAULT 'OPEN' CHECK(status IN ('OPEN','APPROVED','DECLINED','CANCELLED')), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE pastry_quotes (
    id text PRIMARY KEY, request_id text NOT NULL REFERENCES pastry_custom_requests(id), revision integer NOT NULL CHECK(revision>0), specification_version_id text NOT NULL REFERENCES pastry_specification_versions(id), amount_minor bigint NOT NULL CHECK(amount_minor>=0), currency text NOT NULL DEFAULT 'NGN', expires_at timestamptz NOT NULL, status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN ('DRAFT','PENDING_APPROVAL','APPROVED','ACCEPTED','DECLINED','EXPIRED')), capacity_pool_id text, reservation_id text, accepted_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(request_id,revision)
);
CREATE TABLE pastry_accepted_snapshots (
    id text PRIMARY KEY, quote_id text NOT NULL UNIQUE REFERENCES pastry_quotes(id), request_id text NOT NULL REFERENCES pastry_custom_requests(id), quote_revision integer NOT NULL, specification jsonb NOT NULL, amount_minor bigint NOT NULL, currency text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE pastry_order_conversion_intents (
    id text PRIMARY KEY, request_id text NOT NULL UNIQUE REFERENCES pastry_custom_requests(id), quote_id text NOT NULL UNIQUE REFERENCES pastry_quotes(id), snapshot_id text NOT NULL REFERENCES pastry_accepted_snapshots(id), status text NOT NULL DEFAULT 'PENDING_EXTERNAL_DOMAIN' CHECK(status IN ('PENDING_EXTERNAL_DOMAIN','SUBMITTED','COMPLETED','FAILED')), idempotency_key text NOT NULL UNIQUE, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE pastry_milestones (
    id text PRIMARY KEY, request_id text NOT NULL REFERENCES pastry_custom_requests(id) ON DELETE CASCADE, milestone_type text NOT NULL, state text NOT NULL CHECK(state IN ('PENDING','ACTIVE','BLOCKED','READY','COMPLETED','CANCELLED')), position integer NOT NULL DEFAULT 0, version bigint NOT NULL DEFAULT 1, UNIQUE(request_id,milestone_type)
);
CREATE TABLE pastry_milestone_history (
    id text PRIMARY KEY, milestone_id text NOT NULL REFERENCES pastry_milestones(id) ON DELETE CASCADE, from_state text, to_state text NOT NULL, changed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX pastry_quotes_expiry_idx ON pastry_quotes(status,expires_at);
CREATE INDEX pastry_specs_request_idx ON pastry_specification_versions(request_id,revision DESC);
