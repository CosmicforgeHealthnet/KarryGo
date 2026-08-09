# Fleet document domain

Fleet stores a **pointer** to a file plus everything the business needs to know
about it: whose it is, what category it is, when it expires, whether a human
verified it, whether it was replaced or revoked. Fleet stores nothing about the
file itself — not the bytes, not the location, not the detected type, not the
safety verdict — except as a projection of what media-file-service said,
carrying the version that answer belonged to.

The distinction is what this domain protects. Fleet may decide that a verified
insurance certificate satisfies a requirement; it may never decide that the file
is safe, that it still exists, or that it is really a PDF.

| media-file-service owns | fleet owns |
| --- | --- |
| the bytes | which company, branch, provider link or vehicle link it is for |
| the storage location | the category, type and purpose |
| the detected type | the verification decision |
| the checksum | the issue and expiry dates |
| the safety verdict | replacement and revocation |
| whether it still exists | retention policy, legal hold, eligibility impact |

Fleet never stores raw bytes, a permanent public URL, a signed URL, a storage
path or a credential. There is no column for any of them, and the migration
test asserts that.

## Owners

`COMPANY`, `BRANCH`, `PROVIDER_LINK`, `VEHICLE_LINK`, `INCIDENT`, `DISPUTE`,
`APPROVAL`. Fleet holds no vehicle table, so a vehicle-linked document is
anchored to the provider membership that controls the vehicle — a record fleet
does own.

Categories: `INSURANCE`, `REGISTRATION`, `PERMIT`, `COMPLIANCE`, `PROOF`,
`INCIDENT_ATTACHMENT`, `DISPUTE_ATTACHMENT`, `APPROVAL_ATTACHMENT`,
`SUPPORTING`.

## States

`lifecycle_state` carries the full controlled vocabulary:

`UPLOAD_PENDING`, `UPLOADED`, `VALIDATION_PENDING`, `SAFE`, `UNSAFE`,
`QUARANTINED`, `VERIFICATION_PENDING`, `VERIFIED`, `REJECTED`, `EXPIRED`,
`REVOKED`, `REPLACED`, `DELETED`, `UNKNOWN`, `STALE`, `GAP_DETECTED`,
`RECONCILING`.

The last four are not error cases. Each means "nobody can currently vouch for
this document", and every gate treats them exactly as it treats `UNSAFE`: it
blocks. A stale "safe" is not a weaker yes; it is not a yes at all.

Three further columns keep the projections separate from fleet's own opinion:
`verification_state` (`PENDING`/`REQUESTED`/`VERIFIED`/`REJECTED`/
`INVALIDATED`), `safety_projection` and `media_deletion_state` (media's
vocabulary, unchanged), and `projection_freshness`
(`CURRENT`/`STALE`/`GAP_DETECTED`/`UNKNOWN`/`RECONCILING`).

`safety_projection` defaults to `UNKNOWN` and `projection_freshness` defaults to
`UNKNOWN`. A document fleet has heard nothing about is unknown, not fine.

## Upload orchestration

`POST /fleet-partner/v1/documents/upload-requests`

1. Shape and tenant ownership are checked first, so a malformed or cross-tenant
   request never reaches the network.
2. The idempotent replay is resolved, so a retry cannot create a second
   document.
3. Media is asked for a grant **before** the transaction, because an external
   call must not hold a database lock.
4. The document, its history, the audit row and the outbox event commit
   together.

The media object id is minted by media and carried into the row. Fleet never
invents one: a document pointing at an object media does not know about would be
unverifiable forever.

The upload token is returned once in the response. It is not stored on the fleet
side, does not appear in the document row, and is never logged. A replay returns
the existing document without a token; a caller that still needs to upload asks
again with a new key.

## Media event ingestion

`POST /internal/fleet-partner/v1/documents/media-events`, behind the internal
route class (HMAC + timestamp freshness + nonce) plus a per-route caller
allowlist.

Six protections apply to every inbound event, each for a specific way a
projection goes wrong:

| Protection | Catches |
| --- | --- |
| Inbox deduplication | the same event delivered twice |
| Payload hash | the same event id with a **different** body |
| Source-version conflict | a new event id claiming a recorded version with a different body |
| Older-event guard | a late delivery overwriting newer state |
| Version-gap detection | a jump in versions meaning something was missed |
| Tenant check | an event claiming a tenant the document does not belong to |

