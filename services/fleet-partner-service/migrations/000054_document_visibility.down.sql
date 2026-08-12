-- Rollback the Section 25 document visibility/archival gap-close.

DELETE FROM fleet_partner_role_permissions WHERE permission_code = 'documents.download_restricted';
DELETE FROM fleet_partner_permissions WHERE code = 'documents.download_restricted';

DROP INDEX IF EXISTS idx_fleet_partner_document_visibility;

ALTER TABLE fleet_partner_documents
    DROP COLUMN IF EXISTS archived_by_actor_id,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS visibility;
