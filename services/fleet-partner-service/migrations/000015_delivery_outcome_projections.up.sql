-- WP6.3–6.4: fleet projections of the two new authoritative delivery outcomes.
--
-- Dispatch owns both facts. Fleet projects them so an organization can see, on
-- its own trip records, that a delivery was confirmed by its recipient or that
-- it failed — without asking dispatch and without inferring either from a
-- status string.
--
-- Two deliberate omissions:
--
--   * no recipient name, phone or address. The confirmation projection stores
--     the opaque surrogate dispatch published and the match verdict, nothing
--     more.
--   * no monetary column of any kind on the failure projection. Whether a
--     failed delivery moves money is the wallet service's to state, and a
--     column here would be an invitation to decide it locally.

-- 1. Recipient confirmation --------------------------------------------------

ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS recipient_confirmed_at timestamptz;

ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS recipient_confirmation_method text;

-- Opaque surrogate minted by dispatch per confirmation. Not an identity.
ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS recipient_reference text;

ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS recipient_matched_expected boolean;

-- 2. Delivery failure --------------------------------------------------------

ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS failed_at timestamptz;

ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS failure_reason_code text;

ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS failure_reason_category text;

ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS failure_retry_eligible boolean;

ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS failure_recipient_unavailable boolean;

-- Opaque pointer to a support-dispute case, when dispatch supplied one. No
-- foreign key: that service owns it.
ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS failure_incident_reference text;

-- Operators list failed deliveries by recency and by whether another attempt
-- is permitted.
CREATE INDEX IF NOT EXISTS idx_fleet_trip_projection_failed
    ON fleet_partner_trip_projections (organization_id, failed_at DESC)
    WHERE failed_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_trip_projection_unconfirmed
    ON fleet_partner_trip_projections (organization_id, updated_at DESC)
    WHERE recipient_confirmed_at IS NULL;

-- 3. Request projection status ------------------------------------------------
--
-- DELIVERY_FAILED is its own projection status. Reusing CANCELLED would tell
-- every downstream reader that the organization or the customer stood the work
-- down, which is a different fact with different consequences; reusing
-- COMPLETED would be worse still.
-- The original constraint was declared inline, so PostgreSQL named it; the
-- replacement is named explicitly so a future migration can find it.
ALTER TABLE fleet_partner_marketplace_request_projections
    DROP CONSTRAINT IF EXISTS fleet_partner_marketplace_request_proje_projection_status_check;

ALTER TABLE fleet_partner_marketplace_request_projections
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_request_projection_status;

ALTER TABLE fleet_partner_marketplace_request_projections
    ADD CONSTRAINT chk_fleet_partner_request_projection_status
    CHECK (projection_status IN (
        'OFFERED', 'RESERVATION_PENDING', 'ORGANIZATION_ACCEPTED', 'ASSIGNMENT_PENDING',
        'PROVIDER_ACCEPTED', 'EXECUTION_REQUESTED', 'EXECUTION_CREATED', 'IN_PROGRESS',
        'COMPLETED', 'ORGANIZATION_DECLINED', 'EXPIRED', 'CANCELLED',
        'RECONCILIATION_REQUIRED', 'DELIVERY_FAILED'));
