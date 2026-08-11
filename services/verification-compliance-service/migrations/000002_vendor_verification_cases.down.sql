DROP INDEX IF EXISTS verification_outbox_vendor_delivery_idx;
ALTER TABLE verification_outbox
 DROP COLUMN IF EXISTS updated_at,
 DROP COLUMN IF EXISTS last_error,
 DROP COLUMN IF EXISTS delivered_at,
 DROP COLUMN IF EXISTS lease_expires_at,
 DROP COLUMN IF EXISTS lease_owner,
 DROP COLUMN IF EXISTS next_attempt_at,
 DROP COLUMN IF EXISTS delivery_attempts,
 DROP COLUMN IF EXISTS delivery_status;
DROP TABLE IF EXISTS vendor_verification_case_history;
DROP TABLE IF EXISTS vendor_verification_cases;
