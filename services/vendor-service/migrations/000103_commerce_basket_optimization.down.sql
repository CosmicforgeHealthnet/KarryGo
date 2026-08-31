-- Reverse of 000103_commerce_basket_optimization.up.sql.
--
-- Dropped in dependency order: generations reference both templates and
-- proposals, so it goes first. Nothing outside this migration points at any
-- of the three, so the three drops are the whole reversal.
--
-- Worth stating plainly: dropping commerce_recurring_basket_generations
-- destroys the exactly-once ledger. If the tables were recreated afterwards
-- and a scheduler replayed occurrences that had already been generated, every
-- one of them would generate again, because the rows that would have stopped
-- them are gone. That is correct for a rollback - the migration is being
-- undone - but it is not a table to drop and recreate casually in an
-- environment where a scheduler is running.

DROP TABLE IF EXISTS commerce_recurring_basket_generations;
DROP TABLE IF EXISTS commerce_recurring_basket_templates;
DROP TABLE IF EXISTS commerce_basket_proposals;
