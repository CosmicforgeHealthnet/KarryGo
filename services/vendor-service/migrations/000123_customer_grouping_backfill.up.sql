CREATE TABLE commerce_customer_grouping_backfill_runs (
 id text PRIMARY KEY,
 cursor_ref text NOT NULL DEFAULT '',
 eligible bigint NOT NULL DEFAULT 0,
 repaired bigint NOT NULL DEFAULT 0,
 already_correct bigint NOT NULL DEFAULT 0,
 insufficient_evidence bigint NOT NULL DEFAULT 0,
 failed bigint NOT NULL DEFAULT 0,
 completed boolean NOT NULL DEFAULT false,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE commerce_customer_grouping_backfill_results (
 run_id text NOT NULL REFERENCES commerce_customer_grouping_backfill_runs(id),
 parent_order_id text NOT NULL REFERENCES commerce_parent_orders(id),
 outcome text NOT NULL CHECK(outcome IN ('REPAIRED','ALREADY_CORRECT','UNRECOVERABLE')),
 reason_code text NOT NULL,
 plan_id text NOT NULL DEFAULT '',
 observed_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(run_id,parent_order_id)
);
