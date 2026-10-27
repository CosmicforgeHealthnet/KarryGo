package journeyshare

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/geo"
)

// tenMinutes says every leg takes ten minutes, so the arithmetic is visible.
type tenMinutes struct{}

func (tenMinutes) Duration(context.Context, geo.Point, geo.Point) time.Duration { return 10 * time.Minute }

func projection(status, state string) Projection {
	lat, lng := 6.5, 3.4
	return Projection{
		ProductCode: "TRUCK", PublicStatus: status,
		Pickup:   &Place{Lat: 6.52, Lng: 3.37},
		Dropoff:  &Place{Lat: 6.43, Lng: 3.42},
		Location: Location{Available: state != "UNAVAILABLE", State: state, Lat: &lat, Lng: &lng},
	}
}

func TestETAsFollowTheTrip(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return now.Add(time.Duration(m) * time.Minute) }

	// On the way to pickup: 10 min there, 15 min loading, 10 min to drop-off.
	got := EstimateETAs(context.Background(), projection("EN_ROUTE_PICKUP", "FRESH"), tenMinutes{}, now)
	if !got.PickupAt.Equal(at(10)) || !got.DropoffAt.Equal(at(35)) || *got.PickupSeconds != 600 {
		t.Fatalf("en route: %v %v", got.PickupAt, got.DropoffAt)
	}
	// At pickup: pickup is now.
	got = EstimateETAs(context.Background(), projection("ARRIVED", "STALE"), tenMinutes{}, now)
	if !got.PickupAt.Equal(now) || !got.DropoffAt.Equal(at(25)) {
		t.Fatalf("arrived: %v %v", got.PickupAt, got.DropoffAt)
	}
	// Cargo aboard: only the drop-off is ahead.
	got = EstimateETAs(context.Background(), projection("IN_PROGRESS", "FRESH"), tenMinutes{}, now)
	if got.PickupAt != nil || !got.DropoffAt.Equal(at(10)) {
		t.Fatalf("in progress: %v %v", got.PickupAt, got.DropoffAt)
	}
	// No fresh position: no promise.
	for _, status := range []string{"EN_ROUTE_PICKUP", "IN_PROGRESS"} {
		if got := EstimateETAs(context.Background(), projection(status, "STALE"), tenMinutes{}, now); got.PickupAt != nil || got.DropoffAt != nil {
			t.Fatalf("%s with a stale position: %+v", status, got)
		}
	}
	terminal := projection("COMPLETED", "FRESH")
	terminal.Terminal = true
	if got := EstimateETAs(context.Background(), terminal, tenMinutes{}, now); got.DropoffAt != nil {
		t.Fatal("a finished trip got an ETA")
	}

	p := WithETAs(context.Background(), projection("IN_PROGRESS", "FRESH"), tenMinutes{}, now)
	if p.ETA == nil || !p.ETA.Equal(at(10)) || p.PickupETA != nil {
		t.Fatalf("next-stop ETA = %v", p.ETA)
	}
	if snap := p.Public([]string{"eta"}); snap.DropoffETA == nil {
		t.Fatal("ETA permission did not share the drop-off time")
	}
}

func TestStraightLineEstimate(t *testing.T) {
	// 10 km apart, 1.35 road factor at 22 km/h: about 37 minutes.
	from := geo.Point{Lat: 6.5, Lng: 3.4}
	to := geo.Point{Lat: 6.5 + 10.0/111.19, Lng: 3.4}
	d := StraightLineEstimator{}.Duration(context.Background(), from, to)
	if d < 36*time.Minute || d > 38*time.Minute {
		t.Fatalf("estimate %s", d)
	}
	// No routing key: the route estimator falls back.
	if got := (&RouteEstimator{}).Duration(context.Background(), from, to); got != d {
		t.Fatalf("fallback %s != %s", got, d)
	}
}
