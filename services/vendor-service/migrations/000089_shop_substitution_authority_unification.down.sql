-- Restore the 000088 compatibility shape, including its legacy local table,
-- so rollback of this corrective migration is lossless.

CREATE TABLE IF NOT EXISTS commerce_substitution_case_proposals (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES commerce_substitution_cases(id) ON DELETE RESTRICT,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    order_line_id TEXT NOT NULL REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,
    candidate_item_ref TEXT NOT NULL,
    candidate_variant_ref TEXT NOT NULL DEFAULT '',
    candidate_listing_ref TEXT NOT NULL DEFAULT '',
    candidate_snapshot JSONB NOT NULL CHECK (jsonb_typeof(candidate_snapshot) = 'object'),
    proposed_quantity BIGINT NOT NULL CHECK (proposed_quantity > 0),
    proposed_price_minor BIGINT NOT NULL CHECK (proposed_price_minor >= 0),
    price_difference_minor BIGINT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING','ACCEPTED','REJECTED','EXPIRED','CANCELLED')),
    expires_at TIMESTAMPTZ,
    response_ref TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_substitution_case_proposal_scope_fk
        FOREIGN KEY (case_id, parent_order_id)
        REFERENCES commerce_substitution_cases(id, parent_order_id)
        ON DELETE RESTRICT
);

INSERT INTO commerce_substitution_case_proposals(
    id,case_id,parent_order_id,vendor_order_id,order_line_id,customer_id,
    candidate_item_ref,candidate_variant_ref,candidate_listing_ref,
    candidate_snapshot,proposed_quantity,proposed_price_minor,
    price_difference_minor,state,expires_at,response_ref,version,
    idempotency_key,created_at,responded_at,updated_at)
SELECT id,case_id,parent_order_id,vendor_order_id,order_line_id,customer_id,
       proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,
       replacement_snapshot,proposed_quantity,proposed_price_minor,
       price_difference_minor,
       CASE status WHEN 'APPROVED' THEN 'ACCEPTED' ELSE status END,
       expires_at,response_ref,version,idempotency_key,created_at,responded_at,updated_at
  FROM commerce_substitution_proposals
 WHERE case_id IS NOT NULL
 ON CONFLICT (idempotency_key) DO NOTHING;

ALTER TABLE commerce_order_adjustment_evidence
    DROP CONSTRAINT IF EXISTS commerce_order_adjustment_evidence_proposal_id_fkey;
ALTER TABLE commerce_order_adjustment_evidence
    ADD CONSTRAINT commerce_order_adjustment_evidence_proposal_id_fkey
    FOREIGN KEY (proposal_id) REFERENCES commerce_substitution_case_proposals(id)
    ON DELETE RESTRICT;

DROP INDEX IF EXISTS commerce_substitution_one_effective_idx;
DROP INDEX IF EXISTS commerce_substitution_case_idx;
ALTER TABLE commerce_substitution_proposals
    DROP CONSTRAINT IF EXISTS commerce_substitution_proposals_case_scope_fk;
ALTER TABLE commerce_substitution_proposals
    DROP COLUMN IF EXISTS case_id;

ALTER TABLE commerce_shop_picking_snapshots
    DROP COLUMN IF EXISTS taxonomy_version,
    DROP COLUMN IF EXISTS taxonomy_category_code;

ALTER TABLE commerce_order_adjustments
    DROP CONSTRAINT IF EXISTS commerce_order_adjustments_adjustment_type_check;
ALTER TABLE commerce_order_adjustments
    ADD CONSTRAINT commerce_order_adjustments_adjustment_type_check
    CHECK (adjustment_type IN ('POSITIVE_DELTA','CHEAPER_REPLAN','CANCELLATION_REFUND'));
