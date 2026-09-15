# ART-004 — Observability and alerting

Phase baseline `3007d6d42b60bf256578cf2688fe9e330a4a2798`.

**Read this first.** Section 22 splits cleanly in two. The instrumentation
half (OBS-001 … OBS-016) is source-resident and is certified here. The launch
operations half (OPS-001 … OPS-014) requires a deployed staging environment
that this certification run does not have. Those gates are marked
`NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED` and **Section 22 therefore stays
OPEN**. Instrumentation existing in source is not the same as a metric
appearing on a dashboard, and this document does not pretend otherwise.

## 1. OBS-001 … OBS-016

| ID | Outcome | Evidence |
|---|---|---|
| OBS-001 structured log standard | ALREADY SATISFIED | `logging.Structured` emits JSON with level, event, timestamp and safe fields; `httpx` adds requestId, correlationId, service, route, status, duration and safe error class. Values that are not plain scalars are redacted rather than serialized, so an opaque struct cannot leak a request body. |
| OBS-002 correlation propagation | ALREADY SATISFIED | `correlation_id` travels on the completed-work request, the settlement instruction, `fleet_partner_finance_operations`, every outbox envelope and the Commerce handover confirmation. The platform deliberately distinguishes request id (per hop) from correlation id (per flow), which the section explicitly permits. |
| OBS-003 membership metrics | NEEDS HARDENING | Fleet records corporate-context request outcomes through its own metrics registry and `FailureKind` gives the denial reason, but there is no dedicated latency histogram for the corporate-context read specifically. Denied-by-status is derivable from the classified error; timeout rate is not separately labelled. |
| OBS-004 offer / assignment metrics | ALREADY SATISFIED | Dispatch `dispatch_assignment_offer_total`; Fleet's marketplace command operations carry outcome and version-conflict classification. |
| OBS-005 execution authorization metrics | ALREADY SATISFIED | Every refusal is a typed `FailureKind` (`MEMBERSHIP_NOT_ACTIVE`, `PROVIDER_ASSIGNMENT_MISMATCH`, `ORGANIZATION_MISMATCH`, `SERVICE_NOT_ELIGIBLE`, `OPERATING_MODE_NOT_PERMITTED`, `FLEET_DEPENDENCY_UNAVAILABLE`) and is logged with a safe error code, so the breakdown by reason is available. |
| OBS-006 outbox metrics | **DONE this phase — certified `PASS — LIVE LOCAL`** | `shared/go/outboxmetrics` reports pending, oldest pending age, retrying, dead-letter and pending attempts. Dispatch and Fleet already had equivalents; **Taxi, Hauling and Wallet had none** and now do, under the same metric names so one panel covers the platform. Certified live against a booted Hauling: the service starts with the reporter registered (no route collision), an unauthenticated scrape is 401, a wrong token is 401, and an authorized scrape renders real gauges read from the real `hauling_domain_outbox`. See `docs/KARRYAM_SECTION19_24_LIVE_EVIDENCE.txt`. |
| OBS-007 reconciliation metrics | **DONE this phase — certified `PASS — LIVE LOCAL`** | `reconciliation_backlog_operations{service,state}` and `reconciliation_oldest_age_seconds`. A state that empties disappears from the gauge rather than reporting its last value forever. Only states declared unresolved contribute to the oldest-case age — certified live: `DELIVERED` appears in the per-state breakdown but does not set the oldest-case age, so a delivered row from last month cannot leave the metric permanently red. See `docs/KARRYAM_SECTION19_24_LIVE_EVIDENCE.txt`. |
| OBS-008 completion → compensation latency | NEEDS HARDENING | Every stage is individually timestamped (`completed_at`, completed-work `created_at`, resolution version, instruction `created_at`) so the latency is computable, but no metric emits the stage-to-stage duration. |
| OBS-009 settlement latency | NEEDS HARDENING | Same shape: instruction `created_at` and journal `created_at` make it computable; retry and failure rate are visible through `managed_settlements.status` and the new backlog gauge, but end-to-end latency is not emitted. |
| OBS-010 idempotent replay metric | NEEDS HARDENING | Duplicate replay is handled correctly everywhere and is distinguishable in logs, but it is not counted separately from a first delivery, so a retry storm is not directly visible as one. The duplicate-storm load test shows the behaviour is cheap (p95 17ms versus 157ms for real postings) but that is a test, not a running metric. |
| OBS-011 commerce stuck state | ALREADY SATISFIED | Dispatch `dispatch_reconciliation_oldest_seconds`, `dispatch_booking_notification_oldest_seconds`; Vendor's reconciliation workers surface stuck missions; the new backlog gauges cover the settlement tail. |
| OBS-012 credential metrics | ALREADY SATISFIED | `dispatch_handover_verification_total` counts attempts and outcomes. No raw credential value is labelled — attempt outcomes only. |
| OBS-013 payroll metrics | NEEDS HARDENING | Payroll runs, approval and posting states are durable and queryable; posting failures surface through Fleet's finance operations. No payroll-specific metric series is emitted. |
| OBS-014 alerts | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | Alert-worthy series now exist for every item the section lists (see §3). Whether an alert rule exists and fires can only be established in the deployed monitoring system. |
| OBS-015 health endpoints | ALREADY SATISFIED | Every service exposes `/health`, `/ready` and `<api-base>/meta` through `serviceapp.Run`. `/ready` runs the service's declared dependency checks (database, Redis); optional integrations do not fail liveness — Hauling boots and serves health with Redis unreachable, certified live in the previous phase. |
| OBS-016 deployed version | NEEDS HARDENING | `<api-base>/meta` reports name and capabilities. It does **not** report a build commit SHA, so a black-box run cannot record which commit it tested. This is a genuine prerequisite for OPS-002 and is listed as a release blocker. |

