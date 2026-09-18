-- Section 9: truck capability, product eligibility, policy snapshots and match
-- evidence.
--
-- Hauling previously matched cargo on two facts: capacity in kilograms and an
-- exact truck-type string. Everything else a load actually needs -- whether it
-- fits through the door, whether the body is closed, whether the truck can
-- hold -18C, whether a tail lift exists -- was either unrecorded or recorded
-- only as free text on the provider profile. This migration gives those facts
-- a home so the fit engine has something to read.
--
-- Backfill philosophy: conservative to the point of stubbornness. NULL means
-- "nobody has told us", and the fit engine refuses any hard requirement that
-- depends on a NULL. Nothing here invents a measurement, and nothing here
-- copies a fact that the truck TYPE already implies -- internal/truckcatalog
-- supplies those at evaluation time, so writing them into rows would blur a
-- type-level default into a vehicle-level observation and make it impossible
-- to tell later which trucks were genuinely surveyed.

-- ── Physical capability on the authoritative truck row ───────────────────────
--
-- These live on `trucks` rather than in a side table because they describe the
-- truck itself, they are read on every match, and a join per candidate on the
-- hot matching path buys nothing. They are all nullable: a fleet that has
-- never been surveyed keeps working for the legacy weight-only bookings it
-- already served.

ALTER TABLE trucks
    -- Payload, normalized to grams to match every other physical quantity on
    -- the platform. capacity_kg stays for backward compatibility.
    ADD COLUMN IF NOT EXISTS max_payload_grams BIGINT
        CHECK (max_payload_grams IS NULL OR max_payload_grams > 0),

    -- Usable cargo body.
    ADD COLUMN IF NOT EXISTS internal_length_mm BIGINT
        CHECK (internal_length_mm IS NULL OR internal_length_mm > 0),
    ADD COLUMN IF NOT EXISTS internal_width_mm BIGINT
        CHECK (internal_width_mm IS NULL OR internal_width_mm > 0),
    ADD COLUMN IF NOT EXISTS internal_height_mm BIGINT
        CHECK (internal_height_mm IS NULL OR internal_height_mm > 0),
    ADD COLUMN IF NOT EXISTS usable_volume_ml BIGINT
        CHECK (usable_volume_ml IS NULL OR usable_volume_ml > 0),

    -- The doorway. Tracked apart from the body because a load that fits the
    -- body may still not pass the opening, and on an open deck there is no
    -- opening at all.
    ADD COLUMN IF NOT EXISTS opening_width_mm BIGINT
        CHECK (opening_width_mm IS NULL OR opening_width_mm > 0),
    ADD COLUMN IF NOT EXISTS opening_height_mm BIGINT
        CHECK (opening_height_mm IS NULL OR opening_height_mm > 0),

    -- Protection and orientation. Left NULL for every existing truck: what the
    -- type implies is applied at read time from the catalogue, so a NULL here
    -- means "this vehicle has not been surveyed" rather than "unknown body".
    ADD COLUMN IF NOT EXISTS enclosed BOOLEAN,
    ADD COLUMN IF NOT EXISTS weather_protected BOOLEAN,
    ADD COLUMN IF NOT EXISTS upright_safe BOOLEAN,
    ADD COLUMN IF NOT EXISTS fragile_safe BOOLEAN,

    -- Temperature. refrigerated is three-valued via NULL; the band is only
    -- meaningful once refrigerated is true, and an unrecorded band on a reefer
    -- is unknown rather than unlimited.
    ADD COLUMN IF NOT EXISTS refrigerated BOOLEAN,
    ADD COLUMN IF NOT EXISTS temperature_min_c SMALLINT,
    ADD COLUMN IF NOT EXISTS temperature_max_c SMALLINT,
    ADD COLUMN IF NOT EXISTS temperature_class TEXT NOT NULL DEFAULT '',

    -- Loading equipment.
    ADD COLUMN IF NOT EXISTS tail_lift BOOLEAN,
    ADD COLUMN IF NOT EXISTS crane BOOLEAN,
    ADD COLUMN IF NOT EXISTS hydraulic BOOLEAN,
    ADD COLUMN IF NOT EXISTS loading_ramp BOOLEAN,

    -- Configuration. axle_count is the normalized integer; number_of_axles
    -- (free text, migration 005) stays because the provider form still writes
    -- it and historical rows depend on it.
    ADD COLUMN IF NOT EXISTS axle_count INT
        CHECK (axle_count IS NULL OR axle_count > 0),
    ADD COLUMN IF NOT EXISTS trailer_capable BOOLEAN,
    ADD COLUMN IF NOT EXISTS supported_container_sizes TEXT[] NOT NULL DEFAULT '{}',

    -- Goods. supported is a permit list, prohibited a deny list; the deny list
    -- always wins. goods_types (migration 005) is the provider's free-text
    -- selection and is deliberately NOT promoted into either, because its
    -- vocabulary was never constrained and silently reinterpreting it as a
    -- permit list would start refusing cargo nobody meant to refuse.
    ADD COLUMN IF NOT EXISTS supported_goods_classes TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS prohibited_goods_classes TEXT[] NOT NULL DEFAULT '{}',

    -- Compliance projections and provenance.
    ADD COLUMN IF NOT EXISTS insurance_valid BOOLEAN,
    ADD COLUMN IF NOT EXISTS inspection_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS certification_ref TEXT NOT NULL DEFAULT '',

    -- capability_version increments on every capability write, so evidence can
    -- pin what a decision saw and a stale write can be refused outright.
    ADD COLUMN IF NOT EXISTS capability_version BIGINT NOT NULL DEFAULT 0,
    -- capability_source_version orders callbacks from the authority that owns
    -- a projected fact (verification, compliance).
    ADD COLUMN IF NOT EXISTS capability_source_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS capability_observed_at TIMESTAMPTZ;

