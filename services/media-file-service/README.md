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

## Local Bootstrap

For a full local setup, use the helper script from the repo root:

```bash
./scripts/media-local-bootstrap.sh
```

It starts local Postgres on `5441`, creates the `media_file_service` database if
needed, applies the media migrations, and prints the command to run the
service.

If you prefer to do it manually:

```bash
mkdir -p /tmp/postgres-media
initdb -D /tmp/postgres-media
pg_ctl -D /tmp/postgres-media -o "-p 5441" start
psql -p 5441 -d template1
```

Inside `psql`:

```sql
CREATE USER cosmicforge_logistics WITH PASSWORD 'cosmicforge_logistics';
CREATE DATABASE media_file_service OWNER cosmicforge_logistics;
```

Then apply the migration:

```bash
psql "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5441/media_file_service?sslmode=disable" \
  -f services/media-file-service/migrations/001_media_assets.sql
```

## V4 Section 18 customer upload security (local)

Media remains the byte and metadata authority. Customer Service's HMAC-only
authorization issues the existing short-lived, purpose/source/owner/MIME/size
grant. The token-only public content route redeems that grant once, bounds the
body and validates authoritative metadata before returning MediaRef. Wrong
token, replay, expiry and cross-scope use fail closed. The existing cleanup and
retention policy handles unused/expired grants and unattached objects. See
`docs/private-media-api.md` and `docs/customer-evidence-upload.openapi.json`.
Real PostgreSQL redemption and replay tests pass locally. Production evidence
attachment remains dependent on configured malware scanning, Media private
HMAC callers and ingress; test fakes are not a production scanner.

## Secure Admin (Section 19) - media, documents and evidence

### Findings fixed

- **The legacy `GET /api/v1/media-files/:id/download` route was
  unauthenticated** (registered outside the service-token group, and
  `serviceapp` adds no auth middleware) and served *any* `media_assets` row
  inline by UUID - including `document_file`, `proof_image` and `signature`.
  It now serves **only `profile_photo`** by default. Every other purpose
  answers `404` (a copied link does not confirm the file exists).
