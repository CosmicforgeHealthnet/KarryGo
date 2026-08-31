-- Section 9B: Pricing, Availability, Scheduling and Operational Profiles.
--
-- Section 9A answered "what does this branch sell". Section 9B answers "can it
-- be sold now, at this branch, at this time, at what effective price, with what
-- preparation and handling requirements".
--
-- Everything here is additive to the accepted 9A schema. No 9A table is
-- redesigned; three of them gain nothing at all, because the operational layer
-- lives beside the catalogue rather than inside it.
--
-- Money remains integer minor units. There is no numeric or float column in
-- this migration either, and the business's own currency (Section 6) is still
-- the single currency truth — price history records it for evidence, it does
-- not redefine it.

-- ---------------------------------------------------------------------------
-- Menu schedules
-- ---------------------------------------------------------------------------
--
-- A schedule says WHEN an already-ACTIVE menu is operationally available. It
-- can never make a DRAFT, PAUSED or ARCHIVED menu sellable: administrative
-- status is decided a layer above and a schedule is not allowed to argue.
--
-- There is deliberately no timezone column. The branch already carries an
-- authoritative IANA timezone (Section 6, vendor_branches.timezone) and
-- duplicating it here would create two answers to one question.
CREATE TABLE food_menu_schedules (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    menu_id text NOT NULL,
    label text NOT NULL
        CHECK (label IN ('BREAKFAST','LUNCH','DINNER','SEASONAL','CUSTOM')),
    -- ISO weekdays, 1 = Monday .. 7 = Sunday. An overnight window belongs to
    -- the day it STARTS on, so a Friday 22:00-02:00 window is Friday's.
    days_of_week smallint[] NOT NULL
        CHECK (array_length(days_of_week, 1) BETWEEN 1 AND 7
               AND days_of_week <@ ARRAY[1,2,3,4,5,6,7]::smallint[]),
    start_local_time time NOT NULL,
    end_local_time time NOT NULL,
    -- Seasonal/date-limited availability. NULL on either side means unbounded
    -- in that direction.
    effective_start_date date,
    effective_end_date date,
    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_menu_schedules_menu_scope_fk
        FOREIGN KEY (menu_id, business_id) REFERENCES food_menus (id, business_id),
    -- A zero-length window is a configuration mistake, not an "always" or a
    -- "never"; both of those have honest representations already.
    CONSTRAINT food_menu_schedules_window_check
        CHECK (start_local_time <> end_local_time),
    CONSTRAINT food_menu_schedules_date_range_check
        CHECK (effective_start_date IS NULL OR effective_end_date IS NULL
               OR effective_start_date <= effective_end_date),
    CONSTRAINT food_menu_schedules_archived_state_check
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    UNIQUE (id, business_id),
    UNIQUE (id, menu_id)
);

CREATE INDEX food_menu_schedules_menu_status_idx
    ON food_menu_schedules (menu_id, status);
CREATE INDEX food_menu_schedules_business_idx
    ON food_menu_schedules (business_id, status);

-- ---------------------------------------------------------------------------
-- Item operational availability
-- ---------------------------------------------------------------------------
--
-- Separate from the branch override table on purpose. Marking an item sold out
-- is kitchen work that happens many times a service; changing its price is a
-- commercial decision that happens rarely and needs a different grant. Sharing
-- one row would mean sharing one version fence, so a cook toggling sold-out
-- would collide with a manager editing a price.
--
-- Section 9A items are already bound to exactly one branch, so this row IS the
-- branch's operational state for that item; there is no second scope to model.
CREATE TABLE food_item_availability (
    item_id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    status text NOT NULL DEFAULT 'AVAILABLE'
        CHECK (status IN ('AVAILABLE','SOLD_OUT','PAUSED')),
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 280),
    -- NULL means indefinite: a sold-out item stays sold out until someone says
    -- otherwise. Requiring an expiry would force operators to invent one.
    effective_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_item_availability_item_scope_fk
        FOREIGN KEY (item_id, business_id) REFERENCES food_items (id, business_id) ON DELETE CASCADE,
    CONSTRAINT food_item_availability_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    -- AVAILABLE is the resting state and carries no expiry: "available until
    -- Tuesday" is a schedule, not an availability override.
    CONSTRAINT food_item_availability_expiry_check
        CHECK (status <> 'AVAILABLE' OR effective_until IS NULL)
);

CREATE INDEX food_item_availability_branch_status_idx
    ON food_item_availability (business_id, branch_id, status);
