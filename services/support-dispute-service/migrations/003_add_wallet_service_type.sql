-- Add 'wallet' to the service_type enum so customers can file wallet/payment
-- disputes (the customer app's wallet dispute flow posts service_type='wallet').
--
-- NOTE: this file must contain ONLY this single statement. Postgres forbids
-- running ALTER TYPE ... ADD VALUE inside a transaction block, and the migration
-- runner applies each file via one db.Exec (a multi-statement file would run as
-- an implicit transaction and fail here).
ALTER TYPE service_type ADD VALUE IF NOT EXISTS 'wallet';
