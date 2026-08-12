-- Section 25A: authoritative Media checksum, capture metadata, an ORDER
-- owner type (the missing link for customer-linked documents — an order
-- already carries customer_id), and an append-only access-audit table.
--
-- Every new document column here is populated ONLY from
-- internal/features/documents/usecases/ingestion.go's media-event path
-- (applyMediaState) — never from RequestUpload's caller-supplied input, and
-- never overwritten by a lower media source_version than what is already
-- stored (media_source_version's existing forward-only guard already
-- protects every column this migration adds, since they are all set in the
-- same function, under the same version check, as the fields that guard
-- already protects).

ALTER TABLE fleet_partner_documents
    ADD COLUMN IF NOT EXISTS media_checksum_sha256 text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS captured_at timestamptz,
    ADD COLUMN IF NOT EXISTS source_timestamp timestamptz,
    ADD COLUMN IF NOT EXISTS capture_method text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS device_reference text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS actor_reference text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS original_mime_type text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS original_size_bytes bigint NOT NULL DEFAULT 0 CHECK (original_size_bytes >= 0);

-- The verification decision retains the checksum the decision was actually
-- taken against, alongside the media_source_version it already stored — a
-- decision must remain answerable ("what exact file were you looking at")
-- even after the document's own projection has since moved on.
ALTER TABLE fleet_partner_document_verifications
    ADD COLUMN IF NOT EXISTS media_checksum_sha256 text NOT NULL DEFAULT '';

-- Widen owner_type additively to add ORDER — the missing link that lets a
-- document be tied to a private order, which itself carries customer_id, so
-- a customer-facing surface has something concrete to authorize against.
DO $$
DECLARE
    old_check text;
BEGIN
    SELECT con.conname INTO old_check
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    WHERE rel.relname = 'fleet_partner_documents'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) ILIKE '%owner_type%'
      AND pg_get_constraintdef(con.oid) ILIKE '%COMPANY%';
    IF old_check IS NOT NULL THEN
        EXECUTE 'ALTER TABLE fleet_partner_documents DROP CONSTRAINT ' || quote_ident(old_check);
    END IF;
END $$;

ALTER TABLE fleet_partner_documents
    ADD CONSTRAINT fleet_partner_documents_owner_type_check_v2
    CHECK (owner_type IN ('COMPANY', 'BRANCH', 'PROVIDER_LINK', 'VEHICLE_LINK',
                           'INCIDENT', 'DISPUTE', 'APPROVAL', 'ORDER'));

-- ---------------------------------------------------------------------------
-- Access audit
-- ---------------------------------------------------------------------------
--
-- Every restricted, provider-facing, customer-facing, proof, incident,
-- compliance and financial file access is recorded here, append-only. No
-- signed URL, no download token, ever — only that access was authorized (or
-- refused, and why).

CREATE TABLE IF NOT EXISTS fleet_partner_document_access_audit (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    document_id         text NOT NULL,
    actor_type          text NOT NULL,
    actor_reference     text NOT NULL DEFAULT '',
    subject_type        text NOT NULL DEFAULT '',
    subject_reference   text NOT NULL DEFAULT '',
    visibility          text NOT NULL DEFAULT '',
    action              text NOT NULL CHECK (action IN ('LIST', 'READ', 'DOWNLOAD_REQUEST')),
    result              text NOT NULL CHECK (result IN ('ALLOWED', 'DENIED')),
    denial_reason_code  text NOT NULL DEFAULT '',
    correlation_id      text NOT NULL DEFAULT '',
    occurred_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_fleet_document_access_audit_document
    ON fleet_partner_document_access_audit (organization_id, document_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_document_access_audit_subject
    ON fleet_partner_document_access_audit (organization_id, subject_type, subject_reference, occurred_at DESC);
CREATE TRIGGER trg_fleet_document_access_audit_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_document_access_audit
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
