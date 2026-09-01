-- No new event store: historical aggregate over the canonical deduplicated ingest.
CREATE VIEW analytics_dispatch_route_daily_metrics AS
SELECT date_trunc('day',occurred_at AT TIME ZONE 'UTC') AS metric_day,
 event_type, payload->>'alert_type' AS alert_type,
 payload->>'severity' AS severity, payload->>'policy_version' AS policy_version,
 count(*) AS event_count
FROM analytics_commerce_events
WHERE source_service='driver-dispatch-delivery-service'
 AND source_aggregate='dispatch_route_metric'
GROUP BY 1,2,3,4,5;
