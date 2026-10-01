package main

import "cosmicforge/logistics/shared/go/observability"

// customerGaugeQueries are the durable Customer V4 §27.2 gauges. Labels are
// closed vocabularies (authority, status); no customer or source identifiers.
var customerGaugeQueries = []observability.GaugeQuery{
	{Name: "karry_movement_projection_age_seconds", Labels: []string{"authority"},
		SQL: `SELECT authority, COALESCE(EXTRACT(EPOCH FROM now()-max(last_event_at)),0)::float8 FROM customer_movement_history_projection GROUP BY authority`},
	{Name: "karry_movement_projection_rows", Labels: []string{"authority"},
		SQL: `SELECT authority, count(*)::float8 FROM customer_movement_history_projection GROUP BY authority`},
	{Name: "karry_movement_rejections_24h",
		SQL: `SELECT count(*)::float8 FROM customer_movement_event_rejections WHERE received_at > now()-interval '24 hours'`},
	{Name: "karry_journeyshare_sessions", Labels: []string{"status"},
		SQL: `SELECT status, count(*)::float8 FROM journey_share_sessions GROUP BY status`},
	// Sessions the expiry worker should already have closed. A non-zero value
	// that persists means the sweep is stalled; the public read still refuses
	// an expired token, so this is an operations signal, not a leak.
	{Name: "karry_journeyshare_overdue_sessions",
		SQL: `SELECT count(*)::float8 FROM journey_share_sessions WHERE status IN ('ACTIVE','COMPLETED') AND expires_at < now()-interval '5 minutes'`},
	{Name: "karry_journeyshare_notification_backlog",
		SQL: `SELECT count(*)::float8 FROM journey_share_notification_outbox WHERE delivered_at IS NULL AND dead_at IS NULL`},
	{Name: "karry_journeyshare_notification_dead_letters",
		SQL: `SELECT count(*)::float8 FROM journey_share_notification_outbox WHERE dead_at IS NOT NULL`},
}