-- The one backfill, and the only one that is safe: capacity_kg is a real
-- measurement a provider entered, so converting it loses nothing. Rows with a
-- non-positive capacity stay NULL, which exactly reproduces the old matcher's
-- `if truck.CapacityKg > 0` guard -- except that an unknown payload now
-- REFUSES a weighted load instead of skipping the check.
UPDATE trucks
   SET max_payload_grams = capacity_kg::BIGINT * 1000
 WHERE max_payload_grams IS NULL
   AND capacity_kg > 0;

-- has_insurance defaults to false, so a false is indistinguishable from "the
-- provider never answered". Only a true is promoted; a false stays NULL and
-- therefore never grounds an existing truck on the strength of a default.
UPDATE trucks
   SET insurance_valid = TRUE
 WHERE insurance_valid IS NULL
   AND has_insurance IS TRUE;

COMMENT ON COLUMN trucks.max_payload_grams IS
'Normalized payload. NULL means unmeasured, which the fit engine treats as unknown and refuses for any weighted load -- never as unlimited.';
COMMENT ON COLUMN trucks.enclosed IS
'NULL means this vehicle has not been surveyed. What the truck TYPE implies is applied at read time from internal/truckcatalog, so this column records only vehicle-level observation.';
COMMENT ON COLUMN trucks.capability_version IS
'Increments on every capability write. Pinned into match evidence and used to reject stale writes.';

-- ── Product eligibility ─────────────────────────────────────────────────────
--
-- A truck type is not a certification. "refrigerated" says the body cools;
-- it does not say anyone has inspected the unit and admitted it to the
-- cold-chain product. This table is where that admission lives.

