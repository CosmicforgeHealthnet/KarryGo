DROP TRIGGER IF EXISTS commerce_quote_events_immutable ON commerce_quote_events;
DROP TRIGGER IF EXISTS commerce_quote_approvals_immutable ON commerce_quote_approvals;
DROP TRIGGER IF EXISTS commerce_quote_adjustments_immutable ON commerce_quote_adjustments;
DROP TRIGGER IF EXISTS commerce_quote_missions_immutable ON commerce_quote_missions;
DROP TRIGGER IF EXISTS commerce_quote_lines_immutable ON commerce_quote_lines;
DROP TRIGGER IF EXISTS commerce_quote_groups_immutable ON commerce_quote_groups;
DROP TRIGGER IF EXISTS commerce_quotes_immutable ON commerce_quotes;

DROP TABLE IF EXISTS commerce_quote_events;
DROP TABLE IF EXISTS commerce_quote_approvals;
DROP TABLE IF EXISTS commerce_quote_adjustments;
DROP TABLE IF EXISTS commerce_quote_missions;
DROP TABLE IF EXISTS commerce_quote_lines;
DROP TABLE IF EXISTS commerce_quote_groups;
DROP TABLE IF EXISTS commerce_quotes;

DROP FUNCTION IF EXISTS commerce_quote_reject_mutation();
