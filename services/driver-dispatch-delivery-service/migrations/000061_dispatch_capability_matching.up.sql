-- Section 8: bike capability, product eligibility, policy snapshots and match
-- evidence.
--
-- Dispatch's candidate SQL currently decides vehicle suitability by mapping a
-- requested class onto a set of bike_type strings:
--
--   ($2='MOTORCYCLE' AND b.bike_type IN ('motorcycle','dispatch_bike','scooter','electric_bike'))
--
-- That answers "is this roughly the right kind of bike". It cannot answer
-- whether the parcel fits the box, whether the box is closed, whether the rider
-- carries a thermal bag, or whether 18kg exceeds what the machine may legally
-- carry. Those facts have never had anywhere to live. This migration gives them
-- one.
--
-- Backfill philosophy: nothing is inferred. A bike_type tells you almost
-- nothing measurable -- two motorcycles with the same type string can have a
-- top box and no box at all -- so every capability column starts NULL and the
-- fit engine refuses any hard requirement that depends on a NULL. Inventing a
-- box size from a type string is exactly how a fragile parcel ends up bungeed
-- to a rack.

-- ── Bike capability ─────────────────────────────────────────────────────────
--
-- A separate table rather than columns on `bikes`, unlike Hauling's equivalent.
-- The reason is provenance: these are survey observations with their own
-- version and observation time, they are written by a different actor from the
-- registration record, and keeping them apart means a capability write can
-- never accidentally touch a verification status.

