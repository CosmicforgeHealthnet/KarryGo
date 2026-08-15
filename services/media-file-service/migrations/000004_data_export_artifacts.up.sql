-- Section 8H support: private data-export artifacts.
--
-- This migration is deliberately small. It adds two facts the export path
-- needs and nothing else: how an object may be reached, and what it was
-- called. No existing column, constraint or behaviour changes, so every
-- document, image and proof purpose keeps exactly the semantics it had.
--
-- The reason a visibility column exists at all is that DATA_EXPORT must never
-- be publicly reachable, and "the private contract has no public path today"
-- is an accident of the current surface rather than a rule. Writing the rule
-- down as a CHECK makes it survive whatever surface is added next: a
-- DATA_EXPORT object marked PUBLIC is unstorable, not merely unwritten.

ALTER TABLE media_objects
    ADD COLUMN visibility text NOT NULL DEFAULT 'PRIVATE'
        CHECK (visibility IN ('PRIVATE', 'PUBLIC'));
ALTER TABLE media_objects
    ADD COLUMN file_name text NOT NULL DEFAULT '';

ALTER TABLE media_upload_authorizations
    ADD COLUMN visibility text NOT NULL DEFAULT 'PRIVATE'
        CHECK (visibility IN ('PRIVATE', 'PUBLIC'));
ALTER TABLE media_upload_authorizations
    ADD COLUMN file_name text NOT NULL DEFAULT '';

-- The rule, in the one place it cannot be forgotten. A generated export is
-- customer data in bulk; there is no configuration, caller or future endpoint
-- that should be able to make one publicly reachable.
ALTER TABLE media_objects
    ADD CONSTRAINT chk_media_object_data_export_private CHECK (
        upload_purpose <> 'DATA_EXPORT' OR visibility = 'PRIVATE');
ALTER TABLE media_upload_authorizations
    ADD CONSTRAINT chk_upload_authorization_data_export_private CHECK (
        upload_purpose <> 'DATA_EXPORT' OR visibility = 'PRIVATE');

-- Retention sweeps already index retention_until. Export artifacts are
-- short-lived and numerous relative to documents, so the purpose is worth
-- narrowing on when the sweeper looks for them.
CREATE INDEX IF NOT EXISTS idx_media_objects_data_export_retention
    ON media_objects (retention_until)
    WHERE upload_purpose = 'DATA_EXPORT' AND retention_until IS NOT NULL;
