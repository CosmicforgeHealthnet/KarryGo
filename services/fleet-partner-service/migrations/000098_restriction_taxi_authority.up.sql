-- Taxi as a compliance-restriction propagation target.
--
-- driver-taxi-service now holds a restriction authority with the same
-- contract as Dispatch and Hauling (/api/v1/taxi/internal/restrictions), so a
-- restriction's per-authority operation may name TAXI.
ALTER TABLE fleet_partner_restriction_operations
    DROP CONSTRAINT IF EXISTS fleet_partner_restriction_operations_authority_service_check;
ALTER TABLE fleet_partner_restriction_operations
    ADD CONSTRAINT fleet_partner_restriction_operations_authority_service_check
    CHECK (authority_service IN ('DISPATCH','HAULING','TAXI'));
