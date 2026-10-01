-- A receiver account is optional and separate from contact evidence. If a
-- third-party receiver supplies only a phone, custody credentials remain
-- unavailable until the sender provides a supported verified channel.
ALTER TABLE commerce_cart_receivers
    ADD COLUMN receiver_customer_id uuid;
