-- Rollback Section 14 CRM structures.

DELETE FROM fleet_partner_role_permissions WHERE permission_code LIKE 'crm.%';
DELETE FROM fleet_partner_permissions WHERE code LIKE 'crm.%';

ALTER TABLE fleet_partner_contracts
    DROP CONSTRAINT IF EXISTS fleet_partner_contracts_active_version_fkey;
ALTER TABLE fleet_partner_quotations
    DROP CONSTRAINT IF EXISTS fleet_partner_quotations_accepted_version_fkey;
ALTER TABLE fleet_partner_quotations
    DROP CONSTRAINT IF EXISTS fleet_partner_quotations_issued_version_fkey;
ALTER TABLE fleet_partner_company_customers
    DROP CONSTRAINT IF EXISTS fleet_partner_company_customers_credit_profile_fkey;

DROP INDEX IF EXISTS idx_fleet_crm_notifications_subject;
DROP INDEX IF EXISTS idx_fleet_crm_notifications_lease;
DROP INDEX IF EXISTS idx_fleet_crm_notifications_due;
DROP TABLE IF EXISTS fleet_partner_crm_notification_operations;

DROP TRIGGER IF EXISTS trg_fleet_crm_credit_history_append_only
    ON fleet_partner_customer_credit_history;
DROP INDEX IF EXISTS idx_fleet_crm_credit_history;
DROP TABLE IF EXISTS fleet_partner_customer_credit_history;

DROP INDEX IF EXISTS idx_fleet_crm_credit_hold;
DROP TABLE IF EXISTS fleet_partner_customer_credit_profiles;

DROP TRIGGER IF EXISTS trg_fleet_crm_sla_observations_append_only
    ON fleet_partner_sla_observations;
DROP INDEX IF EXISTS idx_fleet_crm_sla_observations;
DROP TABLE IF EXISTS fleet_partner_sla_observations;

DROP INDEX IF EXISTS idx_fleet_crm_sla_active_metric;
DROP INDEX IF EXISTS idx_fleet_crm_sla_customer;
DROP INDEX IF EXISTS idx_fleet_crm_sla_cursor;
DROP TABLE IF EXISTS fleet_partner_service_level_agreements;

DROP TRIGGER IF EXISTS trg_fleet_crm_contract_history_append_only
    ON fleet_partner_contract_history;
DROP INDEX IF EXISTS idx_fleet_crm_contract_history;
DROP TABLE IF EXISTS fleet_partner_contract_history;

DROP INDEX IF EXISTS idx_fleet_crm_contract_documents_contract;
DROP TABLE IF EXISTS fleet_partner_contract_documents;

DROP TRIGGER IF EXISTS trg_fleet_crm_contract_versions_immutable
    ON fleet_partner_contract_versions;
DROP INDEX IF EXISTS idx_fleet_crm_contract_versions_contract;
DROP TABLE IF EXISTS fleet_partner_contract_versions;

DROP INDEX IF EXISTS idx_fleet_crm_contracts_expiry;
DROP INDEX IF EXISTS idx_fleet_crm_contracts_customer;
DROP INDEX IF EXISTS idx_fleet_crm_contracts_status;
DROP INDEX IF EXISTS idx_fleet_crm_contracts_cursor;
DROP INDEX IF EXISTS idx_fleet_crm_contracts_number;
DROP TABLE IF EXISTS fleet_partner_contracts;

DROP INDEX IF EXISTS idx_fleet_crm_conversion_intent_idempotency;
DROP INDEX IF EXISTS idx_fleet_crm_conversion_intent_version;
DROP TABLE IF EXISTS fleet_partner_quotation_conversion_intents;

DROP TRIGGER IF EXISTS trg_fleet_crm_acceptances_immutable
    ON fleet_partner_quotation_acceptances;
DROP INDEX IF EXISTS idx_fleet_crm_acceptance_idempotency;
DROP INDEX IF EXISTS idx_fleet_crm_acceptance_quotation;
DROP TABLE IF EXISTS fleet_partner_quotation_acceptances;

DROP INDEX IF EXISTS idx_fleet_crm_quotation_lines;
DROP TABLE IF EXISTS fleet_partner_quotation_line_items;

DROP TRIGGER IF EXISTS trg_fleet_crm_quotation_versions_immutable
    ON fleet_partner_quotation_versions;
DROP INDEX IF EXISTS idx_fleet_crm_quotation_versions_quotation;
DROP TABLE IF EXISTS fleet_partner_quotation_versions;

DROP INDEX IF EXISTS idx_fleet_crm_quotations_expiry;
DROP INDEX IF EXISTS idx_fleet_crm_quotations_customer;
DROP INDEX IF EXISTS idx_fleet_crm_quotations_status;
DROP INDEX IF EXISTS idx_fleet_crm_quotations_cursor;
DROP INDEX IF EXISTS idx_fleet_crm_quotations_number;
DROP TABLE IF EXISTS fleet_partner_quotations;

DROP TRIGGER IF EXISTS trg_fleet_crm_opportunity_history_append_only
    ON fleet_partner_sales_opportunity_history;
DROP INDEX IF EXISTS idx_fleet_crm_opportunity_history;
DROP TABLE IF EXISTS fleet_partner_sales_opportunity_history;

DROP INDEX IF EXISTS idx_fleet_crm_opportunities_customer;
DROP INDEX IF EXISTS idx_fleet_crm_opportunities_stage;
DROP INDEX IF EXISTS idx_fleet_crm_opportunities_cursor;
DROP TABLE IF EXISTS fleet_partner_sales_opportunities;

DROP INDEX IF EXISTS idx_fleet_crm_locations_type;
DROP INDEX IF EXISTS idx_fleet_crm_locations_customer;
DROP TABLE IF EXISTS fleet_partner_customer_locations;

DROP INDEX IF EXISTS idx_fleet_crm_contacts_customer;
DROP INDEX IF EXISTS idx_fleet_crm_contacts_primary;
DROP TABLE IF EXISTS fleet_partner_customer_contacts;

DROP INDEX IF EXISTS idx_fleet_crm_customers_name;
DROP INDEX IF EXISTS idx_fleet_crm_customers_branch;
DROP INDEX IF EXISTS idx_fleet_crm_customers_status;
DROP INDEX IF EXISTS idx_fleet_crm_customers_cursor;
DROP INDEX IF EXISTS idx_fleet_crm_customers_duplicate;
DROP TABLE IF EXISTS fleet_partner_company_customers;
