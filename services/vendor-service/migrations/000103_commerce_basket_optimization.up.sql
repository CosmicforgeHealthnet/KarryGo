-- Customer basket optimization: proposals, recurring templates, and the
-- exactly-once recurrence ledger.
--
-- Three tables, and the constraints on them are the feature's safety model
-- rather than an afterthought about data hygiene.
--
--   commerce_basket_proposals
--       An OFFER. Owned by one customer, bound to the cart version it was
--       computed against, and expiring. Accepting one creates a new cart
--       version - once, however many times Accept is pressed.
--
--   commerce_recurring_basket_templates
--       A reusable list of NEEDS, not of old SKUs. It stores what the
--       customer requires and what they will accept, so every occurrence goes
--       shopping again instead of re-buying a decision made months ago.
--
--   commerce_recurring_basket_generations
--       One row per scheduled occurrence. The UNIQUE index on
--       (template_id, occurrence_key) is what makes exactly-once a property
--       of the database instead of a property of the worker that happened to
--       run. Application checks alone cannot survive two workers, a replay
--       and a restart; a unique index survives all three.
--
-- Nothing here holds stock, holds money or creates an order. A proposal is a
-- suggestion with an expiry date, and a generation is a suggestion that
-- arrived on schedule.

-- ---------------------------------------------------------------------------
-- Proposals
-- ---------------------------------------------------------------------------

CREATE TABLE commerce_basket_proposals (
    id text PRIMARY KEY,

    customer_id text NOT NULL CHECK (customer_id <> ''),

    -- The cart this proposal was computed against.
    --
    -- The VERSION is the concurrency guard and the reason it is stored at
    -- all. A proposal built on cart v5 and accepted after the customer
    -- reached v6 would silently discard whatever they did in between;
    -- comparing versions is how that is detected rather than hoped for.
    --
    -- Both are nullable because a recurring occurrence proposes a basket
    -- without starting from a cart at all.
    base_cart_id text,
    base_cart_version bigint CHECK (
        base_cart_version IS NULL OR base_cart_version > 0
    ),

    objective text NOT NULL
        CHECK (objective IN ('VALUE', 'FEWEST_VENDORS', 'QUALITY_BALANCED')),

    state text NOT NULL DEFAULT 'OPEN'
        CHECK (state IN ('OPEN', 'ACCEPTED', 'EXPIRED', 'SUPERSEDED')),

    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),

    merchandise_total_minor bigint NOT NULL DEFAULT 0
        CHECK (merchandise_total_minor >= 0),
    delivery_total_minor bigint NOT NULL DEFAULT 0
        CHECK (delivery_total_minor >= 0),

    -- The only number a customer actually pays, and the only one a budget is
    -- ever evaluated against. Held as a stored column with a CHECK rather
    -- than recomputed on read, so a proposal whose parts and whole disagree
    -- cannot be written at all.
    final_delivered_total_minor bigint NOT NULL DEFAULT 0
        CHECK (final_delivered_total_minor >= 0),

    vendor_count integer NOT NULL DEFAULT 0 CHECK (vendor_count >= 0),

    lines jsonb NOT NULL DEFAULT '[]'::jsonb,
    substitutions jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Semantic identity of the proposed basket, with no timestamps or ids in
    -- it. Two evaluations reaching the same basket produce the same
    -- fingerprint, which is what lets a repeat be recognised as a repeat.
    fingerprint text NOT NULL CHECK (fingerprint <> ''),

    expires_at timestamptz NOT NULL,

    -- The cart version acceptance produced. Its presence IS the record that
    -- acceptance happened, which is why the two are constrained together
    -- below rather than tracked independently.
    accepted_cart_version bigint CHECK (
        accepted_cart_version IS NULL OR accepted_cart_version > 0
    ),
    accepted_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commerce_basket_proposals_total_chk
        CHECK (
            final_delivered_total_minor
            = merchandise_total_minor + delivery_total_minor
        ),

    -- An accepted proposal must say which cart version it produced, and a
    -- proposal that is not accepted must not claim one. Without this, a
    -- replayed acceptance would have nothing truthful to return.
    CONSTRAINT commerce_basket_proposals_accepted_chk
        CHECK (
            (state = 'ACCEPTED')
            = (accepted_cart_version IS NOT NULL AND accepted_at IS NOT NULL)
        ),

    -- Customer isolation enforced by row shape, so a foreign proposal cannot
    -- be reached even by a query that forgot its predicate.
    UNIQUE (id, customer_id)
);