CREATE TABLE IF NOT EXISTS dispatch_bike_capabilities (
    bike_id UUID PRIMARY KEY REFERENCES bikes(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,

    -- Payload in grams, matching the parcel dimensions the commerce mission
    -- already carries. NULL means unmeasured, never unlimited.
    max_payload_grams BIGINT
        CHECK (max_payload_grams IS NULL OR max_payload_grams > 0),

    -- The carrier box. All three are needed for the box to count as measured:
    -- a box with a length and no width has not been surveyed, and treating it
    -- as measured would let the missing axes default to zero and refuse
    -- everything with a spurious fit failure.
    box_length_mm BIGINT CHECK (box_length_mm IS NULL OR box_length_mm > 0),
    box_width_mm  BIGINT CHECK (box_width_mm  IS NULL OR box_width_mm  > 0),
    box_height_mm BIGINT CHECK (box_height_mm IS NULL OR box_height_mm > 0),

    -- Three-valued via NULL. enclosed_box is stricter than weather_protected:
    -- a rain cover keeps water off, a locked box also keeps hands out.
    enclosed_box      BOOLEAN,
    weather_protected BOOLEAN,
    upright_safe      BOOLEAN,
    fragile_safe      BOOLEAN,

    -- Cold chain. thermal_bag is the equipment; temperature_class is what it
    -- can actually hold, and an unrecorded class on a bag-equipped bike is
    -- unknown rather than "anything".
    thermal_bag       BOOLEAN,
    temperature_class TEXT NOT NULL DEFAULT '',

    -- capability_version increments on every write, so evidence can pin what a
    -- decision saw and a stale write can be refused.
    capability_version BIGINT NOT NULL DEFAULT 1,
    -- source_version orders callbacks from an authority that projects a fact
    -- here (verification, compliance).
    source_version BIGINT NOT NULL DEFAULT 0,

    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A box opening cannot exceed the box. Storing an impossible geometry
    -- would make the fit engine accept parcels that cannot physically go in.
    CONSTRAINT chk_bike_capability_temperature
        CHECK (temperature_class = '' OR thermal_bag IS TRUE)
);

CREATE INDEX IF NOT EXISTS idx_dispatch_bike_capabilities_provider
    ON dispatch_bike_capabilities (provider_id);

COMMENT ON TABLE dispatch_bike_capabilities IS
'Surveyed physical capability per bike. NULL means unmeasured, which the fit engine refuses for any hard requirement -- never "unlimited" and never inferred from bike_type.';
COMMENT ON COLUMN dispatch_bike_capabilities.enclosed_box IS
'Stricter than weather_protected: an enclosed box also secures the parcel. NULL means the bike has not been surveyed.';

-- ── Product eligibility ─────────────────────────────────────────────────────
--
-- A bike_type is not a certification. "dispatch_bike" says what the machine
-- is; admission to a cold-chain or high-value product says somebody checked
-- the rider and the equipment.

CREATE TABLE IF NOT EXISTS dispatch_vehicle_product_eligibility (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    bike_id UUID NOT NULL REFERENCES bikes(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,

    product_code TEXT NOT NULL,

    status TEXT NOT NULL
        CHECK (status IN ('PENDING', 'ELIGIBLE', 'SUSPENDED', 'EXPIRED', 'REJECTED')),

    policy_version TEXT NOT NULL DEFAULT '',
    certification_ref TEXT NOT NULL DEFAULT '',

    effective_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,

    reason_code TEXT NOT NULL DEFAULT '',

    -- Orders callbacks from the certifying authority. A message carrying a
    -- version no newer than the stored one changes nothing, so a delayed
    -- "approved" cannot resurrect a certification that was revoked after it
    -- was sent.
    source_version BIGINT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- One row per bike per product. Without this a bike could hold both an
    -- ELIGIBLE and a SUSPENDED row and the answer would depend on row order.
    CONSTRAINT uq_dispatch_vehicle_product UNIQUE (bike_id, product_code),

    CONSTRAINT chk_dispatch_eligibility_window
        CHECK (expires_at IS NULL OR effective_at IS NULL OR expires_at > effective_at)
);

CREATE INDEX IF NOT EXISTS idx_dispatch_vehicle_eligibility_lookup
    ON dispatch_vehicle_product_eligibility (bike_id, status);
CREATE INDEX IF NOT EXISTS idx_dispatch_vehicle_eligibility_expiry
    ON dispatch_vehicle_product_eligibility (expires_at)
    WHERE status = 'ELIGIBLE' AND expires_at IS NOT NULL;

COMMENT ON TABLE dispatch_vehicle_product_eligibility IS
'Formal per-bike admission to a service product. A legacy bike_type never substitutes for this when a product requires certification.';

-- ── Policy snapshots ────────────────────────────────────────────────────────
--
-- Assignment must not make the admin service a synchronous dependency: an
-- admin outage would then stop every dispatch. Policy is pulled and stored
-- locally, and matching reads the local copy.

CREATE TABLE IF NOT EXISTS dispatch_policy_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    policy_kind TEXT NOT NULL,
    policy_version TEXT NOT NULL,

    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    checksum TEXT NOT NULL DEFAULT '',

    effective_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Immutable: committed missions pin a version, so mutating a snapshot in
    -- place would silently rewrite the rules a past decision was made under.
    CONSTRAINT uq_dispatch_policy_snapshot UNIQUE (policy_kind, policy_version)
);

CREATE INDEX IF NOT EXISTS idx_dispatch_policy_snapshots_current
    ON dispatch_policy_snapshots (policy_kind, effective_at DESC);

-- ── Match decision evidence ─────────────────────────────────────────────────
--
-- Operator evidence for hard eligibility and final selection. It is NEVER
-- returned to a customer or to Vendor: the set of providers considered is
-- exactly the information that would let a caller infer, and then try to
-- steer, the assignment.

CREATE TABLE IF NOT EXISTS dispatch_match_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    mission_id TEXT NOT NULL DEFAULT '',
    booking_id TEXT NOT NULL DEFAULT '',

    provider_id UUID NOT NULL,
    bike_id UUID,

    decision TEXT NOT NULL CHECK (decision IN ('ELIGIBLE', 'REJECTED', 'SELECTED')),

    -- Comma-separated stable reason codes from the shared Appendix B
    -- vocabulary. Text rather than an enum because the catalogue is shared
    -- across services and grows there, not here.
    reason_codes TEXT NOT NULL DEFAULT '',

    -- Bounded, low-cardinality evidence. Never identity beyond the two ids
    -- above, never customer data: this is an operations table, not analytics.
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,

    distance_m BIGINT NOT NULL DEFAULT 0,

    capability_version BIGINT NOT NULL DEFAULT 0,
    policy_version TEXT NOT NULL DEFAULT '',

    -- shadow marks a decision computed for comparison only, while the legacy
    -- matcher still chose. It is what makes the rollout measurable before it
    -- is enforced.
    shadow BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dispatch_match_decisions_mission
    ON dispatch_match_decisions (mission_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dispatch_match_decisions_retention
    ON dispatch_match_decisions (created_at);

COMMENT ON TABLE dispatch_match_decisions IS
'Operator evidence for hard-fit outcomes. Never exposed to customers or Vendor: the candidate set would reveal, and invite steering of, the assignment.';

-- ── Commerce mission capability evidence ────────────────────────────────────
--
-- The mission already pins agreed_fee_minor and its quote; these pin what it
-- was admitted under. They are added only where the newer multi-stop schema
-- does not already carry them, and none of them is ever rewritten after
-- commitment: an immutable evidence field that can be updated is not evidence.

ALTER TABLE commerce_missions
    ADD COLUMN IF NOT EXISTS service_product_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS service_product_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS requirement_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS capability_policy_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS guarantee_profile_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS commitment_reference TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN commerce_missions.requirement_snapshot IS
'The physical requirement this mission was admitted under, frozen at commitment. Never rewritten: an evidence field that can be updated is not evidence.';
