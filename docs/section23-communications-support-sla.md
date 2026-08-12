# Section 23 — Communications, support, quality and SLA

Section 23 gives corporate organizations an auditable communication and service-quality layer while preserving external authority boundaries. Notification Service remains the delivery authority. support-dispute-service remains the canonical platform support lifecycle. Fleet stores safe projections and never becomes a second call-centre or messaging engine.

## Communications and notifications

Threads have explicit participants, visibility and branch scope. Restricted threads are readable only by authorized participants. Messages are append-only and may reference only safe, organization-owned Media objects. `INTERNAL_NOTE` messages never create external notification intents. Other external-facing messages create durable, per-recipient intents; workers submit to Notification Service and read back by the same idempotency key after unknown outcomes.

Notification rules validate IANA timezones, `HH:MM` quiet-hour windows, recipient scope, channels and severity. Rules influence intent generation but do not deliver email, SMS, WhatsApp, push or realtime messages inside Fleet.

## Support and quality separation

Creating a support case calls the service-authenticated organization-case contract in support-dispute-service. The canonical case is created there; Fleet persists only organization-scoped status, priority, owner and source-version projections. Inbound source events are deduplicated by event ID. Version gaps mark a projection for reconciliation, where the worker reads the canonical case.

Internal quality cases are a distinct Fleet model. They link to operational sources but do not masquerade as platform support cases. Resolution requires both root cause and corrective-action summary, and versioned history records each transition.

## SLA and escalations

SLA measurements are immutable snapshots keyed by agreement, metric, period and formula version. Decimal comparison uses exact rational arithmetic rather than floating point. Incomplete evidence produces `UNKNOWN`. Complete evidence deterministically produces `MET` or `BREACHED` according to direction. A breach creates a deduplicated escalation; notification scheduling is durable. Acknowledge and resolve commands use optimistic concurrency and append-only history.

## Routes and operations

The canonical route list is in [section23-communications-support-sla.openapi.json](openapi/section23-communications-support-sla.openapi.json). Corporate routes live under `/fleet-partner/v1`; support events and SLA measurement ingestion are HMAC-authenticated internal routes. Commands require `Idempotency-Key`, and transitions require `expectedVersion`/`If-Match`.

Permissions are grouped as `communications.*`, `notification_rules.*`, `support.*`, `quality.*`, `sla.*`, and `escalations.*`. Important thread, message, rule, projection, quality, measurement and escalation transitions publish versioned, safe-reference domain events without evidence contents or delivery secrets.

## Flags, rollout and rollback

All flags default false: `COMMUNICATIONS_ENABLED`, `NOTIFICATION_RULES_ENABLED`, `SUPPORT_PROJECTIONS_ENABLED`, `QUALITY_CASES_ENABLED`, `SLA_MEASUREMENTS_ENABLED`, `ESCALATIONS_ENABLED`, and `SECTION23_WORKERS_ENABLED`. The global integration kill switch stops writes and schedulers. Enable the schemas and read surfaces first, then connect Notification and Support HMAC credentials, then enable writes and workers. Rollback disables flags/workers before paired migration downs. Append-only messages, SLA snapshots, support inbox records and escalation history are intentionally immutable while the feature is installed.
