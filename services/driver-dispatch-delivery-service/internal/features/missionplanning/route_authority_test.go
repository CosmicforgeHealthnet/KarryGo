package missionplanning

import (
	"context"
	"errors"
	"testing"

	dispatchquote "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/serviceability"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

type routePlannerFake struct {
	plan dispatchquote.RoutePlan
	err  error
}

func (f routePlannerFake) Plan(context.Context, []dispatchquote.RoutePoint, dispatchcommerce.Mode) (dispatchquote.RoutePlan, error) {
	return f.plan, f.err
}

type zoneFake struct{ inside, restricted, available bool }

func (z zoneFake) Evaluate(serviceability.Point, serviceability.Point) (bool, bool, bool, bool) {
	return z.inside, z.inside, z.restricted, z.available
}

func TestSection6RouteAuthorityFailClosedAndDegradedPolicy(t *testing.T) {
	in := fixture()
	good := NewSection6RouteAuthority(routePlannerFake{plan: dispatchquote.RoutePlan{Provider: "route", EvidenceVersion: "v1", DistanceMeters: 1000, DurationSeconds: 120}}, zoneFake{inside: true, available: true}, false)
	evidence, err := good.Evaluate(context.Background(), in)
	if err != nil || !evidence.Serviceable || evidence.Degraded {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	failed := NewSection6RouteAuthority(routePlannerFake{err: errors.New("down")}, zoneFake{inside: true, available: true}, false)
	if _, err = failed.Evaluate(context.Background(), in); err == nil {
		t.Fatal("route outage did not fail closed")
	}
	degraded := NewSection6RouteAuthority(routePlannerFake{err: errors.New("down")}, zoneFake{inside: true, available: true}, true)
	evidence, err = degraded.Evaluate(context.Background(), in)
	if err != nil || !evidence.Degraded || !evidence.Serviceable {
		t.Fatalf("degraded=%+v err=%v", evidence, err)
	}
}
