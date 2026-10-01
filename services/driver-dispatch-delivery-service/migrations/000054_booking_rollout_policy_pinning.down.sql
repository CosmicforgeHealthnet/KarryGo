DROP TRIGGER IF EXISTS dispatch_booking_rollout_policy_guard ON dispatch_bookings;
DROP FUNCTION IF EXISTS dispatch_reject_rollout_policy_downgrade();
DROP INDEX IF EXISTS dispatch_bookings_protected_active;
ALTER TABLE dispatch_bookings
    DROP CONSTRAINT IF EXISTS dispatch_booking_rollout_requirements_consistent,
    DROP COLUMN IF EXISTS provider_transfer_required,
    DROP COLUMN IF EXISTS delivery_evidence_required,
    DROP COLUMN IF EXISTS pickup_evidence_required,
    DROP COLUMN IF EXISTS handover_required,
    DROP COLUMN IF EXISTS rollout_policy_version,
    DROP COLUMN IF EXISTS rollout_protection_mode;
