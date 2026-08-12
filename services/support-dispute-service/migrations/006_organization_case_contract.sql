-- Authoritative organization-scoped support case contract for Fleet Section 23.
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS organization_reference text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS source_idempotency_key text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS source_version bigint NOT NULL DEFAULT 1 CHECK (source_version > 0);
CREATE UNIQUE INDEX IF NOT EXISTS uq_complaints_source_idempotency
    ON complaints (organization_reference, source_idempotency_key)
    WHERE organization_reference IS NOT NULL AND source_idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_complaints_organization
    ON complaints (organization_reference, updated_at DESC)
    WHERE organization_reference IS NOT NULL;
