-- WP9.5: replay protection for the private vehicle authority contract.
--
-- The shared HMAC signature proves who signed a request and that the body was
-- not altered, and its timestamp bounds how long a captured signature stays
-- acceptable. That window is minutes wide, which is plenty of time to replay a
-- captured lookup. A per-caller single-use nonce closes it: the first use of a
-- nonce claims the row, and every later use collides.
--
-- The request fingerprint is stored alongside so a rejected replay can be told
-- apart from a caller that reused a nonce on a genuinely different request —
-- one is an attack, the other is a client bug, and they need different
-- responses from an operator.
CREATE TABLE IF NOT EXISTS dispatch_service_request_nonces (
    service_name        text NOT NULL,
    nonce               text NOT NULL,
    request_fingerprint text NOT NULL,
    expires_at          timestamptz NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (service_name, nonce)
);

-- Expiry sweep support. Nonces only need to outlive the signature freshness
-- window; keeping them forever would grow without bound for no security gain.
CREATE INDEX IF NOT EXISTS idx_dispatch_service_request_nonces_expiry
    ON dispatch_service_request_nonces (expires_at);