CREATE INDEX commerce_basket_proposals_customer_idx
    ON commerce_basket_proposals (customer_id, created_at DESC, id);

-- The expiry sweep. A proposal nobody reads must still stop being
-- acceptable, or a stale browser tab keeps yesterday's prices clickable.
CREATE INDEX commerce_basket_proposals_expiry_idx
    ON commerce_basket_proposals (expires_at, id)
    WHERE state = 'OPEN';

-- One live offer per distinct basket per cart version.
--
-- Re-running the optimizer on an unchanged cart is an ordinary thing for a
-- customer to do, and it must not accumulate a pile of identical offers each
-- of which could be accepted separately.
CREATE UNIQUE INDEX commerce_basket_proposals_live_idx
    ON commerce_basket_proposals (
        customer_id, base_cart_id, base_cart_version, fingerprint
    )
    WHERE state = 'OPEN' AND base_cart_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Recurring templates
-- ---------------------------------------------------------------------------

CREATE TABLE commerce_recurring_basket_templates (
    id text PRIMARY KEY,

    customer_id text NOT NULL CHECK (customer_id <> ''),

    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),

    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),

    -- The customer's own saved-location REFERENCE, never coordinates. A
    -- customer who corrects their address should not have to correct every
    -- template that ships to it.
    address_ref text NOT NULL CHECK (address_ref <> ''),

    -- Requirements, stated as NEEDS.
    --
    -- This is the decision the whole feature rests on. A template bound to
    -- specific listings breaks the first time a vendor runs out, raises a
    -- price or leaves the platform - and breaks silently. One that stores
    -- what the customer requires survives all of it, because every occurrence
    -- goes shopping again.
    requirements jsonb NOT NULL,

    fulfilment_preference text NOT NULL DEFAULT ''
        CHECK (fulfilment_preference IN ('', 'FASTEST', 'BALANCED', 'SAVER')),

    objective text NOT NULL DEFAULT 'VALUE'
        CHECK (objective IN ('VALUE', 'FEWEST_VENDORS', 'QUALITY_BALANCED')),

    approval_policy text NOT NULL DEFAULT 'ALWAYS_ASK'
        CHECK (approval_policy IN (
            'ALWAYS_ASK',
            'AUTO_IF_UNCHANGED',
            'AUTO_VENDOR_CHANGE_WITHIN_CEILING'
        )),

    -- What an auto-approved occurrence may cost, delivered.
    ceiling_minor bigint NOT NULL DEFAULT 0 CHECK (ceiling_minor >= 0),

    cadence text NOT NULL
        CHECK (cadence IN ('DAILY', 'WEEKLY', 'FORTNIGHTLY', 'MONTHLY')),

    -- An IANA zone name, and it is NOT NULL for a reason. "Every Tuesday at
    -- nine" means nine in the customer's morning; a schedule resolved against
    -- server-local time drifts with deployments and is silently wrong for
    -- every customer outside the server's zone.
    timezone text NOT NULL CHECK (timezone <> ''),

    local_hour integer NOT NULL CHECK (local_hour BETWEEN 0 AND 23),
    local_minute integer NOT NULL CHECK (local_minute BETWEEN 0 AND 59),

    weekday integer NOT NULL DEFAULT 0 CHECK (weekday BETWEEN 0 AND 6),

    -- Capped at 28 deliberately: 29, 30 and 31 do not exist in every month,
    -- and a monthly basket that silently skipped February would recur less
    -- often than the customer was told it would.
    day_of_month integer CHECK (day_of_month IS NULL OR day_of_month BETWEEN 1 AND 28),

    enabled boolean NOT NULL DEFAULT true,

    -- Optimistic concurrency. Two people editing one household's recurring
    -- basket must not silently overwrite each other's preferences.
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- An unbounded standing purchase authority is not something this feature
    -- offers. A preauthorizing policy without a ceiling is refused here as
    -- well as in the service, because this is the constraint that survives a
    -- future code path that forgets to check.
    CONSTRAINT commerce_recurring_basket_templates_ceiling_chk
        CHECK (approval_policy = 'ALWAYS_ASK' OR ceiling_minor > 0),

    CONSTRAINT commerce_recurring_basket_templates_monthly_chk
        CHECK (cadence <> 'MONTHLY' OR day_of_month IS NOT NULL),

    UNIQUE (id, customer_id)
);

