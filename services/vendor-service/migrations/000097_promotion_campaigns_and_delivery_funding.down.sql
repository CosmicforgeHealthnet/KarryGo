DELETE FROM vendor_role_permissions WHERE permission_id IN ('perm_promotions_read','perm_promotions_manage');
DELETE FROM vendor_permissions WHERE id IN ('perm_promotions_read','perm_promotions_manage');
ALTER TABLE vendor_delivery_policies
    DROP CONSTRAINT IF EXISTS vendor_delivery_hybrid_funding_authority_ck,
    DROP COLUMN IF EXISTS hybrid_platform_approval_ref,
    DROP COLUMN IF EXISTS hybrid_vendor_share_bps,
    DROP COLUMN IF EXISTS hybrid_fallback_funding_sponsor;
DROP TABLE IF EXISTS commerce_delivery_funding_operations;
ALTER TABLE commerce_vendor_orders DROP COLUMN IF EXISTS delivery_funding_snapshot, DROP COLUMN IF EXISTS promotion_snapshot;
ALTER TABLE commerce_parent_orders DROP COLUMN IF EXISTS delivery_funding_snapshot, DROP COLUMN IF EXISTS promotion_snapshot;
ALTER TABLE commerce_quote_adjustments
    DROP COLUMN IF EXISTS applied_at,
    DROP COLUMN IF EXISTS stack_priority,
    DROP COLUMN IF EXISTS delivery_discount_minor,
    DROP COLUMN IF EXISTS calculation_base_minor,
    DROP COLUMN IF EXISTS coupon_digest,
    DROP COLUMN IF EXISTS platform_approval_ref,
    DROP COLUMN IF EXISTS eligibility_evidence,
    DROP COLUMN IF EXISTS campaign_version,
    DROP COLUMN IF EXISTS promotion_type,
    DROP COLUMN IF EXISTS branch_id,
    DROP COLUMN IF EXISTS business_id;
DROP TABLE IF EXISTS commerce_promotion_redemptions;
DROP TABLE IF EXISTS commerce_promotion_coupons;
DROP TABLE IF EXISTS commerce_promotion_quantity_tiers;
DROP TABLE IF EXISTS commerce_promotion_bundle_requirements;
DROP TABLE IF EXISTS commerce_promotion_scopes;
DROP TABLE IF EXISTS commerce_promotion_campaigns;
