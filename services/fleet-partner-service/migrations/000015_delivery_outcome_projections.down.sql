-- Reverses WP6.3–6.4 fleet delivery-outcome projections.
--
-- Any row already carrying DELIVERY_FAILED would violate the restored
-- constraint, so it is moved to RECONCILIATION_REQUIRED first: that status
-- says "a human must look", which is exactly true of a failed delivery whose
-- projection vocabulary has just been removed.
UPDATE fleet_partner_marketplace_request_projections
SET projection_status = 'RECONCILIATION_REQUIRED'
WHERE projection_status = 'DELIVERY_FAILED';

ALTER TABLE fleet_partner_marketplace_request_projections
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_request_projection_status;

ALTER TABLE fleet_partner_marketplace_request_projections
    ADD CONSTRAINT fleet_partner_marketplace_request_proje_projection_status_check
    CHECK (projection_status IN (
        'OFFERED', 'RESERVATION_PENDING', 'ORGANIZATION_ACCEPTED', 'ASSIGNMENT_PENDING',
        'PROVIDER_ACCEPTED', 'EXECUTION_REQUESTED', 'EXECUTION_CREATED', 'IN_PROGRESS',
        'COMPLETED', 'ORGANIZATION_DECLINED', 'EXPIRED', 'CANCELLED',
        'RECONCILIATION_REQUIRED'));

DROP INDEX IF EXISTS idx_fleet_trip_projection_unconfirmed;
DROP INDEX IF EXISTS idx_fleet_trip_projection_failed;

ALTER TABLE fleet_partner_trip_projections DROP COLUMN IF EXISTS failure_incident_reference;
ALTER TABLE fleet_partner_trip_projections DROP COLUMN IF EXISTS failure_recipient_unavailable;
ALTER TABLE fleet_partner_trip_projections DROP COLUMN IF EXISTS failure_retry_eligible;
ALTER TABLE fleet_partner_trip_projections DROP COLUMN IF EXISTS failure_reason_category;
ALTER TABLE fleet_partner_trip_projections DROP COLUMN IF EXISTS failure_reason_code;
ALTER TABLE fleet_partner_trip_projections DROP COLUMN IF EXISTS failed_at;

ALTER TABLE fleet_partner_trip_projections DROP COLUMN IF EXISTS recipient_matched_expected;
ALTER TABLE fleet_partner_trip_projections DROP COLUMN IF EXISTS recipient_reference;
ALTER TABLE fleet_partner_trip_projections DROP COLUMN IF EXISTS recipient_confirmation_method;
ALTER TABLE fleet_partner_trip_projections DROP COLUMN IF EXISTS recipient_confirmed_at;
