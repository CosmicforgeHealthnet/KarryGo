-- Unify Section 19 orchestration with the existing Section 18 proposal
-- authority. Cases remain orchestration context; proposals remain the single
-- customer-decision/replacement authority.

ALTER TABLE commerce_substitution_proposals
    ADD COLUMN IF NOT EXISTS case_id TEXT;

ALTER TABLE commerce_substitution_proposals
    DROP CONSTRAINT IF EXISTS commerce_substitution_proposals_case_scope_fk;
ALTER TABLE commerce_substitution_proposals
    ADD CONSTRAINT commerce_substitution_proposals_case_scope_fk
    FOREIGN KEY (case_id, parent_order_id)
    REFERENCES commerce_substitution_cases(id, parent_order_id)
    ON DELETE RESTRICT;

ALTER TABLE commerce_shop_picking_snapshots
    ADD COLUMN IF NOT EXISTS taxonomy_version BIGINT NOT NULL DEFAULT 0 CHECK (taxonomy_version >= 0),
    ADD COLUMN IF NOT EXISTS taxonomy_category_code TEXT NOT NULL DEFAULT '';

-- Section 19 uses the existing Section 17 adjustment authority.  Its
-- historical CHECK omitted substitution adjustments even though the existing
-- order path now records them there; extend that vocabulary additively.
ALTER TABLE commerce_order_adjustments
    DROP CONSTRAINT IF EXISTS commerce_order_adjustments_adjustment_type_check;
ALTER TABLE commerce_order_adjustments
    ADD CONSTRAINT commerce_order_adjustments_adjustment_type_check
    CHECK (adjustment_type IN ('POSITIVE_DELTA','CHEAPER_REPLAN','CANCELLATION_REFUND','SUBSTITUTION'));

-- Convert any local rows written to the redundant table before retiring it.
INSERT INTO commerce_substitution_proposals(
    id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,
    branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,
    replacement_snapshot,proposed_quantity,proposed_price_minor,
    price_difference_minor,reason,expires_at,status,response_ref,version,
    idempotency_key,created_at,responded_at,updated_at,case_id)
SELECT cp.id,cp.parent_order_id,cp.vendor_order_id,cp.order_line_id,cp.customer_id,
       c.business_id,c.branch_id,cp.candidate_item_ref,cp.candidate_variant_ref,cp.candidate_listing_ref,
       cp.candidate_snapshot,cp.proposed_quantity,cp.proposed_price_minor,
       cp.price_difference_minor, 'case proposal',
       cp.expires_at,
       CASE cp.state WHEN 'ACCEPTED' THEN 'APPROVED' ELSE cp.state END,
       cp.response_ref,cp.version,cp.idempotency_key,cp.created_at,cp.responded_at,cp.updated_at,
       cp.case_id
  FROM commerce_substitution_case_proposals cp
  JOIN commerce_substitution_cases c ON c.id=cp.case_id
 ON CONFLICT (idempotency_key) DO UPDATE
    SET case_id=COALESCE(commerce_substitution_proposals.case_id, EXCLUDED.case_id);

-- Preserve adjustment evidence before the redundant table disappears.
ALTER TABLE commerce_order_adjustment_evidence
    DROP CONSTRAINT IF EXISTS commerce_order_adjustment_evidence_proposal_id_fkey;
ALTER TABLE commerce_order_adjustment_evidence
    ADD CONSTRAINT commerce_order_adjustment_evidence_proposal_id_fkey
    FOREIGN KEY (proposal_id) REFERENCES commerce_substitution_proposals(id)
    ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS commerce_substitution_case_idx
    ON commerce_substitution_proposals(case_id, created_at, id)
    WHERE case_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS commerce_substitution_one_effective_idx
    ON commerce_substitution_proposals(case_id)
    WHERE case_id IS NOT NULL AND status = 'APPROVED';

DROP TABLE commerce_substitution_case_proposals;

COMMENT ON COLUMN commerce_substitution_proposals.case_id IS
    'Section 19 case orchestration context; this table remains the sole proposal authority.';
