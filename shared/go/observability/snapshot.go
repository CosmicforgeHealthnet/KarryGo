package observability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Rows is the minimal row iterator CollectGauges needs (pgx.Rows satisfies it).
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

// Querier is satisfied by *pgxpool.Pool and pgx.Tx through a thin adapter,
// keeping this package free of a database dependency.
type Querier interface {
	QueryRows(ctx context.Context, sql string, args ...any) (Rows, error)
}

// GaugeQuery is one durable, database-derived gauge family collected at
// scrape time (Customer V4 §27.2). The SQL returns one text column per label
// key, in order, followed by one numeric value. Labels must be bounded
// categories (authority, channel, status), never identifiers.
type GaugeQuery struct {
	Name   string
	Labels []string
	SQL    string
}

// Validate refuses a family whose label keys are identifier-shaped.
func (q GaugeQuery) Validate() error {
	if strings.TrimSpace(q.Name) == "" || strings.TrimSpace(q.SQL) == "" {
		return fmt.Errorf("observability: gauge query requires a name and SQL")
	}
	for _, label := range q.Labels {
		if !LabelKeyIsLowCardinality(label) {
			return fmt.Errorf("observability: gauge %s label %q is not low-cardinality", q.Name, label)
		}
	}
	return nil
}

// CollectGauges runs each family and replaces it atomically, so a drained
// backlog disappears instead of reporting its last value forever. A failing
// family is skipped and reported; the others still publish.
func CollectGauges(ctx context.Context, db Querier, m *Metrics, queries []GaugeQuery) error {
	if db == nil || m == nil {
		return nil
	}
	var failures []string
	for _, query := range queries {
		if err := query.Validate(); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		rows, err := db.QueryRows(ctx, query.SQL)
		if err != nil {
			failures = append(failures, query.Name)
			m.IncCounter("karry_metrics_collection_errors_total", map[string]string{"family": query.Name}, 1)
			continue
		}
		var samples []GaugeSample
		for rows.Next() {
			values := make([]any, len(query.Labels)+1)
			labels := make([]string, len(query.Labels))
			for i := range labels {
				values[i] = &labels[i]
			}
			var value float64
			values[len(labels)] = &value
			if err = rows.Scan(values...); err != nil {
				break
			}
			sample := GaugeSample{Value: value}
			if len(labels) > 0 {
				sample.Labels = map[string]string{}
				for i, key := range query.Labels {
					sample.Labels[key] = labels[i]
				}
			}
			samples = append(samples, sample)
		}
		rows.Close()
		if err == nil {
			err = rows.Err()
		}
		if err != nil {
			failures = append(failures, query.Name)
			m.IncCounter("karry_metrics_collection_errors_total", map[string]string{"family": query.Name}, 1)
			continue
		}
		m.ReplaceGauges(query.Name, samples)
	}
	if len(failures) > 0 {
		return fmt.Errorf("observability: gauge families failed: %s", strings.Join(failures, ", "))
	}
	return nil
}

// Outcome is the bounded V4 §27.1 outcome vocabulary for logs and metrics.
type Outcome string

const (
	OutcomeSuccess               Outcome = "success"
	OutcomeValidation            Outcome = "validation"
	OutcomeConflict              Outcome = "conflict"
	OutcomeDependencyUnavailable Outcome = "dependency_unavailable"
	OutcomeUnknownOutcome        Outcome = "unknown_outcome"
	OutcomeRateLimited           Outcome = "rate_limited"
	OutcomeAuthorizationDenied   Outcome = "authorization_denied"
	OutcomeNotFound              Outcome = "not_found"
)

// OutcomeForStatus maps an HTTP status to the bounded vocabulary.
func OutcomeForStatus(status int) Outcome {
	switch {
	case status < 400:
		return OutcomeSuccess
	case status == 401 || status == 403:
		return OutcomeAuthorizationDenied
	case status == 404:
		return OutcomeNotFound
	case status == 409 || status == 412:
		return OutcomeConflict
	case status == 429:
		return OutcomeRateLimited
	case status == 400 || status == 422:
		return OutcomeValidation
	case status == 502 || status == 503 || status == 504:
		return OutcomeDependencyUnavailable
	}
	return OutcomeDependencyUnavailable
}

// PoolQuerier adapts a pgx pool.
func PoolQuerier(pool *pgxpool.Pool) Querier {
	if pool == nil {
		return nil
	}
	return poolQuerier{pool}
}

type poolQuerier struct{ pool *pgxpool.Pool }

func (q poolQuerier) QueryRows(ctx context.Context, sql string, args ...any) (Rows, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	rows, err := q.pool.Query(ctx, sql, args...)
	if err != nil {
		cancel()
		return nil, err
	}
	return cancelRows{Rows: rows, cancel: cancel}, nil
}

type cancelRows struct {
	pgx.Rows
	cancel context.CancelFunc
}

func (r cancelRows) Close() { r.Rows.Close(); r.cancel() }
