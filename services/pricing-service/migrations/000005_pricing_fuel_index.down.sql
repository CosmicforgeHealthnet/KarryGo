-- Reverse of 000005. The system operator stays: removing it could orphan
-- references in the audit log, and it can propose nothing without this table.
DROP TRIGGER IF EXISTS pricing_fuel_proposals_append_only ON pricing_fuel_proposals;
DROP TRIGGER IF EXISTS pricing_fuel_reports_append_only ON pricing_fuel_reports;

DROP TABLE IF EXISTS pricing_fuel_proposals;
DROP TABLE IF EXISTS pricing_fuel_index;
DROP TABLE IF EXISTS pricing_fuel_reports;
