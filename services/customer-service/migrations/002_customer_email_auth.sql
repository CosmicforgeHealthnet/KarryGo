ALTER TABLE customers ADD COLUMN IF NOT EXISTS email text;
ALTER TABLE customers ALTER COLUMN phone DROP NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_customers_email ON customers(email);

ALTER TABLE customer_auth_events ADD COLUMN IF NOT EXISTS email text;
CREATE INDEX IF NOT EXISTS idx_customer_auth_events_email ON customer_auth_events(email);
