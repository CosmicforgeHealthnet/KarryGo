-- Publisher checkpoint only: no operational movement, provider or price state.
CREATE TABLE commerce_customer_grouping_emissions (
 parent_order_id TEXT PRIMARY KEY REFERENCES commerce_parent_orders(id) ON DELETE CASCADE,
 source_version BIGINT NOT NULL CHECK(source_version>0),
 fingerprint TEXT NOT NULL CHECK(length(fingerprint)=64),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_vendor_customer_grouping_replay ON vendor_outbox(aggregate_id,aggregate_version DESC,created_at DESC)
 WHERE event_type='commerce.fulfilment.customer_grouping';
