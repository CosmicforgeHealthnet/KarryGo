-- Section 8D: vendor-owned CRM loyalty points.
--
-- These tables hold a POINTS ledger, not money. There is deliberately no
-- currency column, no minor-unit amount, no wallet reference and no payout
-- state anywhere in this migration: payment-wallet-service remains the sole
-- monetary authority and loyalty points are never convertible here.
--
-- They also hold no canonical customer PII. A customer is referenced only by
-- the vendor-local CRM profile id, exactly as Sections 8B and 8C do.

CREATE TABLE crm_loyalty_program_configs (
    id text PRIMARY KEY,
    business_id text NOT NULL UNIQUE REFERENCES vendor_businesses (id),
    status text NOT NULL DEFAULT 'DISABLED' CHECK (status IN ('DISABLED', 'ACTIVE', 'PAUSED')),
    points_name_singular text NOT NULL DEFAULT 'point' CHECK (char_length(points_name_singular) BETWEEN 1 AND 40),
    points_name_plural text NOT NULL DEFAULT 'points' CHECK (char_length(points_name_plural) BETWEEN 1 AND 40),
    expiry_policy text NOT NULL DEFAULT 'NEVER' CHECK (expiry_policy IN ('NEVER', 'FIXED_DAYS')),
    expiry_days integer CHECK (expiry_days IS NULL OR expiry_days BETWEEN 1 AND 3650),
    minimum_redemption_points bigint NOT NULL DEFAULT 0
        CHECK (minimum_redemption_points >= 0 AND minimum_redemption_points <= 1000000000),
    redemption_increment_points bigint NOT NULL DEFAULT 1
        CHECK (redemption_increment_points >= 1 AND redemption_increment_points <= 1000000000),
    manual_adjustment_limit bigint NOT NULL DEFAULT 100000
        CHECK (manual_adjustment_limit >= 1 AND manual_adjustment_limit <= 1000000000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    -- NEVER must not carry a retention window and FIXED_DAYS must not be
    -- open-ended: an expiry policy that half-applies is the failure mode that
    -- silently voids customers' points.
    CONSTRAINT crm_loyalty_program_expiry_ck CHECK (
        (expiry_policy = 'NEVER' AND expiry_days IS NULL)
        OR (expiry_policy = 'FIXED_DAYS' AND expiry_days IS NOT NULL)
    )
);

CREATE TABLE crm_loyalty_accounts (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    customer_profile_id text NOT NULL,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'MERGED', 'REDACTED')),
    current_points bigint NOT NULL DEFAULT 0 CHECK (current_points >= 0),
    lifetime_credited_points bigint NOT NULL DEFAULT 0 CHECK (lifetime_credited_points >= 0),
    lifetime_debited_points bigint NOT NULL DEFAULT 0 CHECK (lifetime_debited_points >= 0),
    next_sequence bigint NOT NULL DEFAULT 1 CHECK (next_sequence > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    UNIQUE (id, business_id),
    -- One account per relationship. This is also what makes concurrent
    -- first-credit safe: the loser of the race hits this constraint rather
    -- than creating a second balance for the same customer.
    UNIQUE (business_id, customer_profile_id),
    -- current_points is an aggregate of the ledger, so it can never disagree
    -- with the sum of deltas.
    CONSTRAINT crm_loyalty_account_balance_ck
        CHECK (current_points = lifetime_credited_points - lifetime_debited_points),
    CONSTRAINT crm_loyalty_account_profile_fk FOREIGN KEY (customer_profile_id, business_id)
        REFERENCES vendor_customer_profiles (id, business_id)
);

CREATE INDEX crm_loyalty_accounts_status_idx ON crm_loyalty_accounts (business_id, status, id);

CREATE TABLE crm_loyalty_entries (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    loyalty_account_id text NOT NULL,
    sequence bigint NOT NULL CHECK (sequence > 0),
    entry_type text NOT NULL CHECK (entry_type IN
        ('EARN', 'REDEEM', 'EXPIRE', 'ADJUSTMENT_CREDIT', 'ADJUSTMENT_DEBIT',
         'REVERSAL', 'MERGE_IN', 'MERGE_OUT')),
    -- A zero-delta entry would be a balance change that changed nothing; the
    -- ledger records real movement only.
    delta_points bigint NOT NULL CHECK (delta_points <> 0),
    balance_after bigint NOT NULL CHECK (balance_after >= 0),
    program_version bigint NOT NULL CHECK (program_version > 0),
    source_type text NOT NULL CHECK (source_type IN
        ('MANUAL_ADJUSTMENT', 'ORDER', 'SYSTEM_EXPIRY', 'REVERSAL', 'IDENTITY_MERGE', 'EXTERNAL')),
    source_reference text CHECK (source_reference IS NULL OR char_length(source_reference) <= 200),
    idempotency_reference text CHECK (idempotency_reference IS NULL OR char_length(idempotency_reference) <= 200),
    reverses_entry_id text,
    reason_code text CHECK (reason_code IS NULL OR char_length(reason_code) <= 64),
    created_by_staff_id text,
    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    -- Per-account monotonic ordering. Two concurrent mutations cannot both
    -- claim the same position, which is what makes balance_after replayable.
    UNIQUE (business_id, loyalty_account_id, sequence),
    CONSTRAINT crm_loyalty_entry_account_fk FOREIGN KEY (loyalty_account_id, business_id)
        REFERENCES crm_loyalty_accounts (id, business_id),
    CONSTRAINT crm_loyalty_entry_reverses_fk FOREIGN KEY (reverses_entry_id, business_id)
        REFERENCES crm_loyalty_entries (id, business_id),
    CONSTRAINT crm_loyalty_entry_staff_fk FOREIGN KEY (created_by_staff_id, business_id)
        REFERENCES vendor_staff (id, vendor_id),
    -- Credit types move points in, debit types move points out. The sign is
    -- not a caller's choice.
    CONSTRAINT crm_loyalty_entry_direction_ck CHECK (
        (entry_type IN ('EARN', 'ADJUSTMENT_CREDIT', 'MERGE_IN') AND delta_points > 0)
        OR (entry_type IN ('REDEEM', 'ADJUSTMENT_DEBIT', 'EXPIRE', 'MERGE_OUT') AND delta_points < 0)
        OR entry_type = 'REVERSAL'
    ),
    CONSTRAINT crm_loyalty_entry_reversal_ck CHECK (
        (entry_type = 'REVERSAL' AND reverses_entry_id IS NOT NULL)
        OR (entry_type <> 'REVERSAL' AND reverses_entry_id IS NULL)
    )
);

