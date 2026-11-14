package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// taxiRestrictionAuthority is a driver-taxi-service double whose first apply
// has an unknown outcome, so Fleet must read it back rather than resubmit.
type taxiRestrictionAuthority struct {
	mu         sync.Mutex
	applies    []platformclients.RestrictionCommand
	applyKeys  []string
	reads      []string
	clears     []platformclients.RestrictionCommand
	ambiguous  bool
	configured bool
}

func (a *taxiRestrictionAuthority) Configured() bool { return a.configured }
func (a *taxiRestrictionAuthority) Apply(_ context.Context, meta platformclients.CallMeta, in platformclients.RestrictionCommand) (platformclients.RestrictionOutcome, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applies = append(a.applies, in)
	a.applyKeys = append(a.applyKeys, meta.IdempotencyKey)
	if a.ambiguous {
		a.ambiguous = false
		return platformclients.RestrictionOutcome{}, &platformclients.Error{Service: "driver-taxi-service", Outcome: platformclients.OutcomeUnknown}
	}
	return platformclients.RestrictionOutcome{RestrictionID: in.RestrictionID, State: "APPLIED", SourceVersion: in.ExpectedVersion}, nil
}
func (a *taxiRestrictionAuthority) Read(_ context.Context, _ platformclients.CallMeta, key string) (platformclients.RestrictionOutcome, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reads = append(a.reads, key)
	return platformclients.RestrictionOutcome{RestrictionID: key, State: "APPLIED", SourceVersion: 1}, nil
}
func (a *taxiRestrictionAuthority) Clear(_ context.Context, _ platformclients.CallMeta, in platformclients.RestrictionCommand) (platformclients.RestrictionOutcome, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clears = append(a.clears, in)
	return platformclients.RestrictionOutcome{RestrictionID: in.RestrictionID, State: "CLEARED", SourceVersion: in.ExpectedVersion}, nil
}
func (a *taxiRestrictionAuthority) ReadClear(_ context.Context, _ platformclients.CallMeta, key string) (platformclients.RestrictionOutcome, error) {
	return platformclients.RestrictionOutcome{RestrictionID: key, State: "CLEARED", SourceVersion: 2}, nil
}

func propagationByAuthority(t *testing.T, view compliance.RestrictionView) map[string]compliance.Propagation {
	t.Helper()
	out := map[string]compliance.Propagation{}
	for _, p := range view.Propagation {
		out[p.Authority+":"+p.Action] = p
	}
	return out
}

