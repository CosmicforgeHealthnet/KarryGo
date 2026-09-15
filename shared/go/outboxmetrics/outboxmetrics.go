// Package outboxmetrics reports the delivery health of a durable outbox in
// the same shape for every service that has one.
//
// Sections 17 and 22 ask each service to answer the same operational
// questions: how many events are waiting, how old is the oldest one, how many
// are retrying, how many have dead-lettered, and how big is the
// reconciliation backlog. Dispatch and Fleet already answer them. Taxi's
// metrics covered bookings and providers only; Hauling and Wallet exposed no
// metrics at all, so a stuck Hauling completed-work row or a Wallet
// settlement waiting on reconciliation was invisible until someone opened
// psql.
//
// The metric NAMES are deliberately identical across services, with the
// service as a label. That is the whole point: one alert rule and one
// dashboard panel covers the platform instead of three near-duplicates that
// drift apart. Labels stay low-cardinality — service, target, state — and
// never carry an id, an organization name or anything personal.
package outboxmetrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/observability"
)

// Metric names. Exported so a service's own tests, and an alert rule, can
// refer to the same constant rather than a copied string literal.
const (
	// MetricPending is how many events are waiting to be delivered.
	MetricPending = "outbox_pending_events"
	// MetricOldestPendingAge is the age of the oldest undelivered event. This
	// is the number that actually detects a stalled worker: a backlog that is
	// large but young is a burst, a backlog that is small but hours old is a
	// worker that stopped.
	MetricOldestPendingAge = "outbox_oldest_pending_age_seconds"
	// MetricRetrying is how many events have failed at least once and are
	// scheduled to try again.
	MetricRetrying = "outbox_retrying_events"
	// MetricDeadLetter is how many events gave up and need an operator.
	MetricDeadLetter = "outbox_dead_letter_events"
	// MetricAttempts is the total delivery attempts recorded against events
	// still in the table. A rising value with a flat delivered count is a
	// retry storm.
	MetricAttempts = "outbox_pending_attempts"

	// MetricReconciliationBacklog is how many operations are in a
	// reconciliation state, labelled by that state.
	MetricReconciliationBacklog = "reconciliation_backlog_operations"
	// MetricReconciliationOldestAge is the age of the oldest unresolved
	// reconciliation case.
	MetricReconciliationOldestAge = "reconciliation_oldest_age_seconds"
)

// Snapshot is one reading of an outbox's delivery health.
type Snapshot struct {
	Pending          int64
	OldestPendingAge time.Duration
	Retrying         int64
	DeadLettered     int64
	PendingAttempts  int64

	// ReconciliationByState counts operations per reconciliation state. It is
	// empty for an outbox that does not track one.
	ReconciliationByState map[string]int64
	// ReconciliationOldestAge is the age of the oldest unresolved case.
	ReconciliationOldestAge time.Duration
}

// Querier is the read surface a collector needs. A *pgxpool.Pool and a
// transaction both satisfy it, so a test can read inside its own transaction
// and roll back rather than leaving fixture rows behind.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Config describes one service's outbox table.
//
// The column names differ between services because each outbox grew with its
// own service; naming them here is what lets one collector read all of them
// without a migration that renames live columns.
type Config struct {
	// Service labels every metric. Use the canonical service name.
	Service string
	// Table is the outbox table.
	Table string
	// Target optionally narrows to one destination, and labels the metrics.
	// Empty means every destination in the table.
	Target string
	// TargetColumn is the destination column. Required when Target is set.
	TargetColumn string

	// CreatedColumn is when the event was recorded; it is what the oldest-age
	// reading is measured from.
	CreatedColumn string
	// DeliveredColumn is NULL while an event is undelivered. Leaving it empty
	// marks a states-only source: a table that tracks reconciliation but has
	// no delivery queue, such as Wallet's managed-settlement records. Such a
	// source reports the backlog breakdown and nothing else, rather than
	// pretending to a pending count it does not have.
	DeliveredColumn string
	// DeadLetterColumn is non-NULL once an event has given up. Some outboxes
	// use a status value instead; leave it empty and set DeadLetterStatus.
	DeadLetterColumn string
	// StatusColumn and DeadLetterStatus express dead-lettering as a status.
	StatusColumn     string
	DeadLetterStatus string
	// AttemptsColumn counts delivery attempts.
	AttemptsColumn string

	// ReconciliationColumn is the per-row reconciliation state, when the
	// outbox tracks one.
	ReconciliationColumn string
}

