-- 003_partner_delivery_details_down.sql
ALTER TABLE partner_bookings
    DROP COLUMN IF EXISTS pickup_contact_name,
    DROP COLUMN IF EXISTS pickup_contact_phone,
    DROP COLUMN IF EXISTS distance_km,
    DROP COLUMN IF EXISTS package_description;