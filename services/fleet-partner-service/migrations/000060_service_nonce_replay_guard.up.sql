-- Section 26 gap-close: HMAC nonce replay protection.
--
-- shared/go/serviceauth.Verifier requires a nonce to be present and signs it
-- as part of the HMAC digest, but verifies only that the nonce accompanies a
-- valid, freshness-windowed signature -- it never records that a nonce has
-- already been consumed. A captured, valid request (signature and all) can
-- therefore be replayed verbatim any number of times within the timestamp
-- skew window, and the shared verifier accepts it every time. This closes
-- that gap on fleet-partner-service's own internal HMAC surface, without
-- editing shared/go/serviceauth -- that package is used by every other
-- service in the platform, and closing this the same way for all of them is
-- a separate, platform-wide change outside this service's scope.
CREATE TABLE IF NOT EXISTS fleet_partner_service_nonces (
    caller_service text NOT NULL,
    nonce          text NOT NULL,
    consumed_at    timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz NOT NULL,
    PRIMARY KEY (caller_service, nonce)
);

-- Every read this guard performs is "has this exact (caller, nonce) pair
-- been consumed before", answered by the primary key itself; this index
-- exists only to make the expiry sweep's WHERE clause an index scan instead
-- of a sequential scan as the table grows.
CREATE INDEX IF NOT EXISTS idx_fleet_service_nonces_expiry
    ON fleet_partner_service_nonces (expires_at);