// TestRestrictionPropagatesToTaxiReadsBackAndClears makes Taxi a restriction
// target beside Dispatch and Hauling: the restriction is propagated to Taxi
// with its own operation, an ambiguous Taxi apply is read back rather than
// resubmitted, FullyPropagated waits for Taxi, and the clearance reaches Taxi
// before the restriction is cleared.
func TestRestrictionPropagatesToTaxiReadsBackAndClears(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	scope, _ := section1921Scope(t, pool)
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	dispatch := &recordingAuthority{configured: true}
	hauling := &recordingAuthority{configured: true}
	taxi := &taxiRestrictionAuthority{configured: true, ambiguous: true}
	svc := compliance.NewService(compliance.Options{Pool: pool, Events: publisher, Dispatch: dispatch, Hauling: hauling, Taxi: taxi,
		Flags: compliance.Flags{Compliance: true, Restrictions: true}})

	r, err := svc.ApplyRestriction(ctx, scope, compliance.RestrictInput{SubjectType: "PROVIDER", SubjectReference: "driver-taxi-1",
		ReasonCode: "safety_block", SourceType: "INCIDENT", SourceReference: "inc-taxi-1", Severity: "CRITICAL"})
	if err != nil {
		t.Fatal(err)
	}
	var required []string
	if err = json.Unmarshal(r.RequiredAuthorities, &required); err != nil || strings.Join(required, ",") != "DISPATCH,HAULING,TAXI" {
		t.Fatalf("required authorities = %s %v", r.RequiredAuthorities, err)
	}
	if len(taxi.applies) != 1 || taxi.applies[0].RestrictionID != r.ID || taxi.applies[0].SubjectReference != "driver-taxi-1" ||
		taxi.applies[0].ReasonCode != "SAFETY_BLOCK" || taxi.applyKeys[0] != r.ID+":apply" {
		t.Fatalf("taxi apply = %+v keys=%v", taxi.applies, taxi.applyKeys)
	}
	view, err := svc.RestrictionDetail(ctx, scope, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	ops := propagationByAuthority(t, view)
	if view.FullyPropagated || len(view.Propagation) != 3 || ops["TAXI:APPLY"].Status != "OUTCOME_UNKNOWN" ||
		ops["DISPATCH:APPLY"].Status != "CONFIRMED" || ops["HAULING:APPLY"].Status != "CONFIRMED" {
		t.Fatalf("propagation after first attempt = %+v", view.Propagation)
	}

	// Reconciliation reads the ambiguous Taxi apply back, never resubmits it.
	if err = svc.ProcessRestriction(ctx, scope.OrganizationID, r.ID); err != nil {
		t.Fatal(err)
	}
	if len(taxi.applies) != 1 || len(taxi.reads) != 1 || taxi.reads[0] != r.ID+":apply" {
		t.Fatalf("taxi applies=%d reads=%v", len(taxi.applies), taxi.reads)
	}
	if view, err = svc.RestrictionDetail(ctx, scope, r.ID); err != nil || !view.FullyPropagated {
		t.Fatalf("restriction not fully propagated after read-back: %+v %v", view, err)
	}
	if ops = propagationByAuthority(t, view); ops["TAXI:APPLY"].Status != "CONFIRMED" || ops["TAXI:APPLY"].ConfirmedAt == nil {
		t.Fatalf("taxi operation = %+v", ops["TAXI:APPLY"])
	}

	cleared, err := svc.ClearRestriction(ctx, scope, r.ID, "Investigation closed", view.Version)
	if err != nil || cleared.State != "CLEARED" {
		t.Fatalf("clear = %+v %v", cleared, err)
	}
	if len(taxi.clears) != 1 || taxi.clears[0].RestrictionID != r.ID || taxi.clears[0].ExpectedVersion != cleared.Version-1 {
		t.Fatalf("taxi clears = %+v (cleared version %d)", taxi.clears, cleared.Version)
	}
	if view, err = svc.RestrictionDetail(ctx, scope, r.ID); err != nil || !view.FullyPropagated || propagationByAuthority(t, view)["TAXI:CLEAR"].Status != "CONFIRMED" {
		t.Fatalf("propagation after clear = %+v %v", view, err)
	}
}

// TestTaxiRestrictionTargetFollowsDeploymentWiring: an unwired Taxi is not a
// target (Dispatch and Hauling alone decide FullyPropagated, as before), and a
// wired but unconfigured Taxi client is recorded and retried exactly like an
// unconfigured Dispatch or Hauling client.
func TestTaxiRestrictionTargetFollowsDeploymentWiring(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	scope, _ := section1921Scope(t, pool)
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	authority := &recordingAuthority{configured: true}

	unwired := compliance.NewService(compliance.Options{Pool: pool, Events: publisher, Dispatch: authority, Hauling: authority,
		Flags: compliance.Flags{Compliance: true, Restrictions: true}})
	r, err := unwired.ApplyRestriction(ctx, scope, compliance.RestrictInput{SubjectType: "ASSET", SubjectReference: "vehicle-unwired",
		ReasonCode: "INSPECTION_FAILED", SourceType: "INSPECTION", SourceReference: "insp-1", Severity: "HIGH"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := unwired.RestrictionDetail(ctx, scope, r.ID)
	if err != nil || !view.FullyPropagated || len(view.Propagation) != 2 {
		t.Fatalf("unwired taxi propagation = %+v %v", view, err)
	}

	unconfigured := compliance.NewService(compliance.Options{Pool: pool, Events: publisher, Dispatch: authority, Hauling: authority,
		Taxi: &taxiRestrictionAuthority{configured: false}, Flags: compliance.Flags{Compliance: true, Restrictions: true}})
	r, err = unconfigured.ApplyRestriction(ctx, scope, compliance.RestrictInput{SubjectType: "ASSET", SubjectReference: "vehicle-unconfigured",
		ReasonCode: "INSPECTION_FAILED", SourceType: "INSPECTION", SourceReference: "insp-2", Severity: "HIGH"})
	if err != nil {
		t.Fatalf("an unconfigured authority must not fail a durable restriction: %v", err)
	}
	if view, err = unconfigured.RestrictionDetail(ctx, scope, r.ID); err != nil || view.FullyPropagated || len(view.Propagation) != 3 {
		t.Fatalf("unconfigured taxi propagation = %+v %v", view, err)
	}
	op := propagationByAuthority(t, view)["TAXI:APPLY"]
	if op.Status != "RETRY_SCHEDULED" || op.LastErrorCode != "AUTHORITY_NOT_CONFIGURED" || op.Attempts != 1 {
		t.Fatalf("unconfigured taxi operation = %+v", op)
	}
	if gate, err := unconfigured.CheckSubject(ctx, scope.OrganizationID, "ASSET", "vehicle-unconfigured"); err != nil || gate.Eligible {
		t.Fatalf("local enforcement = %+v %v", gate, err)
	}
}
