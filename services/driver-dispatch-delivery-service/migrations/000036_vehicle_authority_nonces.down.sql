-- Reverses WP9.5 vehicle authority replay protection.

DROP INDEX IF EXISTS idx_dispatch_service_request_nonces_expiry;
DROP TABLE IF EXISTS dispatch_service_request_nonces;
