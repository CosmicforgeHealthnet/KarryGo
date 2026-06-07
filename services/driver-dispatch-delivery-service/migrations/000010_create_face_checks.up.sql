CREATE TABLE IF NOT EXISTS face_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    step_id UUID NOT NULL REFERENCES verification_steps(id) ON DELETE CASCADE,
    selfie_url TEXT NOT NULL,
    id_doc_url TEXT NOT NULL,
    match_score NUMERIC(5,2) NULL,
    result TEXT NULL,
    provider_used TEXT NOT NULL DEFAULT 'smile_identity',
    error_message TEXT NULL,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_face_checks_result CHECK (
        result IS NULL OR result IN ('pass', 'fail')
    )
);

CREATE INDEX IF NOT EXISTS idx_face_provider
    ON face_checks (provider_id);