## 2. Metric inventory

Labels are low cardinality by construction: service, target, state, route template, method, status. No identifier, organization name or personal data appears in a label.

| Metric | Service(s) | Type | Labels | Alert-worthy | Added |
|---|---|---|---|---|---|
| `outbox_pending_events` | taxi, hauling, wallet | gauge | service, target | on sustained growth | this phase |
| `outbox_oldest_pending_age_seconds` | taxi, hauling, wallet | gauge | service, target | **yes — primary stalled-worker signal** | this phase |
| `outbox_retrying_events` | taxi, hauling, wallet | gauge | service, target | with attempts | this phase |
| `outbox_dead_letter_events` | taxi, hauling, wallet | gauge | service, target | **yes — any growth** | this phase |
| `outbox_pending_attempts` | taxi, hauling, wallet | gauge | service, target | retry storm | this phase |
| `reconciliation_backlog_operations` | taxi, hauling, wallet | gauge | service, state, target | **yes — RECONCILIATION_REQUIRED** | this phase |
| `reconciliation_oldest_age_seconds` | taxi, hauling, wallet | gauge | service, target | **yes** | this phase |
| `dispatch_event_outbox_lag_seconds` | dispatch | gauge | target | yes | existing |
| `dispatch_event_outbox_retry_total` | dispatch | counter | target, outcome | yes | existing |
| `dispatch_event_delivery_total` | dispatch | counter | target, outcome | yes | existing |
| `dispatch_reconciliation_oldest_seconds` | dispatch | gauge | kind | yes | existing |
| `dispatch_reconciliation_attempts_total` | dispatch | counter | kind, outcome | — | existing |
| `dispatch_handover_verification_total` | dispatch | counter | stage, outcome | anomaly | existing |
| `dispatch_assignment_offer_total` | dispatch | counter | outcome | — | existing |
| `dispatch_dependency_requests_total` | dispatch | counter | dependency, outcome | yes | existing |
| `dispatch_delivery_attempt_total` | dispatch | counter | outcome | — | existing |
| `dispatch_replacement_total` | dispatch | counter | reason | — | existing |
| `dispatch_worker_runs_total` | dispatch | counter | worker, outcome | yes | existing |
| `dispatch_queue_wait_seconds` | dispatch | duration | queue | — | existing |
| `dispatch_booking_notification_oldest_seconds` | dispatch | gauge | — | yes | existing |
| `dispatch_serviceability_observe_total` | dispatch | counter | outcome | — | existing |
| `dispatch_route_alert_total` | dispatch | counter | kind | — | existing |
| `dispatch_rollout_booking_total` | dispatch | counter | cohort | — | existing |
| `fleet_partner_db_pool_acquire_count_total` | fleet | counter | — | — | existing |
| `fleet_partner_db_pool_empty_acquire_count_total` | fleet | counter | — | **yes — saturation** | existing |
| `fleet_partner_db_pool_canceled_acquire_count_total` | fleet | counter | — | yes | existing |
| `fleet_partner_db_pool_acquire_duration_seconds` | fleet | duration | — | yes | existing |
| `fleet_partner_db_pool_new_conns_total` | fleet | counter | — | — | existing |
| `requests_total` | fleet, dispatch | counter | route, method, status | yes | existing |

