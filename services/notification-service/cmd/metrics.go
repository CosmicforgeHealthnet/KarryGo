package main

// Customer V4 §27.2 Notification metrics: queue age, delivery outcome by
// channel, dead letters and active websocket sessions. Labels are closed
// vocabularies (channel, status); recipients never become a series.

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	"cosmicforge/logistics/shared/go/observability"
	"cosmicforge/logistics/shared/go/serviceauth"
)

var notificationMetrics = observability.NewMetrics()

var notificationGaugeQueries = []observability.GaugeQuery{
	{Name: "karry_notification_queue_oldest_age_seconds", Labels: []string{"channel"},
		SQL: `SELECT channel, COALESCE(EXTRACT(EPOCH FROM now()-min(created_at)),0)::float8 FROM notification_deliveries WHERE status IN ('queued','retrying') GROUP BY channel`},
	{Name: "karry_notification_deliveries_24h", Labels: []string{"channel", "status"},
		SQL: `SELECT channel, status, count(*)::float8 FROM notification_deliveries WHERE updated_at > now()-interval '24 hours' GROUP BY channel, status`},
	{Name: "karry_notification_dead_letters", Labels: []string{"channel"},
		SQL: `SELECT channel, count(*)::float8 FROM notification_deliveries WHERE status='dead_lettered' GROUP BY channel`},
}

func registerNotificationMetricsRoute(group *gin.RouterGroup, db *pgxpool.Pool, hub *messageclients.WebSocketHub, secrets serviceauth.Secrets) {
	if group == nil || db == nil || len(secrets) == 0 {
		return
	}
	verifier := serviceauth.NewVerifier(secrets, 5*time.Minute)
	group.GET("/internal/metrics", serviceauth.Middleware(verifier), func(c *gin.Context) {
		switch c.GetString("service_name") {
		case "admin-backoffice-service", "analytics-service":
		default:
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		_ = observability.CollectGauges(c.Request.Context(), observability.PoolQuerier(db), notificationMetrics, notificationGaugeQueries)
		notificationMetrics.SetGauge("karry_notification_websocket_sessions", nil, float64(hub.ActiveSessions()))
		c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(notificationMetrics.Render()))
	})
}