CREATE INDEX commerce_recurring_basket_templates_customer_idx
    ON commerce_recurring_basket_templates (customer_id, created_at DESC, id);

-- The generation sweep reads only enabled templates.
CREATE INDEX commerce_recurring_basket_templates_due_idx
    ON commerce_recurring_basket_templates (id)
    WHERE enabled;

-- ---------------------------------------------------------------------------
-- Recurrence generations: exactly once
-- ---------------------------------------------------------------------------

CREATE TABLE commerce_recurring_basket_generations (
    id text PRIMARY KEY,

    template_id text NOT NULL,
    customer_id text NOT NULL CHECK (customer_id <> ''),

    -- The deterministic identity of one scheduled moment: the local
    -- wall-clock time plus the zone it was computed in.
    --
    -- Derived ONLY from the schedule. Not from the current time, not from a
    -- worker id, not from an attempt count - any of those would make two
    -- attempts at one occurrence look like two occurrences, which is the
    -- exact failure this column exists to prevent.
    occurrence_key text NOT NULL CHECK (occurrence_key <> ''),
    occurrence_at timestamptz NOT NULL,
    timezone text NOT NULL CHECK (timezone <> ''),

    state text NOT NULL DEFAULT 'PROPOSED'
        CHECK (state IN (
            'PROPOSED',
            'AWAITING_APPROVAL',
            'AUTO_APPROVED',
            'UNFULFILLABLE'
        )),

    -- The offer this occurrence produced, when it produced one. Nullable
    -- because an occurrence today's catalogue cannot satisfy is a real
    -- outcome with no proposal attached, and that is different from a
    -- proposal having gone missing.
    proposal_id text,

    comparison jsonb NOT NULL DEFAULT '{}'::jsonb,

    reason text NOT NULL DEFAULT '',

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commerce_recurring_basket_generations_template_fk
        FOREIGN KEY (template_id, customer_id)
        REFERENCES commerce_recurring_basket_templates (id, customer_id)
        ON DELETE CASCADE,

    CONSTRAINT commerce_recurring_basket_generations_proposal_fk
        FOREIGN KEY (proposal_id, customer_id)
        REFERENCES commerce_basket_proposals (id, customer_id)
        ON DELETE SET NULL,

    -- An unfulfillable occurrence has no basket to propose. Storing one
    -- anyway would mean a customer could accept an offer the generation
    -- itself concluded could not be met.
    CONSTRAINT commerce_recurring_basket_generations_unfulfillable_chk
        CHECK (state <> 'UNFULFILLABLE' OR proposal_id IS NULL),

    -- EXACTLY ONCE.
    --
    -- This is the constraint the whole recurrence guarantee rests on. A
    -- retry, a worker replay, a process restart, a duplicate scheduler
    -- delivery and an at-least-once event all present the same
    -- (template_id, occurrence_key) and all collide here. An in-memory lock
    -- would be scoped to one process, and every one of those failure modes
    -- involves more than one.
    UNIQUE (template_id, occurrence_key)
);

CREATE INDEX commerce_recurring_basket_generations_customer_idx
    ON commerce_recurring_basket_generations
        (customer_id, occurrence_at DESC, id);

CREATE INDEX commerce_recurring_basket_generations_template_idx
    ON commerce_recurring_basket_generations
        (template_id, occurrence_at DESC);

-- Occurrences the customer still has to answer. This is the query the
-- customer's "waiting for you" list is built from.
CREATE INDEX commerce_recurring_basket_generations_pending_idx
    ON commerce_recurring_basket_generations (customer_id, created_at DESC)
    WHERE state IN ('PROPOSED', 'AWAITING_APPROVAL');
