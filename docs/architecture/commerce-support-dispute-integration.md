# Commerce support and dispute integration

Commerce issue reporting is a cross-service workflow with intentionally narrow authority seams:

1. Vendor Service authenticates the customer/order or Vendor/business scope, derives operational references from its own projections, and evaluates the existing Section 24 refund policy where applicable.
2. An ambiguous issue is sent as a typed, fingerprinted HMAC-v2 request to Support/Dispute Service. Support stores only opaque references and is the formal case/adjudication authority.
3. Support commits a controlled, versioned resolution and its outbox event in one transaction. Its retry worker delivers at least once to Vendor's private HMAC-v2 callback.
4. Vendor's event inbox makes replay harmless and prevents a stale resolution from replacing a newer projection. The same projection serves the customer Commerce read surface; Customer Service does not gain a duplicate domain.
5. A refund authorization resumes the already-persisted Section 24 policy decision. Wallet remains the only ledger and its idempotent operation/read-back determines `refund_status`. `RESOLVED` is therefore separate from `APPLIED`/settled.
6. Vendor's transactional outbox sends controlled customer/Vendor semantic notifications through Notification Service. Neither business service owns SMS, email, push, or websocket transport.

Operational evidence remains in its existing authority. Support context references immutable manifests, parcels, custody events, delivery exceptions, return/provider-transfer evidence, ETA history, substitution/refund records, and Media File object IDs. It does not copy media bytes, profiles, addresses, contacts, payment material, raw OTP/QR values, or challenge secrets.

Section 37 was not implemented or modified by this work. No Section 39 capability is introduced.
