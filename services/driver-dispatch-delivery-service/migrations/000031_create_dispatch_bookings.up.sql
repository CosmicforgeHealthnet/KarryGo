CREATE TABLE IF NOT EXISTS dispatch_bookings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'searching',
    pickup_address TEXT NOT NULL,
    pickup_lat NUMERIC(10,7) NOT NULL,
    pickup_lng NUMERIC(10,7) NOT NULL,
    dropoff_address TEXT NOT NULL,
    dropoff_lat NUMERIC(10,7) NOT NULL,
    dropoff_lng NUMERIC(10,7) NOT NULL,
    distance_km NUMERIC(8,2) NOT NULL DEFAULT 0,
    fare_amount BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'NGN',
    payment_method TEXT NOT NULL DEFAULT 'wallet',
    payment_status TEXT NOT NULL DEFAULT 'unpaid',
    payment_ref TEXT NOT NULL DEFAULT '',
    receiver_name TEXT NOT NULL,
    receiver_phone TEXT NOT NULL,
    package_desc TEXT NOT NULL DEFAULT '',
    package_weight NUMERIC(6,2) NULL,
    package_type TEXT NOT NULL DEFAULT '',
    package_size TEXT NOT NULL DEFAULT 'small',
    is_fragile BOOLEAN NOT NULL DEFAULT FALSE,
    service_tier TEXT NOT NULL DEFAULT 'standard',
    provider_id UUID NULL,
    cancel_reason TEXT NOT NULL DEFAULT '',
    cancelled_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_dispatch_booking_status CHECK (
        status IN ('searching','accepted','completed','cancelled','unmatched')
    ),
    CONSTRAINT chk_dispatch_booking_payment_method CHECK (payment_method IN ('wallet')),
    CONSTRAINT chk_dispatch_booking_payment_status CHECK (
        payment_status IN ('unpaid','held','paid','refunded','failed')
    ),
    CONSTRAINT chk_dispatch_booking_package_size CHECK (package_size IN ('small','medium','large')),
    CONSTRAINT chk_dispatch_booking_service_tier CHECK (service_tier IN ('standard','express'))
);

CREATE INDEX IF NOT EXISTS idx_dispatch_bookings_customer
    ON dispatch_bookings (customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dispatch_bookings_status
    ON dispatch_bookings (status);
