package main

// Customer V4 §27.2 Media metrics: durable upload-lifecycle gauges collected
// at scrape time from Media's own authority tables. Labels are closed
// vocabularies (purpose, state); object, owner and source identifiers never
// become a series.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/observability"
	"cosmicforge/logistics/shared/go/serviceauth"
)

var mediaMetrics = observability.NewMetrics()

var mediaGaugeQueries = []observability.GaugeQuery{
	{Name: "karry_media_upload_sessions_24h", Labels: []string{"state"},
		SQL: `SELECT state, count(*)::float8 FROM media_upload_authorizations WHERE created_at > now()-interval '24 hours' GROUP BY state`},
	{Name: "karry_media_upload_failures_24h", Labels: []string{"purpose"},
		SQL: `SELECT upload_purpose, count(*)::float8 FROM media_objects WHERE upload_status='FAILED' AND updated_at > now()-interval '24 hours' GROUP BY upload_purpose`},
	{Name: "karry_media_validation_failures_24h", Labels: []string{"purpose"},
		SQL: `SELECT upload_purpose, count(*)::float8 FROM media_objects WHERE (signature_validation='FAILED' OR validation_failure_code <> '') AND updated_at > now()-interval '24 hours' GROUP BY upload_purpose`},
	{Name: "karry_media_orphans", Labels: []string{"purpose"},
		SQL: `SELECT upload_purpose, count(*)::float8 FROM media_objects WHERE upload_status IN ('EXPIRED','PENDING','AUTHORIZED','UPLOADING') AND created_at < now()-interval '1 hour' GROUP BY upload_purpose`},
}

// registerMediaMetricsRoute mounts the private scrape: HMAC-verified and
// limited to the platform's supervising callers.
func registerMediaMetricsRoute(group *gin.RouterGroup, db *pgxpool.Pool, verifier *serviceauth.Verifier) {
	if group == nil || db == nil || verifier == nil {
		return
	}
	group.GET("/internal/metrics", serviceauth.Middleware(verifier), func(c *gin.Context) {
		switch c.GetString("service_name") {
		case "admin-backoffice-service", "analytics-service":
		default:
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		_ = observability.CollectGauges(c.Request.Context(), observability.PoolQuerier(db), mediaMetrics, mediaGaugeQueries)
		c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(mediaMetrics.Render()))
	})
}
