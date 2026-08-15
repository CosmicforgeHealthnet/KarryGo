-- Section 9A: Food Catalogue Foundation.
--
-- The management-side authoritative answer to "what does this vendor/branch
-- sell". Scheduling, effective availability, branch price overrides and price
-- history are Section 9B and are deliberately absent here; every table below
-- is shaped so 9B can add them additively rather than by rewriting rows.
--
-- Ownership is expressed with composite foreign keys rather than a bare parent
-- id, so a nested-IDOR attempt (a section addressed through the wrong menu, a
-- variant mutated through the wrong item, a modifier group borrowed from
-- another business) is refused by the database and not only by the service.
--
-- Money is integer minor units throughout. There is no numeric/float column in
-- this migration, and the business's own currency (vendor_businesses.currency)
-- stays the single currency truth — no catalogue row restates it.

-- Menus need this composite key so an item can prove, in one foreign key, that
-- it belongs to a menu that belongs to the same business AND the same branch.
CREATE TABLE food_menus (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text NOT NULL,
    capability_type text NOT NULL
        CHECK (capability_type IN ('RESTAURANT','FAST_FOOD')),
    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 160),
    description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','ACTIVE','PAUSED','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_menus_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    CONSTRAINT food_menus_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    UNIQUE (id, business_id),
    UNIQUE (id, business_id, branch_id)
);

CREATE INDEX food_menus_branch_status_idx
    ON food_menus (business_id, branch_id, status, created_at DESC);
CREATE INDEX food_menus_capability_idx
    ON food_menus (business_id, capability_type, status);
-- Two live menus of the same name for the same branch and capability is a data
-- entry mistake, not a product requirement. Archived menus are excluded so a
-- name can be reused after retirement.
CREATE UNIQUE INDEX food_menus_live_name_unique
    ON food_menus (business_id, branch_id, capability_type, lower(name))
    WHERE status <> 'ARCHIVED';

CREATE TABLE food_menu_sections (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    menu_id text NOT NULL,
    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 160),
    description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    position integer NOT NULL DEFAULT 0 CHECK (position BETWEEN 0 AND 9999),
    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_menu_sections_menu_scope_fk
        FOREIGN KEY (menu_id, business_id) REFERENCES food_menus (id, business_id),
    CONSTRAINT food_menu_sections_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    -- Deferred so one atomic reorder statement may permute positions without
    -- tripping the constraint mid-statement. It is still enforced at commit,
    -- which is what makes ordering deterministic rather than best-effort.
    CONSTRAINT food_menu_sections_position_unique
        UNIQUE (menu_id, position) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (id, business_id),
    UNIQUE (id, menu_id)
);

CREATE INDEX food_menu_sections_menu_position_idx
    ON food_menu_sections (menu_id, position);
CREATE INDEX food_menu_sections_business_status_idx
    ON food_menu_sections (business_id, status);
CREATE UNIQUE INDEX food_menu_sections_live_name_unique
    ON food_menu_sections (menu_id, lower(name))
    WHERE status <> 'ARCHIVED';

CREATE TABLE food_items (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    menu_id text NOT NULL,
    section_id text NOT NULL,
    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 160),
    description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 4000),
    -- Integer minor units. The upper bound is a sanity ceiling (about 10^11
    -- minor units) that keeps a mistyped price out of the catalogue while
    -- staying far inside bigint and far above any real menu price.
    base_price_minor bigint NOT NULL
        CHECK (base_price_minor >= 0 AND base_price_minor <= 99999999999),
    dietary_codes jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(dietary_codes) = 'array' AND jsonb_array_length(dietary_codes) <= 16),
    allergen_codes jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(allergen_codes) = 'array' AND jsonb_array_length(allergen_codes) <= 24),
    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','ACTIVE','PAUSED','ARCHIVED')),
    position integer NOT NULL DEFAULT 0 CHECK (position BETWEEN 0 AND 99999),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    -- The section must belong to the addressed menu, and the menu must belong
    -- to the same business AND branch the item claims. Together these two keys
    -- make every cross-menu and cross-branch item impossible to store.
    CONSTRAINT food_items_section_scope_fk
        FOREIGN KEY (section_id, menu_id) REFERENCES food_menu_sections (id, menu_id),
    CONSTRAINT food_items_menu_scope_fk
        FOREIGN KEY (menu_id, business_id, branch_id) REFERENCES food_menus (id, business_id, branch_id),
    CONSTRAINT food_items_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    CONSTRAINT food_items_position_unique
        UNIQUE (section_id, position) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (id, business_id),
    UNIQUE (id, section_id)
);

CREATE INDEX food_items_section_position_idx ON food_items (section_id, position);
CREATE INDEX food_items_menu_status_idx ON food_items (menu_id, status);
CREATE INDEX food_items_business_branch_status_idx
    ON food_items (business_id, branch_id, status);
CREATE UNIQUE INDEX food_items_live_name_unique
    ON food_items (section_id, lower(name))
    WHERE status <> 'ARCHIVED';

