-- Reverses 011_organization_case_messages.sql.
--
-- Kept in migrations/down/ because the runner applies every file directly in
-- migrations/ on every start. Run this by hand, deliberately.
--
-- Messages are never deleted: organization-user messages already written
-- cannot satisfy the original sender check, so it is restored NOT VALID
-- (enforced for new rows, existing rows kept).
ALTER TABLE support_chat_messages DROP CONSTRAINT IF EXISTS support_chat_messages_sender_type_check;
ALTER TABLE support_chat_messages ADD CONSTRAINT support_chat_messages_sender_type_check
    CHECK (sender_type IN ('customer', 'taxi_provider', 'dispatch_provider', 'hauling_provider', 'admin')) NOT VALID;
ALTER TABLE support_chat_messages DROP CONSTRAINT IF EXISTS support_chat_messages_file_ids_chk;
ALTER TABLE support_chat_messages DROP CONSTRAINT IF EXISTS support_chat_messages_visibility_chk;
ALTER TABLE support_chat_messages DROP COLUMN IF EXISTS request_hash;
ALTER TABLE support_chat_messages DROP COLUMN IF EXISTS file_ids;
ALTER TABLE support_chat_messages DROP COLUMN IF EXISTS visibility;
