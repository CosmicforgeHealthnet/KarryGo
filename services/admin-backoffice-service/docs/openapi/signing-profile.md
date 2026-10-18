# Karryam HQ signing profile v1

Pin `openapi.json`, `SHA256SUMS` and the capability matrix to the reviewed Git
commit. Examples are inert fixtures, not live credentials. Only KarryGo owns
this contract; the Centralized Admin repository was not modified.

## Independent proofs

1. Approved private transport, followed by TLS 1.3 with a client chain trusted by
   the dedicated HQ CA. The certificate contains exactly the approved URI SAN.
   Its DER SHA-256 fingerprint must be locally enrolled for fixed principal
   `cosmicforge-hq-karryam-admin`, product `karryam`, pinned audience and an
   unexpired, non-revoked enrollment. Proxy identity headers are ignored.
2. HQ-purpose HMAC key. Include `X-HQ-Key-ID` and the existing `X-Service-Name`,
   `X-Service-Timestamp`, `X-Service-Nonce`, `X-Service-Signature-Version: 2`,
   `X-Service-Signature`. Name is exactly `cosmicforge-hq-karryam-admin`.
   Preserve shared `serviceauth.SignRequest` semantics: LF-separated version,
   service, uppercase method, exact request URI, decimal Unix timestamp, nonce,
   lower-case hex body SHA-256; signature is hex HMAC-SHA256. Acceptance skew is
   30 seconds. Duplicate headers and noncanonical whitespace nonces are rejected.
3. `Authorization: Bearer <staff-assertion>`, an Ed25519 JWS with header exactly
   `alg=EdDSA`, `typ=JWT`, pinned `kid`. No token-controlled key URLs or algorithm
   negotiation. Unpadded base64url. Payload follows `schemas/StaffAssertion.json`.
   Issuer/audience match local policy exactly; product is always `karryam`.

`request_digest` is lower-case hex SHA-256 of UTF-8, without trailing newline:

```text
METHOD + LF + exact URL.RequestURI() + LF + "karryam" + LF + hex(SHA256(exact body bytes))
```

Query order, escaping and body whitespace are part of both bindings. Canonical
paths have no encoded bytes, backslashes, dot segments or duplicate query keys.
Every route requires one `market`, `organization`, `purpose` query parameter.
Command scope also equals URI scope. Actor comes exclusively from verified
claims; unknown command fields such as `performed_by` are rejected.

All assertion claims are required. `iat <= nbf <= now < exp`, `auth_time <= iat`.
Assertion/session limits and allowed/strong ACR values come from approved local
policy (hard safety ceilings: assertion 300 seconds; session one day). No default
MFA or financial thresholds. Permission/market/organization matches are exact;
wildcards are rejected. Roles are never treated as operation permissions.

## Replay and outcomes

Nonce and issuer/JTI are claimed together in one PostgreSQL transaction and
survive replicas/restarts. Revoked staff/sessions and certificate enrollments are
checked per request. Infrastructure failure never falls back to in-memory trust.
Each retry requires fresh nonce, JTI and assertion. Command retries preserve ID,
idempotency key and exact serialized bytes. Different bytes/staff/command ID with
the same key, or the same command ID with a new key, yield 409. Idempotency is
scoped to HQ principal, market and organization; reads also require original staff.

No domain command is executable. HTTP 503 `capability_blocked`, `retryable:false`
with operation ID means durably REJECTED. `target.expected_version` is requested,
while `source_version` is null until an owner actually observes it. Local
timestamps describe bridge metadata. Future owner timeouts must become
UNKNOWN_OUTCOME; only authoritative read-back can resolve them. Never retry an
uncertain irreversible action automatically.

## Rotation, privacy and compatibility

Remote HMAC keys are separate from owner credentials. Internal caller remains
`admin-backoffice-service`; no global domain secret exists. Staff keys are public
Ed25519 keys; HMAC values are at least 32 random bytes. Trust file values use
unpadded base64url. Every key ID has a mandatory Unix expiry in
`staff_key_expires` or `service_key_expires`.

Add a new key ID/expiry alongside the old, deploy policy, switch HQ signing,
wait out the bounded assertion window, then remove old ID and redeploy. Emergency
revocation may skip overlap. Certificate rotation independently enrolls the new
fingerprint; revoke old enrollment after testing. Revocation is checked even on
existing keep-alive TLS connections.

Responses expose minimized bridge metadata only. Unknown data is null or
UNAVAILABLE. JWTs, keys, request bodies, free-text reasons, private evidence,
downstream errors, NIN, OTP, bank data and identity documents are not logged or
returned. Use opaque purpose/case references. Errors contain stable code, safe
message, retryability and a bridge-generated correlation UUID. Durable audit
failure suppresses data responses and emits a sanitized emergency host log.

The dormant cursor helper binds staff/permission/product/market/organization,
expires and signs cursors using a separate key. Resource limits are 100 rows and
31 days. No paginated owner endpoint is advertised. Owner-specific filtering,
ETags and freshness need the actual scoped owner adapter first.

Additive endpoints/schemas may extend v1. Existing semantic request, response,
scope or authentication changes fail the conservative compatibility check and
require reviewed version migration with overlap. `contracts.py` checks generation
and checksums; `validate.py --base <reviewed-base>` checks compatibility and actual
wire samples. HQ must discover capability state and never assume disabled work
is executable.
# Scoped owner-read extension

The additive `StaffAssertionZones` schema is required for the Taxi ride routes.
It includes signed `zone_scopes` in addition to the existing market and
organization scopes. Assertions for the original metadata routes remain valid
without this field. Wildcards, duplicate claims and duplicate scope values are
rejected. Taxi reads also require `taxi.rides.read` and an exact role in the
operator-provisioned `ADMIN_TAXI_READ_ROLES` policy; no role is implicitly approved.

The request digest covers the exact URI, including market, zone, organization,
purpose, time window, page limit and cursor. The bridge supplies the verified
staff identity to the owner in its newly signed HMAC request. HQ cannot supply
the owner `staff` parameter or choose a destination. Neither the HQ JWT nor any
incoming authentication header is forwarded to Taxi.
