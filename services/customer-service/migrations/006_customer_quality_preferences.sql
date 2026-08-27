-- Reusable structured Shop quality preferences. Orders snapshot these rows;
-- later edits never rewrite historical commerce evidence.
CREATE TABLE IF NOT EXISTS customer_quality_preferences (
    id uuid PRIMARY KEY,
    customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    preference_type text NOT NULL CHECK (preference_type IN ('RIPENESS','FIRMNESS','CUT_SIZE','PREPARATION','CLEANING','SORTING')),
    preference_value text NOT NULL CHECK (length(btrim(preference_value)) > 0),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (customer_id, preference_type)
);
CREATE INDEX IF NOT EXISTS customer_quality_preferences_customer_idx ON customer_quality_preferences(customer_id, preference_type);
