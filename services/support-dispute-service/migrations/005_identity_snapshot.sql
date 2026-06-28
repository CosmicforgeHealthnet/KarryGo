-- Workstream G: denormalised identity snapshot resolved from the owning service
-- (customer-service / driver-*-service) at complaint/dispute creation time. This
-- is a copy, not a join/FK across services, so admins see real names without a
-- live lookup and the data survives owning-service downtime.

ALTER TABLE complaints ADD COLUMN IF NOT EXISTS complainant_name  TEXT;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS complainant_phone TEXT;

ALTER TABLE disputes   ADD COLUMN IF NOT EXISTS respondent_name   TEXT;
ALTER TABLE disputes   ADD COLUMN IF NOT EXISTS respondent_phone  TEXT;
