-- Section 17B rollback.

DROP TABLE IF EXISTS commerce_checkout_events;
DROP TABLE IF EXISTS commerce_checkout_lines;
DROP TABLE IF EXISTS commerce_checkout_groups;
DROP TABLE IF EXISTS commerce_checkout_sessions;

-- Restore the Section 17A cart action vocabulary exactly.
ALTER TABLE commerce_cart_history
    DROP CONSTRAINT commerce_cart_history_action_chk;

ALTER TABLE commerce_cart_history
    ADD CONSTRAINT commerce_cart_history_action_chk
    CHECK (
        action IN (
            'commerce.cart.created',
            'commerce.cart.item_added',
            'commerce.cart.item_updated',
            'commerce.cart.item_removed',
            'commerce.cart.cleared',
            'commerce.cart.address_changed',
            'commerce.cart.address_removed',
            'commerce.cart.receiver_changed',
            'commerce.cart.receiver_removed',
            'commerce.cart.abandoned',
            'commerce.cart.expired'
        )
    );
