DROP TRIGGER IF EXISTS trg_fleet_document_access_audit_append_only ON fleet_partner_document_access_audit;
DROP TABLE IF EXISTS fleet_partner_document_access_audit;

ALTER TABLE fleet_partner_documents
    DROP CONSTRAINT IF EXISTS fleet_partner_documents_owner_type_check_v2,
    ADD CONSTRAINT fleet_partner_documents_owner_type_check
        CHECK (owner_type IN ('COMPANY', 'BRANCH', 'PROVIDER_LINK', 'VEHICLE_LINK',
                               'INCIDENT', 'DISPUTE', 'APPROVAL'));

ALTER TABLE fleet_partner_document_verifications
    DROP COLUMN IF EXISTS media_checksum_sha256;

ALTER TABLE fleet_partner_documents
    DROP COLUMN IF EXISTS original_size_bytes,
    DROP COLUMN IF EXISTS original_mime_type,
    DROP COLUMN IF EXISTS actor_reference,
    DROP COLUMN IF EXISTS device_reference,
    DROP COLUMN IF EXISTS capture_method,
    DROP COLUMN IF EXISTS source_timestamp,
    DROP COLUMN IF EXISTS captured_at,
    DROP COLUMN IF EXISTS media_checksum_sha256;
