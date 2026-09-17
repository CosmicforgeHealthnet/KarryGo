package movementusecases

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	movementmodels "cosmicforge/logistics/services/customer-service/internal/features/movement_history/models"
	movementrepositories "cosmicforge/logistics/services/customer-service/internal/features/movement_history/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/events"
)

// memoryRepo applies the same pure movementmodels.Apply rules the Postgres
// repository uses, with an inbox for event-id dedupe.
type memoryRepo struct {
	mu        sync.Mutex
	rows      map[string]movementmodels.Movement
	inbox     map[string]string
	rejected  map[string]string
	groupings map[string]events.MovementGrouping
	failWrite bool
}

func (r *memoryRepo) ProjectedVersions(_ context.Context, authority string, sourceRefs []string) (map[string]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	versions := map[string]int64{}
	for _, ref := range sourceRefs {
		if row, ok := r.rows[authority+"|"+ref]; ok {
			versions[ref] = row.SourceVersion
		}
	}
	return versions, nil
}

func (r *memoryRepo) ApplyGrouping(_ context.Context, envelope events.Envelope, grouping events.MovementGrouping) (string, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failWrite {
		return "", 0, context.DeadlineExceeded
	}
	if _, ok := r.inbox[envelope.EventID]; ok {
		return "DUPLICATE", 0, nil
	}
	r.inbox[envelope.EventID] = "APPLIED"
	if r.groupings == nil {
		r.groupings = map[string]events.MovementGrouping{}
	}
	for _, mission := range grouping.Missions {
		r.groupings[mission.FulfilmentGroupRef] = grouping
	}
	return "APPLIED", len(grouping.Missions), nil
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{rows: map[string]movementmodels.Movement{}, inbox: map[string]string{}, rejected: map[string]string{}}
}

func (r *memoryRepo) Project(_ context.Context, envelope events.Envelope, incoming movementmodels.Movement) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failWrite {
		return "", context.DeadlineExceeded
	}
	if _, ok := r.inbox[envelope.EventID]; ok {
		return movementmodels.OutcomeDuplicate, nil
	}
	key := incoming.Authority + "|" + incoming.SourceRef
	var existing *movementmodels.Movement
	if row, ok := r.rows[key]; ok {
		existing = &row
	}
	next, outcome, err := movementmodels.Apply(existing, incoming)
	if err != nil {
		return "", err
	}
	if outcome == movementmodels.OutcomeApplied {
		r.rows[key] = next
	}
	r.inbox[envelope.EventID] = outcome
	return outcome, nil
}

func (r *memoryRepo) Reject(_ context.Context, envelope events.Envelope, reason string) error {
	r.rejected[envelope.EventID] = reason
	return nil
}

func (r *memoryRepo) List(_ context.Context, customerID string, filter movementrepositories.Filter) ([]movementmodels.Movement, int, error) {
	var out []movementmodels.Movement
	for _, row := range r.rows {
		if row.CustomerID != customerID || (filter.ParentContextRef != "" && (row.ParentContextRef == nil || *row.ParentContextRef != filter.ParentContextRef)) {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SourceRef < out[j].SourceRef })
	return out, len(out), nil
}

func (r *memoryRepo) Get(_ context.Context, customerID, authority, sourceRef string) (movementmodels.Movement, error) {
	row, ok := r.rows[authority+"|"+sourceRef]
	if !ok || row.CustomerID != customerID {
		return row, apperrors.NotFound("Movement not found.", nil)
	}
	return row, nil
}

const customerA = "11111111-1111-4111-8111-111111111111"
const customerB = "22222222-2222-4222-8222-222222222222"

var callers = CallerAuthorities{"driver-taxi-service": events.MovementAuthorityTaxi, "driver-dispatch-delivery-service": events.MovementAuthorityDispatch, "driver-hauling-service": events.MovementAuthorityHauling}

func newService(repo Repository) *Service {
	return NewService(Options{Repository: repo, Callers: callers, ConsumerEnabled: true, ReadEnabled: true})
}

