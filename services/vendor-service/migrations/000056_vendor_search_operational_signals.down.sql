DROP INDEX IF EXISTS shop_search_documents_operational_idx;
DROP INDEX IF EXISTS food_search_documents_operational_idx;

ALTER TABLE shop_search_documents
    DROP COLUMN IF EXISTS promotion_indicator,
    DROP COLUMN IF EXISTS commercial_signal,
    DROP COLUMN IF EXISTS commercial_revision,
    DROP COLUMN IF EXISTS reliability_revision,
    DROP COLUMN IF EXISTS reliability_status,
    DROP COLUMN IF EXISTS trust_revision,
    DROP COLUMN IF EXISTS trust_status,
    DROP COLUMN IF EXISTS serviceability_revision,
    DROP COLUMN IF EXISTS serviceability_status,
    DROP COLUMN IF EXISTS inventory_status;

ALTER TABLE food_search_documents
    DROP COLUMN IF EXISTS commercial_signal,
    DROP COLUMN IF EXISTS commercial_revision,
    DROP COLUMN IF EXISTS reliability_revision,
    DROP COLUMN IF EXISTS reliability_status,
    DROP COLUMN IF EXISTS trust_revision,
    DROP COLUMN IF EXISTS trust_status,
    DROP COLUMN IF EXISTS serviceability_revision,
    DROP COLUMN IF EXISTS serviceability_status,
    DROP COLUMN IF EXISTS availability_revision,
    DROP COLUMN IF EXISTS availability_status;