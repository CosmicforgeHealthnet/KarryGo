# Core CRM workspace

Enter More ? Customers & relationships. The workspace groups customers, follow-ups, interactions and corporate accounts alongside tags, segments and loyalty configuration. Mock remains the app default; Live requests use the public CRM API and signed-in scope.

## Implemented flows

- Customer roster and customer view: server relationship status, commercial availability, tags/segments, activity, loyalty, consent/privacy and intelligence reads; archive/restore uses the actual version header.
- Tags: create/edit/archive and bulk assignment/removal.
- Segments: static membership, dynamic rules from the actual field registry, preview, rule replacement, materialization and exclusions. Unsupported/unavailable fields cannot be invented by the app.
- Interactions: creation, versioned corrections and preserved revision evidence.
- Tasks: create/edit/reassign, start/complete/cancel and history. Terminal tasks cannot be edited.
- Loyalty: programme configuration, whole-array tier replacement, customer ledger, manual adjustments and append-only reversal.
- Corporate accounts: lifecycle, branch scope, contacts linked to existing customer profiles, locations, cost centres, memberships, interactions and tasks. Related records are selected from the actual account.

## Authority and recovery

The server owns permissions, visibility, privacy suppression, commercial facts, eligibility and lifecycle validation. The app does not create customer identities or edit consent/privacy. Request fields come from the mounted contract. The tier replacement contract uses a tiers array rather than a single flattened tier.

Mutations persist a scoped pending command before sending. Declared idempotency keys are reused on retry. Relationship archive/restore has no idempotency header: recovery reads the actual customer version/status instead. Unknown outcomes prevent unrelated writes until resolved. Read failures do not become empty successful datasets.

## Mock boundaries and remaining work

Mock provides local customer and corporate fixtures, atomic membership updates, task/revision history and loyalty ledger operations. Consent/commercial integration remains explicitly unavailable. Its rule registry supports only demonstrated profile fields; materialization completes immediately locally. Mock is not a production worker, expiry engine, commercial-data integration or certification of every scope/privacy rule.

Campaign preparation/execution is implemented in its own workspace; see crm-campaigns.md. CRM exports and business-wide intelligence overview/distributions/rebuild are implemented; see crm-exports.md and crm-insights.md. Remaining reference discovery and deployed permission/device/theme/recovery verification are tracked in remaining-work.md. The CRM API foundation contains 121 routes; this core workspace does not claim complete UI coverage of that count.

Source contracts: services/vendor-service/internal/features/crm and services/vendor-service/docs/openapi CRM specifications. Local validation: test/crm_workspace_test.dart (two focused scenarios).
