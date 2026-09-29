ALTER TABLE commerce_checkout_lines
    DROP CONSTRAINT IF EXISTS commerce_checkout_lines_food_exact_chk,
    DROP COLUMN IF EXISTS substitution_policy;

ALTER TABLE commerce_cart_items
    DROP CONSTRAINT IF EXISTS commerce_cart_items_food_exact_chk,
    DROP COLUMN IF EXISTS substitution_policy;
