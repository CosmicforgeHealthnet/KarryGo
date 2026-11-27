-- Organization support-case messages for Fleet (spec 59.10:
-- POST /support-cases/{id}/messages with text, fileIds and visibility).
--
-- Support/Dispute stays the one message store: Fleet keeps only a submission
-- ledger. A Fleet message is an ordinary support_chat_messages row carrying
-- its sender (an organization user), its client message id (unique per case
-- since 009), a request hash so a reused client id with a different body is
-- refused, the attached media file ids and its visibility:
--   SHARED   -- visible to the organization and to platform support;
--   INTERNAL -- for platform support only: never returned on a complainant
--               (customer or provider) route.
--
-- The runner re-applies every file on each start, so every change is
-- conditional and a re-run takes no table lock.
SELECT support_admin_ddl_if(support_admin_lacks_column('support_chat_messages', 'visibility'),
    'ALTER TABLE support_chat_messages ADD COLUMN visibility text NOT NULL DEFAULT ''SHARED''');
SELECT support_admin_ddl_if(support_admin_lacks_column('support_chat_messages', 'file_ids'),
    'ALTER TABLE support_chat_messages ADD COLUMN file_ids jsonb NOT NULL DEFAULT ''[]''::jsonb');
SELECT support_admin_ddl_if(support_admin_lacks_column('support_chat_messages', 'request_hash'),
    'ALTER TABLE support_chat_messages ADD COLUMN request_hash text');
SELECT support_admin_ddl_if(NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'support_chat_messages_visibility_chk'),
    'ALTER TABLE support_chat_messages ADD CONSTRAINT support_chat_messages_visibility_chk CHECK (visibility IN (''SHARED'', ''INTERNAL''))');
SELECT support_admin_ddl_if(NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'support_chat_messages_file_ids_chk'),
    'ALTER TABLE support_chat_messages ADD CONSTRAINT support_chat_messages_file_ids_chk CHECK (jsonb_typeof(file_ids) = ''array'' AND jsonb_array_length(file_ids) <= 10)');
-- Allow an organization user as a sender. The original inline check is
-- replaced once; afterwards both statements are no-ops.
SELECT support_admin_ddl_if(EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'support_chat_messages_sender_type_check'
        AND pg_get_constraintdef(oid) NOT LIKE '%organization_user%'),
    'ALTER TABLE support_chat_messages DROP CONSTRAINT support_chat_messages_sender_type_check');
SELECT support_admin_ddl_if(NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'support_chat_messages_sender_type_check'),
    'ALTER TABLE support_chat_messages ADD CONSTRAINT support_chat_messages_sender_type_check CHECK (sender_type IN (''customer'', ''taxi_provider'', ''dispatch_provider'', ''hauling_provider'', ''admin'', ''organization_user''))');
