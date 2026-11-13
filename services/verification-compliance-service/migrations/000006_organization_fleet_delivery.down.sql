DROP INDEX IF EXISTS verification_outbox_organization_delivery_idx;

-- Narrowing back to uuid is only possible while every value is a UUID.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM organization_verification_submissions
            WHERE organization_id !~* '^[0-9a-f-]{36}$' OR submission_id !~* '^[0-9a-f-]{36}$')
    OR EXISTS (SELECT 1 FROM verification_outbox WHERE aggregate_id !~* '^[0-9a-f-]{36}$') THEN
  RAISE EXCEPTION 'non-UUID organization identifiers present; rollback refused';
 END IF;
END $$;

ALTER TABLE verification_outbox
  ALTER COLUMN aggregate_id TYPE uuid USING aggregate_id::uuid;
ALTER TABLE organization_verification_submissions
  ALTER COLUMN organization_id TYPE uuid USING organization_id::uuid,
  ALTER COLUMN submission_id TYPE uuid USING submission_id::uuid;
