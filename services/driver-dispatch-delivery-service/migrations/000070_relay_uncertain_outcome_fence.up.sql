-- An uncertain provider decision cannot be made safe by a retry budget or an
-- operator replay. Only authoritative reconciliation may resolve it.
ALTER TABLE dispatch_fleet_relay_operations
  ADD COLUMN readback_required boolean NOT NULL DEFAULT false;

-- Legacy dead letters have insufficient provenance to prove no side effect.
UPDATE dispatch_fleet_relay_operations
SET readback_required = true
WHERE status IN ('UNKNOWN_OUTCOME', 'DELIVERING', 'DEAD_LETTER');

CREATE INDEX dispatch_relay_readback_assignment
  ON dispatch_fleet_relay_operations(assignment_id) WHERE readback_required;
CREATE INDEX dispatch_relay_readback_offer_provider
  ON dispatch_fleet_relay_operations(offer_id,provider_id) WHERE readback_required;
