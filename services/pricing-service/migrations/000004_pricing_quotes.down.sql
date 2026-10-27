-- Reverse of 000004.
DROP TRIGGER IF EXISTS pricing_quote_reconciliations_append_only ON pricing_quote_reconciliations;
DROP TRIGGER IF EXISTS pricing_quote_bindings_append_only ON pricing_quote_bindings;
DROP TRIGGER IF EXISTS pricing_quotes_append_only ON pricing_quotes;

DROP TABLE IF EXISTS pricing_quote_reconciliations;
DROP TABLE IF EXISTS pricing_quote_bindings;
DROP TABLE IF EXISTS pricing_quotes;