func envelope(t *testing.T, id, service, name string, version int64, s events.MovementSummary) events.Envelope {
	t.Helper()
	e, err := events.NewMovementEnvelope(id, name, service, "movement", version, time.Now(), "corr-"+id, "", s)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func taxi(state string, replacements int) events.MovementSummary {
	return events.MovementSummary{CustomerID: customerA, Authority: events.MovementAuthorityTaxi, SourceRef: "booking-1", MovementType: events.MovementTypePassenger, State: state, ProviderReplacementCount: replacements}
}

func TestIngestIsIdempotentAndIgnoresStaleAndTerminalRegression(t *testing.T) {
	repo := newMemoryRepo()
	svc := newService(repo)
	ctx := context.Background()
	steps := []struct {
		id      string
		version int64
		state   string
		want    string
	}{
		{"e1", 1, events.MovementStateSearching, movementmodels.OutcomeApplied},
		{"e1", 1, events.MovementStateSearching, movementmodels.OutcomeDuplicate},
		{"e3", 3, events.MovementStateInProgress, movementmodels.OutcomeApplied},
		{"e2", 2, events.MovementStateAssigned, movementmodels.OutcomeStale},
		{"e4", 4, events.MovementStateCompleted, movementmodels.OutcomeApplied},
		{"e5", 5, events.MovementStateInProgress, movementmodels.OutcomeTerminalProtected},
	}
	for _, step := range steps {
		result, err := svc.Ingest(ctx, "driver-taxi-service", envelope(t, step.id, "driver-taxi-service", events.TaxiBookingStarted, step.version, taxi(step.state, 0)))
		if err != nil || result.Outcome != step.want {
			t.Fatalf("%s v%d: outcome=%q err=%v want %q", step.id, step.version, result.Outcome, err, step.want)
		}
	}
	if got := repo.rows["TAXI|booking-1"]; got.State != events.MovementStateCompleted || got.SourceVersion != 4 {
		t.Fatalf("projection = %+v", got)
	}
}

func TestProviderReplacementReprojectsMonotonically(t *testing.T) {
	repo := newMemoryRepo()
	svc := newService(repo)
	ctx := context.Background()
	for i, s := range []events.MovementSummary{taxi(events.MovementStateInProgress, 0), taxi(events.MovementStateInProgress, 1), taxi(events.MovementStateInProgress, 0)} {
		if i == 1 {
			s.RecoveryState = "RESOLVED"
		}
		if _, err := svc.Ingest(ctx, "driver-taxi-service", envelope(t, "r"+string(rune('0'+i)), "driver-taxi-service", events.TaxiRecoveryProviderReplace, int64(i+1), s)); err != nil {
			t.Fatal(err)
		}
	}
	if got := repo.rows["TAXI|booking-1"].ProviderReplacementCount; got != 1 {
		t.Fatalf("provider_replacement_count = %d, want 1 (never decreases)", got)
	}
}

func TestIngestAuthorization(t *testing.T) {
	repo := newMemoryRepo()
	ctx := context.Background()
	if _, err := NewService(Options{Repository: repo, Callers: callers}).Ingest(ctx, "driver-taxi-service", envelope(t, "d", "driver-taxi-service", "x", 1, taxi(events.MovementStateSearching, 0))); apperrors.From(err).Status != http.StatusServiceUnavailable {
		t.Fatalf("disabled consumer status = %d", apperrors.From(err).Status)
	}
	svc := newService(repo)
	if _, err := svc.Ingest(ctx, "vendor-service", envelope(t, "v", "vendor-service", "x", 1, taxi(events.MovementStateSearching, 0))); apperrors.From(err).Status != http.StatusForbidden {
		t.Fatalf("unlisted caller status = %d", apperrors.From(err).Status)
	}
	if _, err := svc.Ingest(ctx, "driver-taxi-service", envelope(t, "s", "driver-hauling-service", "x", 1, taxi(events.MovementStateSearching, 0))); apperrors.From(err).Status != http.StatusForbidden {
		t.Fatalf("source_service spoof status = %d", apperrors.From(err).Status)
	}
	dispatch := taxi(events.MovementStateSearching, 0)
	dispatch.Authority, dispatch.MovementType = events.MovementAuthorityDispatch, events.MovementTypeDispatch
	if _, err := svc.Ingest(ctx, "driver-taxi-service", envelope(t, "a", "driver-taxi-service", "x", 1, dispatch)); apperrors.From(err).Status != http.StatusForbidden {
		t.Fatalf("taxi publishing DISPATCH status = %d", apperrors.From(err).Status)
	}
	if repo.rejected["a"] != "AUTHORITY_NOT_OWNED_BY_CALLER" {
		t.Fatalf("authority violation must be retained for reconciliation: %v", repo.rejected)
	}
	if _, err := svc.Ingest(ctx, "driver-taxi-service", envelope(t, "ok", "driver-taxi-service", "x", 1, taxi(events.MovementStateSearching, 0))); err != nil {
		t.Fatal(err)
	}
	hijack := taxi(events.MovementStateAssigned, 0)
	hijack.CustomerID = customerB
	if _, err := svc.Ingest(ctx, "driver-taxi-service", envelope(t, "h", "driver-taxi-service", "x", 2, hijack)); apperrors.From(err).Status != http.StatusConflict {
		t.Fatalf("customer rebinding status = %d", apperrors.From(err).Status)
	}
}

func TestOneCommerceOrderExposesMultipleMovementsAcrossAuthorities(t *testing.T) {
	repo := newMemoryRepo()
	svc := newService(repo)
	ctx := context.Background()
	one, two := 1, 2
	order := "vendor-order-77"
	mission1 := events.MovementSummary{CustomerID: customerA, Authority: events.MovementAuthorityTaxi, SourceRef: "taxi-mission-1", MovementType: events.MovementTypeGoods, State: events.MovementStateAssigned,
		ProductCode: "TAXI_GOODS_STANDARD", ParentContextType: "VENDOR_ORDER", ParentContextRef: order, FulfilmentPlanRef: "plan-9", MissionSequence: &one, MissionCount: &two}
	mission2 := events.MovementSummary{CustomerID: customerA, Authority: events.MovementAuthorityDispatch, SourceRef: "dispatch-mission-2", MovementType: events.MovementTypeDispatch, State: events.MovementStateSearching,
		ParentContextType: "VENDOR_ORDER", ParentContextRef: order, FulfilmentPlanRef: "plan-9", MissionSequence: &two, MissionCount: &two}
	if _, err := svc.Ingest(ctx, "driver-taxi-service", envelope(t, "m1", "driver-taxi-service", "taxi.goods.mission.committed", 1, mission1)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ingest(ctx, "driver-dispatch-delivery-service", envelope(t, "m2", "driver-dispatch-delivery-service", "dispatch.delivery.created", 1, mission2)); err != nil {
		t.Fatal(err)
	}
	items, page, err := svc.List(ctx, customerA, ListQuery{ParentContextRef: order})
	if err != nil || page.Total != 2 || items[0].Authority == items[1].Authority {
		t.Fatalf("order movements = %+v total=%d err=%v", items, page.Total, err)
	}
	if other, _, _ := svc.List(ctx, customerB, ListQuery{ParentContextRef: order}); len(other) != 0 {
		t.Fatalf("customer B sees %d of customer A's movements", len(other))
	}
	if _, err := svc.Get(ctx, customerB, "TAXI", "taxi-mission-1"); apperrors.From(err).Status != http.StatusNotFound {
		t.Fatalf("cross-customer get status = %d", apperrors.From(err).Status)
	}
}

func TestProjectionFailureIsReportedForProducerRetry(t *testing.T) {
	repo := newMemoryRepo()
	repo.failWrite = true
	if _, err := newService(repo).Ingest(context.Background(), "driver-taxi-service", envelope(t, "f", "driver-taxi-service", "x", 1, taxi(events.MovementStateSearching, 0))); err == nil {
		t.Fatal("a projection write failure must surface so the producer outbox retries asynchronously")
	}
}
