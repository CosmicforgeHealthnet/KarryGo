# Support admin event authorization scope

This change connects Support events to the existing signed source-owner allocations. It does not assign native business ownership, infer geography from deployment defaults, expand business writes, or bypass HQ approval and dataset certification.

## Source behavior

Migration 010 adds a candidate scope selector and replaces the existing event-builder function without replacing its identity or business triggers. Only a current, unambiguous allocation for `cases` with `support.cases.read` can supply administrative scope. A known native organization must agree. Missing, malformed, expired, revoked or ambiguous metadata falls back to the original incomplete native scope; authorization metadata cannot fail a business transaction. No business record or historical outbox payload is rewritten.

SQL is not a signature verifier. Before an event can be signed or sent, Support's Go guard verifies the current signed policy, an explicit `hq_event_sync` purpose, the exact authorized scope, and the current source-owner context through the existing Ed25519/binding verifier. The record's actual canonical links, fingerprint, capability, expiry and revocation are rechecked. Relay metadata must agree with the immutable payload's event ID and type. The snapshot applies the same guard. Enabled snapshots cannot operate without a configured guard.

Both source switches remain default-off. Other owners retain their existing relay behavior. Existing unscoped historical outbox events remain unchanged and are ineligible for export; current-state backfill creates new event identities. An unmapped snapshot page fails rather than claiming an empty complete result.

## Required staging release sequence

1. Pass the required exact-commit CI and disposable restore certification, then release only `support-dispute-service` through the existing delegated staging workflow. Dispatch remains on hold.
2. Keep snapshot/relay switches off during migration and initial verification. Verify native business ownership and existing outbox bytes are unchanged.
3. Install a current approved source policy explicitly allowing `hq_event_sync` for the approved case scope, plus the corresponding Bridge feed, acknowledgement and projection-status permissions. A support-review permission alone is insufficient.
4. Provision distinct source relay and snapshot workload credentials. Preserve the fixed private Bridge route, existing mTLS boundary and minimized event schema. Do not reuse reader, context publisher, command or staff keys.
5. Verify current-state backfill and incremental export with real eligible records; verify revoked/expired/binding-mismatched records are denied. Verify Stage A, dataset certification, HQ onboarding and Stage B separately before claiming full acceptance.

## Recovery

Disable snapshot and relay switches before reverting code or migration 010. The down migration restores the previous native-scope event builder and removes only the helper. Preserve owner allocations and audit/history records. Never replay old events by rewriting their payload, ID or published status, and never change business ownership to make a scope check pass. A failed guard requires investigation of the policy/allocation/binding, not a bypass.

## Verification limits

Local shared relay checks verify denial before signing/network access, exact payload preservation, and backwards compatibility for other owners. Linux/PostgreSQL checks must use an isolated temporary database and record actual passes, skips, cleanup and unchanged pre-existing containers. These results are implementation evidence, not live deployment or signed-policy approval.

Validated candidate: 76 tests passed, zero failed or skipped, with test-result caching disabled. The isolated Linux/PostgreSQL run exercised migration reversal/reapplication, current signed scope checks, policy purpose, revocation, canonical-link changes and unchanged source event bytes. Pre-existing containers were unchanged and temporary resources were removed. Sanitized evidence: `validation/support-admin-event-scope-2026-11-06.json`. This is not a live deployment claim.
