-- Replacement validation is evidence for an existing order, not a second
-- stock reservation or an ordinary live checkout for the cart.
ALTER TABLE commerce_checkout_sessions
    ADD COLUMN replacement_source_session_id TEXT REFERENCES commerce_checkout_sessions(id),
    ADD COLUMN replacement_key TEXT,
    ADD COLUMN replacement_source_digest TEXT;
ALTER TABLE commerce_checkout_sessions ADD CONSTRAINT replacement_source_evidence_chk
    CHECK ((replacement_source_session_id IS NULL AND replacement_key IS NULL AND replacement_source_digest IS NULL)
        OR (replacement_source_session_id IS NOT NULL AND replacement_key <> '' AND replacement_source_digest <> ''));
CREATE UNIQUE INDEX commerce_checkout_replacement_key_idx
    ON commerce_checkout_sessions(customer_id,replacement_key) WHERE replacement_key IS NOT NULL;
DROP INDEX commerce_checkout_sessions_one_live_idx;
CREATE UNIQUE INDEX commerce_checkout_sessions_one_live_idx ON commerce_checkout_sessions(cart_id)
    WHERE status IN ('VALIDATING','VALIDATED') AND replacement_source_session_id IS NULL;
CREATE FUNCTION commerce_checkout_evidence_digest(source_id TEXT) RETURNS TEXT
LANGUAGE sql STABLE AS $$
    SELECT md5(to_jsonb(s)::text ||
        COALESCE((SELECT jsonb_agg(to_jsonb(g) ORDER BY g.id)::text FROM commerce_checkout_groups g WHERE g.session_id=s.id),'[]') ||
        COALESCE((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id)::text FROM commerce_checkout_lines l WHERE l.session_id=s.id),'[]'))
    FROM commerce_checkout_sessions s WHERE s.id=source_id
$$;