CREATE INDEX food_item_availability_expiry_idx
    ON food_item_availability (effective_until) WHERE effective_until IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Branch item overrides
-- ---------------------------------------------------------------------------
--
-- Three independent optional dimensions: price, and the two preparation
-- bounds. Every one of them is nullable, and NULL means inherit — which is why
-- clearing a price override is a distinct operation from setting it to zero.
-- A zero-priced item is a real product decision; "no override" is the absence
-- of one, and the schema keeps those two apart.
CREATE TABLE food_branch_item_overrides (
    item_id text PRIMARY KEY,
    id text NOT NULL UNIQUE,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    base_price_override_minor bigint
        CHECK (base_price_override_minor IS NULL
               OR (base_price_override_minor >= 0 AND base_price_override_minor <= 99999999999)),
    prep_min_minutes integer
        CHECK (prep_min_minutes IS NULL OR (prep_min_minutes >= 0 AND prep_min_minutes <= 1440)),
    prep_max_minutes integer
        CHECK (prep_max_minutes IS NULL OR (prep_max_minutes >= 0 AND prep_max_minutes <= 1440)),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_branch_item_overrides_item_scope_fk
        FOREIGN KEY (item_id, business_id) REFERENCES food_items (id, business_id) ON DELETE CASCADE,
    CONSTRAINT food_branch_item_overrides_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id),
    -- Both bounds are overridden together or not at all: a max below the
    -- inherited min would be a silently impossible range.
    CONSTRAINT food_branch_item_overrides_prep_pair_check
        CHECK ((prep_min_minutes IS NULL) = (prep_max_minutes IS NULL)),
    CONSTRAINT food_branch_item_overrides_prep_range_check
        CHECK (prep_min_minutes IS NULL OR prep_max_minutes >= prep_min_minutes)
);

CREATE INDEX food_branch_item_overrides_branch_idx
    ON food_branch_item_overrides (business_id, branch_id);

-- ---------------------------------------------------------------------------
-- Immutable price history
-- ---------------------------------------------------------------------------
--
-- One table rather than four, because the management surface asks one question
-- — "what has this item's pricing done" — and four tables would answer it with
-- a four-way union on every read. The subject discriminator keeps the rows
-- distinguishable without splitting the index.
--
-- These rows are the evidence a future Order authority will snapshot. They are
-- append-only, enforced by a trigger below rather than by convention: a
-- historical price that can be edited is not history.
CREATE TABLE food_price_versions (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id text,
    subject_type text NOT NULL
        CHECK (subject_type IN ('ITEM','VARIANT','MODIFIER','BRANCH_OVERRIDE')),
    subject_ref text NOT NULL,
    -- The item this record belongs to, so the aggregate history read is one
    -- indexed query. NULL for a modifier, which belongs to a shared group and
    -- therefore to no single item.
    item_ref text,
    currency char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    -- NULL amount means "inherits" — a variant with no override, or a branch
    -- override that has been cleared. The boolean says so explicitly so a
    -- reader never has to infer intent from a null.
    previous_amount_minor bigint
        CHECK (previous_amount_minor IS NULL
               OR (previous_amount_minor >= 0 AND previous_amount_minor <= 99999999999)),
    amount_minor bigint
        CHECK (amount_minor IS NULL OR (amount_minor >= 0 AND amount_minor <= 99999999999)),
    previous_inherited boolean NOT NULL DEFAULT false,
    inherited boolean NOT NULL DEFAULT false,
    change_reason text NOT NULL
        CHECK (change_reason IN ('BASELINE_IMPORT','CREATED','PRICE_CHANGED',
                                 'OVERRIDE_SET','OVERRIDE_CLEARED')),
    -- The subject entity's own version AFTER the change, and a monotonic
    -- per-subject revision so a snapshot can name an exact price without
    -- depending on a timestamp comparison.
    subject_version bigint NOT NULL CHECK (subject_version > 0),
    revision bigint NOT NULL CHECK (revision > 0),
    actor_type text NOT NULL DEFAULT 'vendor_staff',
    actor_id text,
    correlation_id text,
    command_ref text,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    effective_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT food_price_versions_inherit_check
        CHECK ((amount_minor IS NULL) = inherited),
    UNIQUE (subject_type, subject_ref, revision)
);

CREATE INDEX food_price_versions_item_idx
    ON food_price_versions (business_id, item_ref, recorded_at DESC)
    WHERE item_ref IS NOT NULL;
CREATE INDEX food_price_versions_subject_idx
    ON food_price_versions (subject_type, subject_ref, revision DESC);
