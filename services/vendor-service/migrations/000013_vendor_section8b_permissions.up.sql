-- Section 8B permissions: tags and segmentation.
--
-- Five permissions, not one per endpoint. Bulk tagging is the same authority as
-- tagging, and previewing or materializing a definition is the same authority as
-- reading or managing it, so splitting those apart would only create ways for a
-- role to be half-granted. Listing a segment's actual members is separate,
-- because naming the customers in an audience is a strictly larger disclosure
-- than reading the audience's definition and size.
--
-- None is high-risk and none requires step-up: Section 8B produces audiences and
-- sends nothing. Marketing eligibility is still decided by the Section 8A
-- consent and privacy rules at send time, never by segment membership.
INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_crm_tags_read','vendor.crm.tags.read','View vendor-defined customer tags and tag membership.','crm',false),
 ('perm_vendor_crm_tags_manage','vendor.crm.tags.manage','Create, rename, archive and assign vendor-defined customer tags.','crm',false),
 ('perm_vendor_crm_segments_read','vendor.crm.segments.read','View customer segment definitions, previews and evaluation state.','crm',false),
 ('perm_vendor_crm_segments_manage','vendor.crm.segments.manage','Create, edit, materialize and maintain customer segments.','crm',false),
 ('perm_vendor_crm_segment_members_read','vendor.crm.segments.members.read','List the customers who belong to a segment.','crm',false)
ON CONFLICT(id) DO NOTHING;

-- Owner and General Manager: the full segmentation surface.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_owner',id FROM vendor_permissions
WHERE code IN ('vendor.crm.tags.read','vendor.crm.tags.manage','vendor.crm.segments.read',
               'vendor.crm.segments.manage','vendor.crm.segments.members.read')
ON CONFLICT DO NOTHING;
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_general_manager',id FROM vendor_permissions
WHERE code IN ('vendor.crm.tags.read','vendor.crm.tags.manage','vendor.crm.segments.read',
               'vendor.crm.segments.manage','vendor.crm.segments.members.read')
ON CONFLICT DO NOTHING;

-- CRM / Support: the day-to-day CRM role owns tagging and segmentation.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_crm_support',id FROM vendor_permissions
WHERE code IN ('vendor.crm.tags.read','vendor.crm.tags.manage','vendor.crm.segments.read',
               'vendor.crm.segments.manage','vendor.crm.segments.members.read')
ON CONFLICT DO NOTHING;

-- Marketing: building audiences is this role's core work, so it gets the full
-- tag and segment surface including member reads. It still gets no campaign
-- execution — Section 8B does not send anything, and Sections 8E/8F will need
-- their own permissions.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_marketing',id FROM vendor_permissions
WHERE code IN ('vendor.crm.tags.read','vendor.crm.tags.manage','vendor.crm.segments.read',
               'vendor.crm.segments.manage','vendor.crm.segments.members.read')
ON CONFLICT DO NOTHING;

-- Branch manager: tags and segments, but branch-scoped by assignment (Section 5)
-- and structurally confined to BRANCH-scope objects for their own branches by
-- the usecase layer. Without an explicit business-wide grant they cannot create
-- a BUSINESS-scope tag or segment, cannot preview at business scope, and cannot
-- see a business-wide member count.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions
WHERE code IN ('vendor.crm.tags.read','vendor.crm.tags.manage','vendor.crm.segments.read',
               'vendor.crm.segments.manage','vendor.crm.segments.members.read')
ON CONFLICT DO NOTHING;

-- Analyst / Auditor: read and preview only. No mutation, matching the Section 8A
-- treatment of the same role.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',id FROM vendor_permissions
WHERE code IN ('vendor.crm.tags.read','vendor.crm.segments.read','vendor.crm.segments.members.read')
ON CONFLICT DO NOTHING;

-- Deliberately unmapped: role_finance, role_kitchen_pastry, role_shop_inventory,
-- role_procurement, role_vendor_rider — none has a Section 8B segmentation need.
