package routing

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/shared/go/geo"
	sharedrouting "cosmicforge/logistics/shared/go/routing"
)

// fakeFetcher implements sharedrouting.Fetcher without a network call.
// failFor marks specific destinations that should error, so tests can force a
// partial failure deterministically rather than relying on real API flakiness.
type fakeFetcher struct {
	failFor map[geo.Point]bool
	calls   int
}

func (f *fakeFetcher) Route(_ context.Context, _, destination geo.Point, _ sharedrouting.TravelMode) (sharedrouting.Route, error) {
	f.calls++
	if f.failFor[destination] {
		return sharedrouting.Route{}, errors.New("simulated leg failure")
	}
	return sharedrouting.Route{DistanceM: 1000, DurationS: 120, Polyline: "abc"}, nil
}

func TestGoogleClient_RouteMapsFieldsAndTagsProvider(t *testing.T) {
	fetcher := &fakeFetcher{}
	client := NewGoogleRoutingClient(fetcher)

	result, err := client.Route(context.Background(), lagos, ikeja)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Provider != "google-routes" || result.Confidence != 1.0 {
		t.Fatalf("expected a tagged real result, got %+v", result)
	}
	if result.DistanceM != 1000 || result.DurationS != 120 || result.Polyline != "abc" {
		t.Fatalf("fields should map straight through, got %+v", result)
	}
	if result.RequestID == "" {
		t.Fatal("every real result should carry a stable request id for investigation")
	}
}

func TestGoogleClient_MatrixSucceedsWithAllCellsGood(t *testing.T) {
	fetcher := &fakeFetcher{}
	client := NewGoogleRoutingClient(fetcher)

	result, err := client.Matrix(context.Background(), []geo.Point{lagos, ikeja}, []geo.Point{lagos, ikeja})
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if len(result.Rows) != 2 || len(result.Rows[0]) != 2 {
		t.Fatalf("expected a 2x2 grid, got %+v", result.Rows)
	}
	for i := range result.Rows {
		for j := range result.Rows[i] {
			if result.Rows[i][j].Provider != "google-routes" {
				t.Fatalf("cell [%d][%d] should be a real result, got %+v", i, j, result.Rows[i][j])
			}
		}
	}
	if fetcher.calls != 4 {
		t.Fatalf("expected 4 Route calls for a 2x2 matrix, got %d", fetcher.calls)
	}
}

// One bad candidate must not doom scoring for every other good one -- the
// core reason Matrix does per-cell degradation instead of an all-or-nothing
// error.
func TestGoogleClient_MatrixPartialFailureKeepsGoodCellsUsable(t *testing.T) {
	badPoint := geo.Point{Lat: 1.0, Lng: 1.0}
	fetcher := &fakeFetcher{failFor: map[geo.Point]bool{badPoint: true}}
	client := NewGoogleRoutingClient(fetcher)

	result, err := client.Matrix(context.Background(), []geo.Point{lagos}, []geo.Point{ikeja, badPoint})
	if err != nil {
		t.Fatalf("a partial failure must not fail the whole matrix: %v", err)
	}
	if result.Rows[0][0].Provider != "google-routes" {
		t.Fatalf("the good cell should still be usable, got %+v", result.Rows[0][0])
	}
	if result.Rows[0][1].Provider != "unavailable" || result.Rows[0][1].Confidence != 0 {
		t.Fatalf("the bad cell should be an explicitly marked placeholder, got %+v", result.Rows[0][1])
	}
}

func TestGoogleClient_MatrixTotalFailureReturnsError(t *testing.T) {
	fetcher := &fakeFetcher{failFor: map[geo.Point]bool{lagos: true, ikeja: true}}
	client := NewGoogleRoutingClient(fetcher)

	_, err := client.Matrix(context.Background(), []geo.Point{lagos}, []geo.Point{lagos, ikeja})
	if err == nil {
		t.Fatal("a matrix with zero usable cells should return an error, not a silently empty result")
	}
}

func TestGoogleClient_SnapAndGeocodeAreExplicitlyNotImplemented(t *testing.T) {
	client := NewGoogleRoutingClient(&fakeFetcher{})
	if _, err := client.SnapToRoad(context.Background(), []geo.Point{lagos}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
	if _, err := client.ReverseGeocode(context.Background(), lagos.Lat, lagos.Lng); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}
