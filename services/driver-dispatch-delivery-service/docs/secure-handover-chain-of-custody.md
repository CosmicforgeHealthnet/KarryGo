# Secure handover and chain of custody

The capability is additive and disabled by default. `DISPATCH_HANDOVER_CHALLENGE_ENABLED` switches Trip handover from the legacy code/proof flow to versioned, manifest-bound pickup and delivery challenges. Pickup and delivery remain separate stages:

- At pickup, the assigned provider retrieves a provider-held OTP/QR. The owning customer or an allowlisted, source-scoped Vendor/partner adapter submits it. Successful verification consumes both representations of that challenge. Planned multi-stop missions must also scan every expected parcel and, when configured, attach authorized evidence before custody activates.
- At delivery, notification-service transports a new receiver-held credential through configured email, in-app, or push channels. The assigned provider submits it. Required delivery evidence is appended before custody releases and the canonical Trip becomes completion-eligible.

Only HMAC hashes are persisted. OTP and QR values are deterministic cryptographic derivations of a random challenge reference and the deployment secret, so authorized display/transport can be retried without storing plaintext. QR is a one-time opaque signed token and is not parcel identity. OTP and QR share one challenge status; consuming either invalidates replay of both.

Challenges bind the exact Trip, mission, stop, manifest reference/version, provider, recipient, source version, expiry, and challenge version. Resend invalidates the old row and creates the next version. Database triggers invalidate active challenges atomically when the Trip provider, customer, or receiver phone changes. A shared Redis limiter protects issue, resend, and verification; unavailable limiting fails closed. A worker expires stale active challenges and never auto-resends.

Evidence capture begins only after the matching stage challenge is consumed. Capture sessions are short-lived, provider/mission/stop/manifest/stage scoped, and single-purpose. Dispatch stores immutable references and safe capture metadata; media-file-service remains byte and attachment-metadata authority. Possessing a MediaRef alone is insufficient: Dispatch performs authenticated metadata read-back and rejects wrong owner, purpose, mission, or parcel relations.

Pickup and delivery gates are fully implemented here. Existing Section 6 package-policy/repricing remains the authority for package discrepancies; the evidence schema accepts discrepancy/failed-attempt/provider-transfer/return-compatible stages so existing operational evidence can correlate without letting a provider change price or creating a second exception domain.

`dispatch_parcel_scan_events`, handover attempts, and evidence references are append-only. Pickup custody activation transactionally claims all manifest parcels through the existing active-custody uniqueness table. Delivery releases that membership only when the configured challenge/evidence gate is complete. Corrections append a superseding evidence reference rather than rewriting history.

Configuration:

- `DISPATCH_HANDOVER_CHALLENGE_ENABLED=false`
- `DISPATCH_PICKUP_EVIDENCE_GATE_ENABLED=false`
- `DISPATCH_DELIVERY_EVIDENCE_GATE_ENABLED=false`
- `DISPATCH_HANDOVER_QR_ENABLED=false`
- `DISPATCH_HANDOVER_CHALLENGE_TTL_SECONDS=900`
- `DISPATCH_EVIDENCE_CAPTURE_SESSION_TTL_SECONDS=600`
- `DISPATCH_HANDOVER_RESEND_COOLDOWN_SECONDS=45`
- `DISPATCH_HANDOVER_MAX_ATTEMPTS=5`
- `DISPATCH_HANDOVER_MAX_RESENDS=5`
- `DISPATCH_HANDOVER_WORKER_INTERVAL_SECONDS=30`
- `DISPATCH_HANDOVER_POLICY_VERSION=dispatch_secure_handover_v1`
- `DELIVERY_HANDOVER_EMAIL_ALLOWED=true`
- `DELIVERY_HANDOVER_IN_APP_ALLOWED=true`
- `DELIVERY_HANDOVER_PUSH_ALLOWED=true`
- `DELIVERY_HANDOVER_SMS_ALLOWED=false`

SMS domain intent is reserved, but transport is not activated because notification-service currently has no SMS channel. Dispatch does not integrate directly with an SMS provider.

Frontend clients must handle controlled expired, invalidated, locked, evidence-required, MediaRef-scope, and 429 states. Provider apps may render only pickup credentials for their current assignment. Provider, Vendor, partner, Admin, Support, generic read models, analytics, and outbox consumers must never receive the receiver delivery credential.
