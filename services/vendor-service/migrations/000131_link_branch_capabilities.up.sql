-- Offer each business's types (restaurant, shop, ...) at its existing branches
-- as DRAFT branch capabilities. New branches and new types are linked by the
-- service from now on; this backfills businesses created before that, whose
-- merchants could not build a menu because no branch offered their type.
INSERT INTO vendor_branch_capabilities(id,business_id,branch_id,vendor_capability_id,capability_type,status)
SELECT 'vbc_'||gen_random_uuid()::text, c.business_id, b.id, c.id, c.capability_type, 'DRAFT'
FROM vendor_capabilities c
JOIN vendor_branches b ON b.business_id=c.business_id
WHERE c.status<>'REJECTED' AND b.status<>'CLOSED'
ON CONFLICT (branch_id,capability_type) DO NOTHING;
