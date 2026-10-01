# Customer V4 monitoring definitions

- `alerts.rules.yml`: Prometheus rule groups (29 alerts).
- `dashboard.json`: Grafana dashboard (7 rows, 23 panels, `${datasource}` variable).

Both use only series the §27 code emits. `scripts/customer-v4/monitoring_check.py` derives that catalogue from the Go sources and fails on any unknown metric, un-emitted label, unbounded label or PromQL outside its strict subset. CI additionally runs `promtool check rules`.

**Every threshold is an initial operational default pending SRE tuning, not a production SLO.** No SLO has been agreed for this programme. Each rule carries `threshold_status: initial_default` and says so in its description.

## Scrape targets (private; HMAC, callers `admin-backoffice-service` and `analytics-service` only)

| Service | Path |
|---|---|
| customer-service | `GET /internal/customer/v1/metrics` |
| driver-taxi-service | `GET /api/v1/taxi/internal/metrics` |
| driver-hauling-service | `GET /api/v1/hauling/internal/metrics` |
| vendor-service | `GET /internal/vendor/v1/metrics` |
| media-file-service | `GET /api/v1/media-files/internal/metrics` |
| notification-service | `GET /api/v1/notifications/internal/metrics` |

Dispatch exports its registry through structured logs, not a scrape, so its reconciliation counters appear only through a log pipeline. No route is publicly reachable; the scraper must sign requests with a trusted caller secret.

## Alert catalogue (initial defaults)

| Family | Alert | Default |
|---|---|---|
| Share | ShareJourneyInvalidTokenRateHigh | invalid/inaccessible > 25% of public reads for 15m, at > 3 reads/min |
| Share | ShareJourneyAuthorityReadErrors | authority read errors > 5% for 10m |
| Share | ShareJourneyExpirySweepStalled | any session past expiry still ACTIVE/COMPLETED for 20m |
| Share | ShareJourneyNotificationDeadLetters | any dead letter |
| Share | ShareJourneyRateLimitSaturation | > 50% of public reads rate limited for 15m |
| Taxi location | TaxiLocationStaleOrUnavailableRatioHigh | > 50% STALE/UNAVAILABLE for 15m |
| Taxi location | TaxiLocationIngestLagHigh | mean fix-to-read lag > 30s (the FRESH window) |
| Taxi location | TaxiLocationAuthorizationFailureSpike | > 20% not-owned/missing for 10m |
| Movement | MovementProjectionAgeHigh | newest projected event > 1h old for 30m (quiet periods can trip it) |
| Movement | MovementReconciliationFailing | > 2 failed sweeps in 30m |
| Movement | MovementReconciliationMismatchHigh | > 50 republished per hour |
| Movement | MovementStaleEventSpike | > 20% stale events for 15m |
| Movement | MovementIngestLagHigh | mean occurrence-to-projection lag > 2m |
| Movement | MovementRejectionsHigh | > 20 rejections in 24h |
| Commerce | CommerceCheckoutOrderPaymentErrorRate | dependency/unknown outcomes > 5% for 10m (critical) |
| Commerce | CommercePaymentUnresolved | any leg PENDING > 5m, for 30m (critical) |
| Commerce | CommerceMissionUnresolved | any mission PENDING > 5m, for 15m |
| Commerce | CommerceRefundAgeHigh | oldest open refund > 1h |
| Commerce | CommerceStockConflictSpike | > 25 new conflicts per hour |
| Commerce | CommerceSubstitutionDecisionOverdue | oldest pending decision > 30m |
| Commerce | VendorOutboxBacklogAge | oldest pending event > 5m |
| Media | MediaUploadFailureRatioHigh | > 10% failed of ≥ 20 sessions in 24h |
| Media | MediaValidationFailuresHigh | > 25 validation failures in 24h |
| Media | MediaOrphanBacklog | > 100 orphans for 1h |
| Notification | NotificationQueueAgeHigh | oldest queued/retrying > 5m |
| Notification | NotificationDeliveryFailureRatioHigh | failed + dead-lettered > 10% per channel in 24h |
| Notification | NotificationDeadLettersGrowing | > 5 new dead letters per hour per channel |
| Notification | NotificationWebsocketSessionsCollapsed | > 70% drop within 1h from > 50 sessions |
| Platform | MetricsGaugeCollectionFailing | any gauge-family collection failure for 15m |

Deployed alerts, the Alertmanager routes and loading the dashboard are B23 work: this repository contains no Prometheus, Alertmanager or Grafana deployment.
