-- Workstream F3/F4: complaint categorisation, priority/emergency, and a small
-- help-article (FAQ) store served to all four apps.

-- category: optional, validated against a code-level allow-list (kept as TEXT,
-- not an enum, so new categories never need a migration).
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS category TEXT;

-- priority: 'normal' | 'high' | 'emergency'. SOS reports land as 'emergency'.
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS priority TEXT NOT NULL DEFAULT 'normal';

-- optional incident location captured by the SOS path.
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS incident_lat DOUBLE PRECISION;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS incident_lng DOUBLE PRECISION;

-- Sort/filter the admin queue by priority (emergency first), newest within.
CREATE INDEX IF NOT EXISTS idx_complaints_priority ON complaints (priority, created_at DESC);

-- help_articles: published FAQ/help-center content the apps render for self-service.
CREATE TABLE IF NOT EXISTS help_articles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    audience     TEXT NOT NULL DEFAULT 'all',   -- all | customer | provider
    category     TEXT,
    title        TEXT NOT NULL,
    body         TEXT NOT NULL,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    is_published BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_help_articles_audience
    ON help_articles (audience, is_published, sort_order);
