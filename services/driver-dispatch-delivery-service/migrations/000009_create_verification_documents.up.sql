CREATE TABLE IF NOT EXISTS verification_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    step_id UUID NOT NULL REFERENCES verification_steps(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    document_type TEXT NOT NULL,
    file_url TEXT NOT NULL,
    file_size INT NULL,
    mime_type TEXT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_verification_documents_document_type CHECK (
        document_type IN (
            'govt_id',
            'profile_photo',
            'licence_doc',
            'bike_registration',
            'insurance',
            'selfie'
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_vdocs_step
    ON verification_documents (step_id);

CREATE INDEX IF NOT EXISTS idx_vdocs_provider
    ON verification_documents (provider_id);
