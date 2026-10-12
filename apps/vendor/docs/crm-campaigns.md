# CRM campaigns

Entry: More ? Customers & relationships ? Campaigns. This is separate from checkout promotions.

## Flow

1. Create a draft from an existing active source segment. Choose business/branch scope, channels and exclusions. Configure paired frequency limits, quiet hours/timezone, service-recovery suppression and optional experiment variants. Experiment allocations use integer basis points, total 10,000, with one to five treatments and at most one control capped at 5,000.
2. Preview current audience counts, eligibility, warnings and safe samples. Preview sends nothing. Source/exclusion freshness and consent remain server authority.
3. Prepare the audience. Live preparation is asynchronous; the accepted snapshot may be BUILDING. Refresh the snapshot list/detail to see CURRENT or FAILED. Preparing never grants permission to contact customers.
4. From a PREPARED campaign, create an immediate or future scheduled run using its actual current snapshot and current campaign version. Supply bounded plain text or an existing template key. The public API has no template catalogue: the app does not invent selectable template identifiers.
5. Follow run totals, individual deliveries and history. Pause/resume/cancel are available only in their backend states. Cancellation cannot recall already handed-off messages. Archiving the definition is separate from run controls.

Editing replaces the complete definition and preserves immutable business/branch scope. Changed definitions require fresh preparation before a new run. Previously saved snapshots remain historical evidence.

## Requests and recovery

Uses only the existing public campaign routes in the CRM API. Definition writes, preparation and run creation reuse declared idempotency keys through the scoped durable CRM journal. Preparation is bodyless. Preview is a read-only POST without an idempotency header. Run pause/resume/cancel use expected_version without an invented idempotency header; an accepted lost response can be reconciled from the actual run version/status. Unresolved outcomes block unrelated writes. Server conflicts, feature-flag failures and authorization errors are surfaced, not turned into empty successful data.

Internal callbacks, scheduler workers, consent resolution and notification handoff are not vendor controls. No backend business logic or new endpoint was added.

## Mock

Mock supplies a draft campaign and an actual local static segment, plus historical snapshot/run/delivery evidence. Historical fixture consent allowed a target at preparation and became unavailable at handoff, demonstrating suppression safely.

New Mock previews/preparations use the local CRM memberships and privacy status. Consent is UNKNOWN/unavailable; ineligible snapshot channels create no new delivery rows, matching the backend. No real messages are sent. Experiment cohorts are deterministic using a local version identifier and the backend bucket algorithm; controls and counts are historical evidence, not invented consent.

Local preparation finishes immediately rather than running a worker. Local immediate runs finish immediately; scheduled runs retain controls but have no background scheduler. IANA timezone, frequency-history enforcement, full privacy/service-recovery integration and deployed staff/branch scopes require Live verification. The app does not claim those workers or authorities are implemented by Mock.

## Validation and sources

Focused checks: test/campaign_workspace_test.dart and the existing core CRM flow tests. Deployed feature flags, consent and notification adapters, async preparation, final device/theme/responsiveness and full recovery checks remain in the final integration backlog.

Sources: services/vendor-service/internal/features/crm/campaigns_http.go, campaigns.go, campaign_audience.go, campaign_experiments.go, campaign_execution_http.go and campaign_execution.go; Section 8E and 8F OpenAPI specifications.
