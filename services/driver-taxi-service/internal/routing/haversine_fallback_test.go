package routing

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/geo"
)

// fakeRoutingClient lets tests control exactly when the underlying dependency
// fails, without a network call.
type fakeRoutingClient struct {
	routeErr     error
	matrixErr    error
	routeCalls   int
	matrixCalls  int
	snapCalls    int
	geocodeCalls int
}

func (f *fakeRoutingClient) Route(context.Context, geo.Point, geo.Point) (RouteResult, error) {
	f.routeCalls++
	if f.routeErr != nil {
		return RouteResult{}, f.routeErr
	}
	return RouteResult{DistanceM: 5000, DurationS: 600, Confidence: 1.0, Provider: "google-routes"}, nil
}

func (f *fakeRoutingClient) Matrix(_ context.Context, origins, destinations []geo.Point) (MatrixResult, error) {
	f.matrixCalls++
	if f.matrixErr != nil {
		return MatrixResult{}, f.matrixErr
	}
	rows := make([][]RouteResult, len(origins))
	for i := range rows {
		rows[i] = make([]RouteResult, len(destinations))
		for j := range rows[i] {
			rows[i][j] = RouteResult{DistanceM: 1000, Confidence: 1.0, Provider: "google-routes"}
		}
	}
	return MatrixResult{Rows: rows}, nil
}

func (f *fakeRoutingClient) SnapToRoad(context.Context, []geo.Point) (SnappedRoute, error) {
	f.snapCalls++
	return SnappedRoute{}, ErrNotImplemented
}

func (f *fakeRoutingClient) ReverseGeocode(context.Context, float64, float64) (AddressResult, error) {
	f.geocodeCalls++
	return AddressResult{}, ErrNotImplemented
}

var (
	lagos = geo.Point{Lat: 6.52, Lng: 3.37}
	ikeja = geo.Point{Lat: 6.60, Lng: 3.35}
)

func TestFallback_PassesThroughWhenBreakerClosed(t *testing.T) {
	fake := &fakeRoutingClient{}
	client := NewHaversineFallbackClient(fake, 3, time.Second)

	result, err := client.Route(context.Background(), lagos, ikeja)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Provider != "google-routes" || result.Confidence != 1.0 {
		t.Fatalf("expected a real result while the breaker is closed, got %+v", result)
	}
	if fake.routeCalls != 1 {
		t.Fatalf("expected the underlying client to be called once, got %d", fake.routeCalls)
	}
}

func TestFallback_SubstitutesHaversineOnUnderlyingFailure(t *testing.T) {
	fake := &fakeRoutingClient{routeErr: errors.New("upstream timeout")}
	client := NewHaversineFallbackClient(fake, 3, time.Second)

	result, err := client.Route(context.Background(), lagos, ikeja)
	if err != nil {
		t.Fatalf("Route should degrade rather than propagate the error: %v", err)
	}
	if result.Provider != "haversine-fallback" || result.Confidence != 0 {
		t.Fatalf("expected a marked fallback result, got %+v", result)
	}
	// A real distance in the right ballpark, not a zero value silently passed
	// through -- lagos to ikeja is roughly 12-14km as the crow flies.
	if result.DistanceM < 5000 || result.DistanceM > 25000 {
		t.Fatalf("fallback distance looks wrong: %d meters", result.DistanceM)
	}
}

func TestFallback_OpensAfterRepeatedFailuresAndStopsCallingUnderlying(t *testing.T) {
	fake := &fakeRoutingClient{routeErr: errors.New("upstream down")}
	client := NewHaversineFallbackClient(fake, 2, time.Minute)

	for i := 0; i < 2; i++ {
		if _, err := client.Route(context.Background(), lagos, ikeja); err != nil {
			t.Fatalf("Route: %v", err)
		}
	}
	if !client.Breaker().IsOpen() {
		t.Fatal("breaker should be open after reaching the failure threshold")
	}

	callsBefore := fake.routeCalls
	result, err := client.Route(context.Background(), lagos, ikeja)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Provider != "haversine-fallback" {
		t.Fatalf("expected a fallback result once open, got %+v", result)
	}
	if fake.routeCalls != callsBefore {
		t.Fatalf("an open breaker must not call the underlying client at all, but call count moved from %d to %d", callsBefore, fake.routeCalls)
	}
}

func TestFallback_RecoversAfterCooldownOnSuccessfulProbe(t *testing.T) {
	fake := &fakeRoutingClient{routeErr: errors.New("upstream down")}
	client := NewHaversineFallbackClient(fake, 1, 50*time.Millisecond)

	if _, err := client.Route(context.Background(), lagos, ikeja); err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !client.Breaker().IsOpen() {
		t.Fatal("breaker should be open after one failure with threshold 1")
	}

	fake.routeErr = nil // the dependency recovers
	time.Sleep(60 * time.Millisecond)

	result, err := client.Route(context.Background(), lagos, ikeja)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Provider != "google-routes" {
		t.Fatalf("expected the probe to succeed and return a real result, got %+v", result)
	}
	if client.Breaker().IsOpen() {
		t.Fatal("breaker should close after a successful probe")
	}
}

func TestFallback_MatrixDegradesEveryCellWhenOpen(t *testing.T) {
	fake := &fakeRoutingClient{matrixErr: errors.New("upstream down")}
	client := NewHaversineFallbackClient(fake, 1, time.Minute)

	if _, err := client.Matrix(context.Background(), []geo.Point{lagos}, []geo.Point{ikeja}); err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if !client.Breaker().IsOpen() {
		t.Fatal("breaker should be open after the matrix call failed")
	}

	result, err := client.Matrix(context.Background(), []geo.Point{lagos, ikeja}, []geo.Point{lagos, ikeja})
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	for i := range result.Rows {
		for j := range result.Rows[i] {
			if result.Rows[i][j].Provider != "haversine-fallback" {
				t.Fatalf("cell [%d][%d] should be a fallback result, got %+v", i, j, result.Rows[i][j])
			}
		}
	}
}

// SnapToRoad and ReverseGeocode must never be substituted, even with the
// breaker wide open -- there is no straight-line stand-in for either.
func TestFallback_NeverSubstitutesSnapOrGeocode(t *testing.T) {
	fake := &fakeRoutingClient{routeErr: errors.New("upstream down")}
	client := NewHaversineFallbackClient(fake, 1, time.Minute)
	_, _ = client.Route(context.Background(), lagos, ikeja) // opens the breaker

	if _, err := client.SnapToRoad(context.Background(), []geo.Point{lagos}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("SnapToRoad should call through to the underlying client's own error, got %v", err)
	}
	if _, err := client.ReverseGeocode(context.Background(), lagos.Lat, lagos.Lng); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("ReverseGeocode should call through to the underlying client's own error, got %v", err)
	}
	if fake.snapCalls != 1 || fake.geocodeCalls != 1 {
		t.Fatalf("both calls should have reached the underlying client exactly once, got snap=%d geocode=%d", fake.snapCalls, fake.geocodeCalls)
	}
}
