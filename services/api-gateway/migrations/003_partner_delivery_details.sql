-- 003_partner_delivery_details.sql
-- Adds pickup contact details, computed trip distance, and a free-text package
-- description to partner_bookings. These were on the dashboard/Figma design but
-- missing from the original 001_partner_schema.sql create/list/detail flow.
-- receiver_phone already existed and is NOT touched here - it was just never
-- returned in the DeliveryDetail JSON response (fixed in model.go, no migration
-- needed for that one).

ALTER TABLE partner_bookings
    ADD COLUMN IF NOT EXISTS pickup_contact_name text,
    ADD COLUMN IF NOT EXISTS pickup_contact_phone text,
    ADD COLUMN IF NOT EXISTS distance_km numeric(8,2),
    ADD COLUMN IF NOT EXISTS package_description text;