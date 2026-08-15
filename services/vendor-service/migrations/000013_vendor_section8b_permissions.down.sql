-- Reverse Section 8B permissions. Role mappings first, then the permissions
-- themselves; nothing from Sections 5-8A is touched.
DELETE FROM vendor_role_permissions WHERE permission_id IN (
 'perm_vendor_crm_tags_read','perm_vendor_crm_tags_manage',
 'perm_vendor_crm_segments_read','perm_vendor_crm_segments_manage',
 'perm_vendor_crm_segment_members_read');
DELETE FROM vendor_permissions WHERE id IN (
 'perm_vendor_crm_tags_read','perm_vendor_crm_tags_manage',
 'perm_vendor_crm_segments_read','perm_vendor_crm_segments_manage',
 'perm_vendor_crm_segment_members_read');