CREATE TABLE IF NOT EXISTS hauling_truck_product_eligibility (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    truck_id UUID NOT NULL REFERENCES trucks(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES truck_providers(id) ON DELETE CASCADE,

    product_code TEXT NOT NULL,

    status TEXT NOT NULL
        CHECK (status IN ('PENDING', 'ELIGIBLE', 'SUSPENDED', 'EXPIRED', 'REJECTED')),

    policy_version TEXT NOT NULL DEFAULT '',
    certification_ref TEXT NOT NULL DEFAULT '',

    effective_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,

    reason_code TEXT NOT NULL DEFAULT '',

    -- source_version orders callbacks from the certifying authority. A
    -- callback carrying an older version than the row already holds is
    -- ignored, so a retried or delayed message cannot resurrect a revoked
    -- certification.
    source_version BIGINT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- One eligibility row per truck per product. Without this a truck could
    -- hold both an ELIGIBLE and a SUSPENDED row for the same product and the
    -- answer would depend on row order.
    CONSTRAINT uq_hauling_truck_product UNIQUE (truck_id, product_code),

    -- An expiry that precedes its own effective date describes no window at
    -- all and would silently never apply.
    CONSTRAINT chk_hauling_eligibility_window
        CHECK (expires_at IS NULL OR effective_at IS NULL OR expires_at > effective_at)
);

CREATE INDEX IF NOT EXISTS idx_hauling_truck_eligibility_lookup
    ON hauling_truck_product_eligibility (truck_id, status);
CREATE INDEX IF NOT EXISTS idx_hauling_truck_eligibility_provider
    ON hauling_truck_product_eligibility (provider_id, product_code);
CREATE INDEX IF NOT EXISTS idx_hauling_truck_eligibility_expiry
    ON hauling_truck_product_eligibility (expires_at)
    WHERE status = 'ELIGIBLE' AND expires_at IS NOT NULL;

COMMENT ON TABLE hauling_truck_product_eligibility IS
'Formal per-truck admission to a service product. A generic truck_type never substitutes for this when a product requires certification.';

-- ── Policy snapshots ────────────────────────────────────────────────────────
--
-- Matching must not make the admin service a synchronous dependency of every
-- assignment: an admin outage would then stop the road. Policy is pulled and
-- stored locally, and matching reads the local copy.

CREATE TABLE IF NOT EXISTS hauling_policy_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    policy_kind TEXT NOT NULL,
    policy_version TEXT NOT NULL,

    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    checksum TEXT NOT NULL DEFAULT '',

    effective_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Snapshots are immutable: a given (kind, version) is one fixed document.
    -- Committed work pins a version, so mutating one in place would silently
    -- rewrite the rules a past decision was made under.
    CONSTRAINT uq_hauling_policy_snapshot UNIQUE (policy_kind, policy_version)
);

CREATE INDEX IF NOT EXISTS idx_hauling_policy_snapshots_current
    ON hauling_policy_snapshots (policy_kind, effective_at DESC);

COMMENT ON TABLE hauling_policy_snapshots IS
'Immutable local copies of network/product policy. Matching reads these so an admin outage cannot stop assignment.';

-- ── Match decision evidence ─────────────────────────────────────────────────
--
-- Operations needs to answer "why did this booking not match" without
-- reconstructing the fleet's state from memory. This records the hard-fit
-- outcome per candidate.
--
-- It is operator evidence only. It is never returned to a customer or to
-- Vendor, because the set of providers considered for a job is exactly the
-- information that would let a caller infer -- and then try to steer -- the
-- assignment.

CREATE TABLE IF NOT EXISTS hauling_match_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    booking_id UUID NOT NULL,
    mission_ref TEXT NOT NULL DEFAULT '',

    provider_id UUID NOT NULL,
    truck_id UUID,

    decision TEXT NOT NULL CHECK (decision IN ('ELIGIBLE', 'REJECTED', 'SELECTED')),

    -- Comma-separated stable reason codes from the shared Appendix B
    -- vocabulary. Stored as text rather than an enum because the catalogue is
    -- shared across services and grows there, not here.
    reason_codes TEXT NOT NULL DEFAULT '',

    -- Bounded, low-cardinality evidence. Never identity, never free customer
    -- data: this table is retained for operations, not for analytics.
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,

    distance_m BIGINT NOT NULL DEFAULT 0,

    capability_version BIGINT NOT NULL DEFAULT 0,
    policy_version TEXT NOT NULL DEFAULT '',

    -- shadow marks a decision computed for comparison only, while the legacy
    -- matcher still chose. It is what makes the capability rollout measurable
    -- before it is enforced.
    shadow BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_hauling_match_decisions_booking
    ON hauling_match_decisions (booking_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_hauling_match_decisions_retention
    ON hauling_match_decisions (created_at);
CREATE INDEX IF NOT EXISTS idx_hauling_match_decisions_shadow
    ON hauling_match_decisions (shadow, created_at DESC)
    WHERE shadow;

COMMENT ON TABLE hauling_match_decisions IS
'Operator evidence for hard-fit outcomes. Never exposed to customers or Vendor: the candidate set would reveal, and invite steering of, the assignment.';
