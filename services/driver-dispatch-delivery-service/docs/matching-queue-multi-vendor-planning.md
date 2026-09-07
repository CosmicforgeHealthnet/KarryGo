# Dispatch matching, queue, and multi-vendor planning

This capability is additive. The established Booking → Request → broadcast → Trip path remains the default and is unchanged. `BROADCAST_FIRST_ACCEPT` is the server-policy default; existing clients need no new fields.

## Assignment and queue

- `SEQUENTIAL_NEAREST` uses the existing Redis live-location authority and filters candidates against authoritative provider verification, onboarding, activity, and active-Trip state. Ranking is deterministic by distance and provider ID.
- `ORGANIZATION_DISPATCH` remains gated, validates the provider's active Dispatch membership through Fleet Partner's HMAC read-back, then applies the same Dispatch availability, verification, vehicle, restriction, and active-job checks before issuing an exclusive provider offer.
- `ADMIN_ASSISTED` is disabled by default and reachable only through an allowlisted Admin Backoffice HMAC route. It requires a trusted actor, reason, provider, idempotency key, and an append-only assignment audit row; it cannot bypass provider safety checks.

Exclusive offers default to 75 seconds and are clamped to the required 60–90 second range. PostgreSQL partial unique indexes enforce one live offer and one winner per mission. Accept commits the winner, then idempotently ensures a canonical Trip through the existing Trip authority. The durable queue persists bounded priority, expiry, retries, scheduling, quote freshness, and customer choices. Stale pricing moves to `WAITING_CUSTOMER`; it never silently changes a charge. Wallet remains the refund/reversal authority.

## Multi-vendor planning and manifests

The Vendor HMAC v2 contract creates controlled consolidation, split, readiness-wait, operational-review, or incompatibility outcomes. It reuses Section 6 route/zone evidence and evaluates readiness, express isolation, server-bounded vehicle capacity, temperature compatibility, and regulated-goods boundaries. Each executable Delivery Mission is correlated transactionally to a canonical Booking and Request payload; only the established Trip service creates the rider Trip.

Every Parcel has a server-generated stable KarryGo reference and opaque scannable code in addition to its caller source reference. Plans and logical manifests are versioned, manifest membership is immutable, and membership changes increment the stable manifest reference. Pickup stops persist exact expected parcel membership. Transactional custody activation uses a hard uniqueness table so one parcel cannot join two active custody manifests. Replanning requires source ownership, expected plan/source versions, and an idempotent operation fingerprint; it fails closed once assignment or custody execution has begun.

Parcel identity is not handover authorization. No OTP/QR, evidence chain, recovery/replacement, or route-integrity capability is added here.

## Rollout

All capability flags default to false: `DISPATCH_SEQUENTIAL_MATCHING_ENABLED`, `DISPATCH_QUEUE_ENABLED`, `DISPATCH_ORGANIZATION_ASSIGNMENT_ENABLED`, `DISPATCH_ADMIN_ASSISTED_ASSIGNMENT_ENABLED`, and `DISPATCH_MULTI_STOP_MISSIONS_ENABLED`. The older internal aliases `DISPATCH_SEQUENTIAL_ASSIGNMENT_ENABLED` and `DISPATCH_MULTI_VENDOR_PLANNING_ENABLED` are accepted for deployment compatibility. `DISPATCH_ASSIGNMENT_KILL_SWITCH` disables new paths. Offer and worker timing are centrally configured, including `DISPATCH_QUEUE_RETRY_INTERVAL_SECONDS`; capacity defaults are centrally configurable and callers may only lower, never raise, their effective limit.

`DISPATCH_ASSIGNMENT_ROLLOUT_SERVICE_TIERS` is a server-owned sequential
cohort selector. When configured, a sequential request outside those tiers is
handled by the existing `BROADCAST_FIRST_ACCEPT` compatibility path. During a
kill switch the worker performs drain-only reconciliation: it creates no new
offer or Queue work, but it still completes canonical Trip creation for an
already accepted winner and reconciles previously requested Wallet outcomes.

New clients display server offer expiry and queue state, send `Idempotency-Key` on decisions, prompt for explicit requote on stale pricing, and never treat a parcel reference as custody proof. Legacy clients are unaffected.
