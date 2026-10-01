-- Vendor alone supplies a commerce mission's FOOD/SHOP classification.
-- Existing execution state and product variants remain authority-owned.
ALTER TABLE customer_movement_groupings ADD COLUMN IF NOT EXISTS product_code TEXT
    CHECK (product_code IS NULL OR product_code IN ('FOOD','SHOP'));
