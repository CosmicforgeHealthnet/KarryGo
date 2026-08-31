package observability

import (
	"context"
	"time"

	"cosmicforge/logistics/shared/go/logging"
	platform "cosmicforge/logistics/shared/go/observability"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Snapshot queries run periodically, not per event. They use only Dispatch's
// own durable authorities. Gauges are rebuilt to avoid stale nonzero buckets.
func Collect(ctx context.Context, db *pgxpool.Pool) error {
	for _, query := range durableQueries {
		rows, err := db.Query(ctx, query.sql)
		if err != nil {
			return err
		}
		var values, ages []platform.GaugeSample
		for rows.Next() {
			var a, b string
			var count, age float64
			if err = rows.Scan(&a, &b, &count, &age); err != nil {
				rows.Close()
				return err
			}
			labels := map[string]string{query.labelA: Bounded(a, query.allowedA...), query.labelB: Bounded(b, query.allowedB...)}
			if query.name == "dispatch_queue_depth" || query.name == "dispatch_assignment_offer_total" || query.name == "dispatch_route_alert_total" {
				labels["service_area"] = "unpartitioned"
			}
			values = append(values, platform.GaugeSample{Labels: labels, Value: count})
			if query.age != "" {
				ages = append(ages, platform.GaugeSample{Labels: labels, Value: age})
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		Metrics.ReplaceGauges(query.name, values)
		if query.age != "" {
			Metrics.ReplaceGauges(query.age, ages)
		}
	}
	return nil
}

type durableQuery struct {
	name, age, labelA, labelB, sql string
	allowedA, allowedB             []string
}

var durableQueries = []durableQuery{
	{name: "dispatch_reconciliation_exhausted", labelA: "worker", labelB: "outcome", allowedA: []string{"wallet_settlement", "wallet_hold", "notification", "replacement"}, allowedB: []string{"manual_review"},
		sql: `SELECT 'wallet_settlement','manual_review',count(*)::float8,0::float8 FROM trips WHERE wallet_settlement_state='DEAD_LETTER' UNION ALL SELECT 'wallet_hold','manual_review',count(*)::float8,0::float8 FROM dispatch_bookings WHERE hold_reconciliation_state='DEAD_LETTER' UNION ALL SELECT 'notification','manual_review',count(*)::float8,0::float8 FROM dispatch_handover_challenges WHERE notification_state='DEAD_LETTER' UNION ALL SELECT 'replacement','manual_review',count(*)::float8,0::float8 FROM dispatch_delivery_incidents WHERE recovery_result='MANUAL_REVIEW'`},
	{name: "dispatch_reconciliation_attempts_total", labelA: "worker", labelB: "outcome", allowedA: []string{"wallet_settlement", "wallet_hold", "notification", "replacement"}, allowedB: []string{"attempts"},
		sql: `SELECT 'wallet_settlement','attempts',COALESCE(sum(wallet_settlement_attempts),0)::float8,0::float8 FROM trips UNION ALL SELECT 'wallet_hold','attempts',COALESCE(sum(hold_reconciliation_attempts),0)::float8,0::float8 FROM dispatch_bookings UNION ALL SELECT 'notification','attempts',COALESCE(sum(notification_attempt_count),0)::float8,0::float8 FROM dispatch_handover_challenges UNION ALL SELECT 'replacement','attempts',COALESCE(sum(recovery_attempts),0)::float8,0::float8 FROM dispatch_delivery_incidents`},
	{name: "dispatch_booking_notification_backlog", age: "dispatch_booking_notification_oldest_seconds", labelA: "target", labelB: "outcome", allowedA: []string{"notification-service"}, allowedB: []string{"pending"},
		sql: `SELECT 'notification-service','pending',count(*)::float8,COALESCE(max(extract(epoch FROM now()-created_at)),0)::float8 FROM dispatch_marketplace_outbox WHERE aggregate_type='booking_notification' AND status IN ('PENDING','FAILED','IN_FLIGHT')`},
	{name: "dispatch_offer_latency_seconds_bucket", labelA: "outcome", labelB: "le", allowedA: []string{"ACCEPTED", "DECLINED", "EXPIRED"}, allowedB: []string{"10", "30", "60", "90", "300", "+Inf"},
		sql: `SELECT statuses.status,b.label,count(o.id) FILTER(WHERE extract(epoch FROM o.responded_at-o.offered_at)<=b.upper)::float8,0::float8 FROM (VALUES('ACCEPTED'),('DECLINED'),('EXPIRED')) statuses(status) CROSS JOIN (VALUES('10',10::float8),('30',30::float8),('60',60::float8),('90',90::float8),('300',300::float8),('+Inf','Infinity'::float8)) b(label,upper) LEFT JOIN (SELECT id,status,offered_at,responded_at FROM dispatch_assignment_offers UNION ALL SELECT id,CASE status WHEN 'pending' THEN 'OFFERED' WHEN 'rejected' THEN 'DECLINED' ELSE upper(status) END,received_at,responded_at FROM provider_request_inbox) o ON o.status=statuses.status AND o.responded_at IS NOT NULL GROUP BY 1,2`},
	{name: "dispatch_assignment_offer_total", age: "dispatch_offer_latency_seconds_mean", labelA: "strategy", labelB: "outcome",
		allowedA: []string{"BROADCAST_FIRST_ACCEPT", "SEQUENTIAL_NEAREST", "ORGANIZATION_DISPATCH", "ADMIN_ASSISTED"}, allowedB: []string{"OFFERED", "ACCEPTED", "DECLINED", "EXPIRED", "CANCELLED"},
		sql: `SELECT strategy,status,count(*)::float8,COALESCE(avg(latency),0)::float8 FROM (SELECT COALESCE(p.strategy,'SEQUENTIAL_NEAREST') strategy,o.status,extract(epoch FROM o.responded_at-o.offered_at) latency FROM dispatch_assignment_offers o LEFT JOIN dispatch_assignment_policies p ON p.mission_id=o.mission_id UNION ALL SELECT 'BROADCAST_FIRST_ACCEPT',CASE status WHEN 'pending' THEN 'OFFERED' WHEN 'rejected' THEN 'DECLINED' ELSE upper(status) END,extract(epoch FROM responded_at-received_at) FROM provider_request_inbox) offers GROUP BY 1,2`},
	{name: "dispatch_queue_depth", age: "dispatch_queue_wait_seconds", labelA: "outcome", labelB: "service_tier", allowedA: []string{"QUEUED", "RETRYING", "WAITING_CUSTOMER", "SCHEDULED"}, allowedB: []string{"standard", "express"},
		sql: `SELECT s.status,t.tier,count(q.id)::float8,COALESCE(max(extract(epoch FROM now()-q.created_at)),0)::float8 FROM (VALUES('QUEUED'),('RETRYING'),('WAITING_CUSTOMER'),('SCHEDULED')) s(status) CROSS JOIN (VALUES('standard'),('express')) t(tier) LEFT JOIN dispatch_queue_entries q ON q.status=s.status AND q.service_tier=t.tier GROUP BY 1,2`},
	{name: "dispatch_delivery_attempt_total", labelA: "outcome", labelB: "recovery_action", allowedA: []string{"GRACE_PERIOD", "RECEIVER_UNAVAILABLE", "RESOLVED", "RECOVERY_SELECTED", "CANCELLED"}, allowedB: []string{"RETRY_SAME_DAY", "RESCHEDULE", "ALTERNATIVE_RECEIVER", "RETURN_TO_SENDER", "HUB_HOLD", "OPERATIONS_REVIEW", "none"},
		sql: `SELECT a.state,COALESCE((SELECT r.action FROM dispatch_delivery_recovery_actions r WHERE r.delivery_attempt_id=a.id ORDER BY r.created_at DESC LIMIT 1),'none'),count(*)::float8,0::float8 FROM dispatch_delivery_attempts a GROUP BY 1,2`},
	{name: "dispatch_route_alert_total", labelA: "type", labelB: "severity", allowedA: []string{"ROUTE_DEVIATION", "EXCESSIVE_STATIONARY", "WRONG_WAY_MOVEMENT", "STALE_LOCATION", "SUSPICIOUS_LOCATION", "POSSIBLE_ROUTE_TAMPERING"}, allowedB: []string{"LOW", "MEDIUM", "HIGH"},
		sql: `SELECT alert_type,severity,count(*)::float8,0::float8 FROM dispatch_route_alerts GROUP BY 1,2`},
	{name: "dispatch_replacement_total", labelA: "reason", labelB: "outcome", allowedA: []string{"BREAKDOWN", "ACCIDENT", "VEHICLE_FAILURE", "ILLNESS", "SAFETY_CONCERN", "OPERATIONS_DIRECTED_REPLACEMENT", "OTHER_REVIEW_REQUIRED"}, allowedB: []string{"PENDING", "UNKNOWN", "READY", "MANUAL_REVIEW", "CANCELLED"},
		sql: `SELECT reason_code,recovery_result,count(*)::float8,0::float8 FROM dispatch_delivery_incidents GROUP BY 1,2`},
	{name: "dispatch_event_outbox_depth", age: "dispatch_event_outbox_lag_seconds", labelA: "target", labelB: "outcome", allowedA: []string{"vendor-service", "analytics-service", "notification-service", "fleet-partner-service"}, allowedB: []string{"PENDING", "FAILED", "IN_FLIGHT", "DEAD_LETTER", "PUBLISHED"},
		sql: `SELECT targets.target,statuses.status,count(o.id)::float8,COALESCE(max(extract(epoch FROM now()-o.created_at)) FILTER(WHERE o.status IN ('PENDING','FAILED','IN_FLIGHT')),0)::float8 FROM (VALUES('vendor-service'),('analytics-service'),('notification-service'),('fleet-partner-service')) targets(target) CROSS JOIN (VALUES('PENDING'),('FAILED'),('IN_FLIGHT'),('DEAD_LETTER'),('PUBLISHED')) statuses(status) LEFT JOIN dispatch_marketplace_outbox o ON o.target_service=targets.target AND o.status=statuses.status GROUP BY 1,2`},
	{name: "dispatch_event_outbox_retry_total", labelA: "target", labelB: "outcome", allowedA: []string{"vendor-service", "analytics-service", "notification-service", "fleet-partner-service"}, allowedB: []string{"attempts"},
		sql: `SELECT target_service,'attempts',COALESCE(sum(attempt_count),0)::float8,0::float8 FROM dispatch_marketplace_outbox GROUP BY 1`},
	{name: "dispatch_reconciliation_backlog", age: "dispatch_reconciliation_oldest_seconds", labelA: "worker", labelB: "outcome", allowedA: []string{"wallet_settlement", "wallet_hold", "notification", "scheduled_recovery", "replacement"}, allowedB: []string{"pending"},
		sql: `SELECT 'wallet_settlement','pending',count(*)::float8,COALESCE(max(extract(epoch FROM now()-updated_at)),0)::float8 FROM trips WHERE status='completed' AND wallet_settlement_state IN ('PENDING','UNKNOWN') UNION ALL SELECT 'wallet_hold','pending',count(*)::float8,COALESCE(max(extract(epoch FROM now()-updated_at)),0)::float8 FROM dispatch_bookings WHERE payment_status='unpaid' AND hold_reconciliation_state IN ('PENDING','UNKNOWN') UNION ALL SELECT 'notification','pending',count(*)::float8,COALESCE(max(extract(epoch FROM now()-created_at)),0)::float8 FROM dispatch_handover_challenges WHERE notification_state IN ('REQUESTED','FAILED') UNION ALL SELECT 'replacement','pending',count(*)::float8,COALESCE(max(extract(epoch FROM now()-created_at)),0)::float8 FROM dispatch_delivery_incidents WHERE recovery_result IN ('PENDING','UNKNOWN') UNION ALL SELECT 'scheduled_recovery','pending',count(*)::float8,COALESCE(max(extract(epoch FROM now()-created_at)),0)::float8 FROM dispatch_delivery_recovery_actions WHERE state='SCHEDULED'`},
}

func StartCollector(ctx context.Context, db *pgxpool.Pool, interval time.Duration) func() {
	if db == nil {
		return func() {}
	}
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	run, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer WorkerRunning("collector")()
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-ticker.C:
				start := time.Now()
				bounded, stop := context.WithTimeout(run, 5*time.Second)
				err := Collect(bounded, db)
				stop()
				Worker("collector", start, err)
				logging.Structured("info", "dispatch_metrics", map[string]any{"outcome": Outcome(err), "metrics": Metrics.Render()})
			}
		}
	}()
	return func() { cancel(); <-done }
}