-- Exactly one reversal per original entry, enforced by the database rather
-- than by a read-then-write check that a concurrent transaction could race.
CREATE UNIQUE INDEX crm_loyalty_entries_one_reversal_uk
    ON crm_loyalty_entries (business_id, reverses_entry_id)
    WHERE reverses_entry_id IS NOT NULL;

-- Durable command-level exactly-once for every domain primitive, including
-- the ones that have no HTTP route and therefore cannot rely on the
-- Idempotency-Key store.
CREATE UNIQUE INDEX crm_loyalty_entries_command_uk
    ON crm_loyalty_entries (business_id, idempotency_reference)
    WHERE idempotency_reference IS NOT NULL;

-- A future trusted producer that redelivers the same order/event must not
-- award points twice, independently of how the command was keyed.
CREATE UNIQUE INDEX crm_loyalty_entries_source_event_uk
    ON crm_loyalty_entries (business_id, source_type, source_reference)
    WHERE source_reference IS NOT NULL AND source_type IN ('ORDER', 'EXTERNAL');

CREATE INDEX crm_loyalty_entries_account_sequence_idx
    ON crm_loyalty_entries (business_id, loyalty_account_id, sequence DESC);
CREATE INDEX crm_loyalty_entries_account_created_idx
    ON crm_loyalty_entries (business_id, loyalty_account_id, created_at DESC, sequence DESC);
CREATE INDEX crm_loyalty_entries_account_type_idx
    ON crm_loyalty_entries (business_id, loyalty_account_id, entry_type, created_at DESC);

-- Point lots carry provenance and expiry. Expiry is stamped when the credit
-- is issued and is never recomputed, so a later policy change cannot void or
-- extend points a customer already holds.
CREATE TABLE crm_loyalty_point_lots (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    loyalty_account_id text NOT NULL,
    source_entry_id text NOT NULL,
    issued_points bigint NOT NULL CHECK (issued_points > 0),
    remaining_points bigint NOT NULL CHECK (remaining_points >= 0),
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (id, business_id),
    CONSTRAINT crm_loyalty_lot_remaining_ck CHECK (remaining_points <= issued_points),
    CONSTRAINT crm_loyalty_lot_account_fk FOREIGN KEY (loyalty_account_id, business_id)
        REFERENCES crm_loyalty_accounts (id, business_id),
    CONSTRAINT crm_loyalty_lot_entry_fk FOREIGN KEY (source_entry_id, business_id)
        REFERENCES crm_loyalty_entries (id, business_id)
);

-- FEFO selection and the expiry sweep are the two hot paths; both read
-- unspent lots ordered by expiry.
CREATE INDEX crm_loyalty_point_lots_fefo_idx
    ON crm_loyalty_point_lots (business_id, loyalty_account_id, expires_at NULLS LAST, created_at, id)
    WHERE remaining_points > 0;
CREATE INDEX crm_loyalty_point_lots_due_idx
    ON crm_loyalty_point_lots (expires_at)
    WHERE remaining_points > 0 AND expires_at IS NOT NULL;
CREATE INDEX crm_loyalty_point_lots_source_idx
    ON crm_loyalty_point_lots (business_id, source_entry_id);

-- Which lots funded which debit. This is what makes a debit reversal exact
-- rather than an approximate re-credit, and what lets a restored allocation
-- keep the expiry it originally had.
CREATE TABLE crm_loyalty_lot_consumptions (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses (id),
    loyalty_account_id text NOT NULL,
    debit_entry_id text NOT NULL,
    lot_id text NOT NULL,
    consumed_points bigint NOT NULL CHECK (consumed_points > 0),
    restored_points bigint NOT NULL DEFAULT 0 CHECK (restored_points >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (business_id, debit_entry_id, lot_id),
    CONSTRAINT crm_loyalty_consumption_restore_ck CHECK (restored_points <= consumed_points),
    CONSTRAINT crm_loyalty_consumption_account_fk FOREIGN KEY (loyalty_account_id, business_id)
        REFERENCES crm_loyalty_accounts (id, business_id),
    CONSTRAINT crm_loyalty_consumption_entry_fk FOREIGN KEY (debit_entry_id, business_id)
        REFERENCES crm_loyalty_entries (id, business_id),
    CONSTRAINT crm_loyalty_consumption_lot_fk FOREIGN KEY (lot_id, business_id)
        REFERENCES crm_loyalty_point_lots (id, business_id)
);

CREATE INDEX crm_loyalty_lot_consumptions_entry_idx
    ON crm_loyalty_lot_consumptions (business_id, debit_entry_id);
CREATE INDEX crm_loyalty_lot_consumptions_lot_idx
    ON crm_loyalty_lot_consumptions (business_id, lot_id);
