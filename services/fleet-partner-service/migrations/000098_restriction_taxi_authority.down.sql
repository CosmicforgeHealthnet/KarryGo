-- Restore the Dispatch/Hauling-only target list. TAXI operation rows cannot
-- be deleted (their attempts are append-only and reference them), so the
-- narrower check is added NOT VALID: it refuses new TAXI operations without
-- requiring the history that already exists to disappear.
ALTER TABLE fleet_partner_restriction_operations
    DROP CONSTRAINT IF EXISTS fleet_partner_restriction_operations_authority_service_check;
ALTER TABLE fleet_partner_restriction_operations
    ADD CONSTRAINT fleet_partner_restriction_operations_authority_service_check
    CHECK (authority_service IN ('DISPATCH','HAULING')) NOT VALID;
