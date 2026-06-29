ALTER TABLE verification_steps
    ADD COLUMN IF NOT EXISTS licence_expiry_date DATE NULL;
