package observability

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRows struct {
	data [][]any
	i    int
}

func (r *fakeRows) Next() bool { r.i++; return r.i <= len(r.data) }
func (r *fakeRows) Scan(dest ...any) error {
	row := r.data[r.i-1]
	for i, d := range dest {
		switch p := d.(type) {
		case *string:
			*p = row[i].(string)
		case *float64:
			*p = row[i].(float64)
		}
	}
	return nil
}
func (r *fakeRows) Close()     {}
func (r *fakeRows) Err() error { return nil }

type fakeQuerier map[string][][]any

func (q fakeQuerier) QueryRows(_ context.Context, sql string, _ ...any) (Rows, error) {
	data, ok := q[sql]
	if !ok {
		return nil, errors.New("down")
	}
	return &fakeRows{data: data}, nil
}

// Customer V4 §27.2: a durable gauge family replaces atomically, refuses
// identifier labels, and one failing family never hides the others.
func TestCollectGaugesReplacesBoundedFamilies(t *testing.T) {
	m := NewMetrics()
	db := fakeQuerier{"age": {{"TAXI", 12.0}, {"DISPATCH", 3.0}}, "dlq": {{7.0}}}
	queries := []GaugeQuery{
		{Name: "karry_movement_projection_age_seconds", Labels: []string{"authority"}, SQL: "age"},
		{Name: "karry_notification_dead_letters", SQL: "dlq"},
		{Name: "karry_broken", SQL: "missing"},
		{Name: "karry_unsafe", Labels: []string{"customer_id"}, SQL: "age"},
	}
	err := CollectGauges(context.Background(), db, m, queries)
	if err == nil || !strings.Contains(err.Error(), "karry_broken") || !strings.Contains(err.Error(), "customer_id") {
		t.Fatalf("failures not reported: %v", err)
	}
	out := m.Render()
	for _, want := range []string{`karry_movement_projection_age_seconds{authority="TAXI"} 12`, `karry_notification_dead_letters 7`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "karry_unsafe") {
		t.Fatal("an identifier-labelled family was published")
	}
	// The backlog drains: the family is replaced, not left stale.
	db["age"] = [][]any{{"TAXI", 1.0}}
	_ = CollectGauges(context.Background(), db, m, queries[:1])
	out = m.Render()
	if strings.Contains(out, `authority="DISPATCH"`) || !strings.Contains(out, `authority="TAXI"} 1`) {
		t.Fatalf("family not replaced atomically:\n%s", out)
	}
}

func TestOutcomeForStatusIsTheV4Vocabulary(t *testing.T) {
	for status, want := range map[int]Outcome{200: OutcomeSuccess, 201: OutcomeSuccess, 400: OutcomeValidation, 422: OutcomeValidation,
		401: OutcomeAuthorizationDenied, 403: OutcomeAuthorizationDenied, 404: OutcomeNotFound, 409: OutcomeConflict,
		429: OutcomeRateLimited, 503: OutcomeDependencyUnavailable, 500: OutcomeDependencyUnavailable} {
		if got := OutcomeForStatus(status); got != want {
			t.Errorf("%d -> %s, want %s", status, got, want)
		}
	}
}
