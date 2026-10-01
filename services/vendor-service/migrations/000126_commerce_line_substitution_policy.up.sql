-- The customer's substitution rule for a Shop line, captured in the cart and
-- carried unchanged through checkout into the immutable quote evidence, from
-- which the order freezes it into the picking snapshot. Before this every Shop
-- line froze as EXACT_ONLY because nothing upstream recorded a choice, so no
-- vendor could ever propose a replacement.
--
-- Only rules that need no extra parameters are customer-selectable here.
-- Food lines are always EXACT_ONLY.

ALTER TABLE commerce_cart_items
    ADD COLUMN IF NOT EXISTS substitution_policy TEXT NOT NULL DEFAULT 'EXACT_ONLY'
        CONSTRAINT commerce_cart_items_substitution_policy_chk
        CHECK (substitution_policy IN ('EXACT_ONLY','AUTO_REFUND','CONTACT_FIRST','VENDOR_SUGGEST_AND_APPROVE')),
    ADD CONSTRAINT commerce_cart_items_food_exact_chk
        CHECK (domain = 'SHOP' OR substitution_policy = 'EXACT_ONLY');

ALTER TABLE commerce_checkout_lines
    ADD COLUMN IF NOT EXISTS substitution_policy TEXT NOT NULL DEFAULT 'EXACT_ONLY'
        CONSTRAINT commerce_checkout_lines_substitution_policy_chk
        CHECK (substitution_policy IN ('EXACT_ONLY','AUTO_REFUND','CONTACT_FIRST','VENDOR_SUGGEST_AND_APPROVE')),
    ADD CONSTRAINT commerce_checkout_lines_food_exact_chk
        CHECK (domain = 'SHOP' OR substitution_policy = 'EXACT_ONLY');
