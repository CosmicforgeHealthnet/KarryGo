CREATE TABLE IF NOT EXISTS media_assets (
    id uuid PRIMARY KEY,
    owner_service text NOT NULL,
    owner_id text NOT NULL,
    purpose text NOT NULL,
    original_filename text NOT NULL DEFAULT '',
    content_type text NOT NULL,
    size_bytes bigint NOT NULL,
    checksum_sha256 text NOT NULL,
    storage_bucket text NOT NULL,
    storage_path text NOT NULL UNIQUE,
    public_url text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    uploaded_by_service text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_media_assets_owner ON media_assets(owner_service, owner_id);
CREATE INDEX IF NOT EXISTS idx_media_assets_purpose ON media_assets(purpose);
CREATE INDEX IF NOT EXISTS idx_media_assets_status ON media_assets(status) WHERE deleted_at IS NULL;
