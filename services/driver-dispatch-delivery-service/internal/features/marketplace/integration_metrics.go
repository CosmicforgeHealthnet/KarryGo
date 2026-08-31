package marketplace

import "context"

// Fixed target/status dimensions reuse the existing outbox operational counters.
// No principal, mission or source reference becomes a telemetry label.
type IntegrationMetrics struct {
	VendorBacklog, VendorFailures, AnalyticsBacklog, AnalyticsFailures, BookingNotifications int64
	BookingOldestSeconds                                                                     float64
}

func (r *Repository) IntegrationMetrics(ctx context.Context) (IntegrationMetrics, error) {
	var m IntegrationMetrics
	err := r.db.QueryRow(ctx, `SELECT
 count(*) FILTER(WHERE target_service='vendor-service' AND status IN ('PENDING','FAILED','IN_FLIGHT')),
 count(*) FILTER(WHERE target_service='vendor-service' AND status='DEAD_LETTER'),
 count(*) FILTER(WHERE target_service='analytics-service' AND status IN ('PENDING','FAILED','IN_FLIGHT')),
 count(*) FILTER(WHERE target_service='analytics-service' AND status='DEAD_LETTER'),
 count(*) FILTER(WHERE aggregate_type='booking_notification' AND status IN ('PENDING','FAILED','IN_FLIGHT')),
 COALESCE(max(extract(epoch FROM now()-created_at)) FILTER(WHERE aggregate_type='booking_notification' AND status IN ('PENDING','FAILED','IN_FLIGHT')),0)
 FROM dispatch_marketplace_outbox`).Scan(&m.VendorBacklog, &m.VendorFailures, &m.AnalyticsBacklog, &m.AnalyticsFailures, &m.BookingNotifications, &m.BookingOldestSeconds)
	return m, err
}