// Valid reports the first reason the configuration cannot be used.
func (c Config) Valid() error {
	switch {
	case strings.TrimSpace(c.Service) == "":
		return fmt.Errorf("outboxmetrics: service is required")
	case !safeIdentifier(c.Table):
		return fmt.Errorf("outboxmetrics: table %q is not a plain identifier", c.Table)
	case !safeIdentifier(c.CreatedColumn):
		return fmt.Errorf("outboxmetrics: created column %q is not a plain identifier", c.CreatedColumn)
	case !c.StatesOnly() && !safeIdentifier(c.DeliveredColumn):
		return fmt.Errorf("outboxmetrics: delivered column %q is not a plain identifier", c.DeliveredColumn)
	case !c.StatesOnly() && !safeIdentifier(c.AttemptsColumn):
		return fmt.Errorf("outboxmetrics: attempts column %q is not a plain identifier", c.AttemptsColumn)
	case c.StatesOnly() && c.ReconciliationColumn == "":
		return fmt.Errorf("outboxmetrics: a states-only source needs a reconciliation column")
	case c.Target != "" && !safeIdentifier(c.TargetColumn):
		return fmt.Errorf("outboxmetrics: target column %q is not a plain identifier", c.TargetColumn)
	case c.DeadLetterColumn != "" && !safeIdentifier(c.DeadLetterColumn):
		return fmt.Errorf("outboxmetrics: dead-letter column %q is not a plain identifier", c.DeadLetterColumn)
	case c.DeadLetterStatus != "" && !safeIdentifier(c.StatusColumn):
		return fmt.Errorf("outboxmetrics: status column %q is not a plain identifier", c.StatusColumn)
	case c.ReconciliationColumn != "" && !safeIdentifier(c.ReconciliationColumn):
		return fmt.Errorf("outboxmetrics: reconciliation column %q is not a plain identifier", c.ReconciliationColumn)
	}
	return nil
}

// StatesOnly reports whether this source has no delivery queue to measure,
// only a reconciliation breakdown.
func (c Config) StatesOnly() bool { return c.DeliveredColumn == "" }

// safeIdentifier restricts every interpolated name to lowercase letters,
// digits and underscores.
//
// Table and column names cannot be bound as query parameters, so they are
// interpolated. They come from a service's own compiled-in configuration
// rather than a request, but a validator here is what keeps that true after
// someone later makes the configuration dynamic.
func safeIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9':
		case character == '_':
		default:
			return false
		}
	}
	return true
}

// deadLetterPredicate renders the SQL that identifies a dead-lettered row.
func (c Config) deadLetterPredicate() string {
	if c.DeadLetterColumn != "" {
		return c.DeadLetterColumn + " IS NOT NULL"
	}
	if c.DeadLetterStatus != "" {
		return fmt.Sprintf("%s = '%s'", c.StatusColumn, c.DeadLetterStatus)
	}
	return "false"
}

// query builds the single aggregate that answers every outbox question.
//
// One statement rather than five keeps the readings consistent with each
// other: five separate counts taken while a worker is draining can report a
// backlog that never existed at any single moment.
func (c Config) query() string {
	scope := "TRUE"
	if c.Target != "" {
		scope = fmt.Sprintf("%s = $1", c.TargetColumn)
	}
	undelivered := c.DeliveredColumn + " IS NULL"
	deadLetter := c.deadLetterPredicate()
	return fmt.Sprintf(`
		SELECT
			count(*) FILTER (WHERE %[1]s AND NOT (%[2]s)),
			COALESCE(EXTRACT(EPOCH FROM (now() - min(%[3]s) FILTER (WHERE %[1]s AND NOT (%[2]s)))), 0),
			count(*) FILTER (WHERE %[1]s AND NOT (%[2]s) AND %[4]s > 0),
			count(*) FILTER (WHERE %[2]s),
			COALESCE(sum(%[4]s) FILTER (WHERE %[1]s AND NOT (%[2]s)), 0)
		  FROM %[5]s
		 WHERE %[6]s`,
		undelivered, deadLetter, c.CreatedColumn, c.AttemptsColumn, c.Table, scope)
}

// Publish writes a snapshot into the registry under the canonical names.
//
// A nil registry is a no-op so a service that has not wired metrics yet can
// still call the collector without a nil check at every site.
func Publish(metrics *observability.Metrics, config Config, snapshot Snapshot) {
	if metrics == nil {
		return
	}
	labels := map[string]string{"service": config.Service}
	if config.Target != "" {
		labels["target"] = config.Target
	}
	metrics.SetGauge(MetricPending, labels, float64(snapshot.Pending))
	metrics.SetGauge(MetricOldestPendingAge, labels, snapshot.OldestPendingAge.Seconds())
	metrics.SetGauge(MetricRetrying, labels, float64(snapshot.Retrying))
	metrics.SetGauge(MetricDeadLetter, labels, float64(snapshot.DeadLettered))
	metrics.SetGauge(MetricAttempts, labels, float64(snapshot.PendingAttempts))

	if len(snapshot.ReconciliationByState) > 0 {
		samples := make([]observability.GaugeSample, 0, len(snapshot.ReconciliationByState))
		for state, count := range snapshot.ReconciliationByState {
			stateLabels := map[string]string{"service": config.Service, "state": state}
			if config.Target != "" {
				stateLabels["target"] = config.Target
			}
			samples = append(samples, observability.GaugeSample{
				Labels: stateLabels, Value: float64(count),
			})
		}
		// ReplaceGauges rather than SetGauge: a state that has emptied since
		// the last reading must disappear, not keep reporting its old count
		// forever.
		metrics.ReplaceGauges(MetricReconciliationBacklog, samples)
	}
	if snapshot.ReconciliationOldestAge > 0 || len(snapshot.ReconciliationByState) > 0 {
		metrics.SetGauge(MetricReconciliationOldestAge, labels,
			snapshot.ReconciliationOldestAge.Seconds())
	}
}