CREATE INDEX food_price_versions_business_recorded_idx
    ON food_price_versions (business_id, recorded_at DESC);

-- Append-only, enforced by the database. There is no management route that
-- updates or deletes a price version, but a route is a promise and a trigger is
-- a guarantee — and this table is the only evidence a future order has that it
-- was charged what the catalogue said.
CREATE FUNCTION food_price_versions_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'food_price_versions is append-only: % is not permitted', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER food_price_versions_no_update_or_delete
    BEFORE UPDATE OR DELETE ON food_price_versions
    FOR EACH ROW EXECUTE FUNCTION food_price_versions_immutable();

-- ---------------------------------------------------------------------------
-- Preparation profiles
-- ---------------------------------------------------------------------------
--
-- Vendor Service owns how long the kitchen needs. It does not own travel time,
-- driver arrival or any customer-facing ETA, and nothing in this table can be
-- combined into one without leaving this service's authority.
CREATE TABLE food_item_prep_profiles (
    item_id text PRIMARY KEY,
    business_id text NOT NULL,
    prep_min_minutes integer NOT NULL
        CHECK (prep_min_minutes >= 0 AND prep_min_minutes <= 1440),
    prep_max_minutes integer NOT NULL
        CHECK (prep_max_minutes >= 0 AND prep_max_minutes <= 1440),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_item_prep_profiles_item_scope_fk
        FOREIGN KEY (item_id, business_id) REFERENCES food_items (id, business_id) ON DELETE CASCADE,
    CONSTRAINT food_item_prep_profiles_range_check
        CHECK (prep_max_minutes >= prep_min_minutes)
);

CREATE INDEX food_item_prep_profiles_business_idx ON food_item_prep_profiles (business_id);

-- ---------------------------------------------------------------------------
-- Logistics profiles
-- ---------------------------------------------------------------------------
--
-- Handling metadata, and only that. Vehicle entries are SUITABILITY HINTS for a
-- future logistics decision; nothing here assigns a provider, chooses a rider or
-- prices a delivery, and no route in this section calls Dispatch, Taxi or
-- Hauling.
CREATE TABLE food_item_logistics_profiles (
    item_id text PRIMARY KEY,
    business_id text NOT NULL,
    temperature_class text NOT NULL DEFAULT 'AMBIENT'
        CHECK (temperature_class IN ('AMBIENT','HOT','COLD','FROZEN')),
    package_size_class text NOT NULL DEFAULT 'SMALL'
        CHECK (package_size_class IN ('SMALL','MEDIUM','LARGE')),
    spill_risk boolean NOT NULL DEFAULT false,
    fragile boolean NOT NULL DEFAULT false,
    -- Integer grams, the platform's canonical mass unit. NULL means the vendor
    -- has not stated one, which is different from stating zero.
    approx_weight_grams integer
        CHECK (approx_weight_grams IS NULL
               OR (approx_weight_grams > 0 AND approx_weight_grams <= 200000)),
    vehicle_hints text[] NOT NULL DEFAULT ARRAY[]::text[]
        CHECK (array_length(vehicle_hints, 1) IS NULL
               OR (array_length(vehicle_hints, 1) <= 4
                   AND vehicle_hints <@ ARRAY['MOTORCYCLE','CAR','VAN','TRUCK']::text[])),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_item_logistics_profiles_item_scope_fk
        FOREIGN KEY (item_id, business_id) REFERENCES food_items (id, business_id) ON DELETE CASCADE
);

CREATE INDEX food_item_logistics_profiles_business_idx
    ON food_item_logistics_profiles (business_id);