Outcomes returned to the publisher: `PROCESSED`, `DUPLICATE`,
`DUPLICATE_VERSION`, `OLDER`, `GAP_DETECTED`, `IGNORED`. Status codes are
load-bearing: `200` means stop retrying, `409` means the same body will never
succeed, `503` means redeliver.

A version gap marks the projection `GAP_DETECTED`, opens a reconciliation case
and emits `organization.document.reconciliation_required.v1`. The document
authorizes nothing until the case is resolved.

### What invalidates a standing verification

Three things, each a real way a verified document stops meaning what it meant:

- the content is no longer confirmed `SAFE`;
- the object was deleted;
- media is describing a **later source version** of the file than the one that
  was signed off.

Invalidation is recorded as a distinct `INVALIDATED` decision, not a rejection —
nobody decided the document was wrong.

## Verification

| Route | Permission |
| --- | --- |
| `POST /documents/:id/verification-requests` | `documents.manage` |
| `POST /documents/:id/verify` | `documents.verify` |
| `POST /documents/:id/reject` | `documents.verify` |
| `GET /documents/:id/verifications` | `documents.read` |

**Only a `SAFE` document can become `VERIFIED`.** `PENDING`, `UNKNOWN`,
`UNSAFE` and `QUARANTINED` all refuse, because each means the platform does not
actually know the content is clean. The rule is enforced twice: in the use case,
and by
`chk_fleet_partner_document_verified_requires_safe` in the schema, so a future
code path that skipped the check would still fail rather than store a lie.

Also refused: a terminal document, an upload that is not `COMPLETED`, a
projection that is not `CURRENT`, and an expired document.

Separation of duties: the submitter cannot verify their own document unless
`FLEET_DOCUMENTS_SELF_VERIFICATION_ALLOWED=true`. The value of a verification
step is that somebody other than the submitter looked.

Rejections carry a controlled reason (`ILLEGIBLE`, `WRONG_DOCUMENT`,
`EXPIRED_ON_ISSUE`, `DETAIL_MISMATCH`, `INCOMPLETE`, `SUSPECTED_FRAUD`,
`UNSAFE_CONTENT`, `SOURCE_VERSION_CHANGED`) so whoever has to fix it knows what
to do. Decisions are append-only, trigger-enforced, and record the media source
version they were taken against.

## Replacement, revocation, expiry

**Replacement** creates a new document and preserves the old one. The successor
inherits nothing but its owner: its own media object, its own validation, its
own verification. Inheriting a verification would mean a new file is trusted
because an old one was. The original becomes `REPLACED`, gains a pointer to its
successor, and its standing verification is invalidated.

**Revocation** requires a reason (refused if empty, enforced by
`chk_fleet_partner_document_revocation`). History is preserved; what the
document loses is the ability to satisfy a requirement or be released for
download, and both take effect immediately rather than at the next sweep.

**Expiry** uses the explicit `expiry_date`. The recoverable expiry worker claims
a bounded batch with `SKIP LOCKED`, marks each `EXPIRED`, invalidates the
verification, invalidates affected eligibility decisions and emits
`organization.document.expired.v1`. A crash halfway through leaves the committed
ones expired and the rest for the next pass.

## Retention, legal hold and deletion

Two rules, both because deletion cannot be undone:

1. **Fleet cannot mark a document physically deleted without media saying so.**
   A request that timed out is `UNKNOWN`, and `UNKNOWN` is resolved by reading
   media back, never by assuming.
2. **A legal hold or an unexpired retention window blocks on both sides.** Fleet
   refuses to schedule it and media refuses to perform it, so neither a fleet
   bug nor a direct media call is sufficient on its own to destroy evidence
   somebody is required to keep.

`POST /documents/:id/retention` sets the class, window and hold on both sides.
Propagation to media is not best-effort: a failure is reported, because a legal
hold the operator believes is in force but that media never received is worse
than a visible error.

The deletion worker leases a batch, calls media, and settles:

