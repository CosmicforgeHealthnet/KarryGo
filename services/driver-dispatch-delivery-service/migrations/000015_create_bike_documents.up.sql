CREATE TABLE bike_documents (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    bike_id       UUID        NOT NULL REFERENCES bikes(id) ON DELETE CASCADE,
    provider_id   UUID        NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    document_type TEXT        NOT NULL,
    file_url      TEXT        NOT NULL,
    file_size     INT         NULL,
    mime_type     TEXT        NULL,
    expiry_date   DATE        NULL,
    uploaded_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT bike_documents_document_type_check
        CHECK (document_type IN ('registration', 'insurance'))
);

CREATE INDEX idx_bike_docs_bike     ON bike_documents (bike_id);
CREATE INDEX idx_bike_docs_provider ON bike_documents (provider_id);