-- ---------------------------------------------------------------------------
-- Inventory availability seam
-- ---------------------------------------------------------------------------
--
-- The SEAM only. There is no stock table, no quantity, no reservation and no
-- decrement anywhere in this migration, because Vendor Service is not the
-- inventory authority and no such authority exists yet.
--
-- INVENTORY_LINKED with no reachable source resolves to NOT sellable. That is
-- the whole point of the mode: an operator who links an item to inventory is
-- saying "do not sell this unless inventory agrees", and a missing inventory
-- service is not agreement.
CREATE TABLE food_item_availability_sources (
    item_id text PRIMARY KEY,
    business_id text NOT NULL,
    mode text NOT NULL DEFAULT 'MANUAL'
        CHECK (mode IN ('MANUAL','INVENTORY_LINKED')),
    -- An opaque reference into whatever the inventory authority eventually is.
    -- No cross-service foreign key, no modelling of its schema.
    inventory_recipe_ref text
        CHECK (inventory_recipe_ref IS NULL OR char_length(inventory_recipe_ref) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT food_item_availability_sources_item_scope_fk
        FOREIGN KEY (item_id, business_id) REFERENCES food_items (id, business_id) ON DELETE CASCADE,
    CONSTRAINT food_item_availability_sources_reference_check
        CHECK ((mode = 'MANUAL' AND inventory_recipe_ref IS NULL)
               OR (mode = 'INVENTORY_LINKED' AND inventory_recipe_ref IS NOT NULL))
);

CREATE INDEX food_item_availability_sources_mode_idx
    ON food_item_availability_sources (business_id, mode);

-- ---------------------------------------------------------------------------
-- Catalogue revisions
-- ---------------------------------------------------------------------------
--
-- A monotonic counter per (business, branch, capability). Any committed change
-- that could alter what a future reader would see bumps it in the SAME
-- transaction, so "the catalogue changed" is as durable as the change itself.
--
-- This is not a cache and 9B builds no cache to invalidate. It is the evidence
-- Section 9C will use to decide that a previously served view is stale.
CREATE TABLE food_catalogue_revisions (
    business_id text NOT NULL,
    branch_id text NOT NULL,
    capability_type text NOT NULL
        CHECK (capability_type IN ('RESTAURANT','FAST_FOOD')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, branch_id, capability_type),
    CONSTRAINT food_catalogue_revisions_branch_scope_fk
        FOREIGN KEY (branch_id, business_id) REFERENCES vendor_branches (id, business_id)
);

-- ---------------------------------------------------------------------------
-- Price-history baseline for the catalogue that already exists
-- ---------------------------------------------------------------------------
--
-- Section 9A rows already carry prices, and nobody recorded how they got there.
-- BASELINE_IMPORT says exactly that and nothing more: "this was the current
-- known price when price-history authority was introduced". It deliberately
-- does NOT claim to be the creation price, because that cannot be proven from
-- the data that exists.
--
-- previous_amount is NULL and the actor is the system, so a reader can never
-- mistake a baseline for an observed change.
INSERT INTO food_price_versions
    (id, business_id, branch_id, subject_type, subject_ref, item_ref, currency,
     previous_amount_minor, amount_minor, previous_inherited, inherited,
     change_reason, subject_version, revision, actor_type, actor_id, recorded_at, effective_at)
SELECT
    'fpv_baseline_item_' || i.id,
    i.business_id, i.branch_id, 'ITEM', i.id, i.id, b.currency,
    NULL, i.base_price_minor, false, false,
    'BASELINE_IMPORT', i.version, 1, 'system', NULL, now(), i.updated_at
FROM food_items i
JOIN vendor_businesses b ON b.id = i.business_id;

INSERT INTO food_price_versions
    (id, business_id, branch_id, subject_type, subject_ref, item_ref, currency,
     previous_amount_minor, amount_minor, previous_inherited, inherited,
     change_reason, subject_version, revision, actor_type, actor_id, recorded_at, effective_at)
SELECT
    'fpv_baseline_variant_' || v.id,
    v.business_id, i.branch_id, 'VARIANT', v.id, i.id, b.currency,
    NULL, v.price_override_minor, false, v.price_override_minor IS NULL,
    'BASELINE_IMPORT', v.version, 1, 'system', NULL, now(), v.updated_at
FROM food_item_variants v
JOIN food_items i ON i.id = v.item_id
JOIN vendor_businesses b ON b.id = v.business_id;

-- Modifiers belong to a business-level shared group, so they carry no branch
-- and no single item.
INSERT INTO food_price_versions
    (id, business_id, branch_id, subject_type, subject_ref, item_ref, currency,
     previous_amount_minor, amount_minor, previous_inherited, inherited,
     change_reason, subject_version, revision, actor_type, actor_id, recorded_at, effective_at)
SELECT
    'fpv_baseline_modifier_' || m.id,
    m.business_id, NULL, 'MODIFIER', m.id, NULL, b.currency,
    NULL, m.price_delta_minor, false, false,
    'BASELINE_IMPORT', m.version, 1, 'system', NULL, now(), m.updated_at
FROM food_modifiers m
JOIN vendor_businesses b ON b.id = m.business_id;

-- Every branch/capability that already has a menu starts at revision 1, so a
-- future reader always has a revision to compare against rather than an absence.
INSERT INTO food_catalogue_revisions (business_id, branch_id, capability_type, revision)
SELECT DISTINCT business_id, branch_id, capability_type, 1 FROM food_menus
ON CONFLICT DO NOTHING;
