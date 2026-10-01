package outboxmetrics

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/observability"
)

// Source is one outbox a service wants reported.
type Source struct {
	Config Config
	// Unresolved names the reconciliation states that count as a backlog.
	// A state that is not listed is still counted and labelled, but it does
	// not set the oldest-case age: a delivered row from last month is not a
	// backlog, and letting it set the age would leave the metric permanently
	// red and permanently ignored.
	Unresolved []string
}

// Database is the read surface a reporter needs. A *pgxpool.Pool satisfies it.
type Database interface {
	Querier
	QuerierRows
}

// Reporter collects outbox health on a timer and renders it.
//
// It owns its own registry rather than taking the service's, so a service
// with no other instrumentation gets a working /metrics from this alone, and
// a service that already has one can pass it in and have both render
// together.
type Reporter struct {
	Metrics  *observability.Metrics
	DB       Database
	Sources  []Source
	Interval time.Duration
	// AuthToken gates the HTTP handler. An empty token refuses every request
	// rather than serving openly: backlog and dead-letter counts describe
	// where a system is weak, and the safe default for that is closed.
	AuthToken string
}

// NewReporter builds a reporter with a fresh registry.
func NewReporter(db Database, interval time.Duration, sources ...Source) *Reporter {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Reporter{
		Metrics: observability.NewMetrics(), DB: db,
		Sources: sources, Interval: interval,
	}
}

// WithAuthToken sets the bearer token the HTTP handler requires.
func (r *Reporter) WithAuthToken(token string) *Reporter {
	if r != nil {
		r.AuthToken = token
	}
	return r
}

// CollectOnce reads every source and publishes the readings.
//
// A source that fails is logged and skipped rather than aborting the pass:
// one unreadable table must not blank the metrics for the others, because
// the moment a database is struggling is exactly when the remaining readings
// matter most.
func (r *Reporter) CollectOnce(ctx context.Context) {
	if r == nil || r.DB == nil {
		return
	}
	for _, source := range r.Sources {
		snapshot, err := Collect(ctx, r.DB, source.Config)
		if err != nil {
			logging.Structured("warn", "outbox_metrics_collect_failed", map[string]any{
				"service": source.Config.Service,
				"table":   source.Config.Table,
				"outcome": "failed",
			})
			continue
		}
		if source.Config.ReconciliationColumn != "" {
			counts, oldest, stateErr := CollectStates(ctx, r.DB, source.Config, source.Unresolved)
			if stateErr == nil {
				snapshot.ReconciliationByState = counts
				snapshot.ReconciliationOldestAge = oldest
			}
		}
		// A states-only source publishes its backlog and nothing else: a
		// pending count of zero on a table that has no queue would read as
		// "drained" rather than "not measured".
		if source.Config.StatesOnly() {
			PublishStates(r.Metrics, source.Config, snapshot)
			continue
		}
		Publish(r.Metrics, source.Config, snapshot)
	}
}

// Run is a serviceapp worker. It collects immediately so a freshly started
// service does not report nothing for a whole interval, then on the timer.
func (r *Reporter) Run(ctx context.Context) error {
	if r == nil || r.DB == nil || len(r.Sources) == 0 {
		return nil
	}
	r.CollectOnce(ctx)
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.CollectOnce(ctx)
		}
	}
}

// Render returns the Prometheus-style text.
func (r *Reporter) Render() string {
	if r == nil || r.Metrics == nil {
		return ""
	}
	return r.Metrics.Render()
}

// RegisterRoutes mounts GET /metrics on the given group.
//
// The route is mounted even without a token, and refuses: a service that
// silently has no metrics route is indistinguishable from one whose scrape
// configuration is wrong, and an operator chasing that spends the outage
// looking in the wrong place.
func (r *Reporter) RegisterRoutes(group *gin.RouterGroup) {
	if r == nil || group == nil {
		return
	}
	group.GET("/metrics", r.Handler())
}

// Handler serves the rendered metrics behind the bearer token.
func (r *Reporter) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if r == nil || r.AuthToken == "" {
			httpx.RespondError(c, apperrors.Unavailable(
				"Metrics are not configured.", nil))
			c.Abort()
			return
		}
		// Constant time: a scrape token is a credential, and a length- or
		// prefix-dependent comparison leaks it a byte at a time.
		if subtle.ConstantTimeCompare(
			[]byte(c.GetHeader("Authorization")), []byte("Bearer "+r.AuthToken)) != 1 {
			httpx.RespondError(c, apperrors.Unauthorized(
				"Metrics authentication is required.", nil))
			c.Abort()
			return
		}
		c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(r.Render()))
	}
}