| Media says | Fleet records |
| --- | --- |
| `COMPLETED` | document `DELETED`, verification invalidated, decisions invalidated |
| `BLOCKED` | request `BLOCKED` with the reason; document untouched |
| `FAILED` | request `FAILED`, retried with backoff, eventually dead-lettered |
| *(timeout)* | request `UNKNOWN`, document `RECONCILING`, a case opened |
| `404` | treated as deleted — it cannot be reachable |

Dead-lettered requests are replayable by an operator through
`POST /documents/deletion-requests/:id/replay`.

Audit evidence deliberately outlives the file.

## Reconciliation

A case is opened whenever fleet's projection and media's authority might
disagree: `VERSION_GAP`, `STALE_PROJECTION`, `UNKNOWN_OUTCOME`,
`DELETION_UNKNOWN`, `UPLOAD_UNKNOWN`, `SAFETY_UNKNOWN`, `PAYLOAD_CONFLICT`.

While a case is open the document authorizes nothing — not eligibility, not
download — because the honest answer is that nobody currently knows the file's
state.

The repair is always the same and always one-directional: ask media, believe the
answer, write it down. Source versions move forward only, so a read-back never
moves the projection backwards. Fleet never reconciles by deciding what the
state probably was.

The worker recovers abandoned leases first (a worker that died holding leases
must not strand its cases), then runs the stale sweep, then repairs claimed
cases. Failures retry with backoff and dead-letter after
`FLEET_DOCUMENTS_RECONCILE_MAX_ATTEMPTS`; an operator can replay through
`POST /documents/reconciliation-cases/:id/replay`.

The stale sweep is what catches silence. Without it a document whose media
events simply stopped arriving would stay `CURRENT` forever and keep authorizing
work on the strength of an answer that might be months old.

At most one open case per document — a partial unique index enforces it, because
doubling the retry rate against an authority that is already struggling is the
opposite of helpful.

## Eligibility integration (WP9)

There is no second eligibility framework. The WP9 checks were extended.

`ReferenceDocumentStates` evaluates the newest live document per required type
and answers with a controlled reason code:

| Reason | Meaning |
| --- | --- |
| `document_missing` | no document of that type |
| `document_not_verified` | present but nobody signed it off |
| `document_not_safe` | media has not confirmed the content |
| `document_expired` | past its stated expiry |
| `document_revoked` | withdrawn |
| `document_replaced` | superseded and the latest is required |
| `document_deleted` | the bytes are gone |
| `document_projection_stale` | fleet cannot currently vouch for the projection |
| `document_reconciling` | an unresolved case, or a detected gap |

An unresolved reconciliation case outranks everything else.

`document_projection_stale` maps onto the `STALE` decision status and
`document_reconciling` onto `RECONCILING` — both mean "ask again" — while the
rest map onto `INELIGIBLE`, which means asking again will not help.

**Legacy fallback.** For the organization itself, the pre-WP10 onboarding
attachment still satisfies a requirement. An organization onboarded before WP10
must not be blocked for having used the record that existed at the time. What is
not negotiable is the standard the WP10 record has to meet.

**Invalidation.** Anything that could stop a document satisfying a requirement —
revocation, replacement, expiry, deletion, an unsafe verdict, a stale
projection, an open case — supersedes the current eligibility decisions for the
affected subject and records why in the decision's history. The decision rows
stay immutable; what changes is that they are no longer current, so the next
gate re-evaluates instead of reusing a "yes" that was true about a document
nobody stands behind any more.

## Visibility and archival

Every document carries a `visibility` tier, set at creation and defaulting to
the strictest option when the caller does not name one:

`INTERNAL` (default), `PROVIDER_VISIBLE`, `CUSTOMER_VISIBLE`, `RESTRICTED`,
`COMPANY_CONFIDENTIAL`.

Ordinary `documents.download` gates the first three. `RESTRICTED` and
`COMPANY_CONFIDENTIAL` additionally require `documents.download_restricted` —
a caller who can download every other document does not automatically gain
access to the most sensitive tier. The gate is checked at the HTTP layer,
before `AuthorizeDownload` is asked to evaluate anything else, so a caller
without the elevated permission never learns whether the file would otherwise
have been downloadable.

