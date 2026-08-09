-- Rollback the Section 14 service-layer structures.
--
-- `000023` is left exactly as it was applied.

DROP INDEX IF EXISTS idx_fleet_crm_contracts_renewal;
DROP INDEX IF EXISTS idx_fleet_crm_sla_observations_outcome;
DROP INDEX IF EXISTS idx_fleet_crm_sla_observation_source;

ALTER TABLE fleet_partner_sla_observations DROP COLUMN IF EXISTS causation_id;
ALTER TABLE fleet_partner_sla_observations DROP COLUMN IF EXISTS payload_hash;
ALTER TABLE fleet_partner_sla_observations DROP COLUMN IF EXISTS event_id;

ALTER TABLE fleet_partner_crm_notification_operations DROP COLUMN IF EXISTS dead_letter_reason;
ALTER TABLE fleet_partner_crm_notification_operations DROP COLUMN IF EXISTS causation_id;
ALTER TABLE fleet_partner_quotation_conversion_intents DROP COLUMN IF EXISTS causation_id;
ALTER TABLE fleet_partner_quotation_acceptances DROP COLUMN IF EXISTS causation_id;

ALTER TABLE fleet_partner_customer_contacts DROP COLUMN IF EXISTS consent_source;
ALTER TABLE fleet_partner_contracts DROP COLUMN IF EXISTS activated_at;
ALTER TABLE fleet_partner_quotations DROP COLUMN IF EXISTS resolved_at;
ALTER TABLE fleet_partner_quotations DROP COLUMN IF EXISTS issued_at;
ALTER TABLE fleet_partner_quotations DROP COLUMN IF EXISTS internal_notes;
ALTER TABLE fleet_partner_sales_opportunities DROP COLUMN IF EXISTS hold_reason;

DROP INDEX IF EXISTS idx_fleet_crm_quotation_tokens_quotation;
DROP INDEX IF EXISTS idx_fleet_crm_quotation_tokens_hash;
DROP TABLE IF EXISTS fleet_partner_quotation_access_tokens;

DROP INDEX IF EXISTS idx_fleet_crm_quotation_documents;
DROP TABLE IF EXISTS fleet_partner_quotation_documents;

DROP TRIGGER IF EXISTS trg_fleet_crm_sla_history_append_only
    ON fleet_partner_sla_history;
DROP INDEX IF EXISTS idx_fleet_crm_sla_history;
DROP TABLE IF EXISTS fleet_partner_sla_history;

DROP TRIGGER IF EXISTS trg_fleet_crm_quotation_history_append_only
    ON fleet_partner_quotation_history;
DROP INDEX IF EXISTS idx_fleet_crm_quotation_history;
DROP TABLE IF EXISTS fleet_partner_quotation_history;

DROP TRIGGER IF EXISTS trg_fleet_crm_location_history_append_only
    ON fleet_partner_crm_location_history;
DROP INDEX IF EXISTS idx_fleet_crm_location_history;
DROP TABLE IF EXISTS fleet_partner_crm_location_history;

DROP TRIGGER IF EXISTS trg_fleet_crm_contact_history_append_only
    ON fleet_partner_crm_contact_history;
DROP INDEX IF EXISTS idx_fleet_crm_contact_history;
DROP TABLE IF EXISTS fleet_partner_crm_contact_history;

DROP TRIGGER IF EXISTS trg_fleet_crm_customer_history_append_only
    ON fleet_partner_crm_customer_history;
DROP INDEX IF EXISTS idx_fleet_crm_customer_history;
DROP TABLE IF EXISTS fleet_partner_crm_customer_history;
