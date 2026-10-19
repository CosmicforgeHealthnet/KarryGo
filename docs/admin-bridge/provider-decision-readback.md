# Fleet provider decision read-back

This source-owned contract extends the existing Fleet/Dispatch HMAC boundary.
It grants no HQ permission and does not authorize execution or alter custody.

`GET /internal/fleet-partner/v1/assignments/{assignmentId}/provider-decision-receipt?providerId={providerId}&clientResponseId={originalKey}`

Only the allowlisted execution service owning that provider category may read.
Fleet joins its actual assignment, immutable response, membership/organization
binding and evidence set in one PostgreSQL statement. No incoming tenant or
organization is accepted. Missing, historical-unmapped or foreign identity
returns no receipt. Response fields are contract version, receipt/assignment/
provider/original-key references, decision, canonical intent digest and recorded
time. Notes, evidence IDs and organization/membership details are not returned.

The digest binds assignment, provider, original key, ACCEPTED/DECLINED, reason,
note and the sorted unique evidence set. Transport/session identity and the
original optimistic precondition are not new decision inputs. A receipt proves
a historical decision was committed; later cancellation/expiry/completion does
not erase it. Existing acceptance verification still independently decides
whether execution is currently allowed. Fleet rejects same-key replay with a
different decision, reason, note or evidence set.

Dispatch separately leases uncertain rows using a fresh token per claim and
performs only GET read-back. Matching receipts atomically confirm the operation
and append immutable audit evidence. Missing, invalid, mismatched, redirected,
oversized or unavailable responses preserve the uncertainty fence. Audit failure
rolls back confirmation. An expired reader cannot complete a newer lease. There
is no POST fallback and no fresh-key recreation of a fenced decision.

Apply Dispatch migration 071 before running the new reader. Its audit evidence
cannot be updated, deleted or truncated; down migration refuses to erase it.
Do not downgrade to binaries that redeliver UNKNOWN_OUTCOME. The fixed Fleet
origin rejects URL credentials/query/fragment/base paths and bypasses proxy
environment inheritance; redirects and automatic mutation retries are disabled.

Current coverage: standard assignment, private assignment and marketplace-offer accept/decline. Private receipts use the native immutable private decision ledger, canonical planning organization/provider/service and original idempotency key. Native safe-note trimming is reproduced for digest comparison. Fresh-key private replays conflict, even when the rest of the request fingerprint is identical. Private evidence is not silently discarded: new native requests with evidence are refused before mutation, and uncertain historical requests containing unrecorded evidence stay fenced. Protected evidence support still requires its source-owned Media contract. Marketplace-offer accept/decline now record a source-owned immutable receipt atomically with the native decision (Fleet migration 081). Offer proof uses fleet.provider-offer-decision.v1 and binds the offer/provider/original key, exact reason/note/evidence metadata and, for acceptance, the original source reservation and request version. Evidence digests record submitted metadata; they do not validate Media ownership or grant access. The offer endpoint is /internal/fleet-partner/v1/marketplace-offers/{offerId}/provider-decision-receipt. Legacy offer rows without immutable intent evidence remain fenced; no proof is fabricated from mutable status fields. Replay checks use canonical provider membership, organization, projection and source category before matching the original decision digest. Receipt insertion failure rolls back native acceptance/assignment/execution work. This is an
engineering checkpoint, not completion of Sections 6–10 or release approval.

Tests use Fleet's real migration chain/PostgreSQL and actual HMAC handlers,
plus Dispatch PostgreSQL with signed HTTP contract fixtures, twelve concurrent
readers, stale leases, semantic mismatches and audit faults. Fixtures are not a
claim of deployed cross-service certification. The dedicated owner workflow
requires these named scenarios and rejects skipped tests.