-- Opaque media references only. Media File Service owns bytes, MIME, safety,
-- retention and access; this table stores nothing but the reference, its
-- display order and which one is the cover.
CREATE TABLE food_item_media (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    item_id text NOT NULL,
    media_ref text NOT NULL CHECK (char_length(media_ref) BETWEEN 1 AND 200),
    position integer NOT NULL DEFAULT 0 CHECK (position BETWEEN 0 AND 99),
    is_primary boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT food_item_media_item_scope_fk
        FOREIGN KEY (item_id, business_id) REFERENCES food_items (id, business_id) ON DELETE CASCADE,
    CONSTRAINT food_item_media_position_unique
        UNIQUE (item_id, position) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (item_id, media_ref)
);

CREATE INDEX food_item_media_item_position_idx ON food_item_media (item_id, position);
CREATE UNIQUE INDEX food_item_media_single_primary_idx
    ON food_item_media (item_id) WHERE is_primary;

CREATE TABLE food_item_variants (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    item_id text NOT NULL,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    -- NULL inherits the item's base price. A non-NULL value is the variant's
    -- own price in integer minor units. Section 9B adds effective-price
    -- resolution on top of this without changing the column.
    price_override_minor bigint
        CHECK (price_override_minor IS NULL
               OR (price_override_minor >= 0 AND price_override_minor <= 99999999999)),
    position integer NOT NULL DEFAULT 0 CHECK (position BETWEEN 0 AND 999),
    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_item_variants_item_scope_fk
        FOREIGN KEY (item_id, business_id) REFERENCES food_items (id, business_id),
    CONSTRAINT food_item_variants_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    CONSTRAINT food_item_variants_position_unique
        UNIQUE (item_id, position) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (id, business_id),
    UNIQUE (id, item_id)
);

CREATE INDEX food_item_variants_item_position_idx ON food_item_variants (item_id, position);
CREATE UNIQUE INDEX food_item_variants_live_name_unique
    ON food_item_variants (item_id, lower(name))
    WHERE status <> 'ARCHIVED';

-- Modifier groups are BUSINESS-level reusable definitions, not branch-level
-- copies: "Add extras" is written once and attached to as many items as the
-- business likes. Branch reach is decided where it matters — on the item the
-- group is attached to — rather than by duplicating the group per branch.
CREATE TABLE food_modifier_groups (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 160),
    description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    -- min_select > 0 IS the "required" flag. There is deliberately no separate
    -- boolean, because two columns encoding one fact can disagree.
    min_select integer NOT NULL DEFAULT 0 CHECK (min_select BETWEEN 0 AND 50),
    max_select integer NOT NULL DEFAULT 1 CHECK (max_select BETWEEN 1 AND 50),
    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_modifier_groups_selection_check CHECK (max_select >= min_select),
    CONSTRAINT food_modifier_groups_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    UNIQUE (id, business_id)
);

CREATE INDEX food_modifier_groups_business_status_idx
    ON food_modifier_groups (business_id, status, name);
CREATE UNIQUE INDEX food_modifier_groups_live_name_unique
    ON food_modifier_groups (business_id, lower(name))
    WHERE status <> 'ARCHIVED';

CREATE TABLE food_modifiers (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    modifier_group_id text NOT NULL,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    -- A surcharge in integer minor units. Zero is legitimate ("no sauce").
    -- Negative is refused: a discount is a promotion, and promotions are not
    -- this domain's authority.
    price_delta_minor bigint NOT NULL DEFAULT 0
        CHECK (price_delta_minor >= 0 AND price_delta_minor <= 99999999999),
    position integer NOT NULL DEFAULT 0 CHECK (position BETWEEN 0 AND 999),
    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_modifiers_group_scope_fk
        FOREIGN KEY (modifier_group_id, business_id) REFERENCES food_modifier_groups (id, business_id),
    CONSTRAINT food_modifiers_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    CONSTRAINT food_modifiers_position_unique
        UNIQUE (modifier_group_id, position) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (id, business_id),
    UNIQUE (id, modifier_group_id)
);

CREATE INDEX food_modifiers_group_position_idx ON food_modifiers (modifier_group_id, position);
CREATE UNIQUE INDEX food_modifiers_live_name_unique
    ON food_modifiers (modifier_group_id, lower(name))
    WHERE status <> 'ARCHIVED';

-- The relationship, not a copy. Detaching removes this row and leaves the
-- group itself untouched for every other item that uses it.
CREATE TABLE food_item_modifier_groups (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    item_id text NOT NULL,
    modifier_group_id text NOT NULL,
    position integer NOT NULL DEFAULT 0 CHECK (position BETWEEN 0 AND 999),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_item_modifier_groups_item_scope_fk
        FOREIGN KEY (item_id, business_id) REFERENCES food_items (id, business_id),
    -- Same business on both sides, enforced by the key rather than by a check
    -- the service could forget to make.
    CONSTRAINT food_item_modifier_groups_group_scope_fk
        FOREIGN KEY (modifier_group_id, business_id) REFERENCES food_modifier_groups (id, business_id),
    CONSTRAINT food_item_modifier_groups_position_unique
        UNIQUE (item_id, position) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (item_id, modifier_group_id)
);

CREATE INDEX food_item_modifier_groups_item_position_idx
    ON food_item_modifier_groups (item_id, position);
CREATE INDEX food_item_modifier_groups_group_idx
    ON food_item_modifier_groups (business_id, modifier_group_id);
