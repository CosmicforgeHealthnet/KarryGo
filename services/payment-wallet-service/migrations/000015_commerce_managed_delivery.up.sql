-- Section 12 Part B: corporate delivery inside Commerce.
--
-- Until now a platform Commerce delivery credited the whole frozen delivery
-- payable straight to the executing provider. For an independent rider that is
-- right and stays exactly as it was. For a rider employed by an organization it
-- is wrong in a way nothing downstream can detect: a salaried rider looks like
-- they earned per delivery, the organization is never paid what it is owed, and
-- the two figures that happen to be equal today stop being equal the moment a
-- compensation plan says otherwise.
--
-- What separates them is that the customer's delivery amount is frozen at quote
-- time and the provider's compensation is decided later, by Fleet. They are two
-- different numbers that were only ever one column.

ALTER TABLE commerce_payment_allocations
    -- How this allocation's delivery is executed. UNKNOWN is the honest
    -- default for rows written before this column existed and for an
    -- allocation whose executor is not yet assigned: it is deliberately not
    -- INDEPENDENT, because defaulting to independent would pay a corporate
    -- rider directly the moment anything failed to set it.
    ADD COLUMN IF NOT EXISTS delivery_executor_mode text NOT NULL DEFAULT 'UNKNOWN'
        CHECK (delivery_executor_mode IN ('UNKNOWN', 'INDEPENDENT', 'ORGANIZATION_MANAGED')),

    -- The organization that owns a managed executor's work. Empty for every
    -- independent delivery.
    ADD COLUMN IF NOT EXISTS managed_organization_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS managed_membership_id   text NOT NULL DEFAULT '',

    -- The frozen delivery payable, split the way it was actually released.
    -- Kept apart from delivery_released_minor (which stays the total) so the
    -- question "what did the provider get, and what did the organization get?"
    -- has an answer that does not require re-deriving it from a plan.
    ADD COLUMN IF NOT EXISTS provider_delivery_released_minor     bigint NOT NULL DEFAULT 0
        CHECK (provider_delivery_released_minor >= 0),
    ADD COLUMN IF NOT EXISTS organization_delivery_released_minor bigint NOT NULL DEFAULT 0
        CHECK (organization_delivery_released_minor >= 0);

-- WAL-026: one allocation's delivery can be executed by more than one mission.
-- The allocation's single assigned_provider_type/id pair cannot express that --
-- it records whoever was released last -- so each release records its own
-- executor here.
CREATE TABLE IF NOT EXISTS commerce_delivery_executions (
    id                        text PRIMARY KEY,
    allocation_id             text NOT NULL REFERENCES commerce_payment_allocations(id) ON DELETE CASCADE,
    vendor_order_id           text NOT NULL,

    -- The mission this release settles. One per mission, which is what makes a
    -- second release for the same mission a no-op instead of a second payment.
    completion_reference      text NOT NULL,

    executor_mode             text NOT NULL
        CHECK (executor_mode IN ('INDEPENDENT', 'ORGANIZATION_MANAGED')),
    provider_type             text NOT NULL,
    provider_id               text NOT NULL,
    organization_id           text NOT NULL DEFAULT '',
    membership_id             text NOT NULL DEFAULT '',

    currency                  text NOT NULL,
    -- The customer's frozen amount for this mission, and how it was split.
    -- frozen = provider + organization, always; an organization share is what
    -- the customer paid that the provider was not entitled to, never an extra
    -- charge.
    frozen_delivery_minor     bigint NOT NULL CHECK (frozen_delivery_minor >= 0),
    provider_share_minor      bigint NOT NULL CHECK (provider_share_minor >= 0),
    organization_share_minor  bigint NOT NULL CHECK (organization_share_minor >= 0),

    -- Fleet's decision that produced the split, for a managed execution.
    instruction_id            text NOT NULL DEFAULT '',
    instruction_version       bigint NOT NULL DEFAULT 0,

    idempotency_key           text NOT NULL,
    ledger_reference          text NOT NULL DEFAULT '',
    reversed_at               timestamptz,
    created_at                timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commerce_delivery_execution_split
        CHECK (provider_share_minor + organization_share_minor = frozen_delivery_minor)
);

-- One release per mission per allocation.
CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_delivery_execution_mission
    ON commerce_delivery_executions (allocation_id, completion_reference);

CREATE UNIQUE INDEX IF NOT EXISTS uq_commerce_delivery_execution_idempotency
    ON commerce_delivery_executions (idempotency_key);

CREATE INDEX IF NOT EXISTS idx_commerce_delivery_execution_allocation
    ON commerce_delivery_executions (allocation_id);
