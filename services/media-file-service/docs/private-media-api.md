# Private media object contract

media-file-service is the authority on files. It is the only service that knows
whether a file exists, what is actually in it, whether anything vouched for it,
and whether the bytes are still there. Every other service holds a reference
and a projection of that answer, and none of them may contradict it.

This document covers the private, service-to-service contract added by WP10.
The pre-existing public multipart upload surface (`/files`, profile photos,
proof images, signatures) is unchanged and is not described here.

## Authority boundary

| media-file-service owns | consuming services own |
| --- | --- |
| media object identity | the business meaning of the document |
| private storage and the object key | owner association and category |
| upload authorization | the verification workflow |
| checksum, detected MIME, signature result | expiry and eligibility impact |
| malware / safety state | replacement and revocation |
| whether the bytes still exist | retention policy projection |

Consumers never receive the storage bucket, the object key, a permanent URL or
a storage credential. Content leaves this service only through a short-lived,
object-specific grant that is returned once and never stored.

## Three independent state machines

They are deliberately not collapsed into one status. An upload can be
`COMPLETED` while safety is still `PENDING`, and a file can be `SAFE` while its
deletion outcome is `DELETION_UNKNOWN`.

**Upload:** `PENDING` → `AUTHORIZED` → `UPLOADING` → `COMPLETED`, or `FAILED` /
`EXPIRED`.

**Safety:** `PENDING`, `SAFE`, `UNSAFE`, `QUARANTINED`, `UNKNOWN`.

There is no state meaning "assumed safe". `PENDING` is what a file holds before
anything looked at it; `UNKNOWN` is what it holds when something tried and could
not answer. Neither is ever reported as `SAFE`.

**Deletion:** `ACTIVE`, `DELETION_PENDING`, `DELETED`, `DELETION_UNKNOWN`,
`DELETION_FAILED`.

## Transport security

Every private route sits behind four gates, applied in this order:

1. **HMAC service authentication** using the shared `serviceauth` contract
   (`X-Service-Name`, `X-Service-Timestamp`, `X-Service-Signature`). The
   signature covers the method, the request URI, the timestamp and a hash of
   the body; a timestamp more than five minutes from now is refused.
2. **Caller allowlist** (`MEDIA_FILE_PRIVATE_ALLOWED_CALLERS`). Holding a valid
   secret proves who you are, not that you may use this contract.
3. **Single-use nonce** (`X-Service-Nonce`), claimed in
   `media_service_request_nonces` and bound to the exact method, path and body,
   so a captured request cannot be replayed while its signature is still fresh.
4. **Tenant and ownership checks** inside the use case. An object belonging to a
   different caller service or organization answers `404`, never `403`, so the
   contract cannot be used to discover that somebody else holds an object with
   a guessed identifier.

Authentication failures are rendered by the route group itself rather than by a
global error middleware, so mounting these routes on an engine without that
middleware cannot turn a rejected signature into a `200`.

## Routes

