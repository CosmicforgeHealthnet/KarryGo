-- Reverse of 000010_vendor_crm_foundation. Dropped child-first so foreign
-- keys never block the rollback.
DROP TABLE IF EXISTS crm_privacy_processing_history;
DROP TABLE IF EXISTS crm_privacy_projections;
DROP TABLE IF EXISTS crm_consent_projections;
DROP TABLE IF EXISTS crm_customer_order_facts;
DROP TABLE IF EXISTS crm_customer_branch_relations;
DROP TABLE IF EXISTS crm_customer_identity_links;
DROP TABLE IF EXISTS vendor_customer_profiles;

ALTER TABLE vendor_inbox_processed_events
    DROP COLUMN IF EXISTS payload_fingerprint;
