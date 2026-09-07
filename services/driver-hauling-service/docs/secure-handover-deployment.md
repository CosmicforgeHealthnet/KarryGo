# Secure handover deployment transition

Migration `016_secure_handover_challenges` upgrades the existing Hauling
booking handover authority. It does not create another service or change the
Commerce feature flag.

Deployment order:

1. Pause handover issue/resend traffic and place the secure-capable Hauling
   binary on every serving instance. Do not leave a legacy writer serving when
   migration 016 is applied.
2. Apply migration 016. It creates the versioned hash-only challenge table,
   clears legacy redisplay plaintext, and installs the database check that
   rejects future non-empty legacy plaintext writes. Existing legacy HMACs,
   expiry, attempt, resend, and verification timestamps remain intact.
3. Resume traffic. New issue/resend commands create versioned OTP plus opaque
   QR credentials. An actor already holding an unexpired legacy OTP may submit
   it during the bounded drain window; no endpoint can redisplay it. A resend
   converts that booking/stage atomically to secure version 1.
4. After the 15-minute legacy TTL plus in-flight request allowance, verify no
   active legacy hashes remain. They may then be retained as historical
   security metadata or scrubbed under the platform retention policy; they are
   not bearer credentials and cannot be reversed into plaintext.

If a legacy client navigated away before migration and no longer possesses its
OTP, it must use the existing authenticated resend command. The old credential
is not recovered or copied to another store.

Rollback requires disabling issue/resend traffic first. The down migration
removes the new table and plaintext guard, so it must not be used to resume
legacy plaintext issuance. Commerce stays disabled by default throughout.