Mounted at `/api/v1/media-files/internal/media` when
`MEDIA_FILE_PRIVATE_API_ENABLED=true`.

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/upload-authorizations` | Issue a single-use upload grant |
| `POST` | `/objects/:id/content` | Redeem a grant with the file bytes |
| `GET` | `/objects/:id` | Read the authoritative object state |
| `GET` | `/objects/:id/history` | Read the append-only object history |
| `POST` | `/objects/:id/download-authorizations` | Mint a short-lived content grant |
| `GET` | `/objects/:id/content` | Stream content against a grant |
| `POST` | `/objects/:id/deletions` | Request physical deletion |
| `GET` | `/objects/:id/deletion` | Read back the authoritative deletion outcome |
| `POST` | `/objects/:id/retention` | Set retention class, window and legal hold |

### Vendor onboarding consumer

Section 7 does not add a Media route or schema. Vendor Service reuses
`GET /objects/:id` to validate that an opaque onboarding document reference is
owned by the same Vendor business, has completed upload, remains active, and is
not unsafe or quarantined. Vendor stores only the opaque object id and business
metadata; bytes, object key, URLs, grants, and safety/deletion truth remain here.
Deployment must add `vendor-service=<secret>` to the Media private service
secrets and include `vendor-service` in `MEDIA_FILE_PRIVATE_ALLOWED_CALLERS`.

### Upload authorization

The caller supplies the organization, owner type and reference, document
category, upload purpose, permitted MIME types, maximum size, an
`Idempotency-Key` and an `X-Request-Fingerprint`.

Media validates the caller, the allowlist, timestamp freshness, nonce replay,
the purpose, that every requested MIME type is inside the service-wide
allowlist, and the size ceiling. It returns the authorization id, the media
object id, a short-lived upload token, its expiry and the permitted
constraints — and nothing else.

The grant is single-use. It is enforced by a unique index (one authorization
per object) and by a row lock at redemption time, so two concurrent attempts
serialize and exactly one succeeds. Only the SHA-256 of the upload token is
persisted; holding the token itself would turn a database read into the ability
to upload as somebody else.

The object key is minted by media and embeds the tenant, the purpose and the
object id. A caller-chosen path would let one tenant write over another's
object.

A retry with the same key and the same fingerprint replays. A reused key
describing a different request is refused with `409`.

### File validation

Media validates the bytes it actually received and stored. Nothing consults the
filename or the extension to decide what a file *is*; the declared content type
is an input to a comparison, never a substitute for the bytes.

Checks, each with its own controlled failure code:

| Check | Failure code |
| --- | --- |
| Non-empty | `EMPTY_UPLOAD` |
| Size within the grant and the policy ceiling | `SIZE_EXCEEDED` |
| Declared size matches the received bytes | `INCOMPLETE_UPLOAD` |
| Declared checksum matches the received bytes | `CHECKSUM_MISMATCH` |
| Leading bytes match a known signature | `SIGNATURE_MISMATCH` |
| Detected type is inside the policy allowlist AND the grant | `UNSUPPORTED_TYPE` |
| Declared type matches the detected type | `MIME_MISMATCH` |
| No executable or archive signature (when prohibited) | `EXECUTABLE_CONTENT` |
| Stored size matches what was written | `INCOMPLETE_UPLOAD` |

A validation failure marks the object `FAILED`, records the code, removes the
bytes and emits `media.validation_failed.v1`. The object is never left in a
state a consumer could mistake for usable.

Recognised signatures: PDF, PNG, JPEG, GIF, WEBP (RIFF with a `WEBP` marker),
TIFF. Recognised executable/container signatures, refused where prohibited: MZ,
ELF, Mach-O, Java class, shebang scripts and ZIP.

### Safety

`Scanner` is a port with no default implementation and no fake that returns
`SAFE`. There is no production malware provider configured for this platform,
so:

- with no scanner, an object stays `PENDING`;
- with a scanner that errors, it becomes `UNKNOWN`;
- only a scanner that actually inspected the content and returned clean makes it
  `SAFE`.

An object the scanner flags is `QUARANTINED` and its bytes are removed: there is
no legitimate consumer for them, and keeping them only creates a way to serve
malware later.

`MEDIA_FILE_REQUIRE_SAFE_FOR_DOWNLOAD=true` refuses a content grant for anything
not confirmed `SAFE`. With no scanner configured that means no private content
can be served at all, which is the intended fail-closed posture when policy
demands a clean verdict.

### Private download

`POST /objects/:id/download-authorizations` mints a token of the form
`<expiry>.<mac>`, where the MAC covers the object id, the service the grant was
minted for and the expiry. It is verified, never looked up: there is no row to
leak and nothing to clean up when it lapses.

Media's own floor applies before any consumer policy and cannot be waived: the
upload must be `COMPLETED`, deletion state must be `ACTIVE`, and the object must
not be `UNSAFE` or `QUARANTINED`.

The token is presented in the `X-Download-Token` header rather than a query
string, so it does not end up in an access log, a proxy log or a browser
history. An expired or forged token answers `404`.

The authorization is recorded in `media_audit_records` **without** the token.
That is the difference between evidence that access was authorized and a stored
credential anybody with database access could replay.

### Deletion

`POST /objects/:id/deletions` is idempotent on `(requested_by, organization_id,
idempotency_key)`.

- A **legal hold** or an **unexpired retention window** blocks: the request is
  recorded `BLOCKED` with its reason and the object stays `ACTIVE`.
- A successful deletion destroys the bytes, sets `DELETED` with `deleted_at`,
  and emits `media.deletion_completed.v1`.
- A storage failure sets `DELETION_FAILED` and emits
  `media.deletion_failed.v1`. The caller is never told the file is gone when it
  may not be.

`GET /objects/:id/deletion` is the authoritative read-back that resolves an
`UNKNOWN` outcome on the caller's side.

The audit trail deliberately outlives the file: evidence that a document existed
and was destroyed is exactly what a later investigation needs.

## Events

Published through the transactional outbox in
`internal/events`, signed with the same HMAC contract, delivered to the
consumer's ingestion route. Redelivery reuses the same event id and body, which
is what lets the consumer's inbox recognise a duplicate.

| Event | When |
| --- | --- |
| `media.upload_completed.v1` | bytes accepted and recorded |
| `media.validation_passed.v1` | validation succeeded |
| `media.validation_failed.v1` | validation refused the file |
| `media.scan_completed.v1` | a scanner returned a verdict |
| `media.unsafe_detected.v1` | the verdict was not clean |
| `media.quarantined.v1` | the bytes were removed as unsafe |
| `media.deletion_completed.v1` | the bytes were destroyed |
| `media.deletion_failed.v1` | the deletion did not happen |
| `media.reconciliation_required.v1` | reserved for authority-initiated repair |

Every envelope carries a globally unique event id, a versioned type, aggregate
identity and version, correlation and causation ids and the producer. Consumers
use the aggregate version to reject out-of-order deliveries and detect gaps.

An event never carries raw file bytes, a signed URL, a storage credential or the
storage object key. A consumer that needs the file asks for a fresh grant.

## Schema

Migration `000002_media_object_authority` (reversible). It is additive:
`media_assets` and the public upload path are untouched.

- `media_objects` — the authoritative object, with the three state machines,
  detected and declared MIME kept separately, checksum, signature result,
  retention, legal hold and an optimistic `version`.
- `media_upload_authorizations` — single-use grants with hashed tokens.
- `media_object_history` — append-only, trigger-enforced.
- `media_audit_records` — append-only, trigger-enforced.
- `media_outbox` — durable events with leases, retries and dead-lettering.
- `media_service_request_nonces` — replay protection.
- `media_worker_leases` — single-runner coordination for the sweeps.
- `media_deletion_requests` — durable deletion with block reasons.

Constraints worth naming: `chk_media_object_completed_evidence` refuses a
`COMPLETED` upload with no checksum, detected type or size;
`chk_media_object_deleted_at` keeps `DELETED` and `deleted_at` in step;
`uq_media_object_storage_key` means two objects can never share one key.

The migration runner now uses the paired `up`/`down` convention with a
`media_file_schema_migrations` tracking table. The legacy `001_media_assets.sql`
has no down file and rolling it back is refused rather than silently skipped.

## Workers

`RunMaintenance` holds a single-runner lease and expires lapsed upload grants,
stale uploads and old nonces. The outbox worker recovers abandoned leases,
publishes, retries with bounded backoff and dead-letters; an operator can replay
a dead-lettered record.

Both stop when their context is cancelled, so a composition root that waits on
its WaitGroup lets an in-flight pass finish.

## Configuration

Every write and worker flag defaults to `false`, and example secrets are empty.

| Variable | Default | Purpose |
| --- | --- | --- |
| `MEDIA_FILE_PRIVATE_API_ENABLED` | `false` | Mount the private contract |
| `MEDIA_FILE_SERVICE_SECRETS` | *(empty)* | Per-caller HMAC secrets |
| `MEDIA_FILE_PRIVATE_ALLOWED_CALLERS` | `fleet-partner-service` | Caller allowlist |
| `MEDIA_FILE_DOWNLOAD_SIGNING_SECRET` | *(empty)* | Signs content grants; **mandatory** when the private API is on |
| `MEDIA_FILE_PRIVATE_STORAGE_DIR` | *(temp dir)* | Filesystem private storage when no bucket is configured |
| `MEDIA_FILE_UPLOAD_AUTHORIZATION_TTL_SECONDS` | `900` | Upload grant lifetime |
| `MEDIA_FILE_PRIVATE_DOWNLOAD_TTL_SECONDS` | `300` | Content grant lifetime |
| `MEDIA_FILE_PRIVATE_MAX_UPLOAD_BYTES` | `26214400` | Size ceiling |
| `MEDIA_FILE_PRIVATE_ALLOWED_MIME_TYPES` | `application/pdf,image/jpeg,image/png,image/webp` | Service-wide allowlist |
| `MEDIA_FILE_PROHIBIT_EXECUTABLE_CONTENT` | `true` | Refuse executable signatures |
| `MEDIA_FILE_REQUIRE_SAFE_FOR_DOWNLOAD` | `false` | Fail closed without a clean verdict |
| `MEDIA_FILE_DEFAULT_RETENTION_SECONDS` | `0` | Default retention window |
| `MEDIA_FILE_WORKERS_ENABLED` | `false` | Maintenance sweep |
| `MEDIA_FILE_KILL_SWITCH` | `false` | Stop every worker and the private write path |
| `MEDIA_FILE_OUTBOX_ENABLED` | `false` | Event publisher |
| `MEDIA_FILE_OUTBOX_TARGET_URL` | *(empty)* | Consumer base URL |
| `MEDIA_FILE_OUTBOX_SECRET` | *(empty)* | Publisher HMAC secret |

Enabling the private API without `MEDIA_FILE_DOWNLOAD_SIGNING_SECRET` refuses to
boot. Starting anyway would leave the routes present and every private download
failing at request time instead of at deploy time.

## Testing

`internal/integration` is gated on `MEDIA_FILE_TEST_DATABASE_URL` and runs
against real PostgreSQL, the real gin handlers, the real HMAC verifier and real
bytes on a real filesystem. Files are generated with genuine magic-byte headers
so the size, MIME, signature and checksum checks are exercised against actual
content rather than against a description of it.