`POST /documents/:id/archive` (`documents.manage`, `expectedVersion` guarded)
takes a document out of default listings without touching what it says: not
its lifecycle state, not its verification, not its visibility. Archiving an
already-archived document is refused (`409`), matching revoke's convention in
this domain. `GET /documents?includeArchived=true` still finds an archived
document; the default listing does not.

There is deliberately no provider-facing or customer-facing read path today —
every route in this domain is corporate/organization-user only.
`PROVIDER_VISIBLE` and `CUSTOMER_VISIBLE` are recorded and available for a
future surface to filter on; nothing currently reads them for that purpose.

## Permissions

Six capabilities, deliberately separate. Someone who may attach an insurance
certificate is not automatically someone who may declare it verified, and
neither is automatically someone who may read the file.

| Permission | Owner | Admin | Compliance | Operations | Fleet | Dispatcher |
| --- | --- | --- | --- | --- | --- | --- |
| `documents.read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `documents.manage` | ✓ | ✓ | | ✓ | ✓ | |
| `documents.download` | ✓ | ✓ | ✓ | | | |
| `documents.download_restricted` | ✓ | | ✓ | | | |
| `documents.verify` | ✓ | ✓ | ✓ | | | |
| `documents.delete` | ✓ | ✓ | | | | |

Compliance is the verifying role: it reads and decides, and may reach the file
to do so, but does not upload the evidence it judges. It is also the role
trusted with the most sensitive visibility tier; the company administrator's
`documents.download` deliberately does not extend that far.

Default deny is structural: every handler resolves the tenant first and refuses
when the authorizer is absent, so a missing gate can never read as an open one.
Branch scope is applied in SQL for listings and re-checked for reads by
identifier, so a caller restricted to one branch cannot reach another's document
by knowing its id. Out-of-scope identifiers answer `404`, not `403`.

## Private download

`POST /documents/:id/download-requests` (`documents.download`).

Fleet's rules are applied first — tenant, owner relationship, branch scope,
RBAC, lifecycle state, safety projection, deletion and revocation state,
projection freshness, and no open reconciliation case. Media applies its own
floor on top and may still refuse; a document that passes here and is refused
there is working exactly as intended.

The grant is returned once. It is not stored, does not appear in history or
audit, and is never logged. The audit row names the authorization without the
token.

Blocked: `UNSAFE`, `QUARANTINED`, `DELETED`, `UNKNOWN`, `STALE`,
`GAP_DETECTED`, `RECONCILING`, revoked documents, and documents whose media
object is not `ACTIVE`.

## Routes

All tenant-scoped under `/fleet-partner/v1`.

| Method | Path | Permission |
| --- | --- | --- |
| `POST` | `/documents/upload-requests` | `documents.manage` |
| `GET` | `/documents` | `documents.read` |
| `GET` | `/documents/:id` | `documents.read` |
| `GET` | `/documents/:id/history` | `documents.read` |
| `GET` | `/documents/:id/verifications` | `documents.read` |
| `POST` | `/documents/:id/download-requests` | `documents.download` |
| `POST` | `/documents/:id/verification-requests` | `documents.manage` |
| `POST` | `/documents/:id/verify` | `documents.verify` |
| `POST` | `/documents/:id/reject` | `documents.verify` |
| `POST` | `/documents/:id/replace` | `documents.manage` |
| `POST` | `/documents/:id/revoke` | `documents.manage` |
| `POST` | `/documents/:id/archive` | `documents.manage` |
| `POST` | `/documents/:id/retention` | `documents.delete` |
| `POST` | `/documents/:id/deletion-requests` | `documents.delete` |
| `GET` | `/documents/:id/deletion` | `documents.read` |
| `GET` | `/documents/reconciliation-cases` | `documents.read` |
| `POST` | `/documents/reconciliation-cases/:id/replay` | `documents.delete` |
| `POST` | `/documents/deletion-requests/:id/replay` | `documents.delete` |

Writes require an `Idempotency-Key`. State changes accept an
`expectedVersion`; a mismatch answers `409`.

## Events

`organization.document.registered.v1`, `.state_changed.v1`, `.verified.v1`,
`.rejected.v1`, `.replaced.v1`, `.revoked.v1`, `.expired.v1`, `.deleted.v1`,
`.reconciliation_required.v1`, `.archived.v1`.

Payloads are assembled field by field and carry no URL, no token, no storage
reference and no file content — only the document's standing.

## Configuration

Every write and worker flag defaults to `false`.

| Variable | Default |
| --- | --- |
| `FLEET_DOCUMENTS_WRITES_ENABLED` | `false` |
| `FLEET_DOCUMENTS_DOWNLOAD_ENABLED` | `false` |
| `FLEET_DOCUMENTS_EVENT_INGESTION_ENABLED` | `false` |
| `FLEET_DOCUMENTS_KILL_SWITCH` | `false` |
| `FLEET_DOCUMENTS_ALLOWED_CALLERS` | `media-file-service` |
| `FLEET_DOCUMENTS_SELF_VERIFICATION_ALLOWED` | `false` |
| `FLEET_DOCUMENTS_MAX_DOWNLOAD_BYTES` | `33554432` |
| `FLEET_DOCUMENTS_PROJECTION_STALE_SECONDS` | `900` |
| `FLEET_DOCUMENTS_RECONCILIATION_ENABLED` | `false` |
| `FLEET_DOCUMENTS_RECONCILE_INTERVAL_SECONDS` | `30` |
| `FLEET_DOCUMENTS_RECONCILE_BATCH_SIZE` | `25` |
| `FLEET_DOCUMENTS_RECONCILE_LEASE_SECONDS` | `120` |
| `FLEET_DOCUMENTS_RECONCILE_MAX_ATTEMPTS` | `8` |
| `FLEET_DOCUMENTS_RECONCILE_BACKOFF_SECONDS` | `30` |
| `FLEET_DOCUMENTS_RECONCILE_SWEEP_LIMIT` | `200` |
| `FLEET_DOCUMENTS_EXPIRY_WORKER_ENABLED` | `false` |
| `FLEET_DOCUMENTS_EXPIRY_INTERVAL_SECONDS` | `3600` |
| `FLEET_DOCUMENTS_EXPIRY_BATCH_SIZE` | `200` |
| `FLEET_DOCUMENTS_DELETION_WORKER_ENABLED` | `false` |
| `FLEET_DOCUMENTS_DELETION_INTERVAL_SECONDS` | `60` |
| `FLEET_DOCUMENTS_DELETION_BATCH_SIZE` | `25` |
| `FLEET_DOCUMENTS_DELETION_LEASE_SECONDS` | `120` |
| `FLEET_DOCUMENTS_DELETION_BACKOFF_SECONDS` | `60` |
| `FLEET_DOCUMENTS_DELETION_MAX_ATTEMPTS` | `8` |

The media client is built from `FLEET_PARTNER_MEDIA_FILE_SERVICE_BASE_URL` and
`FLEET_PARTNER_MEDIA_FILE_SERVICE_SECRET`. When either is missing the client is
nil, and every path that needs an authoritative answer about a file fails closed
with "unavailable" — it never makes the check disappear.

Both the API binary (with `FLEET_PARTNER_WORKER_ENABLED`) and the dedicated
worker binary start the same three workers, so splitting the deployment does not
silently stop expiries and deletions from happening.

## Schema

Migration `000021_fleet_documents` (reversible), extended additively by
`000054_document_visibility` (`visibility`, `archived_at`,
`archived_by_actor_id`, both reversible).

- `fleet_partner_documents` — the reference, its projections, the replacement
  chain, revocation, retention, legal hold, visibility, archival and an
  optimistic `version`.
- `fleet_partner_document_history` — append-only, trigger-enforced.
- `fleet_partner_document_verifications` — immutable decisions with the media
  source version they were taken against.
- `fleet_partner_document_media_versions` — forward-only source versions with
  payload hashes.
- `fleet_partner_document_reconciliation_cases` — leases, retries,
  dead-lettering.
- `fleet_partner_document_deletion_requests` — scheduling, leases, block
  reasons.

`media_object_id` is opaque text with **no** foreign key: a cross-service
foreign key would make fleet's writes depend on another service's table.
`uq_fleet_partner_document_media_object` means two documents can never share one
object — otherwise a revoked document's file would stay reachable through its
twin.