Exposure: Fleet `GET /metrics` behind `FLEET_PARTNER_METRICS_AUTH_TOKEN`;
Dispatch through its own registry; Taxi `<api>/outbox-metrics` behind
`TAXI_METRICS_AUTH_TOKEN`; Hauling `<api>/metrics` behind
`HAULING_METRICS_AUTH_TOKEN`; Wallet `<api>/metrics` behind
`PAYMENT_WALLET_METRICS_AUTH_TOKEN`. An unset token makes the route refuse
every request rather than serve openly — backlog and dead-letter counts
describe where a system is weak.

## 3. Recommended alert rules

Thresholds are recommendations from measured behaviour, not existing
configuration. None of these has been created or fired in a real monitoring
system (see OPS-007).

| Alert | Expression | Why this threshold |
|---|---|---|
| Outbox stalled | `outbox_oldest_pending_age_seconds > 900` | Workers tick at 5s with bounded backoff to 15 minutes; 15 minutes of unbroken age means retries are failing, not queueing. |
| Dead letters appearing | `increase(outbox_dead_letter_events[15m]) > 0` | A dead letter is always an operator action. There is no benign dead letter. |
| Reconciliation backlog | `reconciliation_backlog_operations{state="RECONCILIATION_REQUIRED"} > 0 for 15m` | An ambiguous settlement outcome is money whose fate is unknown. |
| Reconciliation ageing | `reconciliation_oldest_age_seconds > 3600` | The Wallet reconciliation worker sweeps every 10 minutes with a 30-minute grace; an hour means it is not converging. |
| Settlement posting failures | `increase(managed_settlements_status{status="FAILED_TERMINAL"}[30m]) > 0` | Terminal failure means a provider will not be paid without intervention. |
| Fleet authorization spike | `rate(requests_total{route=~".*corporate-context.*",status="503"}[5m])` above baseline | Distinguishes a Fleet outage (retryable) from a refusal spike (authorization). |
| DB pool saturation | `rate(fleet_partner_db_pool_empty_acquire_count_total[5m]) > 0` sustained | Measured: at concurrency 16 against a pool of 10 every request but one waited. See ART-006 §load. |
| Credential verification anomaly | `rate(dispatch_handover_verification_total{outcome="failed"}[10m])` above baseline | Brute force or a broken client; both need a human. |
| Commerce stuck | `dispatch_reconciliation_oldest_seconds > 1800` | Beyond the longest legitimate mission recovery window. |

## 4. OPS-001 … OPS-014

| ID | Status | Note |
|---|---|---|
| OPS-001 ingress / TLS | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | No deployed staging reachable from this certification environment. |
| OPS-002 deployed version | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | Additionally blocked in source: `/meta` carries no commit SHA (OBS-016). |
| OPS-003 managed backups | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | |
| OPS-004 restore drill | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | A backup never restored is not certification; this must be performed, not inferred. |
| OPS-005 log visibility | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | Structured logs exist; whether on-call can search them across a multi-service flow is an environment fact. |
| OPS-006 metric visibility | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | Instrumentation certified in source and by test; appearance in the staging monitoring system is not. |
| OPS-007 alert delivery | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | No synthetic alert has reached a real destination. |
| OPS-008 real credentials | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | Allowlist *shape* certified live locally; the deployed secret set is not. |
| OPS-009 supporting service reachability | PARTIAL — LIVE LOCAL | Notification, Media, Verification and Support build, migrate and pass their suites here. Reachability from deployed staging is unverified. |
| OPS-010 wallet / payment dependencies | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | No real or sandbox payment provider was contacted; no money moved. |
| OPS-011 black-box deployed test | NOT VERIFIED — ENVIRONMENT ACCESS REQUIRED | The equivalent was run against **locally booted real binaries** (21/21, previous phase) — that is `PASS — LIVE LOCAL`, explicitly not `PASS — DEPLOYED STAGING`. |
| OPS-012 failure injection | PARTIAL — DB-BACKED / LIVE LOCAL | See ART-006 §failure injection. Fleet timeout, Wallet ambiguity, restart-with-pending-outbox and dead-letter repair are all certified; injection against deployed infrastructure is not. |
| OPS-013 bounded load / SLO | PASS — DB-BACKED | Measured; see ART-006 §load. Against the certification database, not deployed staging. |
| OPS-014 runbook | **DONE** | `FINAL_DEPLOYMENT_READINESS_CHECKLIST.md`. |