// Collect reads one outbox's delivery health.
//
// It runs a single aggregate. A reading assembled from several statements
// while a worker is draining can report a backlog that never existed at any
// one moment, which is worse than no reading: an operator chasing a phantom
// backlog stops trusting the dashboard.
func Collect(ctx context.Context, db Querier, config Config) (Snapshot, error) {
	if err := config.Valid(); err != nil {
		return Snapshot{}, err
	}
	if config.StatesOnly() {
		return Snapshot{}, nil
	}
	if db == nil {
		return Snapshot{}, fmt.Errorf("outboxmetrics: no database")
	}

	var snapshot Snapshot
	var oldestSeconds float64
	args := []any{}
	if config.Target != "" {
		args = append(args, config.Target)
	}
	if err := db.QueryRow(ctx, config.query(), args...).Scan(
		&snapshot.Pending, &oldestSeconds, &snapshot.Retrying,
		&snapshot.DeadLettered, &snapshot.PendingAttempts,
	); err != nil {
		return Snapshot{}, fmt.Errorf("outboxmetrics: read %s: %w", config.Table, err)
	}
	snapshot.OldestPendingAge = time.Duration(oldestSeconds * float64(time.Second))
	return snapshot, nil
}

// CollectStates reads a reconciliation-state breakdown and the age of the
// oldest unresolved case.
//
// It is separate from Collect because not every outbox tracks reconciliation,
// and a service that does not should not pay for a second query or report a
// backlog of zero that an alert could never distinguish from "not measured".
func CollectStates(
	ctx context.Context, db QuerierRows, config Config, unresolved []string,
) (map[string]int64, time.Duration, error) {
	if err := config.Valid(); err != nil {
		return nil, 0, err
	}
	if config.ReconciliationColumn == "" {
		return nil, 0, nil
	}
	if db == nil {
		return nil, 0, fmt.Errorf("outboxmetrics: no database")
	}

	scope := "TRUE"
	args := []any{}
	if config.Target != "" {
		scope = fmt.Sprintf("%s = $1", config.TargetColumn)
		args = append(args, config.Target)
	}
	rows, err := db.Query(ctx, fmt.Sprintf(`
		SELECT %[1]s, count(*),
		       COALESCE(EXTRACT(EPOCH FROM (now() - min(%[2]s))), 0)
		  FROM %[3]s
		 WHERE %[4]s
		 GROUP BY %[1]s`,
		config.ReconciliationColumn, config.CreatedColumn, config.Table, scope), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("outboxmetrics: read %s states: %w", config.Table, err)
	}
	defer rows.Close()

	open := map[string]bool{}
	for _, state := range unresolved {
		open[state] = true
	}
	counts := map[string]int64{}
	var oldest time.Duration
	for rows.Next() {
		var state string
		var count int64
		var ageSeconds float64
		if err = rows.Scan(&state, &count, &ageSeconds); err != nil {
			return nil, 0, err
		}
		counts[state] = count
		// Only an UNRESOLVED state contributes to the oldest-case age. A
		// delivered row from last month is not a backlog, and letting it set
		// the age would make the metric permanently red.
		if age := time.Duration(ageSeconds * float64(time.Second)); open[state] && age > oldest {
			oldest = age
		}
	}
	return counts, oldest, rows.Err()
}

// QuerierRows is the multi-row read surface CollectStates needs.
type QuerierRows interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// PublishStates writes only the reconciliation breakdown, for a source that
// has no delivery queue to report.
func PublishStates(metrics *observability.Metrics, config Config, snapshot Snapshot) {
	if metrics == nil || len(snapshot.ReconciliationByState) == 0 {
		return
	}
	samples := make([]observability.GaugeSample, 0, len(snapshot.ReconciliationByState))
	for state, count := range snapshot.ReconciliationByState {
		labels := map[string]string{"service": config.Service, "state": state}
		if config.Target != "" {
			labels["target"] = config.Target
		}
		samples = append(samples, observability.GaugeSample{Labels: labels, Value: float64(count)})
	}
	metrics.ReplaceGauges(MetricReconciliationBacklog, samples)

	labels := map[string]string{"service": config.Service}
	if config.Target != "" {
		labels["target"] = config.Target
	}
	metrics.SetGauge(MetricReconciliationOldestAge, labels,
		snapshot.ReconciliationOldestAge.Seconds())
}
