# Media/File Service

Owns profile photos, uploaded documents, delivery proof images, recipient
signatures, and Firebase Storage object integration.

## Upload API

Internal Karry Go services upload files through:

```text
POST /api/v1/media-files/uploads
```

The request is `multipart/form-data` with:

- `file`: uploaded file bytes.
- `owner_service`: service that owns the file, which must match
  `X-Karrygo-Service`.
- `owner_id`: domain id in the owning service.
- `purpose`: one of `profile_photo`, `document_file`, `proof_image`, or
  `signature`.
- `metadata`: optional JSON object.

Responses use the standard success envelope and include the permanent public
Firebase Storage URL:

```json
{
  "success": true,
  "data": {
    "id": "asset-id",
    "url": "https://storage.googleapis.com/bucket/media/customer-service/profile_photo/customer-id/asset-id/photo.jpg",
    "bucket": "bucket",
    "path": "media/customer-service/profile_photo/customer-id/asset-id/photo.jpg",
    "content_type": "image/jpeg",
    "size_bytes": 12345,
    "checksum_sha256": "..."
  }
}
```

## Internal Service Auth

Every request must include:

```text
X-Karrygo-Service: customer-service
Authorization: Bearer <service-token>
```

Tokens are configured with `MEDIA_FILE_SERVICE_TOKENS` using comma-separated
`service=token` pairs.

## Local Configuration

Copy `.env.example` and set:

| Variable | Purpose |
|---|---|
| `MEDIA_FILE_DATABASE_URL` | Media-owned Postgres connection string |
| `MEDIA_FILE_FIREBASE_BUCKET` | Firebase Storage bucket name |
| `MEDIA_FILE_FIREBASE_CREDENTIALS_FILE` | Optional service-account file path |
| `MEDIA_FILE_FIREBASE_CREDENTIALS_JSON` | Optional service-account JSON |
| `MEDIA_FILE_PUBLIC_BASE_URL` | Optional CDN/custom public base URL |
| `MEDIA_FILE_MAX_UPLOAD_BYTES` | Maximum accepted upload size |
| `MEDIA_FILE_SERVICE_TOKENS` | Internal service auth tokens |

The configured bucket or prefix must be publicly readable because v1 returns
permanent public URLs.