- Owners read their own legacy files through the authenticated
  `GET /api/v1/media-files/files/:id/content` (service token; partner
  sessions only their own partner's files). New non-profile uploads return
  this path as their `url` instead of `/:id/download`.
- New `signature` uploads may no longer be `image/svg+xml`; existing SVG
  signatures are only ever served as `application/octet-stream` attachments.
- `owner_id` / `owner_service` must be opaque tokens
  (`^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$`, no `..`) because they become storage
  path segments.

### Legacy read floor (every legacy read path)

1. An adopted asset is never served by a legacy route: `410
   legacy_asset_adopted`. Its bytes are reachable only through the mapped
   modern private object and an admin case grant.
2. Stored bytes must match the recorded size and SHA-256 and sniff as the
   recorded content type, else `409 integrity_check_failed`.
3. Every purpose except `profile_photo` is high-risk: the bytes are scanned
   at read time and must come back SAFE. No scanner, a scanner error or an
   unverified verdict answers `403 not_downloadable`; a malicious verdict also
   marks the row `quarantined`.
4. Headers: `X-Content-Type-Options: nosniff`, `Content-Security-Policy:
   sandbox; default-src 'none'`, `Cache-Control: private, no-store` and
   `Content-Disposition: attachment` for everything except verified raster
   profile photos (inline, `public, max-age=300`).

### Modern private objects

- `models.HighRiskPurpose` - sensitive evidence purposes (incl. case-only
  purposes), `DATA_EXPORT`, and identity/verification documents
  (`PROVIDER_/VEHICLE_/INSURANCE_/COMPLIANCE_/COMPANY_/BRANCH_DOCUMENT`) -
  require `safety_status = SAFE` on **every** read path (grant issue and
  redemption) regardless of `MEDIA_FILE_REQUIRE_SAFE_FOR_DOWNLOAD`. Without a
  scanner those objects stay `PENDING`/`UNKNOWN` and are not released.
- Admin case grants are **single-use**: redemption is recorded immutably in
  `media_admin_case_redemptions` in the same transaction as the audit record
  and the byte read; a concurrent second redemption fails on the primary key.
  A failed read leaves the grant unspent. (Previously grants were reusable
  until expiry; the existing tests that relied on reuse were updated to issue
  fresh grants.)
- When the legitimate holder's read finds the case closed / proof refused
  (`CASE_PROOF_REFUSED`) or the native case record changed
  (`CASE_BINDING_CHANGED`), Media records an immutable revocation plus a
  `media.admin_case.revoked_case_closed` audit row. Later reads fail fast
  without consulting the source. An outage of the case source is never turned
  into a revocation, and a non-holder can never revoke someone else's grant.

### Legacy mapping lookup

`POST /internal/media/v1/legacy-mappings` (`adminmedia.Client.LegacyMapping`),
caller `admin-backoffice-service` only, HMAC v2 with the private Media case key
(`MEDIA_ADMIN_CASE_SECRET_FILE`), durable nonce. Returns only
`{contract_version, legacy_asset_id, media_object_id, attestation_digest}`.
Each mapping row is re-verified against the legacy row, the modern object
(owner, tenant, subject, purpose, checksum, case binding, PRIVATE) and the
recomputed owner attestation digest; an inconsistent (e.g. forged) row answers
`409 legacy_mapping_inconsistent`. Lookups are audited
(`media.legacy_mapping.resolved`). Adoption remains owner-attested only
(currently `driver-dispatch-delivery-service`, `proof_image` ->
cargo/damage/recovery/incident evidence). `document_file`, `signature` and
`profile_photo` legacy assets have no authoritative case association and are
rejected for adoption - they are never inferred from filenames, URLs or
caller input.

### Legacy privacy deletion

`DELETE /files/:id` is a redacting tombstone: bytes deleted; filename,
metadata, URL and storage path (which embedded owner id + filename) redacted;
checksum/size/purpose/owner kept; `tombstoned_at` set; an immutable
`media_legacy_asset_events` row (`TOMBSTONED`). New nullable columns
`legal_hold` (default false, reason required when true), `legal_hold_reason`,
`retention_until` (NULL - no period is invented for existing rows). A hold or
unexpired recorded retention refuses deletion with `409 deletion_blocked` and
records `DELETION_BLOCKED`.

### HQ / events

Outbox payloads carry no bytes, tokens or URLs (asserted by
`TestSection19OutboxEventsCarryNoBytesOrURLs`).

### Flags

| Variable | Default | Effect |
|---|---|---|
| `MEDIA_LEGACY_PUBLIC_SENSITIVE_DOWNLOAD_ENABLED` | `false` | Re-open `/:id/download` for non-profile purposes (compat only; floor above still applies, never inline). |
| `MEDIA_LEGACY_MAPPING_LOOKUP_ENABLED` | `false` | Mount the private Bridge legacy-mapping lookup. |
| `MEDIA_FILE_SCANNER_ADDRESS` | empty | Also used for legacy read-time scans; empty => sensitive legacy reads and high-risk modern reads fail closed. |

Migration: `000009_secure_admin_section19` (redemption ledger, legacy hold /
retention / tombstone columns, legacy event trail; rollback refuses populated
evidence).

### Remaining external dependencies

- **ClamAV production verification is external.** The test suite uses
  TEST_ONLY verdict sources to exercise Media's fail-closed decisions; it is not
  a substitute for verifying the deployed ClamAV daemon, signatures and
  network isolation.
- Bridge (`admin-backoffice-service`) wiring for `LegacyMapping` and handling
  of single-use grants (issue a fresh grant per read; treat `404` after a
  successful read as spent).
- No route sets a legacy legal hold / retention yet; they are recorded by an
  operator-approved procedure until an owning workflow exists.
- Consumers that rendered non-profile legacy `url`s publicly (see the Section
  19 report) must move to the owner route or a private grant.
