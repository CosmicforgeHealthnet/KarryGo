-- Reverse of 000102_commerce_fulfilment_plan_candidates.up.sql.
--
-- The table owns no foreign keys pointing at it and adds no columns to any
-- existing table, so dropping it is the whole reversal. Its indexes go with
-- it.
--
-- Dropping this table destroys the only record of the alternatives past plans
-- were compared against. The executed plan survives; the answer to "why not
-- the other one" does not, and is not reconstructible afterwards because the
-- prices, routes and runtime capabilities it was decided against have all
-- moved on since.

DROP TABLE IF EXISTS commerce_fulfilment_plan_candidates;
