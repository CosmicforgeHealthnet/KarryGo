-- Reverse of 000101_delivery_requirement_vehicle_selection.up.sql.
--
-- The table owns no foreign keys pointing at it and adds no columns to any
-- existing table, so dropping it is the whole reversal. Its indexes go with
-- it.
--
-- Dropping this table destroys the only record of why past deliveries were
-- assigned the vehicles they were assigned. That is the correct behaviour for
-- a rollback - the migration is being undone - but it is worth stating, since
-- the evidence is not reconstructible from the plan rows that survive.

DROP TABLE IF EXISTS commerce_delivery_requirement_snapshots;
